package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const qwenMainFile = "QWEN.md"
const qwenSettingsFile = ".qwen/settings.json"
const qwenRulesDir = ".qwen/rules"

func importFromQwen(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills, src.Commands, src.MCPs, src.Hooks, src.Settings); err != nil {
		return err
	}
	rules, err := importHierarchicalInstructionRules(root, importSourcePath(root, src.Rules), src, qwenMainFile)
	if err != nil {
		return err
	}
	pathRules, err := importRulesDirectoryWith(root, qwenRulesDir, src, rulesDirImportOpts{NormalizeMeta: normalizeQwenRuleMeta, FlatOnly: true})
	if err != nil {
		return err
	}
	rules += pathRules.rules
	if err := noteQwenAlwaysOnRuleFiles(root); err != nil {
		return err
	}
	agents, err := importFlatMarkdownFilesAt(filepath.Join(root, ".qwen/agents"), importSourcePath(root, src.Agents), allSpecFields, func(data []byte, dst string) ([]byte, error) {
		native, _ := splitMdcFrontmatter([]byte(header.Strip(string(data))))
		if err := guardQwenValues(dst, native, false); err != nil {
			return nil, err
		}
		data, err := importScopedTargetKeys(data, dst, "qwen")
		if err != nil {
			return nil, err
		}
		data, err = importNativeAgentTools(data, "qwen", "tools")
		if err != nil {
			return nil, err
		}
		return scopeQwenAgentFields(data)
	})
	if err != nil {
		return err
	}
	skills, err := importSkillFoldersWith(root, filepath.Join(root, ".qwen/skills"), importSourcePath(root, src.Skills), skillFolderImportOpts{TransformSkill: scopeQwenSkillFields, Fields: &allSpecFields})
	if err != nil {
		return err
	}
	commands, err := importQwenCommands(root, importSourcePath(root, src.Commands))
	if err != nil {
		return err
	}
	mcps, hooks, settings, err := importQwenSettings(root, src)
	if err != nil {
		return err
	}
	if err := captureHookScripts(root, "qwen"); err != nil {
		return err
	}
	ignores, err := importIgnoreFile(root, "qwen", src)
	if err != nil {
		return err
	}
	if _, err := mirrorMainFile(root, qwenMainFile); err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d skills, %d commands, %d mcps, %d hooks, %d settings, %d ignores (from qwen)\n", rules, agents, skills, commands, mcps, hooks, settings, ignores)
	printImportNextSteps(root, "qwen")
	return nil
}

// normalizeQwenRuleMeta reads Qwen's `paths:` as portable globs. Qwen scans
// .qwen/rules/ recursively, but there a subfolder only groups files while an
// imported subfolder would become a directory scope, so import reads the top
// level only.
func normalizeQwenRuleMeta(meta map[string]any) {
	if paths, ok := meta["paths"]; ok {
		delete(meta, "paths")
		meta["globs"] = paths
	}
}

// noteQwenAlwaysOnRuleFiles names the imported rule files without `paths:`.
// Sync inlines those rules into QWEN.md and never owns the original file.
func noteQwenAlwaysOnRuleFiles(root string) error {
	dir := filepath.Join(root, qwenRulesDir)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		if meta, _ := splitMdcFrontmatter([]byte(header.Strip(string(data)))); meta["paths"] == nil {
			keptf("note: %s/%s has no paths, so sync puts it in %s; delete the file so Qwen does not load it twice\n", qwenRulesDir, e.Name(), qwenMainFile)
		}
	}
	return nil
}

