package cli

import (
	"errors"
	"io/fs"
	"os"
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
	writeFile(t, filepath.Join(dir, ".claude", "CLAUDE.md"),
		"# Project\n\n@.claude/rules/tone.md\n\nRead `.claude/skills/style/SKILL.md`.\n\nSee ./.claude/skills/gone/SKILL.md.\n")
	writeFile(t, filepath.Join(dir, ".claude", "rules", "tone.md"), "# Tone\n\nBe plain.\n")
	writeFile(t, filepath.Join(dir, ".claude", "skills", "style", "SKILL.md"),
		"---\nname: style\ndescription: Style guide.\n---\n\nPair with .claude/rules/tone.md.\n")

	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatalf("import: %v", err)
	}

	want := "# Project\n\nRead `skills/style/SKILL.md`.\n\nSee ./.claude/skills/gone/SKILL.md.\n"
	if got := readFile(t, filepath.Join(dir, agnosticMainFile)); got != want {
		t.Errorf("AGNOSTIC_AI.md = %q, want %q", got, want)
	}
	if got := readFile(t, filepath.Join(dir, "skills", "style", "SKILL.md")); !strings.Contains(got, "Pair with rules/tone.md.") {
		t.Errorf("skill should point at the rule source, got:\n%s", got)
	}
	if !strings.Contains(buf.String(), "names .claude/skills/gone/SKILL.md, which no imported spec replaces") {
		t.Errorf("import should report the path it left alone, got:\n%s", buf.String())
	}
}

func TestImportFromClaude_PointsNativeSkillPathAtScopedSingleFileSource(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	buf := captureSummary(t)
	src := rootSources()
	src.Skills = "portable-skills"
	const skill = "---\nname: style\ndescription: Style guide.\n---\n\nUse plain English.\n"
	flat := filepath.Join(dir, src.Skills, "packages", "design", "checklist.md")
	writeFile(t, flat, skill)
	writeFile(t, filepath.Join(dir, src.Skills, "another", "SKILL.md"), "Another skill.\n")
	writeFile(t, filepath.Join(dir, src.Skills, "another", "references", "style.md"), skill)
	edited := strings.Replace(skill, "Use plain English.", "Use short sentences.", 1)
	writeFile(t, filepath.Join(dir, ".claude", "skills", "style", "SKILL.md"), edited)
	writeFile(t, filepath.Join(dir, ".claude", "skills", "style", ".DS_Store"), "Finder metadata\n")
	writeFile(t, filepath.Join(dir, ".claude", "CLAUDE.md"), "# Project\n\nSee ./.claude/skills/style/SKILL.md.\n")

	if err := importFromClaude(dir, src, defaultClaudeLayout()); err != nil {
		t.Fatalf("import: %v", err)
	}

	want := "# Project\n\nSee ./portable-skills/packages/design/checklist.md.\n"
	if got := readFile(t, filepath.Join(dir, agnosticMainFile)); got != want {
		t.Errorf("AGNOSTIC_AI.md = %q, want %q", got, want)
	}
	if got := readFile(t, flat); got != edited {
		t.Errorf("scoped skill = %q, want %q", got, edited)
	}
	if _, err := os.Stat(filepath.Join(dir, src.Skills, "style")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("import created another skill folder: %v", err)
	}
	if strings.Contains(buf.String(), "which no imported spec replaces") {
		t.Errorf("import reported a missing replacement for the flat skill:\n%s", buf.String())
	}
}

func TestImportFromClaude_RefusesAmbiguousScopedFlatSkills(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	src := rootSources()
	const skill = "---\nname: style\ndescription: Style guide.\n---\n\nUse plain English.\n"
	first := filepath.Join(dir, src.Skills, "api", "first.md")
	second := filepath.Join(dir, src.Skills, "web", "second.md")
	writeFile(t, first, skill)
	writeFile(t, second, skill)
	writeFile(t, filepath.Join(dir, ".claude", "skills", "style", "SKILL.md"), strings.Replace(skill, "Use plain English.", "Use short sentences.", 1))

	err := importFromClaude(dir, src, defaultClaudeLayout())
	if err == nil || !strings.Contains(err.Error(), "matches several portable specs") ||
		!strings.Contains(err.Error(), filepath.Join("api", "first.md")) || !strings.Contains(err.Error(), filepath.Join("web", "second.md")) {
		t.Errorf("import error = %v, want both matching skill paths", err)
	}
	for _, path := range []string{first, second} {
		if got := readFile(t, path); got != skill {
			t.Errorf("ambiguous skill %s was changed: %q", path, got)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, src.Skills, "style")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("import created another skill folder: %v", err)
	}
}

// A rule the import did not write is still loaded through the `@` line,
// so the line stays.
func TestImportFromClaude_KeepsImportOfRuleItDidNotWrite(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), "# Project\n\n@.claude/rules/tone.md\n")
	writeFile(t, filepath.Join(dir, ".claude", "rules", "other.md"), "Other.\n")

	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatalf("import: %v", err)
	}
	if got := readFile(t, filepath.Join(dir, agnosticMainFile)); !strings.Contains(got, "@.claude/rules/tone.md") {
		t.Errorf("the @ line should stay, got:\n%s", got)
	}
}
