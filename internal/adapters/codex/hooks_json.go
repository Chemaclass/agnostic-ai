package codex

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const defaultHooksFile = ".codex/hooks.json"

// emitHooksJSON writes `.codex/hooks.json` using the same hook block
// schema Claude's `.claude/settings.json` exposes (per-event arrays of
// `{matcher, hooks: [{type, command, timeout, statusMessage}]}`).
// Codex CLI reads either this file or `[[hooks.<event>]]` blocks in
// `.codex/config.toml`; hooks.json is preferred because it preserves
// the per-hook `timeout` and `statusMessage` metadata that the TOML
// schema lacks.
//
// Matcher-aware dedupe: when two specs share an event and resolve to
// the same shell command (after `RewriteHookPath` normalization), their
// matcher pipe-segments are unioned into a single matcher string so
// Codex does not fire the same command twice on overlapping events.
//
// No-op when no hooks emit.
func emitHooksJSON(sess *emit.Session, hooks []spec.Entry, cfg *config.Config, dryRun bool) error {
	doc := buildHooksJSON(hooks)
	if doc == nil {
		return nil
	}
	body, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	path := emit.OutputHooksFile(cfg, target, defaultHooksFile)
	// `.codex/hooks.json` lives under .codex/ alongside config.toml;
	// WriteFile already handles parent-dir creation.
	return sess.WriteFile(path, string(body)+"\n", dryRun)
}

// hooksDoc is the top-level `.codex/hooks.json` shape:
// `{"hooks": {"<Event>": [matcherGroup, ...]}}`. Stored as an ordered
// list so the lifecycle order (PreToolUse before PostToolUse, etc.)
// survives JSON marshaling. Codex CLI is order-insensitive but the
// emitter writes the order codex docs and hand-authored files use so
// `sync --check` stays stable.
type hooksDoc struct {
	Order  []string
	Events map[string][]matcherGroup
}

// MarshalJSON renders the doc as `{"hooks": {<Event>: [...], ...}}`
// with events serialized in `Order`. Bypasses Go's map-key alphabetizer.
func (d *hooksDoc) MarshalJSON() ([]byte, error) {
	var buf strings.Builder
	buf.WriteString(`{"hooks":{`)
	for i, event := range d.Order {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := json.Marshal(event)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		val, err := json.Marshal(d.Events[event])
		if err != nil {
			return nil, err
		}
		buf.Write(val)
	}
	buf.WriteString("}}")
	return []byte(buf.String()), nil
}

type matcherGroup struct {
	Matcher string      `json:"matcher,omitempty"`
	Hooks   []hookEntry `json:"hooks"`
}

// hookEntry is either a hookCommandEntry or a hookMCPToolEntry.
// learn.chatgpt.com/docs/hooks documents both shapes in the same
// `hooks[]` array; a shared interface (rather than a single struct
// with nullable command/server/tool fields) keeps each shape's
// required fields from leaking into the other's JSON. See #693.
type hookEntry interface {
	isHookEntry()
}

// hookBase holds the two fields learn.chatgpt.com/docs/hooks documents
// as shared between a command hook and an mcp_tool hook.
type hookBase struct {
	Timeout       int    `json:"timeout,omitempty"`
	StatusMessage string `json:"statusMessage,omitempty"`
}

type hookCommandEntry struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	hookBase
	// CommandWindows is Codex's optional Windows-specific command
	// override, propagated from the spec's `commandWindows` Meta key.
	CommandWindows string `json:"commandWindows,omitempty"`
	// AdditionalContextLimit caps how many tokens of this hook's output
	// reach the model, propagated from the spec's `additionalContextLimit`
	// Meta key. See learn.chatgpt.com/docs/hooks.
	AdditionalContextLimit *int `json:"additionalContextLimit,omitempty"`
	// Async runs the command hook in the background instead of blocking
	// the session on it, propagated from the spec's `async` Meta key.
	// See learn.chatgpt.com/docs/hooks.
	Async bool `json:"async,omitempty"`
}

func (hookCommandEntry) isHookEntry() {}

// hookMCPToolEntry calls a tool on an already-connected MCP server
// instead of running a shell command: "It sends structured arguments
// directly to the tool and uses the same trust review and output
// contract as a command hook." learn.chatgpt.com/docs/hooks. See #693.
type hookMCPToolEntry struct {
	Type   string         `json:"type"`
	Server string         `json:"server"`
	Tool   string         `json:"tool"`
	Input  map[string]any `json:"input,omitempty"`
	hookBase
}