func scopeQwenAgentFields(data []byte) ([]byte, error) {
	return scopeQwenFields(data, "name", "description", "model", "color", "maxTurns", "can")
}
func scopeQwenSkillFields(data []byte) ([]byte, error) {
	native, _ := splitMdcFrontmatter([]byte(header.Strip(string(data))))
	if err := guardQwenValues("qwen skill frontmatter", native, false); err != nil {
		return nil, err
	}
	return scopeQwenFields(data, "name", "description", "license")
}
func scopeQwenFields(data []byte, keep ...string) ([]byte, error) {
	mapping, rest, ok := frontmatterNode(data)
	if !ok {
		return data, nil
	}
	var keys []string
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		keys = append(keys, mapping.Content[i].Value)
	}
	for _, key := range keys {
		if key == "x-qwen" || slices.Contains(keep, key) {
			continue
		}
		value := mappingValue(mapping, key)
		if value == nil {
			continue
		}
		setMappingNode(xTargetMapping(mapping, "x-qwen"), key, value)
		for i := 0; i+1 < len(mapping.Content); i += 2 {
			if mapping.Content[i].Value == key {
				mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
				break
			}
		}
	}
	return encodeFrontmatter(mapping, rest, "qwen")
}
func importQwenSettings(root string, src config.Sources) (int, int, int, error) {
	path := filepath.Join(root, qwenSettingsFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, 0, 0, nil
	}
	if err != nil {
		return 0, 0, 0, fmt.Errorf("read %s: %w", path, err)
	}
	data, _ = adapters.StripJSONC(data)
	var native map[string]any
	if err := json.Unmarshal(data, &native); err != nil {
		return 0, 0, 0, fmt.Errorf("parse %s: %w", path, err)
	}
	retained := maps.Clone(native)
	delete(retained, "mcpServers")
	if err := guardQwenValues(path, retained, true); err != nil {
		return 0, 0, 0, err
	}
	mcps := 0
	if servers, ok := native["mcpServers"].(map[string]any); ok {
		for _, raw := range servers {
			if server, ok := raw.(map[string]any); ok {
				normalizeGeminiTransport(server)
				scopeQwenMCP(server)
			}
		}
		mcps, err = writeMCPYAMLs("qwen", servers, importSourcePath(root, src.MCPs))
		if err != nil {
			return mcps, 0, 0, err
		}
	}
	delete(native, "mcpServers")
	hooks := 0
	if byEvent, ok := native["hooks"].(map[string]any); ok {
		used := map[string]bool{}
		for _, event := range sortedHookEventsMap(byEvent) {
			groups, _ := byEvent[event].([]any)
			for _, raw := range groups {
				group, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				matcher, _ := group["matcher"].(string)
				handlers, _ := group["hooks"].([]any)
				var commands []string
				for _, raw := range handlers {
					if handler, ok := raw.(map[string]any); ok {
						dropHookTargetEnv(handler, "qwen")
						if command, _ := handler["command"].(string); command != "" {
							commands = append(commands, command)
						}
					}
				}
				if len(handlers) == 0 {
					continue
				}
				name := hookSpecName(event, matcher, commands)
				base := name
				for suffix := 2; used[name]; suffix++ {
					name = fmt.Sprintf("%s-%d", base, suffix)
				}
				used[name] = true
				extra := maps.Clone(group)
				delete(extra, "matcher")
				doc := map[string]any{"name": name, "event": event, "target": "qwen", "x-qwen": extra}
				if matcher != "" {
					doc["matcher"] = matcher
				}
				if err := writeHookSpecFile(importSourcePath(root, src.Hooks), name, doc); err != nil {
					return mcps, hooks, 0, err
				}
				hooks++
			}
		}
	}
	if hooksObject, ok := native["hooks"].(map[string]any); ok {
		for event, value := range hooksObject {
			if _, list := value.([]any); list {
				delete(hooksObject, event)
			}
		}
		if len(hooksObject) == 0 {
			delete(native, "hooks")
		}
	}
	settings := 0
	if len(native) > 0 {
		doc := map[string]any{"x-qwen": native}
		if model, ok := native["model"].(map[string]any); ok {
			if name, _ := model["name"].(string); name != "" {
				doc["model"] = name
				delete(model, "name")
				if len(model) == 0 {
					delete(native, "model")
				}
			}
		}
		data, err := yaml.Marshal(doc)
		if err != nil {
			return mcps, hooks, 0, fmt.Errorf("marshal settings from %s: %w", path, err)
		}
		out := filepath.Join(importSourcePath(root, src.Settings), "qwen.yaml")
		if err := importWriteFile(out, data, 0644); err != nil {
			return mcps, hooks, 0, fmt.Errorf("write %s: %w", out, err)
		}
		settings = 1
	}
	return mcps, hooks, settings, nil
}

func sortedHookEventsMap(events map[string]any) []string {
	values := make(map[string][]any, len(events))
	for key := range events {
		values[key] = nil
	}
	return sortedHookEvents(values)
}

func scopeQwenMCP(server map[string]any) {
	known := []string{"type", "url", "command", "args", "cwd", "env", "headers", "timeout", "trust", "description", "includeTools", "excludeTools"}
	native := map[string]any{}
	for key, value := range server {
		if !slices.Contains(known, key) && key != "x-qwen" {
			native[key] = value
			delete(server, key)
		}
	}
	if len(native) > 0 {
		server["x-qwen"] = native
	}
}

func importQwenCommands(root, dstDir string) (int, error) {
	src := filepath.Join(root, ".qwen/commands")
	count := 0
	used := map[string]int{}
	err := filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if errors.Is(walkErr, fs.ErrNotExist) && path == src {
			return nil
		}
		if walkErr != nil {
			return fmt.Errorf("read %s: %w", path, walkErr)
		}
		if entry.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".md" && ext != ".toml" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("resolve command %s: %w", path, err)
		}
		name := strings.ReplaceAll(strings.TrimSuffix(filepath.ToSlash(rel), ext), "/", ":")
		var doc map[string]any
		var body string
		if ext == ".toml" {
			description, extra, prompt := parseGeminiCommandTOML(string(data))
			doc = map[string]any{"description": description}
			body = prompt
			if len(extra) > 0 {
				doc["x-qwen"] = extra
			}
		} else {
			parsed, err := spec.ParseMarkdownBytes(spec.KindCommand, []byte(header.Strip(string(data))))
			if err != nil {
				return fmt.Errorf("parse %s: %w", path, err)
			}
			doc = parsed.Meta
			body = parsed.Body
		}
		if doc == nil {
			doc = map[string]any{}
		}
		doc["name"] = name
		front, err := yaml.Marshal(doc)
		if err != nil {
			return fmt.Errorf("render command %s: %w", path, err)
		}
		imported, err := scopeQwenFields([]byte("---\n"+string(front)+"---\n\n"+strings.TrimSpace(body)+"\n"), "name", "description")
		if err != nil {
			return err
		}
		out := filepath.Join(dstDir, dedupSlug(used, strings.ReplaceAll(name, ":", "-"))+".md")
		if err := importWriteSpecMarkdown(out, imported, 0644, allSpecFields); err != nil {
			return fmt.Errorf("write %s: %w", out, err)
		}
		importReadingNative(path)
		count++
		return nil
	})
	return count, err
}
