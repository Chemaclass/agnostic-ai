package augment

import (
	"slices"
	"sort"

	"github.com/chemaclass/agnostic-ai/internal/adapters/claudehooks"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// hooksKey is the settings.json key holding the hooks block.
const hooksKey = "hooks"

// hookLifecycle is the event order docs.augmentcode.com/cli/hooks' own
// "Hook Events" section lists for Auggie 1.x: PreToolUse, PostToolUse,
// Stop, SessionStart, SessionEnd, PromptSubmit, Notification. The 0.x
// reference documents the first five as events with a section and
// Notification only as a `hook_event_name` value; the settings file is
// shared, so one list serves both. A hook on another event is skipped,
// since Auggie never fires it.
var hookLifecycle = []string{"PreToolUse", "PostToolUse", "Stop", "SessionStart", "SessionEnd", "PromptSubmit", "Notification"}

// HookEvents lists the events a hook spec may name for this target.
func HookEvents() []string { return append([]string(nil), hookLifecycle...) }

// UserPromptSubmitHint says what to write instead of Claude Code's
// UserPromptSubmit, which Auggie does not fire.
const UserPromptSubmitHint = "Auggie has no UserPromptSubmit event; its PromptSubmit passes the prompt as user_prompt and cannot block, so set `x-augment: {event: PromptSubmit}` if the hook fits that"

func knownHookEvent(event string) bool { return slices.Contains(hookLifecycle, event) }

// sessionOnlyEvents are the events the vendor documents as taking no
// matcher: Stop, SessionStart, SessionEnd, and, in Auggie 1.x,
// PromptSubmit and Notification. The vendor's own SessionStart example
// omits the "matcher" key outright rather than writing an empty string,
// so this adapter never sets Matcher for these events regardless of what
// a hook spec's matcher field carries.
var sessionOnlyEvents = map[string]bool{
	"Stop": true, "SessionStart": true, "SessionEnd": true,
	"PromptSubmit": true, "Notification": true,
}

// claudeToolNames are the Claude-style matcher values Augment's own
// PreToolUse/PostToolUse tool vocabulary does not answer to
// (`launch-process`, `str-replace-editor`, `save-file`, `remove-files`,
// `view`, `web-fetch`, `web-search`, `codebase-retrieval`, `github-api`,
// `linear`; see the package doc's agent `tools` section for the same
// list). A matcher carried over from a Claude spec parses as a valid
// regex and then matches nothing, so it earns a coverage note rather
// than a silent pass-through, the same treatment openhands and windsurf
// give their own mismatched tool vocabularies. No mapping is attempted:
// there is no vendor-stated Augment counterpart for `Read`, `Grep`, or
// the rest, so guessing one would swap a visible miss for an invisible
// one.
var claudeToolNames = map[string]bool{
	"Bash": true, "Read": true, "Write": true, "Edit": true,
	"Glob": true, "Grep": true, "WebFetch": true, "WebSearch": true,
}

// hookGroup mirrors one `{matcher, hooks}` object inside a
// `.augment/settings.json` hooks event array. Not the shared
// claudehooks.Group: that struct's Matcher field has no `omitempty`, so
// it always writes `"matcher": ""` on a matcher-less event. Augment's
// own vendor examples omit the key outright for the matcher-less
// events, and treat it as genuinely optional (default `.*`) rather than
// required on PreToolUse/PostToolUse too, so this adapter holds its own
// wrapper with `omitempty` to reproduce that shape exactly.
type hookGroup struct {
	Matcher string                     `json:"matcher,omitempty"`
	Hooks   []claudehooks.CommandEntry `json:"hooks"`
}

// buildHooksBlock renders the value merged under the top-level `hooks`
// key in `.augment/settings.json`: `{"<Event>": [{"matcher"?, "hooks":
// [{"type", "command", "args"?, "timeout"?}]}]}`. A hook entry takes
// `type` (always "command", the only type a generic command spec can
// express), `command`, `args`, and `timeout`, so this
// reuses claudehooks.CommandEntry for that level: every field this
// adapter never sets (statusMessage, async, shell, ...) stays absent
// via `omitempty`, the same reuse openhands and qoder already make for
// their own narrower field sets.
//
// `timeout` converts unit: the shared hook spec's own `timeout` field
// is seconds (docs/site/content/docs/spec-format/hooks.md), Augment's is milliseconds
// (default 60000 when the key is absent), so the value is multiplied
// by 1000 before it reaches this struct's Timeout field. A spec that
// never sets `timeout` produces 0, which `omitempty` drops, and
// Augment's own default takes over.
//
// `args` stays apart: Auggie 1.x runs the command with them and no
// shell, and 0.37.1 reads the same field. A command without args runs
// as written; Auggie 1.x hands it to a shell, and 0.37.1 splits it into
// words or runs it with bash -c. A matcher equal to one of Claude
// Code's tool names buffers a coverage note (see claudeToolNames).
//
// Returns nil when no hook spec produces an entry.
func buildHooksBlock(hooks []spec.Entry) *emit.OrderedJSON {
	type matcherKey struct{ event, matcher string }
	byKey := map[matcherKey][]claudehooks.CommandEntry{}
	var keyOrder []matcherKey
	var claudeMatchers, droppedMatchers, promptHooks, otherEvents int

	for _, h := range hooks {
		event, _ := h.Meta["event"].(string)
		switch {
		case event == "":
			continue
		case event == "UserPromptSubmit":
			promptHooks++
			emit.NoteEntryOmitted(target, spec.KindHook, h.Name)
			continue
		case !knownHookEvent(event):
			otherEvents++
			emit.NoteEntryOmitted(target, spec.KindHook, h.Name)
			continue
		}
		commands := emit.HookCommands(h.Meta["command"])
		if len(commands) == 0 {
			continue
		}
		matcher, _ := h.Meta["matcher"].(string)
		if sessionOnlyEvents[event] {
			if matcher != "" {
				droppedMatchers++
			}
			matcher = ""
		} else if claudeToolNames[matcher] {
			claudeMatchers++
		}
		timeoutMs := emit.HookIntMeta(h.Meta, "timeout") * 1000
		args := emit.HookArgs(target, h.Meta)

		k := matcherKey{event: event, matcher: matcher}
		if _, seen := byKey[k]; !seen {
			keyOrder = append(keyOrder, k)
		}
		for _, command := range commands {
			byKey[k] = append(byKey[k], claudehooks.CommandEntry{
				Type:    "command",
				Command: emit.RewriteHookPath(command, target, h.Meta),
				Args:    args,
				Timeout: timeoutMs,
			})
		}
	}
	emit.NoteFieldNoOp(target, spec.KindHook, "matcher", claudeMatchers,
		"Augment names its own tools (launch-process, str-replace-editor, save-file, ...), so a Claude-style matcher parses but matches nothing; use one of Augment's own tool names, `.*`, or a regex over them")
	emit.NoteCoverageGap(target, spec.KindHook, promptHooks, UserPromptSubmitHint)
	emit.NoteCoverageGap(target, spec.KindHook, otherEvents,
		"Auggie has no such hook event; use PreToolUse, PostToolUse, Stop, SessionStart, SessionEnd, PromptSubmit, or Notification")
	emit.NoteFieldNoOp(target, spec.KindHook, "matcher", droppedMatchers,
		"Stop, SessionStart, SessionEnd, PromptSubmit, and Notification take no matcher in Augment, so it is left out")
	if len(keyOrder) == 0 {
		return nil
	}

	byEvent := map[string][]hookGroup{}
	var eventOrder []string
	for _, k := range keyOrder {
		if _, seen := byEvent[k.event]; !seen {
			eventOrder = append(eventOrder, k.event)
		}
		byEvent[k.event] = append(byEvent[k.event], hookGroup{Matcher: k.matcher, Hooks: byKey[k]})
	}
	for event, groups := range byEvent {
		sort.SliceStable(groups, func(i, j int) bool { return groups[i].Matcher < groups[j].Matcher })
		byEvent[event] = groups
	}

	doc := emit.NewOrderedJSON()
	for _, event := range emit.OrderByLifecycle(eventOrder, hookLifecycle) {
		_ = doc.Set(event, byEvent[event])
	}
	return doc
}

// SessionOnlyEvent reports whether event takes no matcher, so a user
// hooks entry for it leaves the key out, as the vendor examples do.
func SessionOnlyEvent(event string) bool { return sessionOnlyEvents[event] }

// NoteUserHookGaps raises the coverage note the project hooks block
// raises for a Claude-style matcher, for hooks sync --global writes to
// ~/.augment/settings.json.
func NoteUserHookGaps(hooks []spec.Entry) { buildHooksBlock(hooks) }

// HookTimeout converts a spec's timeout in seconds to the milliseconds
// Augment reads.
func HookTimeout(meta map[string]any) (any, bool) {
	if timeout := emit.HookIntMeta(meta, "timeout"); timeout > 0 {
		return timeout * 1000, true
	}
	return nil, false
}

// UserMCPServers renders mcps as the `mcpServers` map of
// ~/.augment/settings.json, the same builder the project file uses.
func (Adapter) UserMCPServers(mcps []spec.Entry) map[string]any {
	mcps = emit.DropMCPDisabled(target, mcps, "a disabled server is left out of ~/.augment/settings.json, which has no per-server disable key")
	mcps = emit.DropMCPWebSocket(target, mcps, mcpWebSocketGapReason)
	return emit.BuildMCPServersMap(mcps, emit.MCPSchemaServersMap)
}
