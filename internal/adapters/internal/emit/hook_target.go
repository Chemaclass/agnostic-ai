package emit

import (
	"encoding/json"
	"fmt"
	"strings"
)

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

// SetHookTargetEnv adds the target to doc's `env` object, or with want
// false drops the value sync added, and the object once it is empty.
// A value set by hand stays, and so does the order of the other keys.
func SetHookTargetEnv(doc *OrderedJSON, target string, want bool) error {
	env := NewOrderedJSON()
	if raw, ok := doc.Get("env"); ok {
		if err := json.Unmarshal(raw, env); err != nil {
			return fmt.Errorf("parse env: %w", err)
		}
	}
	raw, ok := env.Get(HookTargetEnv)
	var value string
	if ok {
		_ = json.Unmarshal(raw, &value)
	}
	switch {
	case want && !ok:
		if err := env.Set(HookTargetEnv, target); err != nil {
			return fmt.Errorf("marshal env: %w", err)
		}
	case !want && ok && value == target:
		env.Delete(HookTargetEnv)
	default:
		return nil
	}
	if env.Len() == 0 {
		doc.Delete("env")
		return nil
	}
	if err := doc.Set("env", env); err != nil {
		return fmt.Errorf("marshal env: %w", err)
	}
	return nil
}

// ShellQuote quotes s as one POSIX shell word.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ExecFormCommand folds an exec-form hook's args into a shell-form
// command for a target with no `args` field. Each argument is quoted so
// a POSIX shell passes it verbatim. Left bare, an interpreter such as
// `node` or `bash` would read the hook's JSON payload on stdin as its
// program.
func ExecFormCommand(command string, args []string) string {
	for _, arg := range args {
		command += " " + ShellQuote(arg)
	}
	return command
}
