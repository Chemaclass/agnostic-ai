package emit

import "strings"

// XPrefix is the namespace for target-specific frontmatter extensions.
// Keys named `x-<target>` carry fields that only the named adapter
// consumes. Other adapters strip them on emit so the open contract stays
// portable.
const XPrefix = "x-"

// routingKeys are agnostic-ai-only frontmatter keys that steer which
// target receives a spec. They have no semantics inside the tool's
// consumer (Claude, Codex, etc.), so the adapter must strip them on
// emit; otherwise round-trip imports leak `target: claude` into hand-
// authored files that never declared it (#303).
var routingKeys = map[string]bool{
	"target":          true,
	"targets":         true,
	"target-exclude":  true,
	"targets-exclude": true,
}

// StringSlice coerces a `[]any` of strings (YAML's default unmarshalled
// shape for list-of-strings) into `[]string`. Non-string elements and
// non-slice inputs return nil. Adapters use this to read `args`,
// `nickname_candidates`, etc. from spec frontmatter.
func StringSlice(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, x := range raw {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// StringMap coerces a `map[string]any` of string values (YAML's default
// for a string-keyed string-valued map) into `map[string]string`.
// Non-string values are dropped; a non-map input returns nil. Adapters
// use this to read `env`, `headers`, etc. from spec frontmatter.
func StringMap(v any) map[string]string {
	raw, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, x := range raw {
		if s, ok := x.(string); ok {
			out[k] = s
		}
	}
	return out
}

// ResolveMeta returns a copy of meta with target-specific keys flattened
// for the named target and `x-*` keys for other targets dropped.
//
// Example: meta = {name: r1, x-claude: {allowed-tools: [Read]}, x-cursor: {globs: "src/**"}}
//
//	ResolveMeta(meta, "claude") -> {name: r1, allowed-tools: [Read]}
//	ResolveMeta(meta, "cursor") -> {name: r1, globs: "src/**"}
//	ResolveMeta(meta, "gemini") -> {name: r1}
//
// Adapter-owned keys overwrite top-level keys with the same name.
func ResolveMeta(meta map[string]any, target string) map[string]any {
	out, _ := ResolveMetaOrdered(meta, nil, target)
	return out
}

// ResolveMetaOrdered is ResolveMeta with a source key-order hint.
// Returns the resolved map and the ordered keys derived by walking
// `keys` first (skipping `x-*` entries that get flattened), then
// appending unhinted top-level keys in alphabetical order, then any
// keys nested under `x-<target>` that did not already appear.
//
// Callers that loaded the spec from disk should pass `entry.MetaKeys`
// so the emitted frontmatter preserves the author's intent. Callers
// that built `meta` programmatically pass nil and get alphabetical
// order (deterministic across runs).
func ResolveMetaOrdered(meta map[string]any, keys []string, target string) (map[string]any, []string) {
	if len(meta) == 0 {
		return meta, keys
	}
	out := make(map[string]any, len(meta))
	outKeys := make([]string, 0, len(meta))
	seen := make(map[string]bool, len(meta))
	appendKV := func(k string, v any) {
		if _, dup := out[k]; dup {
			out[k] = v
			return
		}
		out[k] = v
		outKeys = append(outKeys, k)
		seen[k] = true
	}
	walked := make(map[string]bool, len(meta))
	for _, k := range keys {
		walked[k] = true
		if strings.HasPrefix(k, XPrefix) || routingKeys[k] {
			continue
		}
		if v, ok := meta[k]; ok {
			appendKV(k, v)
		}
	}
	for k, v := range meta {
		if walked[k] || strings.HasPrefix(k, XPrefix) || routingKeys[k] || seen[k] {
			continue
		}
		appendKV(k, v)
	}
	if nested, ok := meta[XPrefix+target].(map[string]any); ok {
		for nk, nv := range nested {
			if nv == nil {
				// nil under x-<target> is a delete marker: drop the key
				// entirely so the per-target emit reproduces the source
				// of truth (e.g. a codex agent that never had `model:`
				// must not inherit claude's `model: haiku`). See #304.
				removeKey(out, &outKeys, nk)
				continue
			}
			appendKV(nk, nv)
		}
	}
	collapseModel(out, &outKeys, target)
	return out, outKeys
}

// collapseModel resolves a per-target `model` map down to the single
// string the named target should use. A bare string `model:` value is
// left untouched, so the simple case keeps working. A map form picks
// `model.<target>` first, then `model.default`; if neither matches the
// key is removed so the target inherits no model (mirrors the #304
// delete-marker behavior). Keys overwritten by `x-<target>.model` arrive
// here already as strings and pass through unchanged.
//
//	model: gpt-4o                          -> gpt-4o (every target)
//	model: {claude: opus, default: gpt-4o} -> opus for claude, gpt-4o for codex
//	model: {claude: opus}                  -> opus for claude, dropped for codex
func collapseModel(out map[string]any, keys *[]string, target string) {
	m, ok := out["model"].(map[string]any)
	if !ok {
		return
	}
	pick := func(k string) (string, bool) {
		v, ok := m[k]
		if !ok {
			return "", false
		}
		s, ok := v.(string)
		return s, ok
	}
	if s, ok := pick(target); ok {
		out["model"] = s
		return
	}
	if s, ok := pick("default"); ok {
		out["model"] = s
		return
	}
	removeKey(out, keys, "model")
}

// removeKey strips key from both the resolved map and the ordered keys
// slice so a downstream renderer emits neither a value nor a position
// for it. Used by ResolveMetaOrdered to honor nil delete markers under
// `x-<target>`.
func removeKey(out map[string]any, keys *[]string, key string) {
	delete(out, key)
	for i, k := range *keys {
		if k == key {
			*keys = append((*keys)[:i], (*keys)[i+1:]...)
			return
		}
	}
}
