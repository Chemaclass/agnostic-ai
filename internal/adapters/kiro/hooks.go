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
type hooksFile struct {
	Version int         `json:"version"`
	Hooks   []hookEntry `json:"hooks"`
}

// hookAction is always the `{"type": "command", "command": ...}` shape:
// Kiro also documents a `{"type": "agent", "prompt": ...}` action that
// invokes an agent instead of a shell command, but agnostic-ai's hook
// spec has no generic prompt field, so this adapter never emits it.
type hookAction struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

type hookEntry struct {
	Name    string     `json:"name"`
	Trigger string     `json:"trigger"`
	Matcher string     `json:"matcher,omitempty"`
	Action  hookAction `json:"action"`
	Timeout int        `json:"timeout,omitempty"`
	// Enabled is a pointer so the common case (enabled) omits the key
	// entirely; only disabled: true on the spec sets it to false,
	// mirroring the disabled/enabled convention this adapter already
	// uses for MCP entries.
	Enabled *bool `json:"enabled,omitempty"`
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
		raw, err := emit.MarshalJSONIndent(hooksFile{Version: 1, Hooks: entries})
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

// buildHookEntries renders one hookEntry per command on h. Returns nil
// when h has no event or no usable command, so the caller skips writing
// a file for it entirely.
func buildHookEntries(h spec.Entry) []hookEntry {
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
	var disabledPtr *bool
	if disabled, _ := h.Meta["disabled"].(bool); disabled {
		f := false
		disabledPtr = &f
	}

	entries := make([]hookEntry, 0, len(cmds))
	for i, cmd := range cmds {
		name := h.Name
		if i > 0 {
			name = fmt.Sprintf("%s-%d", h.Name, i+1)
		}
		entries = append(entries, hookEntry{
			Name:    name,
			Trigger: trigger,
			Matcher: matcher,
			Action:  hookAction{Type: "command", Command: emit.RewriteHookPath(cmd, target)},
			Timeout: timeout,
			Enabled: disabledPtr,
		})
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
