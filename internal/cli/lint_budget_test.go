package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// budgetProject writes a project with config and returns its root.
func budgetProject(t *testing.T, config string) string {
	t.Helper()
	dir := testutil.TempCwd(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\n"+config)
	silence(t)
	return dir
}

func words(n int) string {
	return strings.TrimSpace(strings.Repeat("word ", n))
}

func findingLines(out, code string) []string {
	var lines []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, code+" ") {
			lines = append(lines, line)
		}
	}
	return lines
}

func TestLintBudget_WarnsWhenEntryPointExceedsWordBudget(t *testing.T) {
	dir := budgetProject(t, "targets: [codex]\nlint:\n  instructions-words: 100\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), words(60)+"\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "local", "AGNOSTIC_AI.md"), words(20)+"\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), "---\nname: style\n---\n"+words(40)+"\n")

	out, err := runCLI(t, "lint")
	if err != nil {
		t.Fatalf("a budget warning alone must not fail lint: %v\n%s", err, out)
	}
	lines := findingLines(out, "LINT011")
	if len(lines) != 1 {
		t.Fatalf("expected one LINT011 line, got:\n%s", out)
	}
	for _, want := range []string{"[warn] AGENTS.md:", "codex", "AGNOSTIC_AI.md 60", "local/AGNOSTIC_AI.md 20", "rules 54", "budget 100 (lint.instructions-words)", "project_doc_max_bytes"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("LINT011 line lacks %q:\n%s", want, lines[0])
		}
	}

	if out, err := runCLI(t, "lint", "--strict"); err == nil {
		t.Errorf("lint --strict must fail on a budget warning:\n%s", out)
	}
}

func TestLintBudget_DefaultBudgetKeepsSmallProjectsQuiet(t *testing.T) {
	dir := budgetProject(t, "targets: [claude, codex]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), words(1500)+"\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), "---\nname: style\n---\nKeep it short.\n")

	out, err := runCLI(t, "lint", "--strict")
	if err != nil {
		t.Fatalf("1500 words sit under the default budget: %v\n%s", err, out)
	}
	if strings.Contains(out, "LINT011") {
		t.Errorf("unexpected LINT011 under the default budget:\n%s", out)
	}
}

func TestLintBudget_DefaultBudgetIsTwoThousandWords(t *testing.T) {
	dir := budgetProject(t, "targets: [claude]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), words(2100)+"\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), "---\nname: style\n---\nKeep it short.\n")

	out, _ := runCLI(t, "lint")
	lines := findingLines(out, "LINT011")
	if len(lines) != 1 || !strings.Contains(lines[0], "budget 2000") || !strings.Contains(lines[0], "CLAUDE.md") {
		t.Errorf("expected one LINT011 on CLAUDE.md against the 2000-word default, got:\n%s", out)
	}
}

func TestLintBudget_CountsAlwaysOnRuleFilesAndDescriptions(t *testing.T) {
	dir := budgetProject(t, "targets: [claude]\nlint:\n  instructions-words: 50\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), words(10)+"\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "always.md"), "---\nname: always\nalwaysApply: true\n---\n"+words(30)+"\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "go.md"), "---\nname: go\nglobs: \"**/*.go\"\n---\n"+words(500)+"\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "review", "SKILL.md"), "---\nname: review\ndescription: "+words(7)+"\n---\n"+words(500)+"\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "helper.md"), "---\nname: helper\ndescription: "+words(5)+"\n---\n"+words(500)+"\n")

	out, _ := runCLI(t, "lint")
	lines := findingLines(out, "LINT011")
	if len(lines) != 1 {
		t.Fatalf("expected one LINT011 line, got:\n%s", out)
	}
	// The glob-scoped rule and both bodies load on demand, so they stay out.
	for _, want := range []string{"claude loads", "always-on rule files 30,", "skill descriptions 7,", "agent descriptions 5;"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("LINT011 line lacks %q:\n%s", want, lines[0])
		}
	}
}

func TestLintBudget_NamesAntigravityByteCapUnderTheWordBudget(t *testing.T) {
	dir := budgetProject(t, "targets: [antigravity]\n")
	long := strings.Repeat("x", 99) + " "
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), strings.Repeat(long, 250)+"\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), "---\nname: style\n---\nKeep it short.\n")

	out, _ := runCLI(t, "lint")
	lines := findingLines(out, "LINT011")
	if len(lines) != 1 || !strings.Contains(lines[0], ".agents/AGENTS.md") || !strings.Contains(lines[0], "24000 bytes") {
		t.Errorf("expected LINT011 naming the Antigravity 24000-byte cap, got:\n%s", out)
	}
}

