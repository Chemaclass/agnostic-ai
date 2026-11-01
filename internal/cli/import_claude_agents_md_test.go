package cli

import (
	"bytes"
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
	if !strings.Contains(buf.String(), "seeded from AGENTS.md and CLAUDE.md") {
		t.Errorf("expected the seed line to name both files, got:\n%s", buf.String())
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

const companionSyncWarning = "differs from what sync renders"

func syncFencedClaudeBlock(t *testing.T, source string) (string, *bytes.Buffer) {
	t.Helper()
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	buf := captureSummary(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex]\n")
	mustWrite(t, filepath.Join(dir, agnosticMainFile), source)
	if err := runSync(t); err != nil {
		t.Fatalf("sync: %v", err)
	}
	buf.Reset()
	return dir, buf
}

// Right after a clean sync, CLAUDE.md is the @AGENTS.md companion with the
// claude-only text, so importing it has nothing to say (#2935).
func TestImportFromClaude_NoWarnRightAfterSyncWithFencedClaudeBlock(t *testing.T) {
	cases := map[string]string{
		"blank lines inside the fence": "Root text.\n\n::target claude\n\n## Claude specifics\n\n- Claude only line.\n\n::end\n",
		"no blank line before fence":   "Root text.\n::target claude\n\n## Claude specifics\n\n- Claude only line.\n\n::end\n",
		"one-line fence":               "Root text.\n\n::target claude\n- Claude only line.\n::end\n",
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			dir, buf := syncFencedClaudeBlock(t, source)

			if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
				t.Fatalf("import: %v", err)
			}

			out := buf.String()
			if strings.Contains(out, companionSyncWarning) {
				t.Errorf("clean sync must not warn, got:\n%s", out)
			}
			if !strings.Contains(out, "AGNOSTIC_AI.md unchanged (it already holds what AGENTS.md and CLAUDE.md say)") {
				t.Errorf("expected the unchanged line, got:\n%s", out)
			}
			if got := mustRead(t, filepath.Join(dir, agnosticMainFile)); got != source {
				t.Errorf("AGNOSTIC_AI.md changed:\n%s", got)
			}
		})
	}
}

// A rule codex inlines into AGENTS.md, or a code review section, makes sync
// write CLAUDE.md as a full copy instead of the @AGENTS.md companion.
func TestImportFromClaude_NoWarnRightAfterSyncWhenAgentsMDCarriesARule(t *testing.T) {
	source := "Root text.\n\n::target claude\n\n## Claude specifics\n\n- Claude only line.\n\n::end\n"
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	buf := captureSummary(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex]\n")
	mustWrite(t, filepath.Join(dir, agnosticMainFile), source)
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), "---\nname: style\ndescription: Style rule\n---\n\nUse tabs.\n")
	if err := runSync(t); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if strings.Contains(mustRead(t, filepath.Join(dir, "CLAUDE.md")), "@AGENTS.md") {
		t.Fatal("CLAUDE.md is the companion; the test needs the full copy")
	}
	buf.Reset()

	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatalf("import: %v", err)
	}

	if out := buf.String(); strings.Contains(out, companionSyncWarning) {
		t.Errorf("clean sync must not warn, got:\n%s", out)
	}
	if got := mustRead(t, filepath.Join(dir, agnosticMainFile)); got != source {
		t.Errorf("AGNOSTIC_AI.md changed:\n%s", got)
	}
}

func TestImportFromClaude_WarnsWhenCompanionClaudeTextEdited(t *testing.T) {
	source := "Root text.\n\n::target claude\n\n## Claude specifics\n\n- Claude only line.\n\n::end\n"
	dir, buf := syncFencedClaudeBlock(t, source)
	claudePath := filepath.Join(dir, "CLAUDE.md")
	original := mustRead(t, claudePath)
	edited := strings.Replace(original, "- Claude only line.", "- Claude only line, edited.", 1)
	if edited == original {
		t.Fatal("CLAUDE.md does not hold the claude-only line")
	}
	mustWrite(t, claudePath, edited)

	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatalf("import: %v", err)
	}

	if !strings.Contains(buf.String(), companionSyncWarning) {
		t.Errorf("expected the edit warning, got:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "! CLAUDE.md differs") || strings.Contains(buf.String(), "AGENTS.md") {
		t.Errorf("expected the warning to name only CLAUDE.md, got:\n%s", buf.String())
	}
	if got := mustRead(t, filepath.Join(dir, agnosticMainFile)); got != source {
		t.Errorf("AGNOSTIC_AI.md changed:\n%s", got)
	}
}

func TestImportFromClaude_WarningNamesAgentsMDWhenOnlyItWasEdited(t *testing.T) {
	source := "Root text.\n\n::target claude\n\n## Claude specifics\n\n- Claude only line.\n\n::end\n"
	dir, buf := syncFencedClaudeBlock(t, source)
	agentsPath := filepath.Join(dir, "AGENTS.md")
	mustWrite(t, agentsPath, mustRead(t, agentsPath)+"\n## Testing\n\nRun the tests first.\n")

	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatalf("import: %v", err)
	}

	if !strings.Contains(buf.String(), "! AGENTS.md differs from what sync renders") || strings.Contains(buf.String(), "! CLAUDE.md differs") {
		t.Errorf("expected the warning to name AGENTS.md, got:\n%s", buf.String())
	}
	if got := mustRead(t, filepath.Join(dir, agnosticMainFile)); got != source {
		t.Errorf("AGNOSTIC_AI.md changed:\n%s", got)
	}
}

func TestImportFromClaude_WarningNamesBothWhenBothEdited(t *testing.T) {
	source := "Root text.\n\n::target claude\n\n## Claude specifics\n\n- Claude only line.\n\n::end\n"
	dir, buf := syncFencedClaudeBlock(t, source)
	agentsPath := filepath.Join(dir, "AGENTS.md")
	claudePath := filepath.Join(dir, "CLAUDE.md")
	mustWrite(t, agentsPath, mustRead(t, agentsPath)+"\n## Testing\n\nRun the tests first.\n")
	mustWrite(t, claudePath, strings.Replace(mustRead(t, claudePath), "- Claude only line.", "- Claude only line, edited.", 1))

	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatalf("import: %v", err)
	}

	if !strings.Contains(buf.String(), "! AGENTS.md and CLAUDE.md differs") {
		t.Errorf("expected the warning to name both files, got:\n%s", buf.String())
	}
	if got := mustRead(t, filepath.Join(dir, agnosticMainFile)); got != source {
		t.Errorf("AGNOSTIC_AI.md changed:\n%s", got)
	}
}

func TestImportFromClaude_UnchangedLineNamesThePairOnRepeatImport(t *testing.T) {
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	buf := captureSummary(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex]\n")
	mustWrite(t, filepath.Join(dir, "AGENTS.md"), "# Rules\n\nHand written rules.\n")
	mustWrite(t, filepath.Join(dir, "CLAUDE.md"), "@AGENTS.md\nClaude only: use plan mode.\n\n\n")

	for i := 0; i < 2; i++ {
		buf.Reset()
		if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
			t.Fatalf("import %d: %v", i+1, err)
		}
	}

	if !strings.Contains(buf.String(), "it already holds what AGENTS.md and CLAUDE.md say") {
		t.Errorf("expected the unchanged line to name both files, got:\n%s", buf.String())
	}
}
