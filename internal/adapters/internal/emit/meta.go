package emit

import "strings"

// XPrefix is the namespace for target-specific frontmatter extensions.
// Keys named `x-<target>` carry fields that only the named adapter
// consumes. Other adapters strip them on emit so the open contract stays
// portable.
const XPrefix = "x-"

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
		if strings.HasPrefix(k, XPrefix) {
			continue
		}
		if v, ok := meta[k]; ok {
			appendKV(k, v)
		}
	}
	for k, v := range meta {
		if walked[k] || strings.HasPrefix(k, XPrefix) || seen[k] {
			continue
		}
		appendKV(k, v)
	}
	if nested, ok := meta[XPrefix+target].(map[string]any); ok {
		for nk, nv := range nested {
			appendKV(nk, nv)
		}
	}
	return out, outKeys
}
