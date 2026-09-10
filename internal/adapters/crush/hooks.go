package crush

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// crushPreToolUseEvent is the only hook event Crush's own runtime
// consumes today. docs/hooks/README.md: "Crush currently supports
// just one hook, PreToolUse, with plans to support the full gamut"
// (re-verified 2026-09-10 against that doc and the vendor's published
// schema.json, whose `$defs.HookConfig` still carries no per-event
// variant; #629).
const crushPreToolUseEvent = "PreToolUse"

// crushClaudeToolNames flags the Claude/Codex-style capitalized
// matcher values (Bash, Edit, Write, ...) a hook spec copied from a
// Claude or Codex source carries. Crush's own PreToolUse docs match
// against its own lowercase tool names ("bash", "edit", "write",
// "mcp_github_create_pull_request"; the worked examples all use
// lowercase regexes like `^bash$`), so a Claude-cased matcher parses
// as a valid regex and then matches nothing, the same trap openhands
// and windsurf hit with their own tool vocabularies.
var crushClaudeToolNames = map[string]bool{
	"Bash": true, "Read": true, "Write": true, "Edit": true,
	"Glob": true, "Grep": true, "WebFetch": true, "WebSearch": true,
}

// buildHooksBlock renders the `hooks` value merged into crush.json:
// `{"PreToolUse": [{"name": ..., "matcher": ..., "command": ...,
// "timeout": ...}, ...]}`. Crush's schema.json `$defs.HookConfig`
// documents exactly these four fields flat, one array entry per hook
// (`command` required, the rest optional; `timeout` is seconds,
// defaulting to 30 when absent). There is no Claude-style
// `{"matcher": ..., "hooks": [...]}` grouping to nest into, unlike
// claude, codex, openhands, and qoder.
//
// A spec whose `event:` is not PreToolUse earns a coverage note
// instead of a dead entry: Crush's own docs state PreToolUse is the
// only event it runs today, so anything else would parse into
// crush.json and never fire.
//
// `name` comes from the hook spec's own Name (agnostic-ai's hooks/*.yaml
// loader already reads a `name:` key into Entry.Name the same way
// every other kind does), the one field on crush's HookConfig this
// repo's generic hook spec did not already carry a dedicated column
// for; crush is also the only audited target that surfaces a hook's
// name in its own UI (schema.json: "Friendly display name shown in
// the TUI for this hook").
//
// Returns nil when no hook spec produces a PreToolUse entry.
func buildHooksBlock(hooks []spec.Entry) map[string]any {
	var entries []any
	var otherEvents, claudeMatchers int

	for _, h := range hooks {
		event, _ := h.Meta["event"].(string)
		if event != crushPreToolUseEvent {
			if event != "" {
				otherEvents++
			}
			continue
		}
		commands := emit.HookCommands(h.Meta["command"])
		if len(commands) == 0 {
			continue
		}
		matcher, _ := h.Meta["matcher"].(string)
		if crushClaudeToolNames[matcher] {
			claudeMatchers++
		}
		timeout := emit.HookIntMeta(h.Meta, "timeout")

		for _, command := range commands {
			entry := map[string]any{"command": emit.RewriteHookPath(command, target)}
			if h.Name != "" {
				entry["name"] = h.Name
			}
			if matcher != "" {
				entry["matcher"] = matcher
			}
			if timeout > 0 {
				entry["timeout"] = timeout
			}
			entries = append(entries, entry)
		}
	}

	emit.NoteCoverageGap(target, spec.KindHook, otherEvents,
		"Crush only runs the PreToolUse hook event today; other events parse into crush.json but never fire")
	emit.NoteFieldNoOp(target, spec.KindHook, "matcher", claudeMatchers,
		"Crush's own tool names are lowercase (bash, edit, write, mcp_<server>_<tool>), not Claude's; a Claude-style matcher parses but matches nothing")

	if len(entries) == 0 {
		return nil
	}
	return map[string]any{crushPreToolUseEvent: entries}
}
