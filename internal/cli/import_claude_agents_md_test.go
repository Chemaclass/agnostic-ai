package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
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

// A CLAUDE.md that imports AGENTS.md is how Claude Code reads that file.
// Its own text belongs to Claude alone, so it lands in a claude fence
// instead of becoming rules every target loads (#1336).
func TestImportFromClaude_CompanionOfAgentsMDFencesClaudeText(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	buf := captureSummary(t)
	mustWrite(t, filepath.Join(dir, "AGENTS.md"), "# Project\n\nRoot guidance.\n")
	mustWrite(t, filepath.Join(dir, "CLAUDE.md"), "# CLAUDE.md\n\n@AGENTS.md\n\n## Claude Code specifics\n\n- Use the Dashboard launch config.\n")

	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatalf("import: %v", err)
	}

	want := "# Project\n\nRoot guidance.\n\n::target claude\n## Claude Code specifics\n\n- Use the Dashboard launch config.\n::end\n"
	if got := mustRead(t, filepath.Join(dir, agnosticMainFile)); got != want {
		t.Errorf("AGNOSTIC_AI.md = %q, want %q", got, want)
	}
	if entries, _ := os.ReadDir(filepath.Join(dir, rootSources().Rules)); len(entries) != 0 {
		t.Errorf("no rule should come from the companion, got %d files", len(entries))
	}
	if strings.Contains(buf.String(), "unique content") {
		t.Errorf("AGENTS.md is captured, got a warning:\n%s", buf.String())
	}
}

// Importing AGENTS.md alone captures everything a companion CLAUDE.md
// says, so there is nothing to warn about.
func TestImportFromCodex_NoWarnForClaudeCompanion(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	buf := captureSummary(t)
	mustWrite(t, filepath.Join(dir, "AGENTS.md"), "# Project\n\nRoot guidance.\n")
	mustWrite(t, filepath.Join(dir, "CLAUDE.md"), "@AGENTS.md\n")

	if _, err := mirrorMainFile(dir, "AGENTS.md"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "unique content") {
		t.Errorf("CLAUDE.md only imports AGENTS.md, got a warning:\n%s", buf.String())
	}
}
