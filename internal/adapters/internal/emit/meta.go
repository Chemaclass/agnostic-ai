package emit

import "strings"

// XPrefix is the namespace for target-specific frontmatter extensions.
// Keys named `x-<target>` carry fields that only the named adapter
// consumes. Other adapters strip them on emit so the open contract stays
// portable.
const XPrefix = "x-"

// ResolveMeta returns a copy of meta with target-specific keys flattened
// for the named target and `x-*` keys for other targets dropped.
//
// Example: meta = {name: r1, x-claude: {allowed-tools: [Read]}, x-cursor: {globs: "src/**"}}
//   ResolveMeta(meta, "claude") -> {name: r1, allowed-tools: [Read]}
//   ResolveMeta(meta, "cursor") -> {name: r1, globs: "src/**"}
//   ResolveMeta(meta, "gemini") -> {name: r1}
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
