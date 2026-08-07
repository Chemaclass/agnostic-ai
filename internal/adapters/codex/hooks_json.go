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
	Matcher string             `json:"matcher,omitempty"`
	Hooks   []hookCommandEntry `json:"hooks"`
}

type hookCommandEntry struct {
	Type          string `json:"type"`
	Command       string `json:"command"`
	Timeout       int    `json:"timeout,omitempty"`
	StatusMessage string `json:"statusMessage,omitempty"`
	// CommandWindows is Codex's optional Windows-specific command
	// override, propagated from the spec's `commandWindows` Meta key.
	CommandWindows string `json:"commandWindows,omitempty"`
	// AdditionalContextLimit caps how many tokens of this hook's output
	// reach the model, propagated from the spec's `additionalContextLimit`
	// Meta key. See learn.chatgpt.com/docs/hooks.
	AdditionalContextLimit *int `json:"additionalContextLimit,omitempty"`
}

// buildHooksJSON returns the rendered document or nil when no hooks
// produce output. Walks the bundle, normalizes commands via
// RewriteHookPath, then collapses (event, command) duplicates by
// unioning their matcher segments.
func buildHooksJSON(hooks []spec.Entry) *hooksDoc {
	type key struct{ event, command string }
	type accum struct {
		matchers               map[string]bool
		matcherOrder           []string
		timeout                int
		statusMessage          string
		commandWindows         string
		additionalContextLimit *int
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
		commandWindows, _ := h.Meta["commandWindows"].(string)
		additionalContextLimit := hookIntMetaPtr(h.Meta, "additionalContextLimit")
		for _, raw := range hookCommands(h.Meta["command"]) {
			cmd := emit.RewriteHookPath(raw, target)
			k := key{event: event, command: cmd}
			a, ok := byKey[k]
			if !ok {
				a = &accum{matchers: map[string]bool{}}
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
		g.Hooks = append(g.Hooks, hookCommandEntry{
			Type:                   "command",
			Command:                k.command,
			Timeout:                a.timeout,
			StatusMessage:          a.statusMessage,
			CommandWindows:         a.commandWindows,
			AdditionalContextLimit: a.additionalContextLimit,
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
