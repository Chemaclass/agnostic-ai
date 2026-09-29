package emit

import "github.com/chemaclass/agnostic-ai/internal/spec"

// EnvironmentsWithSetup counts the environment specs that set a worktree
// setup command, `setup` or `setup-windows`, for target.
func EnvironmentsWithSetup(target string, envs []spec.Entry) int {
	n := 0
	for _, e := range envs {
		m := ResolveMeta(e.Meta, target)
		if hasCommand(m["setup"]) || hasCommand(m["setup-windows"]) {
			n++
		}
	}
	return n
}

// EnvironmentsWithField counts the environment specs that set field to a
// non-empty value for target.
func EnvironmentsWithField(target string, envs []spec.Entry, field string) int {
	n := 0
	for _, e := range envs {
		if hasValue(ResolveMeta(e.Meta, target)[field]) {
			n++
		}
	}
	return n
}

// hasValue reports whether a spec field holds something: a non-empty
// string, list, or map, true, or any other set value.
func hasValue(v any) bool {
	switch v := v.(type) {
	case nil:
		return false
	case bool:
		return v
	case string:
		return v != ""
	case []any:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	default:
		return true
	}
}

// hasCommand reports whether v holds a command: a non-empty string or a
// list with one.
func hasCommand(v any) bool {
	if s, ok := v.(string); ok {
		return s != ""
	}
	return len(StringSlice(v)) > 0
}
