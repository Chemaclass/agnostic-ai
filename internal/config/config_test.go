package config

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ConfigFileName),
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
	if err := os.WriteFile(filepath.Join(dir, ConfigFileName),
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
	want := []string{
		"claude", "codex", "gemini", "cursor",
		"copilot", "aider", "cline", "windsurf", "continue",
		"amp", "zed", "warp", "opencode",
	}
	targets := DefaultTargets()
	if len(targets) != len(want) {
		t.Fatalf("expected %d targets, got %d: %v", len(want), len(targets), targets)
	}
	wantSet := make(map[string]bool, len(want))
	for _, w := range want {
		wantSet[w] = true
	}
	for _, tgt := range targets {
		if !wantSet[tgt] {
			t.Errorf("unexpected target %q in DefaultTargets()", tgt)
		}
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
	if err := os.WriteFile(filepath.Join(dir, ConfigFileName),
		[]byte("targets: [unclosed\n  - bad\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error on malformed yaml")
	}
}

func TestLoad_LegacyFilenameFallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, LegacyConfigFileName),
		[]byte("version: 1\ntargets:\n  - claude\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, sources, err := LoadWithSources(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Targets) != 1 || cfg.Targets[0] != "claude" {
		t.Errorf("expected claude only, got %v", cfg.Targets)
	}
	if len(sources) != 1 || filepath.Base(sources[0]) != LegacyConfigFileName {
		t.Errorf("expected legacy filename in sources, got %v", sources)
	}
}

func TestLoad_NewFilenamePreferredOverLegacy(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, LegacyConfigFileName),
		[]byte("version: 1\ntargets:\n  - codex\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ConfigFileName),
		[]byte("version: 1\ntargets:\n  - claude\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, sources, err := LoadWithSources(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Targets) != 1 || cfg.Targets[0] != "claude" {
		t.Errorf("expected new filename to win, got %v", cfg.Targets)
	}
	if filepath.Base(sources[0]) != ConfigFileName {
		t.Errorf("expected new filename in sources, got %v", sources)
	}
}

func TestLoad_LocalOverrideDeepMerge(t *testing.T) {
	dir := t.TempDir()
	base := `
version: 1
sources:
  agents: prompts/agents
targets:
  - claude
  - cursor
outputs:
  claude:
    dir: vendor/.claude
    rules-file: vendor/CLAUDE.md
on-unsupported: warn
`
	local := `
on-unsupported: error
outputs:
  claude:
    dir: .claude-local
  codex:
    dir: .codex-local
`
	if err := os.WriteFile(filepath.Join(dir, ConfigFileName),
		[]byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, LocalOverrideFileName),
		[]byte(local), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, sources, err := LoadWithSources(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 {
		t.Fatalf("expected base + local in sources, got %v", sources)
	}
	if cfg.OnUnsupported != "error" {
		t.Errorf("scalar should be replaced by local, got %q", cfg.OnUnsupported)
	}
	if cfg.Outputs["claude"].Dir != ".claude-local" {
		t.Errorf("nested map should merge: claude.dir = %q", cfg.Outputs["claude"].Dir)
	}
	if cfg.Outputs["claude"].RulesFile != "vendor/CLAUDE.md" {
		t.Errorf("base field should survive merge: claude.rules-file = %q",
			cfg.Outputs["claude"].RulesFile)
	}
	if cfg.Outputs["codex"].Dir != ".codex-local" {
		t.Errorf("local-only output should be added: %+v", cfg.Outputs["codex"])
	}
}

func TestLoad_LocalOverrideReplacesSlice(t *testing.T) {
	dir := t.TempDir()
	base := `
version: 1
targets:
  - claude
  - cursor
  - codex
`
	local := `
targets:
  - claude
`
	if err := os.WriteFile(filepath.Join(dir, ConfigFileName),
		[]byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, LocalOverrideFileName),
		[]byte(local), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.Targets, []string{"claude"}) {
		t.Errorf("slice should be replaced wholesale, got %v", cfg.Targets)
	}
}

func TestLoad_LocalOverrideAloneIsNotEnough(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, LocalOverrideFileName),
		[]byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("local-only should error: base file is required")
	}
}

func TestDeepMerge_NestedMaps(t *testing.T) {
	dst := map[string]any{
		"a": map[string]any{"x": 1, "y": 2},
		"b": "keep",
	}
	src := map[string]any{
		"a": map[string]any{"y": 20, "z": 30},
		"c": "new",
	}
	deepMerge(dst, src)

	a := dst["a"].(map[string]any)
	gotKeys := make([]string, 0, len(a))
	for k := range a {
		gotKeys = append(gotKeys, k)
	}
	sort.Strings(gotKeys)
	wantKeys := []string{"x", "y", "z"}
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Errorf("merged map keys = %v, want %v", gotKeys, wantKeys)
	}
	if a["x"] != 1 || a["y"] != 20 || a["z"] != 30 {
		t.Errorf("merged values wrong: %+v", a)
	}
	if dst["b"] != "keep" {
		t.Errorf("unrelated key should survive: %v", dst["b"])
	}
	if dst["c"] != "new" {
		t.Errorf("new top-level key should be added: %v", dst["c"])
	}
}
