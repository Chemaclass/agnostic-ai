package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Claude Code reads "every AGENTS.md and .claude/AGENTS.md in your
// working directory and the directories above it" when no CLAUDE.md is
// present. `import claude` must capture the file the session actually
// loads, not fall through to the generic pointer template.
func TestImportFromClaude_MirrorsRootAgentsMDWhenNoCLAUDEmd(t *testing.T) {
	dir := t.TempDir()
	silence(t)
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"),
		[]byte("# House rules\n\nRun make preflight.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatalf("import: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, agnosticMainFile))
	if err != nil {
		t.Fatalf("AGNOSTIC_AI.md should be seeded from AGENTS.md: %v", err)
	}
	if !strings.Contains(string(got), "Run make preflight.") {
		t.Errorf("expected the AGENTS.md body, got:\n%s", got)
	}
}

// The nested .claude/AGENTS.md is the fourth rung of the same chain.
func TestImportFromClaude_MirrorsNestedAgentsMDWhenNoOtherInstructions(t *testing.T) {
	dir := t.TempDir()
	silence(t)
	if err := os.MkdirAll(filepath.Join(dir, claudeDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, claudeDir, "AGENTS.md"),
		[]byte("# Nested\n\nNested body.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatalf("import: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, agnosticMainFile))
	if err != nil {
		t.Fatalf("AGNOSTIC_AI.md should be seeded from .claude/AGENTS.md: %v", err)
	}
	if !strings.Contains(string(got), "Nested body.") {
		t.Errorf("expected the .claude/AGENTS.md body, got:\n%s", got)
	}
}

// CLAUDE.md still outranks AGENTS.md: the vendor reads AGENTS.md only
// when no CLAUDE.md exists.
func TestImportFromClaude_CLAUDEmdOutranksAgentsMD(t *testing.T) {
	dir := t.TempDir()
	silence(t)
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"),
		[]byte("# Claude\n\nClaude body.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"),
		[]byte("# Agents\n\nAgents body.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatalf("import: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, agnosticMainFile))
	if err != nil {
		t.Fatalf("read AGNOSTIC_AI.md: %v", err)
	}
	if !strings.Contains(string(got), "Claude body.") {
		t.Errorf("CLAUDE.md should win, got:\n%s", got)
	}
}

// When `import codex` runs in the same invocation it owns the root
// AGENTS.md: it mirrors that file and shreds it into rule specs.
// `import claude` must not claim it too.
func TestImportFromClaude_SkipsRootAgentsMDWhenAPeerOwnsIt(t *testing.T) {
	dir := t.TempDir()
	silence(t)
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"),
		[]byte("# House rules\n\nRun make preflight.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	setImportRunSources([]string{"claude", "codex"})
	t.Cleanup(func() { setImportRunSources(nil) })

	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatalf("import: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, agnosticMainFile)); err == nil {
		t.Error("claude must leave the root AGENTS.md to codex when both run")
	}
}
