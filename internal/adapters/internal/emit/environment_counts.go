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

// hasCommand reports whether v holds a command: a non-empty string or a
// list with one.
func hasCommand(v any) bool {
	if s, ok := v.(string); ok {
		return s != ""
	}
	return len(StringSlice(v)) > 0
}
