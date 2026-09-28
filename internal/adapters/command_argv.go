package adapters

import "github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"

// CommandArgv reads a spec command, one string or a list, into the argv a
// tool starts it with. See emit.CommandArgv.
func CommandArgv(v any) []string {
	return emit.CommandArgv(v)
}
