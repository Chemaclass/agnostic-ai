package cli

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/cline"
	"github.com/chemaclass/agnostic-ai/internal/adapters/goose"
	"github.com/chemaclass/agnostic-ai/internal/adapters/grok"
	"github.com/chemaclass/agnostic-ai/internal/adapters/kiro"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func addUserHookFiles(home, source, target string, g userTarget, hooks []spec.Entry, onUnsupported string, add func(string, []byte, fs.FileMode) error) error {
	if target == "grok" {
		return addUserGrokHookFiles(home, source, g, hooks, add)
	}
	if target == "cline" {
		return addUserClineHookFiles(home, source, g, hooks, onUnsupported, add)
	}
	if target == "goose" {
		return addUserGooseHookFiles(home, source, g, hooks, onUnsupported, add)
	}
	if target != "kiro" {
		return fmt.Errorf("%s: native global hook directory rendering is unsupported", target)
	}
	dir := g.path(home, g.hooksDir)
	scriptsDir := g.path(home, g.hookScriptsDir)
	if err := adapters.ReportHookProjectRoot(target, hooks, onUnsupported, true); err != nil {
		return err
	}
	if wrapper, ok := adapters.PortableHookWrapperScript(hooks, target, scriptsDir); ok {
		if err := add(wrapper.Path, wrapper.Body, wrapper.Mode); err != nil {
			return err
		}
	}
	for _, hook := range hooks {
		if event, _ := hook.Meta["event"].(string); event == "" {
			continue
		}
		prepared := hook
		prepared.Meta = maps.Clone(hook.Meta)
		rewrite := func(command string, literal bool) (string, error) {
			scripts, err := adapters.NeutralHookScripts(command, target, filepath.Join(source, "scripts"), scriptsDir, literal)
			if err != nil {
				return "", err
			}
			for _, script := range scripts {
				if err := add(script.Path, script.Body, script.Mode); err != nil {
					return "", err
				}
			}
			return adapters.RewriteUserHookPath(command, target, filepath.ToSlash(scriptsDir), hook.Meta), nil
		}
		if native, ok := hook.Meta["x-kiro"].(map[string]any); ok {
			if action, ok := native["action"].(map[string]any); ok && action["type"] == "command" {
				command, _ := action["command"].(string)
				rewritten, err := rewrite(command, false)
				if err != nil {
					return err
				}
				native = maps.Clone(native)
				action = maps.Clone(action)
				action["command"] = rewritten
				native["action"] = action
				prepared.Meta["x-kiro"] = native
			}
		}
		if native, ok := hook.Meta["x-kiro"].(map[string]any); !ok || native["action"] == nil {
			var commands []string
			for _, command := range userHookCommands(hook.Meta["command"]) {
				rewritten, err := rewrite(command, len(stringSliceFromAny(hook.Meta["args"])) > 0)
				if err != nil {
					return err
				}
				if hook.WrapsCommand(target) {
					rewritten = adapters.PortableHookCommand(hook, target, adapters.ExecFormCommand(rewritten, stringSliceFromAny(hook.Meta["args"])), func(command string) string {
						return adapters.RewriteUserHookPath(command, target, filepath.ToSlash(scriptsDir), hook.Meta)
					})
					delete(prepared.Meta, "args")
				}
				commands = append(commands, rewritten)
			}
			prepared.Meta["command"] = commands
		}
		raw, err := kiro.HookFile(prepared)
		if err != nil {
			return err
		}
		if len(raw) == 0 {
			continue
		}
		path := filepath.Join(dir, hook.Name+".json")
		if existing, err := os.ReadFile(path); err == nil {
			var want, have any
			if json.Unmarshal(raw, &want) == nil && json.Unmarshal(existing, &have) == nil && sameSetting(want, have) {
				raw = existing
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("read %s: %w", path, err)
		}
		if err := add(path, raw, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func addUserGooseHookFiles(home, source string, g userTarget, hooks []spec.Entry, onUnsupported string, add func(string, []byte, fs.FileMode) error) error {
	dir, scriptsDir := g.path(home, g.hooksDir), g.path(home, g.hookScriptsDir)
	if err := adapters.ReportHookProjectRoot("goose", hooks, onUnsupported, true); err != nil {
		return err
	}
	body, err := goose.UserHooksFile(hooks, filepath.ToSlash(scriptsDir))
	if err != nil || body == nil {
		return err
	}
	if wrapper, ok := adapters.PortableHookWrapperScript(hooks, "goose", scriptsDir); ok {
		if err := add(wrapper.Path, wrapper.Body, wrapper.Mode); err != nil {
			return err
		}
	}
	for _, hook := range hooks {
		for _, command := range userHookCommands(hook.Meta["command"]) {
			scripts, err := adapters.NeutralHookScriptsFor(hook, command, "goose", filepath.Join(source, "scripts"), scriptsDir, len(stringSliceFromAny(hook.Meta["args"])) > 0)
			if err != nil {
				return err
			}
			for _, script := range scripts {
				if err := add(script.Path, script.Body, script.Mode); err != nil {
					return err
				}
			}
		}
	}
	manifest, err := goose.UserPluginManifest(filepath.Dir(dir))
	if err != nil {
		return err
	}
	if err := add(filepath.Join(filepath.Dir(dir), "plugin.json"), manifest, 0o644); err != nil {
		return err
	}
	return add(filepath.Join(dir, "hooks.json"), body, 0o644)
}

func addUserClineHookFiles(home, source string, g userTarget, hooks []spec.Entry, onUnsupported string, add func(string, []byte, fs.FileMode) error) error {
	hooks = cline.UserSDKHooks(hooks)
	dir, scriptsDir := g.path(home, g.hooksDir), g.path(home, g.hookScriptsDir)
	if err := adapters.ReportHookProjectRoot("cline", hooks, onUnsupported, true); err != nil {
		return err
	}
	if wrapper, ok := adapters.PortableHookWrapperScript(hooks, "cline", scriptsDir); ok {
		if err := add(wrapper.Path, wrapper.Body, wrapper.Mode); err != nil {
			return err
		}
	}
	for _, hook := range hooks {
		for _, command := range userHookCommands(hook.Meta["command"]) {
			scripts, err := adapters.NeutralHookScripts(command, "cline", filepath.Join(source, "scripts"), scriptsDir, len(stringSliceFromAny(hook.Meta["args"])) > 0)
			if err != nil {
				return err
			}
			for _, script := range scripts {
				if err := add(script.Path, script.Body, script.Mode); err != nil {
					return err
				}
			}
		}
	}
	for _, file := range cline.UserHookFiles(hooks, filepath.ToSlash(scriptsDir)) {
		if err := add(filepath.Join(dir, file.Path), file.Body, file.Mode); err != nil {
			return err
		}
	}
	return nil
}

func addUserGrokHookFiles(home, source string, g userTarget, hooks []spec.Entry, add func(string, []byte, fs.FileMode) error) error {
	for _, hook := range hooks {
		if err := spec.ValidateName(spec.KindHook, hook.Name); err != nil {
			return fmt.Errorf("grok hook %s: %w", hook.Name, err)
		}
		if _, err := grok.HookFile(hook); err != nil {
			return err
		}
	}
	dir, scriptsDir := g.path(home, g.hooksDir), g.path(home, g.hookScriptsDir)
	for _, hook := range hooks {
		prepared := hook
		prepared.Meta = maps.Clone(hook.Meta)
		var commands []string
		for _, command := range grok.NativeHookCommands(hook) {
			scripts, err := adapters.NeutralHookScripts(command, "grok", filepath.Join(source, "scripts"), scriptsDir, len(stringSliceFromAny(hook.Meta["args"])) > 0)
			if err != nil {
				return err
			}
			for _, script := range scripts {
				if err := add(script.Path, script.Body, script.Mode); err != nil {
					return err
				}
			}
			rewritten := adapters.RewriteUserHookPath(command, "grok", filepath.ToSlash(scriptsDir), hook.Meta)
			if runtime.GOOS == "windows" {
				if err := grok.ValidateWindowsScriptRewrite(command, rewritten, filepath.ToSlash(scriptsDir)); err != nil {
					return fmt.Errorf("grok hook %s: %w", hook.Name, err)
				}
			}
			commands = append(commands, rewritten)
		}
		if len(commands) > 0 {
			prepared.Meta["command"] = commands
		}
		raw, err := grok.HookFile(prepared, filepath.ToSlash(scriptsDir))
		if err != nil {
			return err
		}
		if len(raw) == 0 {
			continue
		}
		path := filepath.Join(dir, hook.Name+".json")
		if existing, err := os.ReadFile(path); err == nil {
			var have, want any
			if json.Unmarshal(existing, &have) == nil && json.Unmarshal(raw, &want) == nil && sameSetting(have, want) {
				raw = existing
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("%s: %w", path, err)
		}
		if err := add(path, raw, 0644); err != nil {
			return err
		}
	}
	return nil
}
