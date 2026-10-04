package adapters

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

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

// TargetHooks returns hooks with `command` and `args` read after each
// spec's `x-<target>` override.
func TargetHooks(target string, hooks []spec.Entry) []spec.Entry {
	return emit.TargetHooks(target, hooks)
}

// ExecFormCommand folds an exec-form hook's args into a quoted
// shell-form command.
func ExecFormCommand(command string, args []string) string {
	return emit.ExecFormCommand(command, args)
}

// ShellQuote quotes s as one POSIX shell word.
func ShellQuote(s string) string { return emit.ShellQuote(s) }
