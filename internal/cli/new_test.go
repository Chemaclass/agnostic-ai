package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func setupEmptyProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := `version: 1
sources:
  agents: .agnostic-ai/agents
  skills: .agnostic-ai/skills
  rules: .agnostic-ai/rules
  hooks: .agnostic-ai/hooks
  mcps: .agnostic-ai/mcps
targets:
  - claude
  - cursor
`
	if err := os.WriteFile(filepath.Join(dir, "agnostic.config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestNew_WritesRule(t *testing.T) {
	dir := setupEmptyProject(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"new", "rule", "no-console-log"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, ".agnostic-ai", "rules", "no-console-log.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected rule file at %s: %v", path, err)
	}
	if !strings.Contains(string(body), "name: no-console-log") {
		t.Errorf("rule body missing frontmatter name field:\n%s", body)
	}
	if !strings.Contains(string(body), "alwaysApply: true") {
		t.Errorf("rule body missing alwaysApply: %s", body)
	}
}

func TestNew_HookEmitsYAML(t *testing.T) {
	dir := setupEmptyProject(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"new", "hook", "fmt-on-save"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".agnostic-ai", "hooks", "fmt-on-save.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected hook YAML at %s", path)
	}
}

func TestNew_ErrorsIfFileExists(t *testing.T) {
	dir := setupEmptyProject(t)
	testutil.Chdir(t, dir)
	silence(t)

	first := NewRootCmd("test")
	first.SetArgs([]string{"new", "rule", "dup"})
	if err := first.Execute(); err != nil {
		t.Fatal(err)
	}
	second := NewRootCmd("test")
	second.SetArgs([]string{"new", "rule", "dup"})
	err := second.Execute()
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected already-exists error, got %v", err)
	}
}

func TestNew_RejectsUnknownKind(t *testing.T) {
	dir := setupEmptyProject(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"new", "wat", "x"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown kind") {
		t.Fatalf("expected unknown-kind error, got %v", err)
	}
}

func TestNew_RejectsBadName(t *testing.T) {
	dir := setupEmptyProject(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"new", "rule", "Bad Name!"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "invalid name") {
		t.Fatalf("expected invalid-name error, got %v", err)
	}
}

func TestNew_HonorsConfiguredSources(t *testing.T) {
	dir := t.TempDir()
	cfg := `version: 1
sources:
  agents: specs/agents
  skills: specs/skills
  rules: specs/rules
  hooks: specs/hooks
  mcps: specs/mcps
targets: [claude]
`
	if err := os.WriteFile(filepath.Join(dir, "agnostic.config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"new", "rule", "x"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "specs", "rules", "x.md")); err != nil {
		t.Errorf("expected file under custom sources: %v", err)
	}
}
