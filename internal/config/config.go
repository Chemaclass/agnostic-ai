package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Version       int               `yaml:"version"`
	Sources       Sources           `yaml:"sources"`
	Targets       []string          `yaml:"targets"`
	Outputs       map[string]Output `yaml:"outputs"`
	OnUnsupported string            `yaml:"on-unsupported"`
}

type Sources struct {
	Agents string `yaml:"agents"`
	Skills string `yaml:"skills"`
	Rules  string `yaml:"rules"`
	Hooks  string `yaml:"hooks"`
	MCPs   string `yaml:"mcps"`
}

type Output struct {
	Dir       string `yaml:"dir,omitempty"`
	File      string `yaml:"file,omitempty"`
	RulesFile string `yaml:"rules-file,omitempty"`
	RulesDir  string `yaml:"rules-dir,omitempty"`
	MCPFile   string `yaml:"mcp-file,omitempty"`
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
		Targets: []string{
			"claude", "codex", "gemini", "cursor",
			"copilot", "aider", "cline", "windsurf", "continue",
		},
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
