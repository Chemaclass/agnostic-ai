package kiro

import (
	"fmt"
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// hooksFile is the `.kiro/hooks/<name>.json` shape
// (kiro.dev/docs/hooks/): a `version` plus a `hooks` array. Kiro's own
// examples wrap a single hook in the array; the array shape is what the
// vendor schema documents regardless of how many entries one file
// carries, so a spec with several `command:` entries shares one file
// (see buildHookEntries) instead of spawning one file per command.
//
// Hooks marshal as `map[string]any` rather than a fixed struct: a Go
// struct has no route for a key it does not declare, at any layer,
// including x-kiro, which is how `description` and `confirm` stayed
// unreachable across three audit passes (#642). `map[string]any` keys
// alpha-sort under `encoding/json`, so key order below is illustrative,
// not the emitted order.
type hooksFile struct {
	// Version is a string, not a number: the vendor field reference
	// documents `version` as `Schema version - currently "v1"`.
	Version string           `json:"version"`
	Hooks   []map[string]any `json:"hooks"`
}

// hookAction is always the `{"type": "command", "command": ...}` shape:
// Kiro also documents a `{"type": "agent", "prompt": ...}` action that
// invokes an agent instead of a shell command, but agnostic-ai's hook
// spec has no generic prompt field, so this adapter never emits it by
// hand. `x-kiro.action` is not excluded from the passthrough merge
// below, so an author who wants that shape can still set it directly.
type hookAction struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// emitHooks writes one `<dir>/<name>.json` per hook spec. A spec's
// `command:` field (string or list) becomes one `hooks[]` entry per
// command in that same file, `name` suffixed `-2`, `-3`, ... past the
// first so entries sharing a file stay unique. Hooks without an `event`
// or a `command` produce no output, the same skip-silently rule every
// other adapter's hook builder uses. No file materializes for a hook
// scoped away from kiro by `target:` / `targets:` (b.HooksFor already
// filters those out before this function sees them).
func emitHooks(sess *emit.Session, hooks []spec.Entry, dir string, dryRun bool) error {
	for _, h := range hooks {
		entries := buildHookEntries(h)
		if len(entries) == 0 {
			continue
		}
		raw, err := emit.MarshalJSONIndent(hooksFile{Version: "v1", Hooks: entries})
		if err != nil {
			return fmt.Errorf("kiro hook %s: %w", h.Name, err)
		}
		path := filepath.Join(dir, h.Name+".json")
		if err := sess.WriteFile(path, string(raw)+"\n", dryRun); err != nil {
			return err
		}
	}
	return nil
}

// buildHookEntries renders one hooks[] entry per command on h as a
// map[string]any: `name`, `trigger`, `action`, and the optional
// `matcher`/`timeout`/`enabled`/`description` fields agnostic-ai's spec
// already carries, plus every key under `x-kiro` (e.g. `confirm`, the
// vendor's Stop-hook confirmation block, which has no agnostic-ai spec
// equivalent and so is only reachable this way). Every entry sharing
// this hook spec's command list also shares its `description` and
// `x-kiro` passthrough, since both live on the spec, not per-command.
// Returns nil when h has no event or no usable command, so the caller
// skips writing a file for it entirely.
func buildHookEntries(h spec.Entry) []map[string]any {
	trigger, _ := h.Meta["event"].(string)
	if trigger == "" {
		return nil
	}
	cmds := hookCommands(h.Meta["command"])
	if len(cmds) == 0 {
		return nil
	}
	matcher, _ := h.Meta["matcher"].(string)
	timeout := hookIntMeta(h.Meta, "timeout")
	description, _ := h.Meta["description"].(string)
	disabled, _ := h.Meta["disabled"].(bool)

	entries := make([]map[string]any, 0, len(cmds))
	for i, cmd := range cmds {
		name := h.Name
		if i > 0 {
			name = fmt.Sprintf("%s-%d", h.Name, i+1)
		}
		entry := map[string]any{
			"name":    name,
			"trigger": trigger,
			"action":  hookAction{Type: "command", Command: emit.RewriteHookPath(cmd, target)},
		}
		var keys []string
		if matcher != "" {
			entry["matcher"] = matcher
		}
		if timeout != 0 {
			entry["timeout"] = timeout
		}
		if disabled {
			// The vendor default (enabled) needs no explicit key,
			// mirroring the disabled/enabled convention this adapter
			// already uses for MCP entries.
			entry["enabled"] = false
		}
		if description != "" {
			entry["description"] = description
		}
		emit.MergeCustomTargetMeta(entry, &keys, h.Meta, target,
			"name", "trigger", "matcher", "action", "timeout", "enabled", "description")
		entries = append(entries, entry)
	}
	return entries
}

// hookCommands normalizes a `command:` field that may be a string or a
// list of strings into a single []string. Empty strings drop out.
func hookCommands(raw any) []string {
	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		out := make([]string, 0, len(v))
		for _, s := range v {
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
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
