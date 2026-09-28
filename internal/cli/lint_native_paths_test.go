package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

// A native path to a spec resolves for one target only; the source path
// resolves for all (#1326).
func TestLint_WarnsOnNativePathToASourceSpec(t *testing.T) {
	dir := budgetProject(t, "targets: [claude, codex]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "style", "SKILL.md"), "---\nname: style\ndescription: Style guide.\n---\n\nBody.\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "tone.md"), "---\nname: tone\n---\n\nBe plain.\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "guide.md"),
		"---\nname: guide\n---\n\nRead `.claude/skills/style/SKILL.md`, then .agents/skills/style/references/a.md and .claude/rules/tone.md.\nIgnore .claude/skills/gone/SKILL.md and .claude/settings.json.\n")

	out, err := runCLI(t, "lint")
	if err != nil {
		t.Fatalf("LINT015 is a warning, lint failed: %v\n%s", err, out)
	}
	lines := strings.Join(findingLines(out, "LINT015"), "\n")
	for _, want := range []string{
		"names .claude/skills/style/SKILL.md, a target-native path of skill \"style\"; use the source path .agnostic-ai/skills/style/SKILL.md",
		"names .agents/skills/style/references/a.md, a target-native path of skill \"style\"; use the source path .agnostic-ai/skills/style/references/a.md",
		"names .claude/rules/tone.md, a target-native path of rule \"tone\"; use the source path .agnostic-ai/rules/tone.md",
	} {
		if !strings.Contains(lines, want) {
			t.Errorf("LINT015 lacks %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"gone", "settings.json"} {
		if strings.Contains(lines, unwanted) {
			t.Errorf("LINT015 should only name existing specs, got %q:\n%s", unwanted, out)
		}
	}
}
