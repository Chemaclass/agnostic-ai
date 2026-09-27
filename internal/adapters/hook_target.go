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