func (hookMCPToolEntry) isHookEntry() {}

// buildHooksJSON returns the rendered document or nil when no hooks
// produce output. Walks the bundle, normalizes commands via
// RewriteHookPath, then collapses (event, kind, identity) duplicates by
// unioning their matcher segments. identity is the rewritten command
// for a command hook, or "server\x00tool" for an mcp_tool hook, so the
// two shapes never collide even when both fields happen to be empty
// (both are filtered out before a key is ever created).
func buildHooksJSON(hooks []spec.Entry) *hooksDoc {
	type key struct{ event, kind, identity string }
	type accum struct {
		kind                   string
		matchers               map[string]bool
		matcherOrder           []string
		timeout                int
		statusMessage          string
		commandWindows         string
		additionalContextLimit *int
		async                  bool
		server, tool           string
		input                  map[string]any
	}
	byKey := map[key]*accum{}
	keyOrder := []key{}

	for _, h := range hooks {
		event, _ := h.Meta["event"].(string)
		if event == "" {
			continue
		}
		matcher, _ := h.Meta["matcher"].(string)
		timeout := hookIntMeta(h.Meta, "timeout")
		statusMessage, _ := h.Meta["statusMessage"].(string)

		// An mcp_tool hook calls a tool on an already-connected MCP
		// server instead of running a shell command, so it carries no
		// `command` at all. Routing it through the command path below
		// would make hookCommands return nothing and drop the entry
		// silently. See #693.
		if hookType, _ := h.Meta["type"].(string); hookType == "mcp_tool" {
			server, _ := h.Meta["server"].(string)
			tool, _ := h.Meta["tool"].(string)
			if server == "" || tool == "" {
				continue
			}
			input, _ := h.Meta["input"].(map[string]any)
			k := key{event: event, kind: "mcp_tool", identity: server + "\x00" + tool}
			a, ok := byKey[k]
			if !ok {
				a = &accum{kind: "mcp_tool", matchers: map[string]bool{}, server: server, tool: tool}
				byKey[k] = a
				keyOrder = append(keyOrder, k)
			}
			for _, seg := range matcherSegments(matcher) {
				if !a.matchers[seg] {
					a.matchers[seg] = true
					a.matcherOrder = append(a.matcherOrder, seg)
				}
			}
			if a.timeout == 0 && timeout != 0 {
				a.timeout = timeout
			}
			if a.statusMessage == "" && statusMessage != "" {
				a.statusMessage = statusMessage
			}
			if a.input == nil && len(input) > 0 {
				a.input = input
			}
			continue
		}

		commandWindows, _ := h.Meta["commandWindows"].(string)
		additionalContextLimit := hookIntMetaPtr(h.Meta, "additionalContextLimit")
		async := hookBoolMeta(h.Meta, "async")
		for _, raw := range hookCommands(h.Meta["command"]) {
			cmd := emit.RewriteHookPath(raw, target)
			k := key{event: event, kind: "command", identity: cmd}
			a, ok := byKey[k]
			if !ok {
				a = &accum{kind: "command", matchers: map[string]bool{}}
				byKey[k] = a
				keyOrder = append(keyOrder, k)
			}
			for _, seg := range matcherSegments(matcher) {
				if !a.matchers[seg] {
					a.matchers[seg] = true
					a.matcherOrder = append(a.matcherOrder, seg)
				}
			}
			if a.timeout == 0 && timeout != 0 {
				a.timeout = timeout
			}
			if a.statusMessage == "" && statusMessage != "" {
				a.statusMessage = statusMessage
			}
			if a.commandWindows == "" && commandWindows != "" {
				a.commandWindows = commandWindows
			}
			if a.additionalContextLimit == nil && additionalContextLimit != nil {
				a.additionalContextLimit = additionalContextLimit
			}
			if async {
				a.async = true
			}
		}
	}

	if len(byKey) == 0 {
		return nil
	}

	type matcherCmdKey struct{ event, matcher string }
	groups := map[matcherCmdKey]*matcherGroup{}
	groupOrder := []matcherCmdKey{}
	for _, k := range keyOrder {
		a := byKey[k]
		// Dedupe groups by the canonical (sorted) matcher key so two
		// specs with `Edit|Write` and `Write|Edit` collapse together,
		// but emit the segments in author-supplied order so a hand-
		// authored matcher round-trips byte-stable.
		dedupeKey := joinMatcherSegments(a.matcherOrder)
		display := strings.Join(a.matcherOrder, "|")
		gk := matcherCmdKey{event: k.event, matcher: dedupeKey}
		g, ok := groups[gk]
		if !ok {
			g = &matcherGroup{Matcher: display}
			groups[gk] = g
			groupOrder = append(groupOrder, gk)
		}
		if a.kind == "mcp_tool" {
			g.Hooks = append(g.Hooks, hookMCPToolEntry{
				Type:     "mcp_tool",
				Server:   a.server,
				Tool:     a.tool,
				Input:    a.input,
				hookBase: hookBase{Timeout: a.timeout, StatusMessage: a.statusMessage},
			})
			continue
		}
		g.Hooks = append(g.Hooks, hookCommandEntry{
			Type:                   "command",
			Command:                k.identity,
			hookBase:               hookBase{Timeout: a.timeout, StatusMessage: a.statusMessage},
			CommandWindows:         a.commandWindows,
			AdditionalContextLimit: a.additionalContextLimit,
			Async:                  a.async,
		})
	}

	byEvent := map[string][]matcherGroup{}
	eventOrder := []string{}
	for _, gk := range groupOrder {
		if _, seen := byEvent[gk.event]; !seen {
			eventOrder = append(eventOrder, gk.event)
		}
		byEvent[gk.event] = append(byEvent[gk.event], *groups[gk])
	}

	doc := &hooksDoc{Events: map[string][]matcherGroup{}}
	for _, event := range orderedHookEvents(eventOrder) {
		doc.Order = append(doc.Order, event)
		doc.Events[event] = byEvent[event]
	}
	return doc
}

