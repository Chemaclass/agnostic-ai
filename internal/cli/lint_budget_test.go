package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
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
	dir := budgetProject(t, "targets: [codex, amp, warp]\nlint:\n  instructions-words: 100\n")
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
	for _, want := range []string{"[warn] AGENTS.md: codex, amp, warp load", "AGNOSTIC_AI.md 60", "local/AGNOSTIC_AI.md 20", "rules 54", "budget 100 (lint.instructions-words)"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("LINT011 line lacks %q:\n%s", want, lines[0])
		}
	}
	if strings.Contains(lines[0], "project_doc_max_bytes") {
		t.Errorf("the Codex cap belongs on the line only past the cap:\n%s", lines[0])
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

func TestLintBudget_NamesCodexCapOnlyPastIt(t *testing.T) {
	dir := budgetProject(t, "targets: [codex, amp]\n")
	long := strings.Repeat("x", 99) + " "
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), strings.Repeat(long, 340)+"\n")

	out, _ := runCLI(t, "lint")
	lines := findingLines(out, "LINT011")
	if len(lines) != 1 || !strings.Contains(lines[0], "codex loads") || !strings.Contains(lines[0], "project_doc_max_bytes") {
		t.Errorf("expected one LINT011 for codex alone, past its 32 KiB cap, got:\n%s", out)
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
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "helper.md"), "---\nname: helper\ndescription: "+strings.Repeat("a", 1100)+"\n---\nHelp.\n")

	out, err := runCLI(t, "lint", "--strict")
	if err != nil || strings.Contains(out, "LINT012") {
		t.Errorf("a 1100-character agent description sits under a 2000 budget: %v\n%s", err, out)
	}
}

func TestLintBudget_SkillDescriptionPastSpecLimitWarnsUnderRaisedBudget(t *testing.T) {
	dir := budgetProject(t, "targets: [claude]\nlint:\n  description-chars: 2000\n")
	skill := filepath.Join(".agnostic-ai", "skills", "review", "SKILL.md")
	mustWriteFile(t, filepath.Join(dir, skill), "---\nname: review\ndescription: "+strings.Repeat("a", 1100)+"\n---\nReview it.\n")

	out, _ := runCLI(t, "lint")
	lines := findingLines(out, "LINT012")
	if len(lines) != 1 || !strings.Contains(lines[0], skill) || !strings.Contains(lines[0], "Agent Skills spec allows at most 1024") {
		t.Errorf("the Agent Skills limit must warn whatever the budget, got:\n%s", out)
	}
	if len(lines) == 1 && strings.Contains(lines[0], "budget 2000") {
		t.Errorf("the line must not claim the raised budget was passed:\n%s", lines[0])
	}
}

func TestLintBudget_ReadsTargetDescriptionOverride(t *testing.T) {
	dir := budgetProject(t, "targets: [claude]\nlint:\n  instructions-words: 10\n")
	skill := filepath.Join(".agnostic-ai", "skills", "review", "SKILL.md")
	mustWriteFile(t, filepath.Join(dir, skill), "---\nname: review\ndescription: Review code\nx-claude:\n  description: "+words(1100)+"\n---\nReview it.\n")

	out, _ := runCLI(t, "lint")
	budget := findingLines(out, "LINT011")
	if len(budget) != 1 || !strings.Contains(budget[0], "skill descriptions 1100") {
		t.Errorf("claude lists the x-claude description, so it counts, got:\n%s", out)
	}
	desc := findingLines(out, "LINT012")
	if len(desc) != 1 || !strings.Contains(desc[0], "x-claude.description") {
		t.Errorf("expected LINT012 naming x-claude.description, got:\n%s", out)
	}
}

func TestLintBudget_RenderErrorWarnsAndKeepsOtherFindings(t *testing.T) {
	dir := budgetProject(t, "targets: [codex]\nsync:\n  resolve-imports: inline\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "@docs/missing.md\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "empty.md"), "")

	out, err := runCLI(t, "lint")
	if err != nil {
		t.Fatalf("a budget pass that cannot render must not fail lint: %v\n%s", err, out)
	}
	if len(findingLines(out, "LINT001")) != 1 {
		t.Errorf("the other findings must survive a render error, got:\n%s", out)
	}
	lines := findingLines(out, "LINT011")
	if len(lines) != 1 || !strings.Contains(lines[0], "[warn]") || !strings.Contains(lines[0], "docs/missing.md") {
		t.Errorf("expected one LINT011 warning naming the render error, got:\n%s", out)
	}
}

