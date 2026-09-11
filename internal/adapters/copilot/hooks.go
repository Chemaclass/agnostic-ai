package copilot

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// defaultHooksFile is one of possibly several `*.json` files Copilot
// loads and merges from `.github/hooks/`: "Repository-level hook
// files — .github/hooks/*.json in the repository root"
// (docs.github.com/en/copilot/reference/hooks-reference, "Hooks
// locations"). A distinct filename keeps this write from colliding
// with any hand-authored file a user already has under the same
// directory (#629).
const defaultHooksFile = ".github/hooks/agnostic-ai.json"

// claudeToolNames are Claude Code's own PascalCase tool names. A
// spec's `matcher:` carries one of these over when authored against
// Claude Code, Codex, OpenHands, Windsurf, or Qoder. Copilot answers
// to them correctly when `event:` is also PascalCase (`PreToolUse`),
// the "VS Code compatible" form the vendor documents as applying
// "Claude's matcher semantics instead of the native regex rule". Only
// when an author deliberately switches to Copilot's own camelCase
// event form (`preToolUse`) does a Claude-style matcher stop working:
// it is then tested as a plain, case-sensitive regex against
// Copilot's lowercase runtime tool names (`bash`, `edit`, `view`,
// ...) and matches nothing. See buildHooks and the package doc
// comment for the vendor quotes.
var claudeToolNames = map[string]bool{
	"Bash": true, "Read": true, "Write": true, "Edit": true,
	"Glob": true, "Grep": true, "WebFetch": true, "WebSearch": true,
	"Task": true, "AskUserQuestion": true, "TodoWrite": true,
}

// hookLifecycle orders the PascalCase spelling of every event
// docs.github.com/en/copilot/reference/hooks-reference's own "Hook
// events" table lists, in that table's own row order. A spec authored
// in Copilot's native camelCase form (or any other casing) is not in
// this list and falls through to first-seen order in orderEvents,
// same as every other event this adapter does not recognize.
var hookLifecycle = []string{
	"SessionStart", "UserPromptSubmit",
	"PreToolUse", "PostToolUse", "PostToolUseFailure",
	"SubagentStart", "SubagentStop", "Stop",
	"PreCompact", "PermissionRequest", "Notification",
	"ErrorOccurred", "SessionEnd",
}

// hookEntry mirrors one flat object inside `hooks.<event>`. Unlike
// Claude Code, OpenHands, and Qoder's shared `{matcher, hooks:
// [...]}` Group wrapper, Copilot puts `matcher` directly on the entry
// alongside `type` and the command fields: `{"type": "command",
// "matcher": "bash|edit", "bash": "./scripts/log-tool.sh"}`
// (docs.github.com/en/copilot/reference/hooks-reference, "postToolUse
// output"), so this struct has no nested array.
//
// Only the vendor's documented cross-platform fallback field,
// `command`, emits: "Copied to both bash and powershell when those
// fields are absent". Nothing here sets `bash`/`powershell`/`exec`
// since agnostic-ai's generic hook spec carries one OS-agnostic
// command string, the same choice every other hook emitter in this
// repo makes. `timeoutSec` is the vendor's field name; `timeout` is
// documented only as a deprecated alias consumed when `timeoutSec` is
// absent, so this adapter writes the current name.
type hookEntry struct {
	Type       string `json:"type"`
	Matcher    string `json:"matcher,omitempty"`
	Command    string `json:"command,omitempty"`
	TimeoutSec int    `json:"timeoutSec,omitempty"`
}

// hooksDoc is the `.github/hooks/*.json` shape: an integer `version`
// pegged to 1 and a `hooks` map keyed by event name. Events are held
// in an ordered list so lifecycle order survives marshaling, since Go
// alphabetizes map keys.
type hooksDoc struct {
	order  []string
	events map[string][]hookEntry
}

