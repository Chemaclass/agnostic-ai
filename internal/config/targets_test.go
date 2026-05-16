package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func writeConfig(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readConfig(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "agnostic-ai.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestPersistTargets_ReplacesExistingBlock(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "version: 1\n\ntargets:\n  - claude\n  - codex\n  - gemini\n\non-unsupported: warn\n")

	if err := config.PersistTargets(dir, []string{"claude", "cursor"}); err != nil {
		t.Fatal(err)
	}

	got := readConfig(t, dir)
	want := "version: 1\n\ntargets:\n  - claude\n  - cursor\n\non-unsupported: warn\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestPersistTargets_AppendsWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "version: 1\n")

	if err := config.PersistTargets(dir, []string{"claude", "codex"}); err != nil {
		t.Fatal(err)
	}

	got := readConfig(t, dir)
	if !strings.Contains(got, "targets:\n  - claude\n  - codex\n") {
		t.Errorf("expected appended targets block, got:\n%s", got)
	}
}

func TestPersistTargets_PreservesOtherKeysAndComments(t *testing.T) {
	dir := t.TempDir()
	initial := `# yaml-language-server: $schema=https://example.com/schema.json
version: 1

sources:
  agents: agents

# AI CLIs to emit
targets:
  - claude
  - codex
  - gemini
  - cursor

on-unsupported: warn
`
	writeConfig(t, dir, initial)

	if err := config.PersistTargets(dir, []string{"claude"}); err != nil {
		t.Fatal(err)
	}

	got := readConfig(t, dir)
	for _, want := range []string{
		"# yaml-language-server",
		"sources:",
		"  agents: agents",
		"# AI CLIs to emit",
		"on-unsupported: warn",
		"targets:\n  - claude\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "- codex") {
		t.Errorf("stale target survived:\n%s", got)
	}
}

func TestPersistTargets_HandlesEmptyList(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "version: 1\ntargets:\n  - claude\n  - codex\n")

	if err := config.PersistTargets(dir, []string{}); err != nil {
		t.Fatal(err)
	}

	got := readConfig(t, dir)
	if strings.Contains(got, "- claude") || strings.Contains(got, "- codex") {
		t.Errorf("expected empty targets, got:\n%s", got)
	}
}

func TestPersistTargets_MissingFile(t *testing.T) {
	dir := t.TempDir()
	err := config.PersistTargets(dir, []string{"claude"})
	if err == nil {
		t.Error("expected error when config file missing")
	}
}