// matcherSegments splits a Codex/Claude `matcher` string into its
// pipe-separated alternatives. Empty matcher returns an empty slice so
// the unioner skips it cleanly.
func matcherSegments(matcher string) []string {
	if matcher == "" {
		return nil
	}
	out := make([]string, 0, 2)
	for _, seg := range strings.Split(matcher, "|") {
		seg = strings.TrimSpace(seg)
		if seg != "" {
			out = append(out, seg)
		}
	}
	return out
}

// joinMatcherSegments returns `seg1|seg2|...` with segments sorted so
// equivalent matcher sets produce a byte-stable matcher string. An
// empty slice yields "" (matcher omitted in JSON via omitempty).
func joinMatcherSegments(segments []string) string {
	if len(segments) == 0 {
		return ""
	}
	cp := append([]string(nil), segments...)
	sort.Strings(cp)
	return strings.Join(cp, "|")
}

// hookIntMeta reads an int-typed meta key, accepting int / int64 /
// float64 (yaml.v3 decodes numerics as int). Returns 0 when missing or
// the wrong type.
func hookIntMeta(meta map[string]any, key string) int {
	switch v := meta[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}

// hookIntMetaPtr distinguishes an explicit zero from an absent field. Codex
// assigns special meaning to additionalContextLimit = 0.
func hookIntMetaPtr(meta map[string]any, key string) *int {
	var value int
	switch raw := meta[key].(type) {
	case int:
		value = raw
	case int64:
		value = int(raw)
	case float64:
		value = int(raw)
	default:
		return nil
	}
	return &value
}

// hookBoolMeta reads a bool-typed meta key, accepting bool and the
// string forms "true"/"false" a hand-edited YAML may carry. Returns
// false when missing or any other type.
func hookBoolMeta(meta map[string]any, key string) bool {
	switch v := meta[key].(type) {
	case bool:
		return v
	case string:
		return v == "true"
	}
	return false
}

// orderedHookEvents reorders event names by the canonical Codex/Claude
// lifecycle (Pre before Post, etc.). Events not in the lifecycle list
// fall through to the tail in first-seen order.
func orderedHookEvents(seen []string) []string {
	lifecycle := []string{
		"SessionStart",
		"SubagentStart",
		"UserPromptSubmit",
		"PreToolUse",
		"PermissionRequest",
		"PostToolUse",
		"Notification",
		"PreCompact",
		"PostCompact",
		"Stop",
		"SubagentStop",
		"SessionEnd",
	}
	have := map[string]bool{}
	for _, e := range seen {
		have[e] = true
	}
	out := make([]string, 0, len(seen))
	for _, e := range lifecycle {
		if have[e] {
			out = append(out, e)
			have[e] = false
		}
	}
	for _, e := range seen {
		if have[e] {
			out = append(out, e)
			have[e] = false
		}
	}
	return out
}
