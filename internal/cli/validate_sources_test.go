package cli

import (
	"path/filepath"
	"testing"
)

// A declared source whose directory is missing is reported; a declared
// source that exists, and an undeclared (defaulted) kind, are not.
func TestLintMissingSources_FlagsDeclaredMissingDirs(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"),
		"version: 1\nsources:\n  rules: .agnostic-ai/rules\n  skills: .agnostic-ai/skills\n")
	// Only rules/ exists on disk.
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "r.md"), "body\n")

	got := lintMissingSources(dir)
	if len(got) != 1 {
		t.Fatalf("expected 1 issue (skills missing), got %d: %+v", len(got), got)
	}
	if got[0].Field != "sources.skills" {
		t.Errorf("Field = %q, want sources.skills", got[0].Field)
	}
	if got[0].Path != ".agnostic-ai/skills" {
		t.Errorf("Path = %q, want .agnostic-ai/skills", got[0].Path)
	}
}

// When no sources are declared, defaults apply and nothing is flagged
// even though the convention dirs do not exist.
func TestLintMissingSources_IgnoresUndeclaredDefaults(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets:\n  - claude\n")

	if got := lintMissingSources(dir); len(got) != 0 {
		t.Errorf("expected no issues for undeclared sources, got %+v", got)
	}
}

// A config with no file at all yields no issues (handled elsewhere).
func TestLintMissingSources_NoConfig(t *testing.T) {
	if got := lintMissingSources(t.TempDir()); len(got) != 0 {
		t.Errorf("expected no issues without a config file, got %+v", got)
	}
}
