package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestEmit_RootAgentsMd(t *testing.T) {
	dir := chdir(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "rule body"},
		{Kind: spec.KindAgent, Name: "ag1", Body: "agent body",
			Meta: map[string]any{"description": "agent desc"}},
		{Kind: spec.KindHook, Name: "h1", Meta: map[string]any{"event": "X"}},
	}
	if err := New().Emit(entries, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "AGENTS.md"))
	if !strings.Contains(got, "rule body") {
		t.Errorf("missing rule body in:\n%s", got)
	}
	if !strings.Contains(got, "agent body") {
		t.Errorf("missing agent body in:\n%s", got)
	}
	if !strings.Contains(got, "_agent desc_") {
		t.Errorf("missing agent description in:\n%s", got)
	}
}

func TestEmit_NestedByGlobs(t *testing.T) {
	dir := chdir(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "root-rule",
			Meta: map[string]any{"globs": "**/*"}, Body: "root content"},
		{Kind: spec.KindRule, Name: "src-rule",
			Meta: map[string]any{"globs": "src/**"}, Body: "src content"},
		{Kind: spec.KindRule, Name: "tests-rule",
			Meta: map[string]any{"globs": "tests/**/*.go"}, Body: "tests content"},
		{Kind: spec.KindRule, Name: "deep-rule",
			Meta: map[string]any{"globs": "docs/api/**"}, Body: "api content"},
	}
	if err := New().Emit(entries, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		path, mustContain string
	}{
		{"AGENTS.md", "root content"},
		{"src/AGENTS.md", "src content"},
		{"tests/AGENTS.md", "tests content"},
		{"docs/api/AGENTS.md", "api content"},
	}
	for _, c := range cases {
		got := readFile(t, filepath.Join(dir, c.path))
		if !strings.Contains(got, c.mustContain) {
			t.Errorf("%s missing %q:\n%s", c.path, c.mustContain, got)
		}
	}
}

func TestEmit_SkillsListedInRoot(t *testing.T) {
	dir := chdir(t)

	entries := []spec.Entry{
		{Kind: spec.KindSkill, Name: "yaml-validator",
			Path: "skills/yaml-validator.md",
			Meta: map[string]any{"description": "Validate YAML."}},
	}
	if err := New().Emit(entries, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "AGENTS.md"))
	if !strings.Contains(got, "## Skills") {
		t.Errorf("missing Skills section:\n%s", got)
	}
	if !strings.Contains(got, "yaml-validator") {
		t.Errorf("missing skill name:\n%s", got)
	}
	if !strings.Contains(got, "skills/yaml-validator.md") {
		t.Errorf("missing skill source path:\n%s", got)
	}
}

func TestEmit_AgentsAndSkillsAttachToRootOnly(t *testing.T) {
	dir := chdir(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "src-rule",
			Meta: map[string]any{"globs": "src/**"}, Body: "src content"},
		{Kind: spec.KindAgent, Name: "ag1", Body: "agent body"},
	}
	if err := New().Emit(entries, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	root := readFile(t, filepath.Join(dir, "AGENTS.md"))
	if !strings.Contains(root, "agent body") {
		t.Errorf("agents must be in root: %s", root)
	}
	srcDoc := readFile(t, filepath.Join(dir, "src", "AGENTS.md"))
	if strings.Contains(srcDoc, "agent body") {
		t.Errorf("agents must not be in nested doc: %s", srcDoc)
	}
}

func TestEmit_OutputOverride(t *testing.T) {
	dir := chdir(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"codex": {File: "vendor/AGENTS.md"}},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "x", Meta: map[string]any{"globs": "src/**"}},
	}
	if err := New().Emit(entries, cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor/src/AGENTS.md")); err != nil {
		t.Errorf("expected nested override path: %v", err)
	}
}

func TestRouteDir(t *testing.T) {
	cases := []struct {
		globs, want string
	}{
		{"", ""},
		{"**/*", ""},
		{"*", ""},
		{"src/**", "src"},
		{"src/**/*.go", "src"},
		{"docs/api/**", "docs/api"},
		{"**/*.go", ""},
		{"/src/**", "src"},
	}
	for _, c := range cases {
		entry := spec.Entry{Meta: map[string]any{"globs": c.globs}}
		if got := routeDir(entry); got != c.want {
			t.Errorf("routeDir(%q) = %q, want %q", c.globs, got, c.want)
		}
	}
}

func TestAdapterName(t *testing.T) {
	if New().Name() != "codex" {
		t.Errorf("expected codex, got %s", New().Name())
	}
}

// helpers

func chdir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
