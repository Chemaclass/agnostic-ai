package augment

import (
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/claudehooks"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// hooksKey is the settings.json key holding the hooks block.
const hooksKey = "hooks"

// hookLifecycle is the event order docs.augmentcode.com/cli/hooks' own
// "Hook Events" section lists: PreToolUse, PostToolUse, Stop,
// SessionStart, SessionEnd. Five events total, re-verified fresh
// 2026-09-10, superseding an earlier audit comment that read six off a
// stale fetch: the vendor's `hook_event_name` common field lists
// `Notification` as one more possible value, but it has no
// configuration section of its own in the same page, so it is not a
// sixth event a settings.json hook can register against. Events
// outside this list follow in first-seen order.
var hookLifecycle = []string{"PreToolUse", "PostToolUse", "Stop", "SessionStart", "SessionEnd"}

// sessionOnlyEvents are the three events whose vendor field table marks
// matcher "Not used ... (SessionStart, SessionEnd, Stop)". The vendor's
// own SessionStart example omits the "matcher" key outright rather than
// writing an empty string, so this adapter never sets Matcher for these
// three regardless of what a hook spec's matcher field carries.
var sessionOnlyEvents = map[string]bool{"Stop": true, "SessionStart": true, "SessionEnd": true}

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

// scriptExtensions are the four extensions docs.augmentcode.com/cli/hooks'
// "Script Requirements" section documents as the only ones Augment
// dispatches: ".sh" runs directly on Unix via its own shebang line,
// ".ps1" is dispatched to `powershell.exe -Command` on Windows, and
// ".cmd"/".bat" are dispatched to `cmd.exe /c` on Windows. A command
// missing one of these four never runs; unlike Claude Code, Codex, or
// Qoder, Augment does not also accept an inline shell string.
var scriptExtensions = []string{".sh", ".ps1", ".cmd", ".bat"}

// hookGroup mirrors one `{matcher, hooks}` object inside a
// `.augment/settings.json` hooks event array. Not the shared
// claudehooks.Group: that struct's Matcher field has no `omitempty`, so
// it always writes `"matcher": ""` on a matcher-less event. Augment's
// own vendor examples omit the key outright for the three session
// events, and treat it as genuinely optional (default `.*`) rather than
// required on PreToolUse/PostToolUse too, so this adapter holds its own
// wrapper with `omitempty` to reproduce that shape exactly.
type hookGroup struct {
	Matcher string                     `json:"matcher,omitempty"`
	Hooks   []claudehooks.CommandEntry `json:"hooks"`
}

// buildHooksBlock renders the value merged under the top-level `hooks`
// key in `.augment/settings.json`: `{"<Event>": [{"matcher"?, "hooks":
// [{"type", "command", "timeout"?}]}]}`. Only three fields are
// documented on a hook entry (`type`, always "command", the only type a
// generic command spec can express; `command`; and `timeout`), so this
// reuses claudehooks.CommandEntry for that level: every field this
// adapter never sets (statusMessage, async, shell, ...) stays absent
// via `omitempty`, the same reuse openhands and qoder already make for
// their own narrower field sets.
//
// `timeout` converts unit: the shared hook spec's own `timeout` field
// is seconds (docs/user/spec-format.md), Augment's is milliseconds
// (default 60000 when the key is absent), so the value is multiplied
// by 1000 before it reaches this struct's Timeout field. A spec that
// never sets `timeout` produces 0, which `omitempty` drops, and
// Augment's own default takes over.
//
// `command` reaching the file without a recognized script extension
// (see scriptExtensions) still emits verbatim, no guessed rename, but
// buffers a coverage note: Augment silently never runs it. A matcher
// equal to one of Claude Code's tool names does the same (see
// claudeToolNames).
//
// Returns nil when no hook spec produces an entry.
func buildHooksBlock(hooks []spec.Entry) *emit.OrderedJSON {
	type matcherKey struct{ event, matcher string }
	byKey := map[matcherKey][]claudehooks.CommandEntry{}
	var keyOrder []matcherKey
	var badExtension, claudeMatchers int

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
		if sessionOnlyEvents[event] {
			matcher = ""
		} else if claudeToolNames[matcher] {
			claudeMatchers++
		}
		timeoutMs := emit.HookIntMeta(h.Meta, "timeout") * 1000

		k := matcherKey{event: event, matcher: matcher}
		if _, seen := byKey[k]; !seen {
			keyOrder = append(keyOrder, k)
		}
		for _, command := range commands {
			command = emit.RewriteHookPath(command, target)
			if !hasScriptExtension(command) {
				badExtension++
			}
			byKey[k] = append(byKey[k], claudehooks.CommandEntry{
				Type:    "command",
				Command: command,
				Timeout: timeoutMs,
			})
		}
	}
	emit.NoteFieldNoOp(target, spec.KindHook, "command", badExtension,
		"Augment only runs a hook command that is a path to a script ending in .sh, .ps1, .cmd, or .bat, not an inline shell string; a command missing one of these extensions is written but never executes")
	emit.NoteFieldNoOp(target, spec.KindHook, "matcher", claudeMatchers,
		"Augment names its own tools (launch-process, str-replace-editor, save-file, ...), so a Claude-style matcher parses but matches nothing; use one of Augment's own tool names, `.*`, or a regex over them")
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

// hasScriptExtension reports whether command ends in one of the four
// extensions Augment dispatches (see scriptExtensions).
func hasScriptExtension(command string) bool {
	for _, ext := range scriptExtensions {
		if strings.HasSuffix(command, ext) {
			return true
		}
	}
	return false
}
