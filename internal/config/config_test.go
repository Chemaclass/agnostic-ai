package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agnostic.config.yaml"),
		[]byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != 1 {
		t.Errorf("expected version 1, got %d", cfg.Version)
	}
	if cfg.Sources.Agents != "agents" {
		t.Errorf("expected default agents source, got %q", cfg.Sources.Agents)
	}
	if cfg.OnUnsupported != "warn" {
		t.Errorf("expected default warn, got %q", cfg.OnUnsupported)
	}
	if len(cfg.Targets) == 0 {
		t.Error("expected default targets to be populated")
	}
}

func TestLoad_Overrides(t *testing.T) {
	dir := t.TempDir()
	yaml := `
version: 1
sources:
  agents: prompts/agents
  rules: prompts/rules
targets:
  - claude
  - cursor
outputs:
  claude:
    dir: vendor/.claude
    rules-file: vendor/CLAUDE.md
on-unsupported: error
`
	if err := os.WriteFile(filepath.Join(dir, "agnostic.config.yaml"),
		[]byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Sources.Agents != "prompts/agents" {
		t.Errorf("agents override not applied: %q", cfg.Sources.Agents)
	}
	if len(cfg.Targets) != 2 {
		t.Errorf("expected 2 targets, got %d", len(cfg.Targets))
	}
	out := cfg.Outputs["claude"]
	if out.Dir != "vendor/.claude" || out.RulesFile != "vendor/CLAUDE.md" {
		t.Errorf("output overrides not applied: %+v", out)
	}
	if cfg.OnUnsupported != "error" {
		t.Errorf("expected error, got %q", cfg.OnUnsupported)
	}
	// Defaults still fill missing source
	if cfg.Sources.Skills != "skills" {
		t.Errorf("expected default skills, got %q", cfg.Sources.Skills)
	}
}

func TestDefaultTargets(t *testing.T) {
	targets := DefaultTargets()
	if len(targets) == 0 {
		t.Fatal("DefaultTargets must not be empty")
	}
	want := map[string]bool{
		"claude": true, "codex": true, "cursor": true, "gemini": true,
	}
	for _, tgt := range targets {
		delete(want, tgt)
	}
	if len(want) > 0 {
		t.Errorf("missing expected targets: %v", want)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error when config file is missing")
	}
}

func TestLoad_MalformedYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agnostic.config.yaml"),
		[]byte("targets: [unclosed\n  - bad\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error on malformed yaml")
	}
}
