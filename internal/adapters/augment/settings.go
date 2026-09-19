package augment

import (
	"regexp"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// toolPermissionsKey is the settings.json key holding the rule array.
const toolPermissionsKey = "toolPermissions"

// augmentTool maps agnostic-ai's Claude-style tool identifiers onto the
// six Augment publishes across its Shell, File Operations, and Web
// tables (docs.augmentcode.com/cli/permissions). The shell is one tool,
// `terminal`: "One `terminal` rule gates all shell commands." The four
// legacy names that page lists (`launch-process`, `view`,
// `str-replace-editor`, `save-file`) are aliases of these, so the
// current spelling is the one written.
var augmentTool = map[string]string{
	"Read":      "read",
	"Edit":      "edit",
	"Write":     "write",
	"Bash":      "terminal",
	"WebFetch":  "web-fetch",
	"WebSearch": "web-search",
}

// mcpToolNameLimit is the length Augment truncates an MCP tool name to:
// "Truncated to 64 characters if longer". A composed name past it would
// never match the tool it was written for.
const mcpToolNameLimit = 64

// askNoTypeReason explains why the portable ask list reaches nothing.
// Augment's four permission types are allow, deny, webhook-policy, and
// script-policy; none of them prompts the user.
const askNoTypeReason = "Augment's permission types are allow, deny, webhook-policy, and script-policy; none of them prompts"

// rulesUnmappableReason explains, in the flushed coverage note, why
// some rules did not reach the file. Augment gates `read`, `edit`,
// `write`, and `web-fetch` as whole tools with no path or URL matcher
// of their own, so flattening `Read(src/**)` onto `read` would widen it
// into every file on disk.
const rulesUnmappableReason = "rule(s) scoping a path or URL have no Augment matcher (only terminal takes one, shellInputRegex), so flattening them would widen the rule; set x-augment.toolPermissions for those"

// modelNoKeyReason explains why a portable model reaches nothing here.
const modelNoKeyReason = "no vendor-documented model key in .augment/settings.json"

// buildToolPermissions renders the portable allow and deny lists as
// Augment's ordered rule array, and reports how many settings specs
// carried at least one rule with no Augment form.
//
// Order is load-bearing: "Rules are evaluated in order from top to
// bottom" and "The first matching rule determines the permission", so
// deny rules precede allow rules or a broad allow shadows a narrow
// deny. An explicit `x-augment.toolPermissions` array comes first of
// all, ahead of anything translated, so a native rule an author wrote
// directly is never shadowed by a generated one. Specs layer in source
// order and a repeated rule emits once, matching how the same lists
// already merge for every other target with this surface.
//
// Every rule's `permission` is an object with a `type` field. The
// vendor is explicit that the bare-string form is malformed: "A rule
// with a bare-string permission is malformed and is dropped"
// (docs.augmentcode.com/cli/permissions).
func buildToolPermissions(settings []spec.Entry) ([]any, int) {
	var rules []any
	for _, entry := range settings {
		rules = append(rules, nativeRules(entry)...)
	}
	dropped := map[int]bool{}
	seen := map[string]bool{}
	for _, list := range []string{"deny", "allow"} {
		for i, entry := range settings {
			permissions, _ := entry.Meta["permissions"].(map[string]any)
			for _, rule := range emit.StringSlice(permissions[list]) {
				if seen[list+"\x00"+rule] {
					continue
				}
				seen[list+"\x00"+rule] = true
				mapped, ok := augmentRule(rule, list)
				if !ok {
					dropped[i] = true
					continue
				}
				rules = append(rules, mapped)
			}
		}
	}
	return rules, len(dropped)
}

// nativeRules returns one settings spec's x-augment.toolPermissions
// entries: rules an author wrote in Augment's own shape, passed through
// untouched, the same deal x-augment.tools gets on an agent.
func nativeRules(entry spec.Entry) []any {
	custom, _ := emit.CustomTargetMeta(entry.Meta, target)
	if custom == nil {
		return nil
	}
	native, _ := custom[toolPermissionsKey].([]any)
	return native
}

// augmentRule translates one agnostic-ai permission rule into an
// Augment rule object, reporting false for anything with no form there
// rather than guessing one:
//
//	Read/Edit/Write/Bash/... -> {toolName: read|edit|write|terminal|...}
//	Bash(prefix:*)           -> terminal, shellInputRegex ^prefix
//	Bash(command)            -> terminal, shellInputRegex ^command$
//	mcp__server__tool        -> {toolName: tool_server}
//
// A path- or URL-scoped rule has no entry. Augment gates `read`,
// `edit`, `write`, and `web-fetch` as whole tools, so `Read(src/**)`
// would have to become an unrestricted `read` to fit, which grants far
// more than the author asked for.
func augmentRule(rule, permissionType string) (map[string]any, bool) {
	permission := map[string]any{"type": permissionType}
	if name, ok := mcpToolName(rule); ok {
		return map[string]any{"toolName": name, "permission": permission}, true
	}
	if scope, arg, ok := spec.SplitPermissionRule(rule); ok {
		if scope != "Bash" {
			return nil, false
		}
		regex := "^" + regexp.QuoteMeta(arg) + "$"
		if prefix, isPrefixRule := strings.CutSuffix(arg, ":*"); isPrefixRule {
			if prefix == "" {
				return nil, false
			}
			regex = "^" + regexp.QuoteMeta(prefix)
		}
		return map[string]any{
			"toolName":        "terminal",
			"shellInputRegex": regex,
			"permission":      permission,
		}, true
	}
	name, ok := augmentTool[rule]
	if !ok {
		return nil, false
	}
	return map[string]any{"toolName": name, "permission": permission}, true
}

// mcpToolName rewrites `mcp__<server>__<tool>` into the
// `{tool-name}_{server-name}` spelling Augment matches MCP tools by,
// example `query_database-mcp`. A wildcard rule (`mcp__github__*`,
// `mcp__*`) names no single tool and Augment documents no wildcard, so
// it is not one.
func mcpToolName(rule string) (string, bool) {
	server, tool, ok := spec.SplitMCPPermissionRule(rule)
	if !ok || strings.Contains(rule, "*") {
		return "", false
	}
	name := tool + "_" + server
	if len(name) > mcpToolNameLimit {
		name = name[:mcpToolNameLimit]
	}
	return name, true
}

// noteSettingsGaps records the portable settings fields that reach
// nothing on Augment: `model`, which has no documented key in this
// file, and the `ask` list, which has no permission type there.
func noteSettingsGaps(settings []spec.Entry, droppedRules int) {
	model, ask := 0, 0
	for _, entry := range settings {
		if value, _ := entry.Meta["model"].(string); value != "" {
			model++
		}
		permissions, _ := entry.Meta["permissions"].(map[string]any)
		if len(emit.StringSlice(permissions["ask"])) > 0 {
			ask++
		}
	}
	emit.NoteFieldNoOp(target, spec.KindSettings, "model", model, modelNoKeyReason)
	emit.NoteFieldNoOp(target, spec.KindSettings, "permissions.ask", ask, askNoTypeReason)
	emit.NoteFieldNoOp(target, spec.KindSettings, "permissions", droppedRules, rulesUnmappableReason)
}