func TestLintBudget_WarnsOnLongDescription(t *testing.T) {
	dir := budgetProject(t, "targets: [claude]\n")
	skill := filepath.Join(".agnostic-ai", "skills", "review", "SKILL.md")
	mustWriteFile(t, filepath.Join(dir, skill), "---\nname: review\ndescription: "+strings.Repeat("a", 1100)+"\n---\nReview it.\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "helper.md"), "---\nname: helper\ndescription: Help out.\n---\nHelp.\n")

	out, err := runCLI(t, "lint")
	if err != nil {
		t.Fatalf("a description warning alone must not fail lint: %v\n%s", err, out)
	}
	lines := findingLines(out, "LINT012")
	if len(lines) != 1 {
		t.Fatalf("expected one LINT012 line, got:\n%s", out)
	}
	for _, want := range []string{skill, "1100 characters", "budget 1024 (lint.description-chars)"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("LINT012 line lacks %q:\n%s", want, lines[0])
		}
	}
	if out, err := runCLI(t, "lint", "--strict"); err == nil {
		t.Errorf("lint --strict must fail on a description warning:\n%s", out)
	}
}

func TestLintBudget_ConfigRaisesDescriptionBudget(t *testing.T) {
	dir := budgetProject(t, "targets: [claude]\nlint:\n  description-chars: 2000\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "review", "SKILL.md"), "---\nname: review\ndescription: "+strings.Repeat("a", 1100)+"\n---\nReview it.\n")

	out, err := runCLI(t, "lint", "--strict")
	if err != nil || strings.Contains(out, "LINT012") {
		t.Errorf("a 1100-character description sits under a 2000 budget: %v\n%s", err, out)
	}
}

func TestLintBudget_RejectsNegativeBudget(t *testing.T) {
	budgetProject(t, "targets: [claude]\nlint:\n  instructions-words: -1\n")

	out, err := runCLI(t, "lint")
	if err == nil || !strings.Contains(out+err.Error(), "lint.instructions-words") {
		t.Errorf("expected a negative budget rejected naming the key, got %v:\n%s", err, out)
	}
}

func TestLintGlobal_WarnsWhenInstructionsExceedBudget(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [claude, codex]\nlint:\n  instructions-words: 100\n")
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), words(80)+"\n")
	mustWriteGlobalTest(t, filepath.Join(source, "local", "AGNOSTIC_AI.md"), words(15)+"\n")
	mustWriteGlobalTest(t, filepath.Join(source, "rules", "safe.md"), "---\nname: safe\n---\n"+words(10)+"\n")

	out, errOut, err := runGlobalCheck("lint")
	if err != nil {
		t.Fatalf("a budget warning alone must not fail lint --global: %v\n%s", err, out)
	}
	if strings.Contains(errOut, "ignoring") {
		t.Errorf("the home config's lint key must be read, not ignored:\n%s", errOut)
	}
	// Codex names its byte cap, so it cannot share claude's line.
	lines := findingLines(out, "LINT011")
	if len(lines) != 2 {
		t.Fatalf("expected one LINT011 line each for claude and codex, got:\n%s", out)
	}
	instructions := filepath.Join(source, "AGNOSTIC_AI.md")
	for i, target := range []string{"claude", "codex"} {
		for _, want := range []string{instructions + ": " + target + " loads 107 words", "(AGNOSTIC_AI.md 80, rules 12, local/AGNOSTIC_AI.md 15)", "budget 100"} {
			if !strings.Contains(lines[i], want) {
				t.Errorf("LINT011 line lacks %q:\n%s", want, lines[i])
			}
		}
	}
	if !strings.Contains(lines[1], "project_doc_max_bytes") {
		t.Errorf("the codex line must name its byte cap:\n%s", lines[1])
	}
	if _, _, err := runGlobalCheck("lint", "--strict"); err == nil {
		t.Error("lint --global --strict must fail on a budget warning")
	}
}

func TestLintGlobal_LocalConfigReplacesBudget(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [claude]\nlint:\n  instructions-words: 10\n  description-chars: 10\n")
	mustWriteGlobalTest(t, filepath.Join(source, "local", "agnostic-ai.yaml"), "lint:\n  instructions-words: 5000\n")
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), words(80)+"\n")
	skill := filepath.Join(source, "skills", "review", "SKILL.md")
	mustWriteGlobalTest(t, skill, "---\nname: review\ndescription: Review the code in front of you\n---\nReview it.\n")

	out, _, _ := runGlobalCheck("lint")
	if strings.Contains(out, "LINT011") {
		t.Errorf("local/agnostic-ai.yaml raised the word budget, so no LINT011 expected:\n%s", out)
	}
	if lines := findingLines(out, "LINT012"); len(lines) != 1 || !strings.Contains(lines[0], skill) {
		t.Errorf("the shared description budget must survive a local file that does not set it, got:\n%s", out)
	}
}
