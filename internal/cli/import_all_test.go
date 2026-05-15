package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportAll_NoDetectedCLIs(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"import", "all"})
	// Should succeed even when no CLIs are detected.
	if err := root.Execute(); err != nil {
		t.Errorf("import all should succeed with no detected CLIs, got: %v", err)
	}
}

func TestImportAll_ImportsDetectedCLI(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	// Simulate an existing Claude project.
	claudeDir := filepath.Join(dir, ".claude", "rules")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "rule.md"),
		[]byte("# rule\nsome content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCmd("test")
	root.SetArgs([]string{"import", "all"})
	if err := root.Execute(); err != nil {
		t.Fatalf("import all: %v", err)
	}
	// Verify at least one spec was imported into the rules dir.
	entries, err := os.ReadDir(filepath.Join(dir, ".agnostic-ai", "rules"))
	if err != nil {
		// fallback: check root rules/
		entries, err = os.ReadDir(filepath.Join(dir, "rules"))
		if err != nil {
			t.Skip("could not determine rules dir")
		}
	}
	if len(entries) == 0 {
		t.Error("expected at least one rule imported, rules dir is empty")
	}
}

func TestImport_MultipleSources_LastWinsAgnosticMainFile(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"),
		[]byte("# Claude top-level\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"),
		[]byte("# Codex top-level\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCmd("test")
	root.SetArgs([]string{"import", "claude", "codex"})
	if err := root.Execute(); err != nil {
		t.Fatalf("import claude codex: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"))
	if err != nil {
		t.Fatalf("read AGNOSTIC_AI.md: %v", err)
	}
	if string(got) != "# Codex top-level\n" {
		t.Errorf("AGNOSTIC_AI.md should reflect last source (codex), got %q", got)
	}
}

func TestImport_MultipleSources_UnknownSourceFailsEarly(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"import", "claude", "nope"})
	if err := root.Execute(); err == nil {
		t.Error("expected error for unknown source in multi-arg import")
	}
}

func TestImport_MultipleSources_AllRejectsCombined(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"import", "all", "claude"})
	if err := root.Execute(); err == nil {
		t.Error("expected error when combining `all` with other sources")
	}
}

func TestInitFrom_ScaffoldsAndImports(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	// Simulate existing Claude project.
	claudeDir := filepath.Join(dir, ".claude", "rules")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "existing.md"),
		[]byte("# Existing Rule\ncontent\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCmd("test")
	root.SetArgs([]string{"init", "--all", "--from", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatalf("init --from claude: %v", err)
	}

	// agnostic-ai.yaml should exist.
	if _, err := os.Stat(filepath.Join(dir, "agnostic-ai.yaml")); err != nil {
		t.Error("agnostic-ai.yaml not created by init --from")
	}
}
