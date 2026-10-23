package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func grokHookEnvironment(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	hooks, _ := doc["hooks"].(map[string]any)
	return grokHookEntries(hooks), nil
}
func grokHookEntries(hooks map[string]any) map[string]any {
	probe := map[string]any{}
	index := 0
	for _, raw := range hooks {
		groups, _ := raw.([]any)
		for _, rawGroup := range groups {
			group, _ := rawGroup.(map[string]any)
			handlers, _ := group["hooks"].([]any)
			for _, rawHandler := range handlers {
				handler, _ := rawHandler.(map[string]any)
				entry := map[string]any{}
				for _, field := range []string{"command", "url", "env", "headers"} {
					if value, ok := handler[field]; ok {
						entry[field] = value
					}
				}
				probe[fmt.Sprint(index)] = entry
				index++
			}
		}
	}
	return probe
}
func validateGlobalGrokImport(home string, targets []string) error {
	for _, target := range targets {
		if target != "grok" {
			continue
		}
		g := globalTargets[target]
		servers, _, err := readGrokConfig(g.mcpPath(home))
		if err != nil {
			return err
		}
		var names []string
		for name := range servers {
			names = append(names, name)
		}
		if err := spec.ValidateMCPNames(names); err != nil {
			return fmt.Errorf("%s: %w", g.mcpPath(home), err)
		}
		dir := g.path(home, g.hooksDir)
		files, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
		for _, file := range files {
			if file.IsDir() {
				continue
			}
			name := file.Name()
			if len(name) < 5 || name[len(name)-5:] != ".json" {
				continue
			}
			if err := spec.ValidateName(spec.KindHook, name[:len(name)-5]); err != nil {
				return fmt.Errorf("%s: %w", dir, err)
			}
		}
	}
	return nil
}

func grokHookCredentialValue(key, value string) bool {
	stripped, _ := spec.StripEnvRefDefaults(value)
	return mcpCredentialKey(key) && value != "" && (!mcpRefOnly(value) || stripped != value) || mcpCredentialSetting(key, value)
}
