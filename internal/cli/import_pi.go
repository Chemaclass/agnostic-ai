package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func importFromPi(root string, cfg *config.Config) error {
	src := cfg.Sources
	native := map[string]string{}
	for _, artifact := range adapters.NativeArtifactsFor("pi", cfg) {
		native[artifact.Label] = config.ResolveSourcePath(root, strings.TrimSuffix(artifact.Location, "/"))
	}
	if err := mkdirAllSources(root, src.Rules, src.Skills, src.Commands, src.MCPs, src.Settings); err != nil {
		return err
	}
	rules, err := importCodexRules(root, importSourcePath(root, src.Rules), src, importCodexOpts{})
	if err != nil {
		return err
	}
	skills, err := importSkillFoldersWith(root, native["Skills"], importSourcePath(root, src.Skills), skillFolderImportOpts{Fields: &piSkillFields})
	if err != nil {
		return err
	}
	commands, err := importFlatMarkdownFiles(native["Commands"], importSourcePath(root, src.Commands), factoryCommandFields)
	if err != nil {
		return err
	}
	servers, err := readJSONMapAt(native["MCP servers"], "mcpServers")
	if err != nil {
		return err
	}
	for _, raw := range servers {
		if server, ok := raw.(map[string]any); ok {
			normalizePiMCPEntry(server)
		}
	}
	mcps, err := writeMCPYAMLs("pi", servers, importSourcePath(root, src.MCPs))
	if err != nil {
		return err
	}
	settings, err := importPiSettings(native["Settings"], importSourcePath(root, src.Settings))
	if err != nil {
		return err
	}
	if _, err := mirrorMainFile(root, "AGENTS.md"); err != nil {
		return err
	}
	summaryf("imported %d rules, %d skills, %d commands, %d mcps, %d settings (from pi)\n", rules, skills, commands, mcps, settings)
	printImportNextSteps(root, "pi")
	return nil
}

var piSkillFields = fieldsOf("name", "description", "license", "compatibility", "metadata", "allowed-tools", "disable-model-invocation")

func normalizePiMCPEntry(server map[string]any) {
	if enabled, ok := server["enabled"].(bool); ok {
		server["disabled"] = !enabled
	}
	delete(server, "enabled")
	native := map[string]any{}
	for _, key := range []string{"exposure", "toolExposure", "oauth"} {
		if value, ok := server[key]; ok {
			native[key] = value
			delete(server, key)
		}
	}
	if server["type"] == "streamable-http" {
		native["type"] = "streamable-http"
		server["type"] = "http"
	}
	if _, command := server["command"]; !command {
		if _, remote := server["url"]; !remote {
			if disabled, ok := server["disabled"].(bool); ok {
				native["enabled"] = !disabled
				delete(server, "disabled")
			}
		}
	}
	if len(native) > 0 {
		server["x-pi"] = native
	}
	if _, command := server["command"]; !command {
		if _, remote := server["url"]; !remote {
			server["targets"] = []any{"pi"}
		}
	}
}

func importPiSettings(path, dst string) (int, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}
	var native map[string]any
	if err := json.Unmarshal(data, &native); err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}
	doc := map[string]any{}
	if model, ok := native["defaultModel"].(string); ok && model != "" {
		doc["model"] = model
	}
	plan, level, err := planSettingsEffort("pi", native["defaultThinkingLevel"], dst, "pi.yaml")
	if err != nil {
		return 0, err
	}
	maps.Copy(doc, settingsEffortSpec(plan, "pi", "defaultThinkingLevel", level))
	if provider, ok := native["defaultProvider"]; ok {
		custom, _ := doc["x-pi"].(map[string]any)
		if custom == nil {
			custom = map[string]any{}
		}
		custom["defaultProvider"] = provider
		doc["x-pi"] = custom
	}
	if len(doc) == 0 {
		return 0, nil
	}
	if err := writeSettingsSpec(dst, "pi.yaml", doc); err != nil {
		return 0, err
	}
	return 1, nil
}
