package cli

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/vibe"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func readVibeTOML(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return decodeTOMLDocument(path, data)
}
func importFromVibe(root string, src config.Sources, cfg *config.Config) error {

	agents := ".vibe/agents"
	skills := ".vibe/skills"
	conf := ".vibe/config.toml"
	hooks := ".vibe/hooks.toml"
	mcp := ""
	mainFile := "AGENTS.md"
	rulesFile := ""
	if cfg != nil {
		o := cfg.Outputs["vibe"]
		if o.File != "" {
			mainFile = o.File
		}
		rulesFile = o.RulesFile
		if o.AgentsDir != "" {
			agents = o.AgentsDir
		}
		if o.SkillsDir != "" {
			skills = o.SkillsDir
		}
		if o.ConfFile != "" {
			conf = o.ConfFile
		}
		if o.HooksFile != "" {
			hooks = o.HooksFile
		}
		mcp = o.MCPFile
	}
	if mcp == "" {
		mcp = conf
	}
	hookPath := filepath.Join(root, hooks)
	hd, err := readVibeTOML(hookPath)
	if err != nil {
		return err
	}
	records, err := nativeTOMLRecords(hookPath, "hooks", hd["hooks"])
	if err != nil {
		return err
	}
	if err := validateImportedVibeHookNames(hookPath, records); err != nil {
		return err
	}
	if err := validateVibeMCPImport(root, cfg); err != nil {
		return err
	}
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills, src.MCPs, src.Hooks, src.Settings); err != nil {
		return err
	}
	if _, err := importCodexRules(root, importSourcePath(root, src.Rules), src, importCodexOpts{}); err != nil {
		return err
	}
	if rulesFile != "" {
		if _, err := importCodexRulesFile(root, importSourcePath(root, src.Rules), importCodexOpts{RulesFile: rulesFile}); err != nil {
			return err
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, agents))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", agents, err)
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".toml" {
			continue
		}
		path := filepath.Join(root, agents, e.Name())
		doc, err := readVibeTOML(path)
		if err != nil {
			return err
		}
		if err := guardVibeCredentials(path, doc); err != nil {
			return err
		}
		name := strings.TrimSuffix(e.Name(), ".toml")
		if vibe.IsReservedAgentName(name) {
			keptf("note: Vibe reserves the name main for hook matching and does not load %s; rename the agent before the next sync\n", filepath.ToSlash(filepath.Join(agents, e.Name())))
		}
		body, _ := doc["instructions"].(string)
		if body == "" {
			if _, ok := doc["system_prompt_id"]; ok {
				return fmt.Errorf("%s: import requires inline instructions; copy the referenced system prompt first", path)
			}
		}
		meta := map[string]any{"name": name}
		if d, ok := doc["description"]; ok {
			meta["description"] = d
			delete(doc, "description")
		}
		if m, ok := doc["active_model"]; ok {
			meta["model"] = m
			delete(doc, "active_model")
		}
		delete(doc, "instructions")
		if len(doc) > 0 {
			meta["x-vibe"] = doc
		}
		data, err := writeFrontmatter(meta, body)
		if err != nil {
			return fmt.Errorf("render %s: %w", path, err)
		}
		if err := importWriteSpecMarkdown(filepath.Join(importSourcePath(root, src.Agents), name+".md"), data, 0o644, allSpecFields); err != nil {
			return err
		}
	}
	if _, err := importSkillFoldersWith(root, filepath.Join(root, skills), importSourcePath(root, src.Skills), skillFolderImportOpts{Fields: &vibeSkillFields}); err != nil {
		return err
	}
	doc, err := readVibeTOML(filepath.Join(root, conf))
	if err != nil {
		return err
	}
	if mcp != conf {
		other, err := readVibeTOML(filepath.Join(root, mcp))
		if err != nil {
			return err
		}
		doc["mcp_servers"] = other["mcp_servers"]
	}
	servers, err := nativeTOMLRecords(filepath.Join(root, mcp), "mcp_servers", doc["mcp_servers"])
	if err != nil {
		return err
	}
	if err := importVibeMCPRecords(servers, importSourcePath(root, src.MCPs)); err != nil {
		return err
	}
	delete(doc, "mcp_servers")
	if len(doc) > 0 {
		if err := guardVibeCredentials(filepath.Join(root, conf), doc); err != nil {
			return err
		}
		meta := map[string]any{"x-vibe": doc}
		if m, ok := doc["active_model"]; ok {
			meta["model"] = m
			delete(doc, "active_model")
		}
		if err := writeSettingsSpec(importSourcePath(root, src.Settings), "vibe.yaml", meta); err != nil {
			return err
		}
	}

	for _, r := range records {
		name, _ := r["name"].(string)
		if name == "" {
			return fmt.Errorf("%s: hook requires name", hooks)
		}
		meta := map[string]any{"name": name, "target": "vibe", "event": r["type"], "command": strings.ReplaceAll(vibe.DeclaredHookCommand(fmt.Sprint(r["command"])), ".vibe/hooks/", agnosticScriptsDir+"/")}
		rest := maps.Clone(r)
		delete(rest, "name")
		delete(rest, "type")
		delete(rest, "command")
		if len(rest) > 0 {
			meta["x-vibe"] = rest
		}
		if err := writeHookSpecFile(importSourcePath(root, src.Hooks), name, meta); err != nil {
			return err
		}
	}
	if _, err := mirrorMainFile(root, mainFile); err != nil {
		return err
	}
	return captureHookScripts(root, "vibe")
}
func importVibeMCPRecords(records []map[string]any, dst string) error {
	if err := validateImportedVibeMCPNames("vibe MCP import", records); err != nil {
		return err
	}
	if err := importMkdirAll(dst, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	servers := map[string]any{}
	for _, r := range records {
		name, _ := r["name"].(string)
		if name == "" {
			return fmt.Errorf("vibe MCP import: missing server name")
		}
		rest := maps.Clone(r)
		delete(rest, "name")
		meta := map[string]any{}
		switch rest["transport"] {
		case "http", "streamable-http":
			meta["type"] = "http"
		case nil, "stdio":
			meta["type"] = "stdio"
		default:
			return fmt.Errorf("vibe MCP %s: unsupported transport %v", name, rest["transport"])
		}
		delete(rest, "transport")
		for _, key := range []string{"args", "env", "cwd", "url", "disabled"} {
			if v, ok := rest[key]; ok {
				meta[key] = v
				delete(rest, key)
			}
		}
		if cmd, ok := rest["command"].(string); ok {
			meta["command"] = cmd
			delete(rest, "command")
		} else if argv := stringSliceFromAny(jsonRoundTrip(rest["command"])); len(argv) > 0 {
			meta["command"] = argv[0]
			nativeArgs, ok := meta["args"]
			if !ok {
				nativeArgs = []any{}
			}
			portableArgs := append([]string(nil), argv[1:]...)
			portableArgs = append(portableArgs, stringSliceFromAny(jsonRoundTrip(nativeArgs))...)
			if len(portableArgs) > 0 {
				meta["args"] = portableArgs
			}
			rest["args"] = nativeArgs
		}
		if len(rest) > 0 {
			meta["x-vibe"] = rest
		}
		servers[name] = meta
	}
	normalized, _ := jsonRoundTrip(servers).(map[string]any)
	for name, raw := range normalized {
		server, _ := raw.(map[string]any)
		if err := guardVibeCredentials("vibe MCP "+name, server); err != nil {
			return err
		}
		adapters.EscapeMCPLiterals("vibe", server)
	}
	_, err := writeMCPSpecs(normalized, dst)
	return err
}
func nativeTOMLRecords(path, key string, value any) ([]map[string]any, error) {
	if value == nil {
		return nil, nil
	}
	switch records := value.(type) {
	case []map[string]any:
		return records, nil
	case []any:
		out := make([]map[string]any, 0, len(records))
		for _, r := range records {
			m, ok := r.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%s: %s must be an array of tables", path, key)
			}
			out = append(out, m)
		}
		return out, nil
	}
	return nil, fmt.Errorf("%s: %s must be an array of tables", path, key)
}

