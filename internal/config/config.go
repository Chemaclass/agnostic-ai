package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ConfigFileName is the canonical config file. Load also accepts the
// legacy LegacyConfigFileName but emits a deprecation warning.
const (
	ConfigFileName        = "agnostic-ai.yaml"
	LegacyConfigFileName  = "agnostic.config.yaml"
	LocalOverrideFileName = "agnostic-ai.local.yaml"
)

type Config struct {
	Version       int               `yaml:"version"        json:"version"`
	Sources       Sources           `yaml:"sources"        json:"sources"`
	Targets       []string          `yaml:"targets"        json:"targets"`
	Outputs       map[string]Output `yaml:"outputs"        json:"outputs"`
	OnUnsupported string            `yaml:"on-unsupported" json:"on-unsupported"`
	Gitignore     Gitignore         `yaml:"gitignore"      json:"gitignore,omitempty"`
	AutoSync      *bool             `yaml:"autoSync,omitempty"  json:"autoSync,omitempty"`
}

// Gitignore controls automatic management of .gitignore entries for the
// generated target files. When Enabled, every `sync` run rewrites a
// managed block in `.gitignore` (created if missing) listing every path
// the configured adapters would emit.
type Gitignore struct {
	Enabled bool `yaml:"enabled"        json:"enabled"`
	// Path overrides the .gitignore location relative to the project root.
	Path string `yaml:"path,omitempty" json:"path,omitempty"`
}

type Sources struct {
	Agents string `yaml:"agents" json:"agents"`
	Skills string `yaml:"skills" json:"skills"`
	Rules  string `yaml:"rules"  json:"rules"`
	Hooks  string `yaml:"hooks"  json:"hooks"`
	MCPs   string `yaml:"mcps"   json:"mcps"`
}

type Output struct {
	Dir                  string `yaml:"dir,omitempty"                      json:"dir,omitempty"`
	File                 string `yaml:"file,omitempty"                     json:"file,omitempty"`
	RulesFile            string `yaml:"rules-file,omitempty"               json:"rules-file,omitempty"`
	RulesDir             string `yaml:"rules-dir,omitempty"                json:"rules-dir,omitempty"`
	MCPFile              string `yaml:"mcp-file,omitempty"                 json:"mcp-file,omitempty"`
	AgentsDir            string `yaml:"agents-dir,omitempty"               json:"agents-dir,omitempty"`
	SkillsDir            string `yaml:"skills-dir,omitempty"               json:"skills-dir,omitempty"`
	InstructionsDir      string `yaml:"instructions-dir,omitempty"         json:"instructions-dir,omitempty"`
	CommandsDir          string `yaml:"commands-dir,omitempty"             json:"commands-dir,omitempty"`
	ChatmodesDir         string `yaml:"chatmodes-dir,omitempty"            json:"chatmodes-dir,omitempty"`
	WorkflowsDir         string `yaml:"workflows-dir,omitempty"            json:"workflows-dir,omitempty"`
	AssistantsDir        string `yaml:"assistants-dir,omitempty"           json:"assistants-dir,omitempty"`
	TasksFile            string `yaml:"tasks-file,omitempty"               json:"tasks-file,omitempty"`
	ConfFile             string `yaml:"conf-file,omitempty"                json:"conf-file,omitempty"`
	Model                string `yaml:"model,omitempty"                    json:"model,omitempty"`
	WeakModel            string `yaml:"weak-model,omitempty"               json:"weak-model,omitempty"`
	MCPDir               string `yaml:"mcp-dir,omitempty"                  json:"mcp-dir,omitempty"`
	EmitSkillsAsCommands bool   `yaml:"emit-skills-as-commands,omitempty"  json:"emit-skills-as-commands,omitempty"`
}

// Load reads the project config from root. It prefers
// agnostic-ai.yaml and falls back to the legacy
// agnostic.config.yaml (with a stderr deprecation warning). When
// agnostic-ai.local.yaml exists in the same directory, it is
// deep-merged over the base: scalars and slices in the local file
// replace the base, maps merge recursively.
func Load(root string) (*Config, error) {
	cfg, _, err := LoadWithSources(root)
	return cfg, err
}