// MarshalJSON renders `{"version":1,"hooks":{<event>: [...], ...}}` in
// order.
func (d *hooksDoc) MarshalJSON() ([]byte, error) {
	var buf strings.Builder
	buf.WriteString(`{"version":1,"hooks":{`)
	for i, event := range d.order {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := json.Marshal(event)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		val, err := json.Marshal(d.events[event])
		if err != nil {
			return nil, err
		}
		buf.Write(val)
	}
	buf.WriteString("}}")
	return []byte(buf.String()), nil
}

// emitHooks writes `.github/hooks/agnostic-ai.json`. No-op when no
// hooks emit. The path is overridable via `outputs.copilot.hooks-file`.
func emitHooks(sess *emit.Session, hooks []spec.Entry, cfg *config.Config, dryRun bool) error {
	doc := buildHooks(hooks)
	if doc == nil {
		return nil
	}
	body, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	path := emit.OutputHooksFile(cfg, target, defaultHooksFile)
	return sess.WriteFile(path, string(body)+"\n", dryRun)
}

// buildHooks returns the rendered document, or nil when no spec
// produces a hook entry. `event:` passes through verbatim into the
// JSON key, matching every other hook emitter in this repo and
// docs/user/spec-format.md's own stated policy ("agnostic-ai emits
// the event: value verbatim into each target's schema; it does not
// translate event names between tools"): both `PreToolUse` and
// `preToolUse` are literal, vendor-documented, independently valid
// keys here (see the package doc comment), so there is nothing to
// map. A spec that reuses this repo's dominant PascalCase vocabulary
// (Claude Code, Codex, OpenHands, Windsurf, Qoder) lands on Copilot's
// "VS Code compatible" payload format and inherits Claude's matcher
// semantics for free; one authored in Copilot's own camelCase form
// answers to Copilot's own lowercase tool-name matchers instead.
func buildHooks(hooks []spec.Entry) *hooksDoc {
	byEvent := map[string][]hookEntry{}
	var eventOrder []string
	var camelMatcherTraps int

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
		if isCamelCaseEvent(event) && claudeToolNames[matcher] {
			camelMatcherTraps++
		}
		timeout := emit.HookIntMeta(h.Meta, "timeout")
		if _, seen := byEvent[event]; !seen {
			eventOrder = append(eventOrder, event)
		}
		for _, command := range commands {
			byEvent[event] = append(byEvent[event], hookEntry{
				Type:       "command",
				Matcher:    matcher,
				Command:    emit.RewriteHookPath(command, target),
				TimeoutSec: timeout,
			})
		}
	}
	emit.NoteFieldNoOp(target, spec.KindHook, "matcher", camelMatcherTraps,
		"a PascalCase event (e.g. PreToolUse) applies Claude's own matcher semantics and tool names, but Copilot's native camelCase form (preToolUse) tests the matcher as a plain, case-sensitive regex against Copilot's own lowercase tool names, so a Claude-style matcher parses and then matches nothing there; use Copilot's own tool name, switch the event to its PascalCase form, or use a regex")
	if len(eventOrder) == 0 {
		return nil
	}
	for event, entries := range byEvent {
		sort.SliceStable(entries, func(i, j int) bool { return entries[i].Matcher < entries[j].Matcher })
		byEvent[event] = entries
	}
	return &hooksDoc{events: byEvent, order: orderEvents(eventOrder)}
}

// isCamelCaseEvent reports whether event starts with a lowercase
// letter, Copilot's own native casing (`preToolUse`) as opposed to
// the "VS Code compatible" PascalCase form (`PreToolUse`) this repo's
// other hook emitters share.
func isCamelCaseEvent(event string) bool {
	if event == "" {
		return false
	}
	first := event[0]
	return first >= 'a' && first <= 'z'
}

// orderEvents puts the vendor's documented lifecycle first (in its
// PascalCase spelling), then any remaining event — including one
// authored in camelCase or any other casing this adapter does not
// enumerate — in the order it was first seen.
func orderEvents(seen []string) []string {
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