func TestLintBudget_MeasuresProjectWithNoSpecs(t *testing.T) {
	dir := budgetProject(t, "targets: [claude]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), words(3000)+"\n")

	out, err := runCLI(t, "lint", "--strict")
	if err == nil || len(findingLines(out, "LINT011")) != 1 {
		t.Errorf("a 3000-word AGNOSTIC_AI.md with no specs must still fail --strict, got %v:\n%s", err, out)
	}
}

func TestLintBudget_CountsEachTargetsAlwaysOnRuleFiles(t *testing.T) {
	b := spec.Bundle{Rules: []spec.Entry{
		{Kind: spec.KindRule, Name: "go", Meta: map[string]any{"globs": "**/*.go"}, Body: words(300)},
		{Kind: spec.KindRule, Name: "req", Meta: map[string]any{"alwaysApply": false, "description": "Use for requests"}, Body: words(100)},
	}}
	cfg := &config.Config{}
	// go: globs, alwaysApply unset. req: alwaysApply false plus a description.
	cases := map[string]int{
		"claude":      100, // globs become paths; alwaysApply has no Claude meaning
		"cursor":      0,   // globs scope go; false is agent requested
		"trae":        0,
		"antigravity": 0,   // go is glob, req is model_decision
		"augment":     0,   // both inline into AGENTS.md, which Augment always reads
		"cline":       100, // go becomes paths; no description mode, so req stays always active
		"windsurf":    0,   // go is glob, req is model_decision
		"kiro":        100, // globs become fileMatch
		"copilot":     0,   // globs become applyTo; req loads on demand from its description
		"qoder":       300, // globs is not Qoder's key; alwaysApply false is manual
		"continue":    0,   // globs match on demand; alwaysApply false is not always
		"kilo":        0,   // both inline into AGENTS.md, which Kilo always reads
		"codex":       0,   // no rule files; rules inline into AGENTS.md
	}
	for target, want := range cases {
		if got := alwaysOnRuleWords(cfg, b, target); got != want {
			t.Errorf("%s: always-on rule words = %d, want %d", target, got, want)
		}
	}

	legacy := &config.Config{Outputs: map[string]config.Output{
		"claude":  {RulesFile: ".claude/RULES.md"},
		"copilot": {RulesFile: ".github/copilot-instructions.md"},
	}}
	// Claude's legacy file holds every rule; Copilot's keeps only always-on
	// rules, and req loads on demand.
	for target, want := range map[string]int{"claude": 400, "copilot": 0} {
		if got := alwaysOnRuleWords(legacy, b, target); got != want {
			t.Errorf("%s legacy rules-file: always-on rule words = %d, want %d", target, got, want)
		}
	}
}

func TestLintGlobal_MeasuresHomeWithNoSpecs(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [claude]\n")
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), words(3000)+"\n")

	out, _, err := runGlobalCheck("lint", "--strict")
	if err == nil || len(findingLines(out, "LINT011")) != 1 {
		t.Errorf("a 3000-word home AGNOSTIC_AI.md with no specs must still fail --strict, got %v:\n%s", err, out)
	}
}

func TestLintGlobal_ValidatesBudgetAfterLocalOverride(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [claude]\nlint:\n  instructions-words: -1\n")
	mustWriteGlobalTest(t, filepath.Join(source, "local", "agnostic-ai.yaml"), "lint:\n  instructions-words: 100\n")
	mustWriteGlobalTest(t, filepath.Join(source, "rules", "safe.md"), "---\nname: safe\n---\nBe safe.\n")

	if out, _, err := runGlobalCheck("lint", "--strict"); err != nil {
		t.Errorf("local/agnostic-ai.yaml replaces the negative value, so lint must pass: %v\n%s", err, out)
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
	lines := findingLines(out, "LINT011")
	if len(lines) != 1 {
		t.Fatalf("expected claude and codex on one LINT011 line, got:\n%s", out)
	}
	instructions := filepath.Join(source, "AGNOSTIC_AI.md")
	for _, want := range []string{instructions + ": claude, codex load 107 words", "(AGNOSTIC_AI.md 80, rules 12, local/AGNOSTIC_AI.md 15)", "budget 100"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("LINT011 line lacks %q:\n%s", want, lines[0])
		}
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

// A CLAUDE.md that is `@AGENTS.md` plus Claude-only text loads AGENTS.md
// whole every session, so the count includes it.
func TestLintBudget_CountsTheAGENTSMdClaudeMdImports(t *testing.T) {
	dir := budgetProject(t, "targets: [claude, codex]\nlint:\n  instructions-words: 100\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), words(150)+"\n\n::target claude\n\n"+words(5)+"\n\n::end\n")

	out, _ := runCLI(t, "lint")
	var claude string
	for _, line := range findingLines(out, "LINT011") {
		if strings.Contains(line, "claude loads") {
			claude = line
		}
	}
	if claude == "" {
		t.Fatalf("expected a LINT011 line for claude, got:\n%s", out)
	}
	if !strings.Contains(claude, "AGENTS.md import 1") {
		t.Errorf("claude's line lacks the imported AGENTS.md:\n%s", claude)
	}
}

// An `@AGENTS.md` inside a code example is text, not an import, and only a
// tool that resolves imports loads the file.
func TestLintBudget_IgnoresAGENTSMdOutsideARealImport(t *testing.T) {
	dir := budgetProject(t, "targets: [gemini, codex]\nlint:\n  instructions-words: 1000\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), words(600)+"\n\n```markdown\n@AGENTS.md\n```\n")

	out, _ := runCLI(t, "lint")
	if strings.Contains(out, "AGENTS.md import") {
		t.Errorf("no tool here imports AGENTS.md:\n%s", out)
	}
}
