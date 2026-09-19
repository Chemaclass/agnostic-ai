package kilo

import (
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// permissionKey is the `kilo.jsonc` key holding the per-tool map, and
// the agent-frontmatter key holding the same shape: "Permissions are
// configured under the `permission` key in `kilo.jsonc`"
// (kilo.ai/docs/getting-started/settings/auto-approving-actions), and
// "This page focuses on Markdown agent files, where permission rules
// are written as YAML frontmatter under the `permission` key"
// (kilo.ai/docs/customize/agent-permissions).
const permissionKey = "permission"

// anyPattern is Kilo's catch-all. It works as a pattern inside one
// tool's map ("`*` | Any target for that permission") and as a
// top-level key across every tool ("Top-level permission keys follow
// the same rule ... `permission: {"*": ask, bash: allow}`").
const anyPattern = "*"

// kiloPermissionTool maps agnostic-ai's Claude-style tool identifiers
// onto Kilo Code's own permission keys. The vendor publishes both
// halves of this: the permission table on
// kilo.ai/docs/getting-started/settings/auto-approving-actions rows
// `external_directory`, `bash`, `read`, `edit`, `glob`, `grep`, `task`,
// `skill`, `lsp`, `todoread`/`todowrite`, `websearch`, `webfetch`, and
// `doom_loop`, and kilo.ai/docs/automate/tools groups the tool names
// themselves, including `write` ("Edit Group | `edit`, `write`,
// `apply_patch`"), which the agent-permissions page then names outright:
// "File tools such as `read`, `edit`, and `write` resolve the input
// path first".
//
// A name with no row here is never guessed at. It drops and folds into
// one coverage note per sync, the same deal windsurf's `allowed-tools`
// translation gives an unknown name.
var kiloPermissionTool = map[string]string{
	"Read":      "read",
	"Glob":      "glob",
	"Grep":      "grep",
	"Edit":      "edit",
	"Write":     "write",
	"Bash":      "bash",
	"WebFetch":  "webfetch",
	"WebSearch": "websearch",
	"Task":      "task",
	"Skill":     "skill",
	"TodoRead":  "todoread",
	"TodoWrite": "todowrite",
}

// permissionUntranslatedReason explains, in the flushed coverage note,
// why some rules did not reach `kilo.jsonc`.
const permissionUntranslatedReason = "rule(s) outside Kilo Code's own permission vocabulary have no key there; set x-kilo.permission with Kilo's own rules for those"

// agentToolsUntranslatedReason explains the same thing for an agent's
// `tools` list, which reaches the same map through frontmatter.
const agentToolsUntranslatedReason = "tool name(s) outside Kilo Code's own permission vocabulary have no key there; set x-kilo.permission with Kilo's own rules for those"

// permissionRule translates one agnostic-ai permission rule into the
// Kilo key and glob pattern it belongs under, reporting false for
// anything with no faithful spelling there:
//
//	Read(glob)        -> read,     glob
//	Write(glob)       -> write,    glob
//	Edit(glob)        -> edit,     glob
//	Bash(prefix:*)    -> bash,     "prefix *"
//	Bash(cmd)         -> bash,     "cmd"
//	WebFetch(pattern) -> webfetch, pattern
//	Read/Bash/...     -> read/bash/..., "*"
//	mcp__srv__tool    -> srv_tool, "*"
//
// Claude's `:*` prefix marker becomes Kilo's trailing ` *`: "`git *` |
// `git`, `git status`, `git log --oneline`, and other `git` commands".
// Without the marker the command is exact on both sides, so it passes
// through as written; unlike Devin's prefix-only `Exec`, Kilo can spell
// an exact command, so nothing has to widen here.
//
// Claude's `WebFetch(domain:example.test)` shorthand has no Kilo form:
// Kilo matches "the tool's arguments", so a literal `domain:` prefix
// would match no URL and the rule would be inert. It drops instead.
func permissionRule(rule string) (tool, pattern string, ok bool) {
	if server, name, found := spec.SplitMCPPermissionRule(rule); found {
		return server + "_" + name, anyPattern, true
	}
	if scope, arg, found := spec.SplitPermissionRule(rule); found {
		key, known := kiloPermissionTool[scope]
		if !known {
			return "", "", false
		}
		if scope == "Bash" {
			if prefix, isPrefixRule := strings.CutSuffix(arg, ":*"); isPrefixRule {
				if prefix == "" {
					return "", "", false
				}
				return key, prefix + " *", true
			}
		}
		if scope == "WebFetch" && strings.HasPrefix(arg, "domain:") {
			return "", "", false
		}
		return key, arg, true
	}
	if key, known := kiloPermissionTool[rule]; known {
		return key, anyPattern, true
	}
	return "", "", false
}

// settingsPermission builds the `permission` map from the portable
// allow, deny, and ask lists across every settings spec, and reports
// how many specs carried at least one rule with no Kilo spelling so the
// caller can fold them into one coverage note per sync.
//
// The three lists are walked allow, then ask, then deny, so a rule
// repeated across lists resolves to the most restrictive action rather
// than to whichever spec happened to come last.
//
// Ordering inside the emitted object is alphabetical, because both the
// JSON and the YAML encoder sort map keys. That lands `*` ahead of
// every tool name and every command pattern, which is the order Kilo
// asks for: "Put broad fallbacks first and exceptions after them",
// since "the last matching rule wins".
func settingsPermission(settings []spec.Entry) (map[string]any, int) {
	dropped := map[int]bool{}
	out := map[string]any{}
	for _, list := range []string{"allow", "ask", "deny"} {
		for i, entry := range settings {
			rules, native := entryRules(entry, list)
			if native {
				continue
			}
			for _, rule := range rules {
				tool, pattern, ok := permissionRule(rule)
				if !ok {
					dropped[i] = true
					continue
				}
				patterns, _ := out[tool].(map[string]any)
				if patterns == nil {
					patterns = map[string]any{}
					out[tool] = patterns
				}
				patterns[pattern] = list
			}
		}
	}
	// Native maps merge last, one tool key at a time, so an author
	// writing under the kilo namespace wins over any translated rule
	// for the same tool without wiping a sibling spec's rules for
	// other tools. Same deal `x-kilo.permission` already gets on an
	// agent: that author is presumed to know Kilo's own spelling.
	for _, entry := range settings {
		custom, _ := emit.CustomTargetMeta(entry.Meta, target)
		if custom == nil {
			continue
		}
		native, ok := custom[permissionKey].(map[string]any)
		if !ok {
			continue
		}
		for tool, value := range native {
			out[tool] = value
		}
	}
	if len(out) == 0 {
		return nil, len(dropped)
	}
	return out, len(dropped)
}

// entryRules returns one settings spec's rules for list, and whether
// they already hold Kilo's own vocabulary under `x-kilo.permission`.
func entryRules(entry spec.Entry, list string) (rules []string, native bool) {
	if custom, _ := emit.CustomTargetMeta(entry.Meta, target); custom != nil {
		if _, ok := custom[permissionKey].(map[string]any); ok {
			return nil, true
		}
	}
	permissions, _ := entry.Meta["permissions"].(map[string]any)
	return emit.StringSlice(permissions[list]), false
}

// agentPermission turns an agent's portable `tools` allowlist into the
// `permission` frontmatter Kilo actually reads. A tools list is an
// allowlist, so the map denies everything first and re-allows the names
// that translated: "Top-level permission keys follow the same rule",
// with the vendor's own example `permission: {"*": ask, bash: allow}`
// showing a top-level catch-all overridden by a later key.
//
// Returns nil when no name translated, so an agent whose whole list is
// untranslatable keeps its default permissions rather than being locked
// out by a bare `{"*": "deny"}` nobody asked for. unmapped reports
// whether at least one name had no Kilo key, for the coverage note.
func agentPermission(tools []string) (perms map[string]any, unmapped bool) {
	allowed := map[string]any{}
	for _, name := range tools {
		key, known := kiloPermissionTool[name]
		if !known {
			if server, tool, isMCP := spec.SplitMCPPermissionRule(name); isMCP {
				allowed[server+"_"+tool] = "allow"
				continue
			}
			unmapped = true
			continue
		}
		allowed[key] = "allow"
	}
	if len(allowed) == 0 {
		return nil, unmapped
	}
	allowed[anyPattern] = "deny"
	return allowed, unmapped
}
