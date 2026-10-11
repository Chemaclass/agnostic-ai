package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/copilot"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func readGlobalCopilotHooks(path string) (map[string][]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var doc struct {
		Version int              `json:"version"`
		Hooks   map[string][]any `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if doc.Version != 1 {
		return nil, fmt.Errorf("%s: unsupported Copilot hook version %d", path, doc.Version)
	}
	return doc.Hooks, nil
}

func adoptGlobalCopilotHook(managed, event string, item, plain any, meta map[string]any) (bool, error) {
	dir := filepath.Dir(managed)
	files, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", dir, err)
	}
	origin := ""
	if scoped, ok := meta["x-copilot"].(map[string]any); ok {
		origin, _ = scoped["user-hook-file"].(string)
	}
	if origin != "" {
		if filepath.Base(origin) != origin || !strings.HasSuffix(origin, ".json") {
			return false, fmt.Errorf("%s: imported hook source must be a JSON filename inside the native hooks directory", managed)
		}
		originPath := filepath.Join(dir, origin)
		sourceHooks, err := readGlobalCopilotHooks(originPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		if err == nil {
			matches := slices.ContainsFunc(sourceHooks[event], func(existing any) bool { return sameSetting(existing, item) || sameSetting(existing, plain) })
			if !matches {
				return false, fmt.Errorf("%s: imported %s hook differs from its source file; update the native entry to match the spec, or move this file outside the hooks directory before syncing", originPath, event)
			}
		}
	}
	adopted := false
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") || file.Name() == filepath.Base(managed) {
			continue
		}
		path := filepath.Join(dir, file.Name())
		hooks, err := readGlobalCopilotHooks(path)
		if err != nil {
			return false, err
		}
		for _, existing := range hooks[event] {
			if sameSetting(existing, item) || sameSetting(existing, plain) {
				adopted = true
				continue
			}
			if sameGlobalCopilotHandler(existing, item) {
				return false, fmt.Errorf("%s: %s hook runs the same command with different fields; make it match the spec or move the original file outside the hooks directory", path, event)
			}
		}
	}
	return adopted, nil
}

func stageGlobalCopilotHooks(managed, stage string, have []spec.Entry, warn io.Writer) error {
	dir := filepath.Dir(managed)
	files, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, file.Name())
		hooks, err := readGlobalCopilotHooks(path)
		if err != nil {
			return err
		}
		for _, event := range slices.Sorted(maps.Keys(hooks)) {
			for _, raw := range hooks[event] {
				handler, ok := raw.(map[string]any)
				if !ok {
					return fmt.Errorf("%s: %s hook entry must be an object", path, event)
				}
				doc := normalizeCopilotHook(event, maps.Clone(handler))
				if doc != nil {
					doc = jsonRoundTrip(doc).(map[string]any)
				}
				preserved := false
				if doc != nil {
					rendered, err := copilot.HookEntries([]spec.Entry{{Meta: doc}})
					if err != nil {
						return fmt.Errorf("render %s hook: %w", path, err)
					}
					for _, candidate := range rendered[event] {
						plain := jsonRoundTrip(candidate).(map[string]any)
						dropHookTargetEnv(plain, "copilot")
						if sameSetting(handler, candidate) || sameSetting(handler, plain) {
							preserved = true
						}
					}
				}
				if !preserved {
					if _, err := fmt.Fprintf(warn, "warning: %s: skipped %s hook entry; its fields cannot be imported without changing behavior\n", path, event); err != nil {
						return err
					}
					continue
				}
				covered := false
				for _, existing := range (spec.Bundle{Hooks: have}).HooksFor("copilot") {
					if !existing.EmitsTo("copilot") {
						continue
					}
					rendered, err := globalCopilotHookEntries(existing, filepath.Join(dir, "hooks"))
					if err != nil {
						return fmt.Errorf("render existing Copilot hooks: %w", err)
					}
					for _, candidate := range rendered[event] {
						plain := jsonRoundTrip(candidate).(map[string]any)
						dropHookTargetEnv(plain, "copilot")
						if sameSetting(handler, candidate) || sameSetting(handler, plain) {
							covered = true
						}
					}
				}
				if covered {
					continue
				}
				payload, err := json.Marshal(handler)
				if err != nil {
					return fmt.Errorf("marshal %s hook: %w", path, err)
				}
				matcher, _ := handler["matcher"].(string)
				name := hookSpecName(event, matcher, []string{string(payload)})
				doc["name"], doc["target"] = name, "copilot"
				if file.Name() != filepath.Base(managed) {
					doc["x-copilot"] = map[string]any{"user-hook-file": file.Name()}
				}
				dst := filepath.Join(stage, "hooks")
				if err := importMkdirAll(dst, 0o755); err != nil {
					return fmt.Errorf("create %s: %w", dst, err)
				}
				if err := writeHookSpecFile(dst, name, doc); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func sameGlobalCopilotHandler(a, b any) bool {
	left, ok := a.(map[string]any)
	if !ok {
		return false
	}
	right, ok := b.(map[string]any)
	if !ok || !sameSetting(left["matcher"], right["matcher"]) {
		return false
	}
	for _, key := range []string{"command", "bash", "powershell", "exec", "url", "prompt"} {
		if value, ok := left[key].(string); ok && value != "" && value == right[key] {
			return true
		}
	}
	return false
}

func globalCopilotHookEntries(entry spec.Entry, scriptsDir string) (map[string][]any, error) {
	entry.Meta = adapters.ResolveMeta(entry.Meta, "copilot")
	rewrite := func(command string) string {
		return adapters.RewriteGlobalHookPath(command, "copilot", filepath.ToSlash(scriptsDir), entry.Meta)
	}
	commands := globalHookCommands(entry.Meta["command"])
	for i := range commands {
		commands[i] = rewrite(commands[i])
	}
	if len(commands) == 1 {
		entry.Meta["command"] = commands[0]
	} else if len(commands) > 1 {
		entry.Meta["command"] = commands
	}
	if windows, ok := entry.Meta["commandWindows"].(string); ok {
		entry.Meta["commandWindows"] = adapters.RewriteGlobalWindowsHookPath(windows, filepath.ToSlash(scriptsDir))
	}
	return copilot.HookEntries([]spec.Entry{entry}, scriptsDir)
}
