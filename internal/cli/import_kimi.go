package cli

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/adapters/kimi"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func importFromKimi(root string, src config.Sources, cfg *config.Config) error {
	o := cfg.Outputs["kimi"]
	agents, skills, mcp, main := ".kimi-code/agents", ".kimi-code/skills", ".kimi-code/mcp.json", ".kimi-code/AGENTS.md"
	if o.AgentsDir != "" {
		agents = o.AgentsDir
	}
	if o.SkillsDir != "" {
		skills = o.SkillsDir
	}
	if o.MCPFile != "" {
		mcp = o.MCPFile
	}
	if o.File != "" {
		main = o.File
	}
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills, src.MCPs, src.Hooks, src.Settings); err != nil {
		return err
	}
	if err := importKimiAgents(root, filepath.Join(root, agents), importSourcePath(root, src.Agents)); err != nil {
		return err
	}
	fields := fieldsOf("name", "description", "disable-model-invocation", "x-kimi")
	transform := func(data []byte) ([]byte, error) { return normalizeKimiMarkdown("skill", data, "") }
	seen := map[string]bool{}
	if _, err := importSkillFoldersWith(root, filepath.Join(root, skills), importSourcePath(root, src.Skills), skillFolderImportOpts{Fields: &fields, TransformSkill: transform, SkipNames: seen}); err != nil {
		return err
	}
	flat, err := os.ReadDir(filepath.Join(root, skills))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", skills, err)
	}
	for _, f := range flat {
		if f.IsDir() || filepath.Ext(f.Name()) != ".md" {
			continue
		}
		name := strings.TrimSuffix(f.Name(), ".md")
		if seen[name] {
			continue
		}
		path := filepath.Join(root, skills, f.Name())
		if !regularFileInside(root, path) {
			return fmt.Errorf("%s: skill must be a regular file inside the project", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		data, err = normalizeKimiMarkdown("skill", data, name)
		if err != nil {
			return err
		}
		if err := importWriteSpecMarkdown(filepath.Join(importSourcePath(root, src.Skills), name+".md"), data, 0o644, fields); err != nil {
			return err
		}
	}
	servers, err := readJSONMapAt(filepath.Join(root, mcp), "mcpServers")
	if err != nil {
		return err
	}
	if o.RootMCPFile != "" {
		other, err := readJSONMapAt(filepath.Join(root, o.RootMCPFile), "mcpServers")
		if err != nil {
			return err
		}
		for n, v := range other {
			if _, ok := servers[n]; !ok {
				if servers == nil {
					servers = map[string]any{}
				}
				servers[n] = v
			}
		}
	}
	for name, raw := range servers {
		server, _ := raw.(map[string]any)
		if err := guardKimiCredentials("Kimi MCP "+name, server); err != nil {
			return err
		}
	}
	normalizeImportedMCP("kimi", servers)
	if _, err := writeMCPSpecs(servers, importSourcePath(root, src.MCPs)); err != nil {
		return err
	}
	if o.ConfFile != "" {
		doc, err := readVibeTOML(filepath.Join(root, o.ConfFile))
		if err != nil {
			return err
		}
		if err := guardKimiCredentials(o.ConfFile, doc); err != nil {
			return err
		}
		records, err := nativeTOMLRecords(o.ConfFile, "hooks", doc["hooks"])
		if err != nil {
			return err
		}
		if err := importKimiHooks(records, importSourcePath(root, src.Hooks)); err != nil {
			return err
		}
		delete(doc, "hooks")
		if len(doc) > 0 {
			if err := writeSettingsSpec(importSourcePath(root, src.Settings), "kimi.yaml", map[string]any{"x-kimi": doc}); err != nil {
				return err
			}
		}
	}
	if _, err := mirrorMainFile(root, main); err != nil {
		return err
	}
	tree := importTreeFor(root)
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if path != root && (strings.HasPrefix(d.Name(), ".") || d.Name() == "vendor" || tree.skipsDir(rel)) {
			return fs.SkipDir
		}
		native := filepath.Join(path, ".kimi-code", "AGENTS.md")
		if path == root {
			native = filepath.Join(root, main)
		}
		data, err := readEntryFile(root, native)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%s: %w", native, err)
		}
		body := header.Strip(adapters.StripLocalInstructions(string(data)))
		if path == root {
			body = reduceToGeneratedRules(body)
			if !strings.Contains(string(data), adapters.RulesStartMarker) {
				return nil
			}
		}
		scope := ""
		if rel != "." {
			scope = rel
		}
		if strings.Contains(string(data), adapters.RulesStartMarker) {
			for _, r := range generatedBlockRules(body) {
				if err := writeKimiRule(importSourcePath(root, src.Rules), r.slug, r.body, scope); err != nil {
					return err
				}
			}
			if name, _, text, ok := singleGeneratedRule(body); ok {
				return writeKimiRule(importSourcePath(root, src.Rules), name, text, scope)
			}
			return nil
		}
		return writeKimiRule(importSourcePath(root, src.Rules), "kimi-"+slugify(rel), body, scope)
	})
	if err != nil {
		return fmt.Errorf("import Kimi instructions: %w", err)
	}
	return captureHookScripts(root, "kimi")
}
func writeKimiRule(dst, name, body, scope string) error {
	meta := map[string]any{"name": name}
	if scope != "" {
		meta["scope"] = scope
	}
	data, err := writeFrontmatter(meta, strings.TrimSpace(body))
	if err != nil {
		return fmt.Errorf("render Kimi rule %s: %w", name, err)
	}
	return importWriteSpecMarkdown(filepath.Join(dst, name+".md"), data, 0o644, allSpecFields)
}
func normalizeKimiMarkdown(kind string, data []byte, fallback string) ([]byte, error) {
	clean := header.Strip(string(data))
	rest, end, m := claudeFrontmatter(clean)
	body := clean
	if m != nil {
		body = strings.TrimSpace(rest[end+4:])
	} else {
		if kind == "agent" || fallback == "" {
			return nil, fmt.Errorf("Kimi %s requires YAML frontmatter", kind)
		}
		m = map[string]any{}
	}
	if err := guardKimiCredentials("Kimi "+kind+" metadata", m); err != nil {
		return nil, err
	}
	name, _ := m["name"].(string)
	if name == "" {
		name = fallback
	}
	if err := spec.ValidateName(spec.KindAgent, name); err != nil {
		return nil, fmt.Errorf("Kimi %s: %w", kind, err)
	}
	description, _ := m["description"].(string)
	if description == "" {
		if kind == "agent" || fallback == "" {
			return nil, fmt.Errorf("Kimi %s %s requires a nonempty description", kind, name)
		}
		description = "No description provided."
		for _, line := range strings.Split(body, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				runes := []rune(line)
				if len(runes) > 240 {
					line = string(runes[:239]) + "\U00002026"
				}
				description = line
				break
			}
		}
	}
	out := map[string]any{"name": name, "description": description}
	custom := maps.Clone(m)
	delete(custom, "name")
	delete(custom, "description")
	if kind == "skill" {
		for _, key := range []string{"disableModelInvocation", "disable-model-invocation", "disable_model_invocation"} {
			if v, ok := custom[key]; ok {
				out["disable-model-invocation"] = v
				delete(custom, key)
			}
		}
	} else {
		for _, key := range []string{"model", "effort", "maxTurns"} {
			if _, ok := custom[key]; ok {
				summaryf("  ! Kimi agent %s ignores native field %s\n", name, key)
				delete(custom, key)
			}
		}
	}
	if len(custom) > 0 {
		out["x-kimi"] = custom
	}
	return writeFrontmatter(out, body)
}
func importKimiAgents(root, dir, dst string) error {
	if !dirExists(dir) {
		return nil
	}
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		if !regularFileInside(root, path) {
			return fmt.Errorf("%s: agent must be a regular file inside the project", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		out, err := normalizeKimiMarkdown("agent", data, strings.TrimSuffix(filepath.Base(path), ".md"))
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		_, _, m := claudeFrontmatter(string(out))
		name, _ := m["name"].(string)
		return importWriteSpecMarkdown(filepath.Join(dst, name+".md"), out, 0o644, fieldsOf("name", "description", "tools", "x-kimi"))
	})
}
func normalizeKimiMCP(server map[string]any) {
	for _, key := range []string{"command", "cwd"} {
		if v, ok := server[key].(string); ok {
			server[key] = spec.EscapeEnvRefs(v)
		}
	}
	transport, ok := server["transport"]
	if ok {
		server["type"] = transport
		delete(server, "transport")
	}
	if enabled, ok := server["enabled"].(bool); ok {
		server["disabled"] = !enabled
		delete(server, "enabled")
	}
	native := map[string]any{}
	for _, key := range []string{"executor", "runtime_id", "auth", "bearerTokenEnvVar", "deferred", "startupTimeoutMs", "toolTimeoutMs", "enabledTools", "disabledTools"} {
		if v, ok := server[key]; ok {
			native[key] = v
			delete(server, key)
		}
	}
	if len(native) > 0 {
		server["x-kimi"] = native
	}
}
func guardKimiCredentials(path string, doc map[string]any) error {
	field := ""
	walkMCPStrings("", "", jsonRoundTrip(doc), func(p, key, value string) {
		if field != "" {
			return
		}
		if mcpCredentialKey(key) && !mcpRefOnly(value) && mcpSecretValue(mcpDetectorText(value), false, mcpWeakCredential) || mcpCredentialDetected(value) {
			field = strings.TrimPrefix(p, ".")
		}
	})
	if field != "" {
		return fmt.Errorf("%s: %s holds a literal credential; keep it outside committed specs", path, field)
	}
	return nil
}
func importKimiHooks(records []map[string]any, dst string) error {
	if len(records) == 0 {
		return nil
	}
	if err := importMkdirAll(dst, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	for _, r := range records {
		if err := kimi.ValidateHookRecord(r); err != nil {
			return fmt.Errorf("import Kimi hook: %w", err)
		}
		name := "kimi-" + globalSum(kimiRecordJSON(r))[:12]
		meta := maps.Clone(r)
		meta["name"] = name
		meta["target"] = "kimi"
		if cmd, ok := meta["command"].(string); ok {
			meta["command"] = strings.ReplaceAll(kimi.DeclaredHookCommand(cmd), ".kimi-code/hooks/", agnosticScriptsDir+"/")
		}
		if err := writeHookSpecFile(dst, name, meta); err != nil {
			return err
		}
	}
	return nil
}

func kimiRecordJSON(r map[string]any) []byte { data, _ := json.Marshal(r); return data }
