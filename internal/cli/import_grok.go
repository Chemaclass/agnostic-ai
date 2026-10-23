package cli

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/grok"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func importFromGrok(root string, src config.Sources, cfg *config.Config) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills, src.Commands, src.Hooks, src.MCPs, src.Settings); err != nil {
		return err
	}
	paths := grok.OutputPaths(cfg)
	for _, kind := range []struct {
		native, dst string
		fields      specFields
		agents      bool
	}{
		{paths["RULES_DIR"], src.Rules, allSpecFields, false}, {paths["AGENTS_DIR"], src.Agents, allSpecFields, true}, {paths["COMMANDS_DIR"], src.Commands, allSpecFields, false}} {
		dirs, err := findScopedSkillDirs(root, kind.native)
		if err != nil {
			return err
		}
		for _, dir := range dirs {
			if err := importMkdirAll(filepath.Join(importSourcePath(root, kind.dst), dir.scope), 0755); err != nil {
				return err
			}
			if _, err := importFlatMarkdownFilesWith(dir.path, filepath.Join(importSourcePath(root, kind.dst), dir.scope), kind.fields, func(data []byte) ([]byte, error) {
				if kind.agents {
					return importGrokAgentMetadata(data)
				}
				return []byte(header.Strip(string(data))), nil
			}); err != nil {
				return err
			}
		}
	}
	if _, err := importScopedSkillFoldersTransformed(root, []string{paths["SKILLS_DIR"]}, importSourcePath(root, src.Skills), &allSpecFields, importGrokSkillName); err != nil {
		return err
	}
	configPath := filepath.Join(root, paths["MCP_FILE"])
	nativeConfig, err := readGrokConfigDocument(configPath)
	if err != nil {
		return err
	}
	servers, _ := nativeConfig["mcp_servers"].(map[string]any)
	if _, err := writeMCPYAMLs("grok", grokMCPDocs(servers), importSourcePath(root, src.MCPs)); err != nil {
		return err
	}
	if path := cfg.Outputs["grok"].ConfFile; path != "" && path != paths["MCP_FILE"] {
		configPath = filepath.Join(root, path)
		separate, err := readGrokConfigDocument(configPath)
		if err != nil {
			return err
		}
		nativeConfig = separate
	}
	settings := map[string]any{}
	permissions, _ := nativeConfig["permission"].(map[string]any)
	if len(permissions) > 0 {
		settings["permissions"] = permissions
	}
	if plugins, present := nativeConfig["plugins"]; present {
		settings["x-grok"] = map[string]any{"plugins": plugins}
	}
	if len(settings) > 0 {
		raw, err := yaml.Marshal(settings)
		if err != nil {
			return fmt.Errorf("%s: encode Grok settings: %w", configPath, err)
		}
		if err := importWriteFile(filepath.Join(importSourcePath(root, src.Settings), "grok.yaml"), raw, 0644); err != nil {
			return err
		}
	}
	hooksDir := ".grok/hooks"
	if out := cfg.Outputs["grok"]; out.HooksDir != "" {
		hooksDir = out.HooksDir
	} else if out.Dir != "" {
		hooksDir = filepath.Join(out.Dir, "hooks")
	}
	scriptsDir := ".grok/hook-scripts"
	if out := cfg.Outputs["grok"]; out.Dir != "" {
		scriptsDir = filepath.Join(out.Dir, "hook-scripts")
	}
	if err := captureGrokHookScripts(filepath.Join(root, scriptsDir), filepath.Join(root, agnosticScriptsDir, "grok")); err != nil {
		return err
	}
	files, err := filepath.Glob(filepath.Join(root, hooksDir, "*.json"))
	if err != nil {
		return fmt.Errorf("%s: %w", hooksDir, err)
	}
	for _, path := range files {
		if _, err := importGrokHookFile(path, importSourcePath(root, src.Hooks), scriptsDir); err != nil {
			return err
		}
	}
	printImportNextSteps(root, "grok")
	return nil
}

func importGrokSkillName(data []byte) ([]byte, error) {
	mapping, rest, ok := frontmatterNode(data)
	if !ok {
		return data, nil
	}
	var meta map[string]any
	if err := mapping.Decode(&meta); err != nil {
		return nil, fmt.Errorf("invalid Grok skill frontmatter")
	}
	if err := validateGrokMetadataKeys(meta); err != nil {
		return nil, err
	}
	if err := validateGrokRetainedCredentials(map[string]any{"inline": meta}); err != nil {
		return nil, err
	}
	if name := mappingValue(mapping, "name"); name != nil {
		setMappingNode(xTargetMapping(mapping, "x-grok"), "name", name)
		removeMappingKey(mapping, "name")
	}
	return encodeFrontmatter(mapping, rest, "grok")
}
func readGrokConfigDocument(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var doc map[string]any
	if _, err := toml.Decode(string(data), &doc); err != nil {
		return nil, fmt.Errorf("parse %s: invalid Grok settings", path)
	}
	return doc, nil
}
func readGrokConfig(path string) (map[string]any, map[string]any, error) {
	doc, err := readGrokConfigDocument(path)
	if err != nil {
		return nil, nil, err
	}
	servers, _ := doc["mcp_servers"].(map[string]any)
	permissions, _ := doc["permission"].(map[string]any)
	return servers, permissions, nil
}
func grokMCPDocs(servers map[string]any) map[string]any {
	out := map[string]any{}
	for name, raw := range servers {
		native, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		entry := maps.Clone(native)
		if _, explicit := entry["type"]; !explicit {
			if url, ok := entry["url"].(string); ok && strings.HasSuffix(url, "/sse") {
				entry["type"] = "sse"
			}
		}
		if enabled, ok := entry["enabled"].(bool); ok {
			entry["disabled"] = !enabled
			delete(entry, "enabled")
		}
		out[name] = entry
	}
	return out
}
func importGrokHookFile(path, dst string, scriptDirs ...string) (int, error) {
	name := strings.TrimSuffix(filepath.Base(path), ".json")
	if err := spec.ValidateName(spec.KindHook, name); err != nil {
		return 0, fmt.Errorf("%s: %w", path, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", path, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}
	probe, err := grokHookEnvironment(path)
	if err != nil {
		return 0, err
	}
	if err := validateGrokHookCredentials(probe); err != nil {
		return 0, fmt.Errorf("%s: %w", path, err)
	}
	hooks, ok := doc["hooks"].(map[string]any)
	if !ok || len(hooks) == 0 {
		return 0, nil
	}
	if len(scriptDirs) > 0 {
		for _, raw := range hooks {
			groups, _ := raw.([]any)
			for _, rawGroup := range groups {
				group, _ := rawGroup.(map[string]any)
				handlers, _ := group["hooks"].([]any)
				for _, rawHandler := range handlers {
					handler, _ := rawHandler.(map[string]any)
					if command, ok := handler["command"].(string); ok {
						handler["command"] = strings.ReplaceAll(command, filepath.ToSlash(scriptDirs[0])+"/", agnosticScriptsDir+"/")
					}
				}
			}
		}
	}
	events := slices.Sorted(maps.Keys(hooks))
	meta := map[string]any{"name": name, "event": events[0], "target": "grok", "x-grok": map[string]any{"hooks": hooks}}
	if _, err := grok.HookFile(spec.Entry{Kind: spec.KindHook, Name: name, Meta: meta}); err != nil {
		return 0, err
	}
	if err := importMkdirAll(dst, 0755); err != nil {
		return 0, fmt.Errorf("%s: %w", dst, err)
	}
	if err := writeHookSpecFile(dst, name, meta); err != nil {
		return 0, err
	}
	return 1, nil
}
