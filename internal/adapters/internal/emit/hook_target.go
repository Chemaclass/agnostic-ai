package emit

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
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

// cursorCopyGuard ends a command Cursor runs from another tool's hooks
// file, so the hook runs once there, as Cursor's own copy. Cursor's
// sessionStart hook sets AGNOSTIC_AI_TARGET=cursor for later hooks; any
// other value, or none, runs the command.
const cursorCopyGuard = `[ "$` + HookTargetEnv + `" = cursor ] && exit 0; `

// GuardCursorCopy prefixes command with the Cursor copy guard.
func GuardCursorCopy(command string) string {
	return cursorCopyGuard + command
}

// StripCursorGuard undoes GuardCursorCopy, so an import reads back the
// command the spec declared.
func StripCursorGuard(command string) string {
	stripped, _ := strings.CutPrefix(command, cursorCopyGuard)
	return stripped
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
// a POSIX shell passes it verbatim, and so is the command when it holds
// anything but plain word characters, such as a path with a space.
// Left bare, an interpreter such as `node` or `bash` would read the
// hook's JSON payload on stdin as its program. With no args the command
// is shell form and stays as written.
func ExecFormCommand(command string, args []string) string {
	if len(args) == 0 {
		return command
	}
	if strings.IndexFunc(command, func(r rune) bool { return !isShellWordRune(r) }) >= 0 {
		command = ShellQuote(command)
	}
	for _, arg := range args {
		command += " " + ShellQuote(arg)
	}
	return command
}

// TargetHooks returns hooks with the keys that decide when and what runs
// (targetHookKeys) read after each spec's `x-<target>` override, as the
// spec format documents for every key. Emitters read them from the top
// level, so a hook fires and runs as its override says on every target.
// Other keys under `x-<target>` stay for the adapter that reads them.
func TargetHooks(target string, hooks []spec.Entry) []spec.Entry {
	if len(hooks) == 0 {
		return hooks
	}
	out := make([]spec.Entry, len(hooks))
	for i, h := range hooks {
		out[i] = TargetHook(target, h)
	}
	return out
}

// TargetHook is TargetHooks for one hook.
func TargetHook(target string, h spec.Entry) spec.Entry {
	override, _ := h.Meta[XPrefix+target].(map[string]any)
	if !slices.ContainsFunc(targetHookKeys, func(key string) bool { _, ok := override[key]; return ok }) {
		return h
	}
	resolved := ResolveMeta(h.Meta, target)
	meta := maps.Clone(h.Meta)
	for _, key := range targetHookKeys {
		value, ok := resolved[key]
		if !ok {
			delete(meta, key)
			continue
		}
		meta[key] = value
		if h.MetaKeys != nil && !slices.Contains(h.MetaKeys, key) {
			h.MetaKeys = append(slices.Clone(h.MetaKeys), key)
		}
	}
	h.Meta = meta
	return h
}

var targetHookKeys = []string{"event", "matcher", "command", "args"}

// HookArgs returns a hook spec's exec-form args on target, after its
// `x-<target>` override, the same view RewriteHookPath reads.
func HookArgs(target string, meta map[string]any) []string {
	return StringSlice(ResolveMeta(meta, target)["args"])
}

// ShellHookCommand is one entry of a hook spec's command as a target
// with no `args` field writes it: the path rewritten for target, then
// the exec-form args folded in. Use it only where the target hands the
// command to a shell.
func ShellHookCommand(command, target string, meta map[string]any) string {
	return ExecFormCommand(RewriteHookPath(command, target, meta), HookArgs(target, meta))
}

// PowerShellReadsFold reports whether PowerShell runs ExecFormCommand's
// result with the same words as a POSIX shell. It does not when the
// command is quoted, which PowerShell reads as a string, not a program,
// or when an arg is empty or holds a quote: PowerShell doubles an
// apostrophe to escape it, and Windows PowerShell drops an empty arg and
// mangles a double quote.
func PowerShellReadsFold(command string, args []string) bool {
	if len(args) == 0 {
		return true
	}
	if strings.IndexFunc(command, func(r rune) bool { return !isShellWordRune(r) }) >= 0 {
		return false
	}
	for _, arg := range args {
		if arg == "" || strings.ContainsAny(arg, `'"`) {
			return false
		}
	}
	return true
}

func isShellWordRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_./:@%+=,-", r)
}
