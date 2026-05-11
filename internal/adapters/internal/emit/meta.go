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
	if len(meta) == 0 {
		return meta
	}
	out := make(map[string]any, len(meta))
	for k, v := range meta {
		if strings.HasPrefix(k, XPrefix) {
			continue
		}
		out[k] = v
	}
	if nested, ok := meta[XPrefix+target].(map[string]any); ok {
		for nk, nv := range nested {
			out[nk] = nv
		}
	}
	return out
}