// LoadWithSources is Load that also reports the files it parsed, in
// load order (base first, local last). Callers that want to surface
// which files were merged (e.g. `sync` summary) use this variant.
func LoadWithSources(root string) (*Config, []string, error) {
	basePath, legacy, err := ResolveConfigPath(root)
	if err != nil {
		return nil, nil, err
	}
	if legacy {
		fmt.Fprintf(os.Stderr,
			"! %s is deprecated. Rename to %s.\n",
			LegacyConfigFileName, ConfigFileName)
	}

	merged, err := readYAMLMap(basePath)
	if err != nil {
		return nil, nil, err
	}
	sources := []string{basePath}

	localPath := filepath.Join(root, LocalOverrideFileName)
	if _, statErr := os.Stat(localPath); statErr == nil {
		localMap, err := readYAMLMap(localPath)
		if err != nil {
			return nil, nil, err
		}
		deepMerge(merged, localMap)
		sources = append(sources, localPath)
	}

	data, err := yaml.Marshal(merged)
	if err != nil {
		return nil, nil, fmt.Errorf("re-encode merged config: %w", err)
	}
	cfg := defaults()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", basePath, err)
	}
	cfg.applyDefaults()
	return cfg, sources, nil
}

// ResolveConfigPath returns the path of the base config file in root.
// It prefers ConfigFileName and falls back to LegacyConfigFileName;
// legacy is true when the fallback won.
func ResolveConfigPath(root string) (path string, legacy bool, err error) {
	primary := filepath.Join(root, ConfigFileName)
	if _, statErr := os.Stat(primary); statErr == nil {
		return primary, false, nil
	}
	legacyPath := filepath.Join(root, LegacyConfigFileName)
	if _, statErr := os.Stat(legacyPath); statErr == nil {
		return legacyPath, true, nil
	}
	return "", false, fmt.Errorf("read config: no %s or %s in %s",
		ConfigFileName, LegacyConfigFileName, root)
}

func readYAMLMap(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	out := map[string]any{}
	if len(data) == 0 {
		return out, nil
	}
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return out, nil
}

// deepMerge folds src into dst. Maps merge recursively; scalars and
// slices in src replace dst entirely. Slice concat is intentionally
// not supported: targets and similar lists should be authoritative
// per-layer so override semantics stay obvious.
func deepMerge(dst, src map[string]any) {
	for k, sv := range src {
		if dv, ok := dst[k]; ok {
			dstMap, dstIsMap := dv.(map[string]any)
			srcMap, srcIsMap := sv.(map[string]any)
			if dstIsMap && srcIsMap {
				deepMerge(dstMap, srcMap)
				continue
			}
		}
		dst[k] = sv
	}
}

func DefaultTargets() []string {
	return []string{
		"claude", "codex", "gemini", "cursor",
		"copilot", "aider", "cline", "windsurf", "continue",
		"amp", "zed", "warp", "opencode",
	}
}

func defaults() *Config {
	return &Config{
		Version: 1,
		Sources: Sources{
			Agents: "agents",
			Skills: "skills",
			Rules:  "rules",
			Hooks:  "hooks",
			MCPs:   "mcps",
		},
		Targets:       DefaultTargets(),
		OnUnsupported: "warn",
	}
}

func (c *Config) applyDefaults() {
	if c.Sources.Agents == "" {
		c.Sources.Agents = "agents"
	}
	if c.Sources.Skills == "" {
		c.Sources.Skills = "skills"
	}
	if c.Sources.Rules == "" {
		c.Sources.Rules = "rules"
	}
	if c.Sources.Hooks == "" {
		c.Sources.Hooks = "hooks"
	}
	if c.Sources.MCPs == "" {
		c.Sources.MCPs = "mcps"
	}
	if c.OnUnsupported == "" {
		c.OnUnsupported = "warn"
	}
}
