package cli

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/codebuddy"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/preview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func importFromCodebuddy(root string, cfg *config.Config) error {
	src := cfg.Sources
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills, src.Commands, src.Hooks, src.Settings, src.MCPs); err != nil {
		return err
	}
	out := cfg.Outputs["codebuddy"]
	dir := out.Dir
	if dir == "" {
		dir = ".codebuddy"
	}
	rules := out.RulesDir
	if rules == "" {
		rules = filepath.Join(dir, "rules")
	}
	main := out.File
	if main == "" {
		main = "CODEBUDDY.md"
	}
	if err := importCodebuddyInstructions(root, main, dir, out.File != "", src); err != nil {
		return err
	}
	if _, err := importRulesDirectoryWith(root, rules, src, rulesDirImportOpts{NativeTarget: "codebuddy", NativeKeys: []string{"enabled", "alwaysApply", "paths"}}); err != nil {
		return err
	}
	agents := out.AgentsDir
	if agents == "" {
		agents = filepath.Join(dir, "agents")
	}
	if _, err := importFlatMarkdownFilesAt(filepath.Join(root, agents), importSourcePath(root, src.Agents), codebuddyMarkdownFields, func(data []byte, dst string) ([]byte, error) {
		return importCodebuddyMarkdown(data, dst, spec.KindAgent)
	}); err != nil {
		return err
	}
	skills := out.SkillsDir
	if skills == "" {
		skills = filepath.Join(dir, "skills")
	}
	if _, err := importSkillFoldersWith(root, filepath.Join(root, skills), importSourcePath(root, src.Skills), skillFolderImportOpts{Fields: &codebuddyMarkdownFields, TransformSkill: func(data []byte) ([]byte, error) { return importCodebuddyMarkdown(data, "", spec.KindSkill) }}); err != nil {
		return err
	}
	commands := out.CommandsDir
	if commands == "" {
		commands = filepath.Join(dir, "commands")
	}
	if _, err := importFlatMarkdownFilesAt(filepath.Join(root, commands), importSourcePath(root, src.Commands), codebuddyMarkdownFields, func(data []byte, dst string) ([]byte, error) {
		return importCodebuddyMarkdown(data, dst, spec.KindCommand)
	}); err != nil {
		return err
	}
	settings := out.ConfFile
	if settings == "" {
		settings = filepath.Join(dir, "settings.json")
	}
	if err := importCodebuddySettings(root, settings, src); err != nil {
		return err
	}
	if err := captureHookScripts(root, "codebuddy"); err != nil {
		return err
	}
	mcp := out.RootMCPFile
	if mcp == "" {
		mcp = ".mcp.json"
		if !fileExists(filepath.Join(root, mcp)) {
			mcp = "mcp.json"
		}
	}
	servers, err := readCodebuddyMCPMap(filepath.Join(root, mcp))
	if err != nil {
		return err
	}
	normalizeCodebuddyMCP(servers)
	if _, err := writeMCPYAMLs("codebuddy", servers, importSourcePath(root, src.MCPs)); err != nil {
		return err
	}
	printImportNextSteps(root, "codebuddy")
	return nil
}
func importCodebuddyInstructions(root, main, dir string, explicit bool, src config.Sources) error {
	alternate := filepath.Join(dir, "CODEBUDDY.md")
	if !explicit && fileExists(filepath.Join(root, main)) && fileExists(filepath.Join(root, alternate)) {
		return fmt.Errorf("codebuddy: both %s and %s exist; select the intended source with outputs.codebuddy.file before importing", main, alternate)
	}
	hidden := false
	if !explicit && !fileExists(filepath.Join(root, main)) && fileExists(filepath.Join(root, alternate)) {
		main = alternate
		hidden = true
	}
	nested, err := findHierarchicalMainFiles(root, "CODEBUDDY.md", src)
	if err != nil {
		return err
	}
	files := []hierarchicalFile{{path: filepath.Join(root, main)}}
	for _, f := range nested {
		if f.globs != "" && filepath.Clean(f.path) != filepath.Clean(filepath.Join(root, main)) {
			files = append(files, f)
		}
	}
	type document struct {
		file hierarchicalFile
		raw  string
		meta map[string]any
		body string
	}
	var docs []document
	for _, f := range files {
		data, err := readEntryFile(root, f.path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", f.path, err)
		}
		raw := header.Strip(string(data))
		parsed, err := spec.ParseMarkdownBytes(spec.KindRule, []byte(raw))
		if err != nil {
			return fmt.Errorf("parse %s: %w", f.path, err)
		}
		meta, body := parsed.Meta, parsed.Body
		for _, key := range []string{"enabled", "alwaysApply"} {
			if value, set := meta[key]; set {
				if _, ok := value.(bool); !ok {
					return fmt.Errorf("%s: codebuddy: %s must be a boolean", f.path, key)
				}
			}
		}
		docs = append(docs, document{file: f, raw: raw, meta: meta, body: body})
	}
	names := wholeFileRuleNames(root, files)
	used := map[string]int{}
	for _, doc := range docs {
		f := doc.file
		controlled := false
		for _, key := range []string{"enabled", "alwaysApply", "paths"} {
			if _, set := doc.meta[key]; set {
				controlled = true
			}
		}
		if controlled || hidden && f.globs == "" {
			placement := "CODEBUDDY.md"
			if hidden && f.globs == "" {
				placement = ".codebuddy/CODEBUDDY.md"
			}
			scope, err := spec.NormalizeScope(strings.TrimSuffix(f.globs, "/**"))
			if err != nil {
				return fmt.Errorf("%s: codebuddy: instruction scope: %w", f.path, err)
			}
			name := "codebuddy-instructions"
			if scope != "" {
				name = "codebuddy-" + slugify(scope) + "-instructions"
			}
			name = dedupSlug(used, name)
			native := map[string]any{"instruction-file": placement}
			for _, key := range []string{"enabled", "alwaysApply", "paths", "description"} {
				if value, set := doc.meta[key]; set {
					native[key] = value
				}
			}
			meta := map[string]any{"name": name, "target": "codebuddy", "x-codebuddy": native}
			if scope != "" {
				meta["scope"] = scope
			}
			raw, err := writeFrontmatter(meta, doc.body)
			if err != nil {
				return fmt.Errorf("%s: %w", f.path, err)
			}
			dst := filepath.Join(importSourcePath(root, src.Rules), name+".md")
			if err := importWriteSpecMarkdown(dst, raw, 0644, allSpecFields); err != nil {
				return fmt.Errorf("write %s: %w", dst, err)
			}
			continue
		}
		if f.globs == "" {
			if _, err := mirrorBody(root, main, adapters.StripGeneratedAppendices(doc.body)); err != nil {
				return err
			}
		}
		data := []byte(doc.raw)
		text, ok := hierarchicalRulesText(f, string(data))
		if !ok {
			continue
		}
		if name, desc, body, ok := singleGeneratedRule(text); ok {
			if err := writeScopedRuleWithDescription(importSourcePath(root, src.Rules), dedupSlug(used, name), f.globs, desc, body); err != nil {
				return err
			}
			continue
		}
		sections := hierarchicalRuleSections(string(data), text)
		if len(sections) == 0 {
			if err := writeScopedRule(importSourcePath(root, src.Rules), dedupSlug(used, names[f.globs]), f.globs, strings.TrimSpace(text)); err != nil {
				return err
			}
			continue
		}
		for _, s := range sections {
			if err := writeScopedRule(importSourcePath(root, src.Rules), dedupSlug(used, sectionRuleName(names, f.globs, s.slug)), f.globs, s.body); err != nil {
				return err
			}
		}
	}
	return nil
}
func importCodebuddyMarkdown(data []byte, dst string, kind spec.Kind) ([]byte, error) {
	data = []byte(header.Strip(string(data)))
	native, _ := splitMdcFrontmatter(data)
	if err := guardCodebuddyValues(dst, native, true); err != nil {
		return nil, err
	}
	if dst != "" {
		var err error
		data, err = importScopedTargetKeys(data, dst, "codebuddy")
		if err != nil {
			return nil, err
		}
	}
	meta, body := splitMdcFrontmatter(data)
	if name, _ := meta["name"].(string); name != "" && kind == spec.KindAgent {
		if name != slugify(name) || strings.ContainsAny(name, "0123456789_") {
			return nil, fmt.Errorf("codebuddy: invalid native agent name %q", name)
		}
	}
	portable := []string{"name", "description"}
	switch kind {
	case spec.KindAgent:
		portable = append(portable, "model", "effort", "maxTurns", "background", "memory", "tools")
	case spec.KindSkill:
		portable = append(portable, "allowed-tools", "disable-model-invocation", "model")
	case spec.KindCommand:
		portable = append(portable, "argument-hint", "model", "allowed-tools", "disable-model-invocation")
	}
	custom, _ := meta["x-codebuddy"].(map[string]any)
	if custom == nil {
		custom = map[string]any{}
	}
	for _, key := range []string{"model", "allowed-tools"} {
		if value, set := meta[key]; set {
			if _, scoped := custom[key]; !scoped {
				custom[key] = value
				delete(meta, key)
			}
		}
	}
	for key, value := range meta {
		if key != "x-codebuddy" && !slices.Contains(portable, key) {
			custom[key] = value
			delete(meta, key)
		}
	}
	if len(custom) > 0 {
		meta["x-codebuddy"] = custom
	}
	if kind == spec.KindAgent {
		if tools, ok := meta["tools"].(string); ok {
			meta["tools"] = spec.SplitToolList(tools)
		}
	}
	raw, err := writeFrontmatter(meta, body)
	if err != nil {
		return nil, err
	}
	if kind == spec.KindAgent {
		return importNativeAgentTools(raw, "codebuddy", "tools")
	}
	return raw, nil
}
func importCodebuddySettings(root, path string, src config.Sources) error {
	data, err := os.ReadFile(filepath.Join(root, path))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	var native map[string]any
	if err := json.Unmarshal(data, &native); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	for _, key := range codebuddyMapKeys(native) {
		if key == "env" || key == "apiKeyHelper" || key == "awsAuthRefresh" || key == "awsCredentialExport" || preview.CredentialKey(key) {
			delete(native, key)
			summaryf("  ! %s: %s stays in the native file; environment and credential settings are not imported\n", path, key)
		}
	}
	if err := guardCodebuddyValues(path, native, false); err != nil {
		return err
	}
	doc := map[string]any{}
	if model, _ := native["model"].(string); model != "" {
		doc["model"] = map[string]any{"codebuddy": model}
		delete(native, "model")
	}
	settingsDir := importSourcePath(root, src.Settings)
	plan, level, err := planSettingsEffort("codebuddy", native["reasoningEffort"], settingsDir, "codebuddy.yaml")
	if err != nil {
		return err
	}
	switch plan {
	case effortPromote:
		maps.Copy(doc, settingsEffortSpec(plan, "codebuddy", "reasoningEffort", level))
		delete(native, "reasoningEffort")
	case effortStays:
		// The leftover native keys become x-codebuddy, so a value the other
		// specs already resolve to is dropped instead of pinned there.
		if level != "" {
			others, err := otherSettingsSpecs(settingsDir, filepath.Join(settingsDir, "codebuddy.yaml"))
			if err != nil {
				return err
			}
			if adapters.SettingsEffort(others, "codebuddy") == level {
				delete(native, "reasoningEffort")
			}
		}
	}
	// Native permission patterns stay scoped to CodeBuddy.
	if hooks, ok := native["hooks"].(map[string]any); ok {
		remaining := map[string]any{}
		for _, event := range codebuddyMapKeys(hooks) {
			groups, ok := hooks[event].([]any)
			if !ok || !codebuddySimpleHookGroups(event, groups) {
				remaining[event] = hooks[event]
				continue
			}
			for i, raw := range groups {
				group, _ := raw.(map[string]any)
				handlers, _ := group["hooks"].([]any)
				var commands []any
				decision := false
				for _, raw := range handlers {
					handler, _ := raw.(map[string]any)
					command, _ := handler["command"].(string)
					if options, inner, ok := adapters.UnwrapPortableHook(command); ok {
						command = inner
						decision = decision || slices.Contains(options, "--decision")
					}
					commands = append(commands, command)
				}
				name := fmt.Sprintf("codebuddy-%s-%d", slugify(event), i+1)
				hook := map[string]any{"name": name, "targets": []string{"codebuddy"}, "event": event, "command": commands}
				if len(commands) == 1 {
					hook["command"] = commands[0]
				}
				if first, _ := handlers[0].(map[string]any); first["type"] == "prompt" {
					delete(hook, "command")
					hook["type"], hook["prompt"] = "prompt", first["prompt"]
					if first["continueOnBlock"] == true {
						hook["continueOnBlock"] = true
					}
				}
				if decision {
					hook["decision"] = "stdout"
				}
				if matcher, ok := group["matcher"]; ok {
					hook["matcher"] = matcher
				}
				if len(handlers) > 0 {
					first, _ := handlers[0].(map[string]any)
					if timeout, ok := first["timeout"]; ok {
						hook["timeout"] = timeout
					}
				}
				if err := writeHookSpecFile(importSourcePath(root, src.Hooks), name, hook); err != nil {
					return err
				}
			}
		}
		if len(remaining) > 0 {
			native["hooks"] = remaining
		} else {
			delete(native, "hooks")
		}
	}
	if len(native) > 0 {
		doc["x-codebuddy"] = native
	}
	if len(doc) == 0 {
		return nil
	}
	raw, err := yaml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	dst := filepath.Join(settingsDir, "codebuddy.yaml")
	if err := importWriteFile(dst, raw, 0644); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}
func codebuddyMapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
func normalizeCodebuddyMCP(servers map[string]any) {
	for _, raw := range servers {
		if server, ok := raw.(map[string]any); ok {
			custom := map[string]any{}
			for key, value := range server {
				if !slices.Contains([]string{"command", "args", "env", "url", "headers", "type"}, key) {
					custom[key] = value
					delete(server, key)
				}
			}
			if len(custom) > 0 {
				server["x-codebuddy"] = custom
			}
		}
	}
}

func codebuddySimpleHookGroups(event string, groups []any) bool {
	if !slices.Contains(codebuddy.HookEvents(), event) {
		return false
	}
	for _, raw := range groups {
		group, ok := raw.(map[string]any)
		if !ok {
			return false
		}
		for key := range group {
			if key != "hooks" && key != "matcher" {
				return false
			}
		}
		handlers, ok := group["hooks"].([]any)
		if !ok || len(handlers) == 0 {
			return false
		}
		if codebuddyPromptHandler(event, handlers) {
			continue
		}
		var timeout any
		for i, raw := range handlers {
			h, ok := raw.(map[string]any)
			if !ok || h["type"] != "command" {
				return false
			}
			command, _ := h["command"].(string)
			if command == "" {
				return false
			}
			for key := range h {
				if key != "type" && key != "command" && key != "timeout" {
					return false
				}
			}
			if i == 0 {
				timeout = h["timeout"]
			} else if timeout != h["timeout"] {
				return false
			}
		}
	}
	return true
}

