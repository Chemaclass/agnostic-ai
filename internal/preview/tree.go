package preview

import (
	"maps"
	"slices"
	"strconv"
)

// RedactTree returns tree, a decoded JSON or TOML value, with each part
// that holds a credential replaced by Redacted, and the path to each such
// part in key order, as table keys and list indexes. key reports a table
// key that holds one, and its value is still walked; several hidden keys
// of one table are numbered so they stay apart. value reports any other
// part, given its path, with a TOML array of tables as a list, and a part
// it reports is not walked.
func RedactTree(tree any, key func(string) bool, value func(path []any, v any) bool) (any, [][]any) {
	var hidden [][]any
	var walk func(path []any, v any) any
	walk = func(path []any, v any) any {
		if rows, ok := v.([]map[string]any); ok {
			list := make([]any, len(rows))
			for i, row := range rows {
				list[i] = row
			}
			v = list
		}
		if value(path, v) {
			hidden = append(hidden, path)
			return Redacted
		}
		switch v := v.(type) {
		case map[string]any:
			keys := slices.Sorted(maps.Keys(v))
			var named []string
			for _, name := range keys {
				if key != nil && key(name) {
					named = append(named, name)
				}
			}
			out := make(map[string]any, len(v))
			for _, name := range keys {
				at := append(slices.Clip(path), name)
				shown := name
				if i := slices.Index(named, name); i >= 0 {
					hidden = append(hidden, at)
					shown = Redacted
					if len(named) > 1 {
						shown += "#" + strconv.Itoa(i+1)
					}
				}
				out[shown] = walk(at, v[name])
			}
			return out
		case []any:
			out := make([]any, len(v))
			for i, item := range v {
				out[i] = walk(append(slices.Clip(path), i), item)
			}
			return out
		}
		return v
	}
	return walk(nil, tree), hidden
}
