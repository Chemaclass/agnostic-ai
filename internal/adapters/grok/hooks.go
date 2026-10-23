package grok

import (
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func HookEvents() []string {
	return []string{"SessionStart", "SessionEnd", "UserPromptSubmit", "PreToolUse", "PostToolUse", "PostToolUseFailure", "PermissionDenied", "Stop", "StopFailure", "StopCancelled", "Notification", "SubagentStart", "SubagentStop", "PreCompact", "PostCompact"}
}
func HookFile(e spec.Entry, scriptDirs ...string) ([]byte, error) {
	scriptDir := emit.HookScriptsDir(target)
	if len(scriptDirs) > 0 {
		scriptDir = scriptDirs[0]
	}
	if err := spec.ValidateName(spec.KindHook, e.Name); err != nil {
		return nil, fmt.Errorf("grok hook %s: %w", e.Name, err)
	}
	meta := emit.ResolveMeta(e.Meta, target)
	if native, ok := meta["hooks"].(map[string]any); ok {
		encoded, err := json.Marshal(native)
		if err != nil {
			return nil, fmt.Errorf("grok hook %s: %w", e.Name, err)
		}
		var cloned map[string]any
		if err := json.Unmarshal(encoded, &cloned); err != nil {
			return nil, fmt.Errorf("grok hook %s: %w", e.Name, err)
		}
		native = cloned
		for event, raw := range native {
			if !slices.Contains(HookEvents(), event) {
				return nil, fmt.Errorf("grok hook %s: unsupported event %q", e.Name, event)
			}
			groups, ok := raw.([]any)
			if !ok {
				return nil, fmt.Errorf("grok hook %s: invalid groups for %s", e.Name, event)
			}
			for _, rawGroup := range groups {
				group, ok := rawGroup.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("grok hook %s: invalid group", e.Name)
				}
				handlers, ok := group["hooks"].([]any)
				if !ok {
					return nil, fmt.Errorf("grok hook %s: invalid handlers", e.Name)
				}
				for _, rawHandler := range handlers {
					handler, ok := rawHandler.(map[string]any)
					if !ok || handler["type"] != "command" {
						return nil, fmt.Errorf("grok hook %s: only command handlers can be imported", e.Name)
					}
					if command, ok := handler["command"].(string); ok {
						if filepath.IsAbs(scriptDir) {
							handler["command"] = emit.RewriteGlobalHookPath(command, target, scriptDir)
						} else {
							handler["command"] = emit.RewriteNeutralHookPath(command, scriptDir)
						}
					}
				}
			}
		}
		data, err := json.MarshalIndent(map[string]any{"hooks": native}, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("grok hook %s: %w", e.Name, err)
		}
		return append(data, '\n'), nil
	}

	event, _ := meta["event"].(string)
	if event == "" {
		return nil, nil
	}
	if !slices.Contains(HookEvents(), event) {
		return nil, fmt.Errorf("grok hook %s: unsupported event %q", e.Name, event)
	}
	if kind, _ := meta["type"].(string); kind != "" && kind != "command" {
		return nil, fmt.Errorf("grok hook %s: only native command hooks are supported", e.Name)
	}
	for _, key := range []string{"commandWindows", "shell"} {
		if meta[key] != nil {
			return nil, fmt.Errorf("grok hook %s: %s has no verified native field; use a native platform script", e.Name, key)
		}
	}
	var handlers []any
	for _, command := range emit.HookCommands(meta["command"]) {
		args := emit.HookArgs(target, meta)
		if runtime.GOOS == "windows" && !emit.PowerShellReadsFold(command, args) {
			return nil, fmt.Errorf("grok hook %s: args cannot be preserved in Windows PowerShell; use a native wrapper script", e.Name)
		}
		handler := map[string]any{"type": "command", "command": emit.ExecFormCommand(strings.ReplaceAll(emit.RewriteHookPath(command, target, meta), emit.HookScriptsDir(target)+"/", scriptDir+"/"), args), "env": emit.WithHookTarget(emit.StringMap(meta["env"]), target)}
		if timeout, ok := meta["timeout"]; ok {
			handler["timeout"] = timeout
		}
		handlers = append(handlers, handler)
	}
	if len(handlers) == 0 {
		return nil, nil
	}
	group := map[string]any{"hooks": handlers}
	if matcher, ok := meta["matcher"]; ok {
		if event == "Stop" || event == "UserPromptSubmit" {
			emit.NoteFieldNoOp(target, spec.KindHook, "matcher", 1, "Grok ignores matchers for "+event)
		} else {
			group["matcher"] = matcher
		}
	}
	data, err := json.MarshalIndent(map[string]any{"hooks": map[string]any{event: []any{group}}}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("grok hook %s: %w", e.Name, err)
	}
	return append(data, '\n'), nil
}
func emitHooks(sess *emit.Session, hooks []spec.Entry, cfg *config.Config, dryRun bool) error {
	dir := emit.OutputHooksDir(cfg, target, emit.OutputSubDir(cfg, target, "hooks", ".grok/hooks"))
	if err := sess.MaterializeNeutralHookScripts(scriptHooks(hooks), target, emit.OutputSubDir(cfg, target, "hook-scripts", ".grok/hook-scripts"), dryRun); err != nil {
		return err
	}
	for _, e := range hooks {
		data, err := HookFile(e, emit.OutputSubDir(cfg, target, "hook-scripts", ".grok/hook-scripts"))
		if err != nil {
			return err
		}
		if len(data) == 0 {
			continue
		}
		if err := sess.WriteFile(filepath.Join(dir, e.Name+".json"), string(data), dryRun); err != nil {
			return err
		}
	}
	return nil
}

func NativeHookCommands(e spec.Entry) []string {
	meta := emit.ResolveMeta(e.Meta, target)
	commands := emit.HookCommands(meta["command"])
	if native, ok := meta["hooks"].(map[string]any); ok {
		for _, raw := range native {
			groups, _ := raw.([]any)
			for _, rawGroup := range groups {
				group, _ := rawGroup.(map[string]any)
				handlers, _ := group["hooks"].([]any)
				for _, rawHandler := range handlers {
					handler, _ := rawHandler.(map[string]any)
					if command, ok := handler["command"].(string); ok {
						commands = append(commands, command)
					}
				}
			}
		}
	}
	return commands
}
func scriptHooks(hooks []spec.Entry) []spec.Entry {
	var out []spec.Entry
	for _, hook := range hooks {
		copy := hook
		copy.Meta = maps.Clone(hook.Meta)
		copy.Meta["command"] = NativeHookCommands(hook)
		out = append(out, copy)
	}
	return out
}
