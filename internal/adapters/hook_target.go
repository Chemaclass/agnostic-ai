package adapters

import "github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"

// HookTargetEnv names the variable a synced hook reads to learn which
// target ran it.
const HookTargetEnv = emit.HookTargetEnv

// ExportHookTarget prefixes a shell-form command with the target variable.
func ExportHookTarget(command, target string) string {
	return emit.ExportHookTarget(command, target)
}

// StripHookTargetExport undoes ExportHookTarget.
func StripHookTargetExport(command, target string) string {
	return emit.StripHookTargetExport(command, target)
}

// WithHookTarget returns env with the target variable added, unless env
// already sets it.
func WithHookTarget[V any](env map[string]V, target V) map[string]V {
	return emit.WithHookTarget(env, target)
}

// WithoutHookTarget drops the target variable sync added.
func WithoutHookTarget[V any](env map[string]V, target V) map[string]V {
	return emit.WithoutHookTarget(env, target)
}

// SetHookTargetEnv adds the target to doc's `env`, or with want false
// drops the value sync added, keeping the other keys in order.
func SetHookTargetEnv(doc *OrderedJSON, target string, want bool) error {
	return emit.SetHookTargetEnv(doc, target, want)
}

// ExecFormCommand folds an exec-form hook's args into a quoted
// shell-form command.
func ExecFormCommand(command string, args []string) string {
	return emit.ExecFormCommand(command, args)
}

// ShellQuote quotes s as one POSIX shell word.
func ShellQuote(s string) string { return emit.ShellQuote(s) }

// UndoDotSlashHookScript drops the ./ sync adds to a synced hook script
// path, keeping any other ./ the user wrote.
func UndoDotSlashHookScript(command, target string) string {
	return emit.UndoDotSlashHookScript(command, target)
}
