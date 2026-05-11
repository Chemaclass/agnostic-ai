package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
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
	Dir             string `yaml:"dir,omitempty"              json:"dir,omitempty"`
	File            string `yaml:"file,omitempty"             json:"file,omitempty"`
	RulesFile       string `yaml:"rules-file,omitempty"       json:"rules-file,omitempty"`
	RulesDir        string `yaml:"rules-dir,omitempty"        json:"rules-dir,omitempty"`
	MCPFile         string `yaml:"mcp-file,omitempty"         json:"mcp-file,omitempty"`
	AgentsDir       string `yaml:"agents-dir,omitempty"       json:"agents-dir,omitempty"`
	SkillsDir       string `yaml:"skills-dir,omitempty"       json:"skills-dir,omitempty"`
	InstructionsDir string `yaml:"instructions-dir,omitempty" json:"instructions-dir,omitempty"`
}

func Load(root string) (*Config, error) {
	path := filepath.Join(root, "agnostic.config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	cfg := defaults()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.applyDefaults()
	return cfg, nil
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