// A group imports as a prompt spec only when its one handler uses the fields CodeBuddy documents.
func codebuddyPromptHandler(event string, handlers []any) bool {
	if len(handlers) != 1 || !slices.Contains(codebuddy.PromptHookEvents(), event) {
		return false
	}
	h, ok := handlers[0].(map[string]any)
	if !ok || h["type"] != "prompt" {
		return false
	}
	if prompt, _ := h["prompt"].(string); prompt == "" {
		return false
	}
	if v, ok := h["continueOnBlock"]; ok {
		if _, isBool := v.(bool); !isBool {
			return false
		}
	}
	for key := range h {
		if !slices.Contains([]string{"type", "prompt", "timeout", "continueOnBlock"}, key) {
			return false
		}
	}
	return true
}

var codebuddyMarkdownFields = specFields{all: true, omitted: map[string]bool{"model": true, "allowed-tools": true}, preserve: preserveCodebuddySharedFields}

func preserveCodebuddySharedFields(existing, imported *yaml.Node) {
	for _, key := range []string{"model", "allowed-tools"} {
		if mappingValue(existing, key) == nil || mappingValue(imported, key) != nil {
			continue
		}
		custom := xTargetMapping(imported, "x-codebuddy")
		if mappingValue(custom, key) == nil {
			setMappingNode(custom, key, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"})
		}
	}
}
