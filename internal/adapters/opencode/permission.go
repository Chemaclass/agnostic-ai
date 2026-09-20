package opencode

import (
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// permissionKey is the top-level `opencode.json` key holding the rule
// map, and the same key an author may write directly under
// `x-opencode.permission`.
const permissionKey = "permission"

// catchAllPattern is OpenCode's match-everything pattern. A portable
// bare tool name becomes this when the same tool also carries scoped
// rules: "A common pattern is to put the catch-all `"*"` rule first,
// and more specific rules after it" (opencode.ai/docs/permissions).
const catchAllPattern = "*"

// opencodePermissionTool maps this project's Claude-style tool
// identifiers onto the keys OpenCode's `permission` map accepts.
//
// `Write` and `Edit` both land on `edit`, which the vendor says
// "covers edit, write, patch". That collapse is lossy in one
// direction only: a project denying `Write` also denies `Edit` here,
// which is the safe way round.
var opencodePermissionTool = map[string]string{
	"Bash":      "bash",
	"Read":      "read",
	"Edit":      "edit",
	"Write":     "edit",
	"Glob":      "glob",
	"Grep":      "grep",
	"Task":      "task",
	"Skill":     "skill",
	"WebFetch":  "webfetch",
	"WebSearch": "websearch",
}

// opencodePatternTool reports which of those keys accept a pattern
// object rather than a bare action. OpenCode's schema types `webfetch`,
// `websearch`, `todowrite`, `question`, and `doom_loop` as an action
// string alone, so a path- or URL-scoped rule on one of them has no
// form and raises a coverage note instead of being flattened onto the
// whole tool.
var opencodePatternTool = map[string]bool{
	"bash": true, "read": true, "edit": true, "glob": true,
	"grep": true, "task": true, "skill": true,
}

// permissionSeverity ranks the three portable lists so a rule claimed
// by two of them resolves to the more restrictive one. Two lists
// naming the same tool and pattern is a conflict the author did not
// settle, and widening it is the one outcome that cannot be undone by
// a prompt.
var permissionSeverity = map[string]int{"allow": 1, "ask": 2, "deny": 3}

// permissionUnmappableReason explains, in the flushed coverage note,
// why some rules did not reach `opencode.json`.
//
// This used to claim every `mcp__` rule had no key, which was false
// and stopped users looking further (#947). A single MCP tool maps
// onto `<server>_<tool>`; only whole-server denial has no portable
// spelling here, because `mcp__<server>` names no tool.
const permissionUnmappableReason = "rule(s) outside OpenCode's permission vocabulary have no key there, including any path- or URL-scoped webfetch or websearch rule, and whole-server MCP denial, since a portable mcp__ rule names one tool; set x-opencode.permission for those"

// buildPermissions renders the portable allow, deny, and ask lists as
// OpenCode's `permission` map, and reports how many settings specs
// carried at least one rule with no OpenCode form.
//
// OpenCode evaluates "by pattern match, with the last matching rule
// winning", and its rules live in a JSON object. Go marshals a map
// with its keys sorted, and `*` sorts ahead of every alphanumeric
// pattern, so the catch-all lands first exactly as the vendor
// recommends. Among the rest, a narrower pattern sharing a prefix
// sorts after the broader one, because a space precedes letters, so
// `git *` comes before `git push *` and the narrower rule wins. That
// is the intended reading, but it falls out of the sort rather than
// being enforced, so an author who needs a specific order should write
// `x-opencode.permission` directly.
func buildPermissions(settings []spec.Entry) (map[string]any, int) {
	// tool -> pattern -> action, before collapsing single catch-alls
	// back to the bare string form.
	rules := map[string]map[string]string{}
	dropped := map[int]bool{}

	for i, entry := range settings {
		permissions, _ := entry.Meta["permissions"].(map[string]any)
		for _, list := range []string{"allow", "ask", "deny"} {
			for _, rule := range emit.StringSlice(permissions[list]) {
				tool, pattern, ok := translateRule(rule)
				if !ok {
					dropped[i] = true
					continue
				}
				if rules[tool] == nil {
					rules[tool] = map[string]string{}
				}
				if permissionSeverity[list] > permissionSeverity[rules[tool][pattern]] {
					rules[tool][pattern] = list
				}
			}
		}
	}

	out := map[string]any{}
	for tool, patterns := range rules {
		// One catch-all and nothing else is the bare string form,
		// which is what the vendor's own simple example writes.
		if action, only := patterns[catchAllPattern]; only && len(patterns) == 1 {
			out[tool] = action
			continue
		}
		object := map[string]any{}
		for pattern, action := range patterns {
			object[pattern] = action
		}
		out[tool] = object
	}

	// Native maps merge last, one tool key at a time, so an author
	// writing OpenCode's own spelling wins for that tool without
	// wiping a sibling spec's rules for the others. Same deal
	// `x-opencode.permission` already gets on an agent.
	for _, entry := range settings {
		for tool, value := range nativePermission(entry) {
			out[tool] = value
		}
	}

	if len(out) == 0 {
		return nil, len(dropped)
	}
	return out, len(dropped)
}

// nativePermission returns one settings spec's `x-opencode.permission`
// map, or nil when it has none.
func nativePermission(entry spec.Entry) map[string]any {
	native, _ := emit.SettingsCustomObject(entry, target, permissionKey)
	return native
}

// translateRule turns one portable rule into an OpenCode tool key and
// the pattern it applies to, or reports that it has no form here.
//
// A bare tool name covers the whole tool, so it becomes the catch-all
// pattern and collapses back to a bare action when nothing narrows it.
func translateRule(rule string) (tool, pattern string, ok bool) {
	// An MCP tool is a permission key like any other: "Permission keys
	// are matched as wildcard patterns against the underlying tool
	// name, so the same syntax works for built-ins, custom tools, and
	// MCP tools" (opencode.ai/docs/agents), and "MCP server tools are
	// registered with server name as prefix"
	// (opencode.ai/docs/mcp-servers). The server name passes through
	// verbatim, hyphens included: the same page's glob section reads
	// `"my-mcp*"` matches `my-mcp_search`. Same join kilo makes.
	if server, name, isMCP := spec.SplitMCPPermissionRule(rule); isMCP {
		return server + "_" + name, catchAllPattern, true
	}
	scope, arg, scoped := spec.SplitPermissionRule(rule)
	if !scoped {
		key, known := opencodePermissionTool[rule]
		if !known {
			return "", "", false
		}
		return key, catchAllPattern, true
	}
	key, known := opencodePermissionTool[scope]
	if !known || !opencodePatternTool[key] {
		return "", "", false
	}
	return key, translatePattern(arg), true
}

// translatePattern converts a rule argument into OpenCode's glob.
//
// The portable `:*` suffix is this project's prefix convention, as in
// `Bash(go test:*)`. OpenCode has no such convention: it matches a
// bare glob against the command, where `*` is zero or more of any
// character, `?` is exactly one, and everything else is literal. Its
// own examples spell a command prefix `git *`, so that is what the
// convention becomes. An argument without the suffix is already a
// literal or a glob and passes through untouched.
func translatePattern(arg string) string {
	prefix, isPrefixRule := strings.CutSuffix(arg, ":*")
	if !isPrefixRule {
		return arg
	}
	if prefix == "" {
		return catchAllPattern
	}
	return prefix + " *"
}
