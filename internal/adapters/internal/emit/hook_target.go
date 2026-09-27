package emit

import "strings"

// HookTargetEnv names the variable a synced hook reads to learn which
// target ran it, so one shared script can pick that tool's reply
// protocol instead of guessing from the payload (#1219).
const HookTargetEnv = "AGNOSTIC_AI_TARGET"

// ExportHookTarget prefixes a shell-form command so every command in it,
// not only the first of an `a && b` list, sees the target. Use it only
// where the target's runner is a POSIX shell on every platform: cmd.exe
// and PowerShell have no `export`, and the hook would fail there.
func ExportHookTarget(command, target string) string {
	return hookTargetExport(target) + command
}

// StripHookTargetExport undoes ExportHookTarget, so an import reads back
// the command the spec declared.
func StripHookTargetExport(command, target string) string {
	stripped, _ := strings.CutPrefix(command, hookTargetExport(target))
	return stripped
}

func hookTargetExport(target string) string {
	return "export " + HookTargetEnv + "=" + target + "; "
}

// WithHookTarget returns env with the target variable added. A value the
// spec set already wins, so an author can pin or override it.
func WithHookTarget[V any](env map[string]V, target V) map[string]V {
	out := make(map[string]V, len(env)+1)
	for k, v := range env {
		out[k] = v
	}
	if _, ok := out[HookTargetEnv]; !ok {
		out[HookTargetEnv] = target
	}
	return out
}

// WithoutHookTarget drops the target variable sync added, keeping any
// other value, so an import does not copy it into a spec. It returns nil
// when nothing else is left.
func WithoutHookTarget[V any](env map[string]V, target V) map[string]V {
	if value, ok := env[HookTargetEnv]; !ok || any(value) != any(target) {
		return env
	}
	out := make(map[string]V, len(env))
	for k, v := range env {
		if k != HookTargetEnv {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
