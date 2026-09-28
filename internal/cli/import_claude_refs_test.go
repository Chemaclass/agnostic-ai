package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Import points prose at the source of each spec it wrote, drops the
// `@` line for a rule every target now receives, and reports a native
// path no spec replaces (#1326).
func TestImportFromClaude_PointsNativePathsAtSources(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	buf := captureSummary(t)
	mustWrite(t, filepath.Join(dir, ".claude", "CLAUDE.md"),
		"# Project\n\n@.claude/rules/tone.md\n\nRead `.claude/skills/style/SKILL.md`.\n\nSee ./.claude/skills/gone/SKILL.md.\n")
	mustWrite(t, filepath.Join(dir, ".claude", "rules", "tone.md"), "# Tone\n\nBe plain.\n")
	mustWrite(t, filepath.Join(dir, ".claude", "skills", "style", "SKILL.md"),
		"---\nname: style\ndescription: Style guide.\n---\n\nPair with .claude/rules/tone.md.\n")

	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatalf("import: %v", err)
	}

	want := "# Project\n\nRead `skills/style/SKILL.md`.\n\nSee ./.claude/skills/gone/SKILL.md.\n"
	if got := mustRead(t, filepath.Join(dir, agnosticMainFile)); got != want {
		t.Errorf("AGNOSTIC_AI.md = %q, want %q", got, want)
	}
	if got := mustRead(t, filepath.Join(dir, "skills", "style", "SKILL.md")); !strings.Contains(got, "Pair with rules/tone.md.") {
		t.Errorf("skill should point at the rule source, got:\n%s", got)
	}
	if !strings.Contains(buf.String(), "names .claude/skills/gone/SKILL.md, which no imported spec replaces") {
		t.Errorf("import should report the path it left alone, got:\n%s", buf.String())
	}
}

// A rule the import did not write is still loaded through the `@` line,
// so the line stays.
func TestImportFromClaude_KeepsImportOfRuleItDidNotWrite(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	mustWrite(t, filepath.Join(dir, "CLAUDE.md"), "# Project\n\n@.claude/rules/tone.md\n")
	mustWrite(t, filepath.Join(dir, ".claude", "rules", "other.md"), "Other.\n")

	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatalf("import: %v", err)
	}
	if got := mustRead(t, filepath.Join(dir, agnosticMainFile)); !strings.Contains(got, "@.claude/rules/tone.md") {
		t.Errorf("the @ line should stay, got:\n%s", got)
	}
}
