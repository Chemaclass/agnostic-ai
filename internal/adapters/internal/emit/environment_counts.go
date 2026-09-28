package emit

import "github.com/chemaclass/agnostic-ai/internal/spec"

// EnvironmentsWithSetup counts the environment specs that set a worktree
// setup command, `setup` or `setup-windows`.
func EnvironmentsWithSetup(envs []spec.Entry) int {
	n := 0
	for _, e := range envs {
		if e.Meta["setup"] != nil || e.Meta["setup-windows"] != nil {
			n++
		}
	}
	return n
}
