package factory

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const defaultHooksFile = ".factory/hooks.json"

// hookLifecycle is the event order docs.factory.ai/harness/hooks' own
// Event Reference table uses: PreToolUse, PostToolUse, UserPromptSubmit,
// Notification, Stop, SubagentStop, PreCompact, SessionStart,
// SessionEnd (re-verified 2026-09-11 against that page; #629). Events
// outside this list follow in first-seen order.
var hookLifecycle = []string{
	"PreToolUse", "PostToolUse", "UserPromptSubmit", "Notification",
	"Stop", "SubagentStop", "PreCompact", "SessionStart", "SessionEnd",
}

// claudeToolNames flags the Claude-style matcher values Droid CLI's own
// tool vocabulary spells differently. docs.factory.ai/harness/hooks'
// own "Common tool matchers" are Execute, Read, Edit, Create,
// ApplyPatch, LS, Glob, Grep, Task, FetchUrl, and WebSearch, the same
// table tools.go's factoryToolID already translates for the `tools`
// frontmatter field. Read, Edit, Glob, Grep, WebSearch, and Task
// already match Claude's own spelling; Bash, Write, and WebFetch do
// not, so a matcher copied verbatim from a Claude spec parses as a
// valid regex and then matches nothing, the same trap crush and
// windsurf hit with their own tool vocabularies.
var claudeToolNames = map[string]bool{"Bash": true, "Write": true, "WebFetch": true}

// hookEntry mirrors one `{type, command, timeout}` object in a matcher
// group's `hooks` array. docs.factory.ai/harness/hooks' own field
// table marks `type` required and states "Currently only \"command\"
// is supported", so this always writes "command"; there is no `type:
// mcp_tool` or `type: prompt` alternative the way Codex and
// Windsurf/Devin CLI's own hook schemas carry.
type hookEntry struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

// hookGroup mirrors one `{matcher, hooks}` object in a hook event's
// array. Matcher stays non-omitempty: the vendor's own quickstart
// example always writes the key, and its field table states "Empty,
// omitted, or `*` matches everything", making an explicit empty
// string meaningful rather than absent.
//
// The vendor's other matcher-group field, `commandRegex` ("Additional
// regex filter for Execute commands"), has no counterpart on
// agnostic-ai's generic hook spec, so it stays documented here but
// unemitted, the same treatment factory.go's package doc gives
// Factory's unemitted MCP-only fields.
type hookGroup struct {
	Matcher string      `json:"matcher"`
	Hooks   []hookEntry `json:"hooks"`
}

// hooksDoc is `.factory/hooks.json`'s exact top-level shape:
// `{"<Event>": [{matcher, hooks: [...]}]}`, with no wrapper key.
// "Standalone `hooks.json` files are keyed directly by event name"
// (docs.factory.ai/harness/hooks), the one other divergence
// Windsurf/Devin CLI's `.devin/hooks.v1.json` also carries, so this
// borrows that adapter's side-order MarshalJSON rather than the
// wrapped `{"hooks": {...}}` shape claude/codex/qoder/openhands share.
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

// emitHooks writes `.factory/hooks.json` (override via
// outputs.factory.hooks-file), the project-tier file Droid CLI reads
// for hooks: "Project | `.factory/hooks.json` | Commit to share with
// teammates." (docs.factory.ai/harness/hooks, #629). This is its own
// file, separate from `.factory/mcp.json`, so it takes its own single
// `WriteFile` call rather than merging into an existing one. No-op
// when no hooks emit.
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
// produces a hook entry. `type` always renders "command": the
// vendor's field table documents no other value, the same "always
// command" choice qoder.go's buildHooksBlock already makes for the
// identical reason.
func buildHooks(hooks []spec.Entry) *hooksDoc {
	type matcherKey struct{ event, matcher string }
	byKey := map[matcherKey][]hookEntry{}
	var keyOrder []matcherKey
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
		timeout := emit.HookIntMeta(h.Meta, "timeout")

		k := matcherKey{event: event, matcher: matcher}
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
		"Droid CLI's own tool matchers are Execute, Create, and FetchUrl in place of Claude's Bash, Write, and WebFetch (the same three renames tools.go's factoryToolID performs for the tools frontmatter field); a Claude-style matcher parses but matches nothing")
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
