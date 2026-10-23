package kimi

import (
	"fmt"
	"maps"
	"runtime"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func HookEvents() []string {
	return []string{"PreToolUse", "PostToolUse", "PostToolUseFailure", "PermissionRequest", "PermissionResult", "UserPromptSubmit", "UserPromptQueued", "TurnStarted", "Stop", "StopFailure", "Interrupt", "SessionStart", "SessionEnd", "SessionHeartbeat", "SubagentStart", "SubagentStop", "TaskStarted", "PreCompact", "PostCompact", "Notification"}
}
func (Adapter) UserHookRecords(hooks []spec.Entry, scriptsDir string) ([]map[string]any, error) {
	return (Adapter{}).UserHookRecordsForPlatform(hooks, scriptsDir, runtime.GOOS)
}
func (Adapter) UserHookRecordsForPlatform(hooks []spec.Entry, scriptsDir, platform string) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(hooks))
	for _, h := range hooks {
		m := emit.ResolveMeta(h.Meta, target)
		if m["env"] != nil || m["cwd"] != nil {
			return nil, fmt.Errorf("kimi: hook %s has no native env or cwd config field", h.Name)
		}
		for key := range native(h.Meta) {
			if !slices.Contains([]string{"event", "matcher", "command", "timeout", "commandWindows", "args"}, key) {
				return nil, fmt.Errorf("kimi: hook %s unsupported native key %s", h.Name, key)
			}
		}
		event, _ := m["event"].(string)
		if !slices.Contains(HookEvents(), event) {
			emit.NoteFieldNoOp(target, spec.KindHook, "event", 1, "unsupported Kimi Code event "+event)
			continue
		}
		command, _ := m["command"].(string)
		if platform == "windows" {
			if win, ok := m["commandWindows"].(string); ok {
				command = win
			}
		}
		if strings.TrimSpace(command) == "" {
			return nil, fmt.Errorf("kimi: hook %s requires command", h.Name)
		}
		command = emit.RewriteGlobalHookPath(command, target, scriptsDir, m)
		args := list(m["args"])
		if platform == "windows" && !emit.PowerShellReadsFold(command, args) {
			return nil, fmt.Errorf("kimi: hook %s arguments cannot be folded safely on Windows", h.Name)
		}
		command = emit.NativeHookCommand(emit.ExecFormCommand(command, args), target, platform)
		d := map[string]any{"event": event, "command": command}
		if matcher, ok := m["matcher"]; ok {
			d["matcher"] = matcher
		}
		if v, ok := m["timeout"]; ok {
			n, valid := nativeInteger(v)
			if !valid || n < 1 || n > 600 {
				return nil, fmt.Errorf("kimi: hook %s timeout must be an integer from 1 to 600 seconds", h.Name)
			}
			d["timeout"] = n
		}
		if err := ValidateHookRecord(d); err != nil {
			return nil, fmt.Errorf("kimi: hook %s: %w", h.Name, err)
		}
		out = append(out, d)
	}
	return out, nil
}
func (Adapter) CanonicalUserArrayRecord(parent string, record map[string]any) any {
	out := maps.Clone(record)
	if parent == "hooks" {
		if command, ok := out["command"].(string); ok {
			out["command"] = DeclaredHookCommand(command)
		}
	}
	return out
}

func DeclaredHookCommand(command string) string {
	return emit.DeclaredNativeHookCommand(command, target)
}
