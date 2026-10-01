package spec

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// NonJSONValue returns the dotted path of the first value in meta that
// JSON cannot hold, such as a YAML .nan or .inf, and the value itself.
// Keys are walked in sorted order, so the path is stable.
func NonJSONValue(meta map[string]any) (string, any, bool) {
	return nonJSONValue("", meta)
}

func nonJSONValue(prefix string, value any) (string, any, bool) {
	switch v := value.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return prefix, v, true
		}
	case float32:
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return prefix, v, true
		}
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			if p, bad, ok := nonJSONValue(path, v[key]); ok {
				return p, bad, true
			}
		}
	case map[any]any:
		named := make(map[string]any, len(v))
		for key, item := range v {
			named[fmt.Sprint(key)] = item
		}
		return nonJSONValue(prefix, named)
	case []any:
		for i, item := range v {
			if p, bad, ok := nonJSONValue(prefix+"["+strconv.Itoa(i)+"]", item); ok {
				return p, bad, true
			}
		}
	}
	return "", nil, false
}

// CheckMCPJSONValues rejects an MCP spec holding a value JSON cannot
// hold, as target reads it. Most targets write MCP servers as JSON, and
// a value that does not encode would leave the server out or keep a
// stale copy. Another target's `x-<target>` block is skipped, since it
// never reaches this one.
func CheckMCPJSONValues(mcps []Entry, target string) error {
	for _, e := range mcps {
		if field, value, ok := NonJSONValue(metaFor(e.Meta, target)); ok {
			return fmt.Errorf("%s: MCP server %q: %s is %v, which JSON cannot hold", e.Path, e.Name, field, value)
		}
	}
	return nil
}

// metaFor drops every x-<target> block but target's own.
func metaFor(meta map[string]any, target string) map[string]any {
	out := make(map[string]any, len(meta))
	for key, value := range meta {
		if strings.HasPrefix(key, "x-") && key != "x-"+target {
			continue
		}
		out[key] = value
	}
	return out
}
