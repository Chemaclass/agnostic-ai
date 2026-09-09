package openhands

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/claudehooks"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const defaultHooksFile = ".openhands/hooks.json"

// claudeToolNames are the Claude-style matcher values OpenHands does not
// answer to. Its own tool vocabulary is different, and the vendor names
// the trap itself: "The main differences are the file location
// (`.openhands/hooks.json` vs `.claude/settings.json`) and tool names
// (e.g., `terminal` vs `Bash`)"
// (docs.openhands.dev/openhands/usage/customization/hooks). A matcher
// carried over from a Claude spec parses fine and then matches nothing,
// so it earns a coverage note rather than a silent pass-through.
//
// No mapping is attempted. Only `terminal` is documented, with `*` and
// regex as the other two matcher forms, so there is no vendor-stated
// counterpart for the rest and guessing one would swap a visible miss
// for an invisible one.
var claudeToolNames = map[string]bool{
	"Bash": true, "Read": true, "Write": true, "Edit": true,
	"Glob": true, "Grep": true, "WebFetch": true, "WebSearch": true,
}

// hooksDoc is the `.openhands/hooks.json` shape:
// `{"hooks": {"<Event>": [{matcher, hooks: [...]}]}}`. Events are held
// in an ordered list so lifecycle order survives marshaling, since Go
// alphabetizes map keys.
type hooksDoc struct {
	order  []string
	events map[string][]claudehooks.Group
}

// MarshalJSON renders `{"hooks": {<Event>: [...], ...}}` in order.
func (d *hooksDoc) MarshalJSON() ([]byte, error) {
	var buf strings.Builder
	buf.WriteString(`{"hooks":{`)
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

// hookLifecycle is the event order the vendor's own Hook Types table
// uses. Events outside it follow in first-seen order.
var hookLifecycle = []string{
	"PreToolUse", "PostToolUse", "UserPromptSubmit",
	"Stop", "SessionStart", "SessionEnd",
}

// emitHooks writes `.openhands/hooks.json`, the file OpenHands reads
// "per-repository" and honors "across Cloud, CLI, and local GUI setups".
//
// The emitted form is the Claude-compatible one the vendor documents as
// equivalent to its own snake_case layout: "PascalCase event keys (e.g.,
// `PreToolUse`) and the `{"hooks": {...}}` wrapper are both supported, so
// you can share hook scripts between the two tools". Emitting that shape
// rather than the native `pre_tool_use` keys keeps one renderer serving
// both targets, and a hook spec stays byte-comparable across them.
//
// Only the four fields the vendor documents on a hook entry are written:
// `command` (required), `type` (default `command`), `timeout` (seconds,
// default 60), and `async`. Claude-only keys on the shared struct stay
// absent because nothing here sets them.
//
// No-op when no hooks emit.
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

// buildHooks returns the rendered document, or nil when no spec produces
// a hook entry.
func buildHooks(hooks []spec.Entry) *hooksDoc {
	type matcherKey struct{ event, matcher string }
	byKey := map[matcherKey][]claudehooks.CommandEntry{}
	keyOrder := []matcherKey{}
	var claudeMatchers int

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
		if claudeToolNames[matcher] {
			claudeMatchers++
		}
		k := matcherKey{event: event, matcher: matcher}
		if _, seen := byKey[k]; !seen {
			keyOrder = append(keyOrder, k)
		}
		for _, command := range commands {
			byKey[k] = append(byKey[k], claudehooks.CommandEntry{
				Type:    "command",
				Command: emit.RewriteHookPath(command, target),
				Timeout: emit.HookIntMeta(h.Meta, "timeout"),
				Async:   emit.HookBoolMeta(h.Meta, "async"),
			})
		}
	}
	emit.NoteFieldNoOp(target, spec.KindHook, "matcher", claudeMatchers,
		"OpenHands names its own tools (terminal, not Bash), so a Claude-style matcher parses but matches nothing; use the OpenHands tool name, `*`, or a regex")
	if len(keyOrder) == 0 {
		return nil
	}

	doc := &hooksDoc{events: map[string][]claudehooks.Group{}}
	for _, k := range keyOrder {
		if _, seen := doc.events[k.event]; !seen {
			doc.order = append(doc.order, k.event)
		}
		doc.events[k.event] = append(doc.events[k.event], claudehooks.Group{Matcher: k.matcher, Hooks: byKey[k]})
	}
	for event, groups := range doc.events {
		sort.SliceStable(groups, func(i, j int) bool { return groups[i].Matcher < groups[j].Matcher })
		doc.events[event] = groups
	}
	doc.order = orderEvents(doc.order)
	return doc
}

// orderEvents puts the vendor's documented lifecycle first, then any
// remaining event in the order it was seen.
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
