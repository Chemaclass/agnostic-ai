package emit

import "fmt"

// Hook spec `Meta` readers, shared by every adapter that renders hooks.
// A hook spec is loaded from YAML into `map[string]any`, so a field can
// arrive as more than one Go type depending on how it was authored:
// `timeout: 30` decodes as int from one loader and float64 from another,
// and a hand-edited file may quote it. These readers normalize that
// instead of each adapter type-asserting one shape and dropping the
// others.
//
// claude and codex still carry private copies predating this file. New
// hook emitters use these; consolidating those two is a separate change,
// since it touches their emit paths for no behavior difference.

// HookCommands reads a hook's `command` meta key, which is either one
// command string or a list of them. Empty strings are skipped, so a
// spec with a blank command produces no entry rather than an entry that
// runs nothing. Returns nil for a missing key or any other type.
func HookCommands(raw any) []string {
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

// HookIntMeta reads an int-typed meta key, accepting the int, int64,
// float64, and decimal-string forms a YAML loader or a hand-edited file
// may produce. Returns 0 when missing, unparseable, or another type, so
// callers with `omitempty` fields emit nothing rather than a zero.
func HookIntMeta(meta map[string]any, key string) int {
	switch v := meta[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
			return 0
		}
		return n
	}
	return 0
}

// HookBoolMeta reads a bool-typed meta key, accepting bool and the
// string form "true" a hand-edited YAML may carry. Returns false when
// missing or any other type.
func HookBoolMeta(meta map[string]any, key string) bool {
	switch v := meta[key].(type) {
	case bool:
		return v
	case string:
		return v == "true"
	}
	return false
}
