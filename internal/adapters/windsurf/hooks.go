package windsurf

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const defaultHooksFile = ".devin/hooks.v1.json"

// claudeToolNames are the Claude-style matcher values Devin CLI does
// not answer to. Its own tool vocabulary is lowercase and snake_case
// (`exec`, `edit`, `read`, `write`, `apply_patch`, `grep`, `glob`,
// `webfetch`, ...; docs.devin.ai/cli/extensibility/hooks/
// lifecycle-hooks), so a matcher carried over from a Claude spec
// parses as a valid regex and then matches nothing, the same trap
// openhands and antigravity hit with their own tool vocabularies. No
// rename is attempted: the vendor's own note that "the matcher is not
// a permission glob" and the case-sensitive lowercase set give no safe
// guess for a Claude name outside it.
var claudeToolNames = map[string]bool{
	"Bash": true, "Read": true, "Write": true, "Edit": true,
	"Glob": true, "Grep": true, "WebFetch": true, "WebSearch": true,
}

// hookEntry mirrors one `{type, command|prompt, timeout}` object in a
// matcher group's `hooks` array. Devin CLI's hook format documents two
// types: `"command"` runs a shell command (the `command` field);
// `"prompt"` evaluates an LLM prompt (the `prompt` field) instead.
// agnostic-ai's generic hook spec has no `prompt` field, so that shape
// is reachable only through a hand-authored `type: prompt` plus
// `prompt: <text>` on the spec's own Meta (there is nothing to
// translate: the two fields are mutually exclusive on the wire, and
// `omitempty` keeps whichever one a given entry does not set out of
// the rendered JSON).
type hookEntry struct {
	Type    string `json:"type"`
	Command string `json:"command,omitempty"`
	Prompt  string `json:"prompt,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
}

// hookGroup mirrors one `{matcher, hooks}` object in a hook event's
// array. Matcher stays a plain (non-omitempty) string: the vendor
// states an empty matcher is itself meaningful ("use `\"\"` or omit
// the matcher to run the hook for every event of that type"), and its
// own quick-start example always writes the key.
type hookGroup struct {
	Matcher string      `json:"matcher"`
	Hooks   []hookEntry `json:"hooks"`
}

// hooksDoc is `.devin/hooks.v1.json`'s exact top-level shape:
// `{"<Event>": [{matcher, hooks: [...]}]}`, with **no wrapper key**:
// "In `.devin/hooks.v1.json`, the hooks object is the entire file (no
// wrapper key needed)" (docs.devin.ai/cli/extensibility/hooks/
// overview). This is the one divergence from the Claude-shaped
// `{"hooks": {...}}` form Claude Code, Codex, Gemini, and Qoder share,
// so this adapter cannot reuse `claudehooks` or copy claude/codex/
// openhands' wrapped `MarshalJSON`. Order is stored on the side so the
// vendor's own Hook Events table order survives Go's map-key
// alphabetizer.
type hooksDoc struct {
	order  []string
	events map[string][]hookGroup
}

// MarshalJSON renders `{<Event>: [...], ...}` in `order`, with no
// surrounding `"hooks"` key.
func (d *hooksDoc) MarshalJSON() ([]byte, error) {
	var buf strings.Builder
	buf.WriteByte('{')
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
	buf.WriteByte('}')
	return []byte(buf.String()), nil
}

// hookLifecycle is the event order docs.devin.ai/cli/extensibility/
// hooks/overview's own Hook Events table uses. Events outside it
// follow in first-seen order.
var hookLifecycle = []string{
	"PreToolUse", "PostToolUse", "PermissionRequest", "UserPromptSubmit",
	"Stop", "PostCompaction", "SessionStart", "SessionEnd",
}

// emitHooks writes `.devin/hooks.v1.json` (override via
// outputs.windsurf.hooks-file), the file Devin CLI reads at project
// scope: "Create `.devin/hooks.v1.json` in your project" (#629).
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

// buildHooks returns the rendered document, or nil when no spec
// produces a hook entry. A `type: prompt` spec carries its text in a
// `prompt` Meta key (see the hookEntry doc); everything else renders
// as a `type: command` entry the same way claude/codex/openhands do.
func buildHooks(hooks []spec.Entry) *hooksDoc {
	type matcherKey struct{ event, matcher string }
	byKey := map[matcherKey][]hookEntry{}
	keyOrder := []matcherKey{}
	var claudeMatchers int

	for _, h := range hooks {
		event, _ := h.Meta["event"].(string)
		if event == "" {
			continue
		}
		matcher, _ := h.Meta["matcher"].(string)
		if claudeToolNames[matcher] {
			claudeMatchers++
		}
		timeout := emit.HookIntMeta(h.Meta, "timeout")
		k := matcherKey{event: event, matcher: matcher}

		if hookType, _ := h.Meta["type"].(string); hookType == "prompt" {
			prompt, _ := h.Meta["prompt"].(string)
			if prompt == "" {
				continue
			}
			if _, seen := byKey[k]; !seen {
				keyOrder = append(keyOrder, k)
			}
			byKey[k] = append(byKey[k], hookEntry{Type: "prompt", Prompt: prompt, Timeout: timeout})
			continue
		}

		commands := emit.HookCommands(h.Meta["command"])
		if len(commands) == 0 {
			continue
		}
		if _, seen := byKey[k]; !seen {
			keyOrder = append(keyOrder, k)
		}
		for _, command := range commands {
			byKey[k] = append(byKey[k], hookEntry{
				Type:    "command",
				Command: emit.RewriteHookPath(command, target),
				Timeout: timeout,
			})
		}
	}
	emit.NoteFieldNoOp(target, spec.KindHook, "matcher", claudeMatchers,
		"Devin CLI's tool names are lowercase and snake_case (exec, edit, read, ...), not Claude's; a Claude-style matcher parses but matches nothing, see docs.devin.ai/cli/extensibility/hooks/lifecycle-hooks")
	if len(keyOrder) == 0 {
		return nil
	}

	doc := &hooksDoc{events: map[string][]hookGroup{}}
	for _, k := range keyOrder {
		if _, seen := doc.events[k.event]; !seen {
			doc.order = append(doc.order, k.event)
		}
		doc.events[k.event] = append(doc.events[k.event], hookGroup{Matcher: k.matcher, Hooks: byKey[k]})
	}
	for event, groups := range doc.events {
		sort.SliceStable(groups, func(i, j int) bool { return groups[i].Matcher < groups[j].Matcher })
		doc.events[event] = groups
	}
	doc.order = orderHookEvents(doc.order)
	return doc
}

// orderHookEvents puts the vendor's documented lifecycle first, then
// any remaining event in the order it was seen.
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
