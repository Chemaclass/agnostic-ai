package qoder

import (
	"sort"

	"github.com/chemaclass/agnostic-ai/internal/adapters/claudehooks"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// hookLifecycle is the event order docs.qoder.com/cli/hooks' own Event
// Reference table groups by: Session Lifecycle, Tool Calls, Agent Flow,
// Context Compaction, Notifications, Context and Configuration Loading,
// Working Directory and Files, Worktree Isolation, MCP Interaction.
// Events outside this list follow in first-seen order.
//
// docs.qoder.com/cli/hooks-reference is the authoritative event list
// and its Event Types table carries 27 rows; the grouped `/cli/hooks`
// table this order follows covers 23 (both re-read 2026-09-12), so
// count rows on the reference page. The four it adds close the list in
// its own order: `TaskCreated`, `TaskCompleted`, `TeammateIdle`, and
// `Setup` ("During initial installation"). All four emitted before they
// were named here,
// since `event:` passes through verbatim, but they sorted behind any
// unlisted event seen first; naming them moves them ahead of one, which
// TestEmit_Hook_EventOrderIsLifecycleThenFirstSeen pins because it
// changes the bytes of an already-written settings.json (#744). The
// same 27 live in `hookEventsByTarget["qoder"]` in
// internal/cli/native_capabilities.go, where `validate` reads them.
var hookLifecycle = []string{
	"SessionStart", "SessionEnd",
	"UserPromptSubmit",
	"PreToolUse", "PostToolUse", "PostToolUseFailure",
	"PermissionRequest", "PermissionDenied",
	"Stop", "StopFailure",
	"SubagentStart", "SubagentStop",
	"PreCompact", "PostCompact",
	"Notification",
	"InstructionsLoaded", "ConfigChange",
	"CwdChanged", "FileChanged",
	"WorktreeCreate", "WorktreeRemove",
	"Elicitation", "ElicitationResult",
	"TaskCreated", "TaskCompleted", "TeammateIdle", "Setup",
}

// buildHooksBlock renders the `"hooks"` value merged into
// `.qoder/settings.json`: `{"<Event>": [{"matcher": ..., "hooks":
// [...]}]}`. docs.qoder.com/cli/hooks documents that identical nested
// shape ("Configuration Format"), the same one Claude Code, Codex, and
// OpenHands use, so this reuses the shared claudehooks wire structs
// those emitters already carry instead of a third hand-rolled copy.
// Returned as an *emit.OrderedJSON, not a plain map, so the event order
// survives being embedded as a nested value under mcp.go's
// emitSettings, which uses the same recursive-OrderedJSON technique
// claude.go's own settings.json hooks block does.
//
// Qoder's own matcher vocabulary matches Claude's: PreToolUse and
// PostToolUse document "Tool name (e.g. Bash, Write, Edit, Read, Glob,
// Grep; MCP tool names like mcp__server__tool)", the same set Claude
// Code answers to. A Claude-authored matcher therefore passes straight
// through; unlike openhands and windsurf, whose own tool vocabularies
// diverge from Claude's, qoder needs no coverage note here (#629).
//
// Fields emitted per hook entry: `type` (always "command", the only
// type a generic command spec can express), `command`, `args`,
// `timeout` (seconds, vendor default 600), `statusMessage`, `async`,
// `asyncRewake`, `shell`, `if`, and `once`. All nine are documented on
// docs.qoder.com/cli/hooks' `command` hook entry with the same
// semantics claudehooks.CommandEntry already models for Claude Code, so
// no target-specific struct is needed. Qoder additionally documents
// `env`, `rewakeMessage`, and `rewakeSummary` on that same entry, plus
// three more hook entry types (`http`, `prompt`, `agent`). None of
// those six has a field on the shared hook spec (the `type` field's own
// doc entry in docs/user/spec-format.md ties it to Codex's `mcp_tool`
// only), so nothing here can reach them; they stay unset rather than
// guessed.
//
// `args` switches the entry to exec form: "`command` is the path/name
// of a single executable, and each element of `args` is one literal
// argv entry. The CLI runs the binary directly without a shell"
// (docs.qoder.com/cli/hooks, "Exec form vs Shell form", verified
// 2026-09-12, #746). Same semantics Claude Code documents, so the
// shared struct's Args field carries it unchanged. That page also says
// "the `shell` field is ignored when `args` is set". A spec setting
// both still writes both: the key is valid there and dropping it would
// lose what the user authored. The user hears about it through a field
// no-op note instead.
//
// Returns nil when no hook spec produces an entry.
func buildHooksBlock(hooks []spec.Entry) *emit.OrderedJSON {
	type matcherKey struct{ event, matcher string }
	byKey := map[matcherKey][]claudehooks.CommandEntry{}
	var keyOrder []matcherKey
	execFormShell := 0

	for _, h := range hooks {
		event, _ := h.Meta["event"].(string)
		if event == "" {
			continue
		}
		commands := emit.HookCommands(h.Meta["command"])
		if len(commands) == 0 {
			continue
		}
		matcher, _ := h.Meta["matcher"].(string)
		timeout := emit.HookIntMeta(h.Meta, "timeout")
		statusMessage, _ := h.Meta["statusMessage"].(string)
		async := emit.HookBoolMeta(h.Meta, "async")
		asyncRewake := emit.HookBoolMeta(h.Meta, "asyncRewake")
		shell, _ := h.Meta["shell"].(string)
		ifRule, _ := h.Meta["if"].(string)
		once := emit.HookBoolMeta(h.Meta, "once")
		args := emit.StringSlice(h.Meta["args"])
		if len(args) > 0 && shell != "" {
			execFormShell++
		}

		k := matcherKey{event: event, matcher: matcher}
		if _, seen := byKey[k]; !seen {
			keyOrder = append(keyOrder, k)
		}
		for _, command := range commands {
			byKey[k] = append(byKey[k], claudehooks.CommandEntry{
				Type:          "command",
				Command:       emit.RewriteHookPath(command, target),
				Args:          args,
				Timeout:       timeout,
				StatusMessage: statusMessage,
				Async:         async,
				AsyncRewake:   asyncRewake,
				Shell:         shell,
				If:            ifRule,
				Once:          once,
			})
		}
	}
	emit.NoteFieldNoOp(target, spec.KindHook, "shell", execFormShell,
		"Qoder ignores shell once args is set: exec form runs the binary directly, with no shell")
	if len(keyOrder) == 0 {
		return nil
	}

	byEvent := map[string][]claudehooks.Group{}
	var eventOrder []string
	for _, k := range keyOrder {
		if _, seen := byEvent[k.event]; !seen {
			eventOrder = append(eventOrder, k.event)
		}
		byEvent[k.event] = append(byEvent[k.event], claudehooks.Group{Matcher: k.matcher, Hooks: byKey[k]})
	}
	for event, groups := range byEvent {
		sort.SliceStable(groups, func(i, j int) bool { return groups[i].Matcher < groups[j].Matcher })
		byEvent[event] = groups
	}

	doc := emit.NewOrderedJSON()
	for _, event := range orderHookEvents(eventOrder) {
		_ = doc.Set(event, byEvent[event])
	}
	return doc
}

// orderHookEvents puts the vendor's documented lifecycle order first,
// then any remaining event in the order it was first seen.
func orderHookEvents(seen []string) []string {
	present := map[string]bool{}
	for _, e := range seen {
		present[e] = true
	}
	out := make([]string, 0, len(seen))
	for _, e := range hookLifecycle {
		if present[e] {
			out = append(out, e)
			delete(present, e)
		}
	}
	for _, e := range seen {
		if present[e] {
			out = append(out, e)
			delete(present, e)
		}
	}
	return out
}