func guardVibeCredentials(path string, doc map[string]any) error {
	field := ""
	walkMCPStrings("", "", jsonRoundTrip(doc), func(p, key, value string) {
		if field != "" {
			return
		}
		named := mcpCredentialKey(key) && !mcpRefOnly(value) && mcpSecretValue(mcpDetectorText(value), false, mcpWeakCredential)
		if named || mcpCredentialDetected(value) {
			field = strings.TrimPrefix(p, ".")
		}
	})
	if field != "" {
		return fmt.Errorf("%s: %s holds a literal credential; keep it in user configuration or use Vibe's native environment-variable setting before importing", path, field)
	}
	return nil
}

func validateImportedVibeHookNames(path string, records []map[string]any) error {
	if _, err := namedTOMLRecords(path, records); err != nil {
		return err
	}
	for _, record := range records {
		name, _ := record["name"].(string)
		if err := spec.ValidateName(spec.KindHook, name); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	return nil
}

func validateImportedVibeMCPNames(path string, records []map[string]any) error {
	if _, err := adapters.UserMCPArrayIdentities("vibe", records); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	for _, record := range records {
		name, _ := record["name"].(string)
		if err := spec.ValidateName(spec.KindMCP, name); err != nil {
			return fmt.Errorf("%s: MCP %q: %w", path, name, err)
		}
	}
	return nil
}
func validateVibeMCPImport(root string, cfg *config.Config) error {
	conf, mcp := ".vibe/config.toml", ""
	if cfg != nil {
		if cfg.Outputs["vibe"].ConfFile != "" {
			conf = cfg.Outputs["vibe"].ConfFile
		}
		mcp = cfg.Outputs["vibe"].MCPFile
	}
	paths := []string{conf}
	if mcp != "" && mcp != conf {
		paths = append(paths, mcp)
	}
	for _, relative := range paths {
		path := filepath.Join(root, relative)
		doc, err := readVibeTOML(path)
		if err != nil {
			return err
		}
		records, err := nativeTOMLRecords(path, "mcp_servers", doc["mcp_servers"])
		if err != nil {
			return err
		}
		if err := validateImportedVibeMCPNames(path, records); err != nil {
			return err
		}
	}
	return nil
}
