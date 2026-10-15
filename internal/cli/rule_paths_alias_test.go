package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const pathsRule = "---\nname: a\ndescription: d\npaths:\n  - resources/js/**\n---\n\nPATHS rule.\n"

func TestRender_RulePathsScopesCopilotAndCursorLikeGlobs(t *testing.T) {
	dir := budgetProject(t, "targets: [claude, copilot, cursor, codex, junie]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "a.md"), pathsRule)

	cases := map[string][]string{
		"copilot": {`applyTo: "resources/js/**"`},
		"cursor":  {"globs: resources/js/**", "alwaysApply: false"},
		"claude":  {"paths:", "resources/js/**"},
	}
	for target, wants := range cases {
		out, err := runCLI(t, "render", ".agnostic-ai/rules/a.md", "--target", target)
		if err != nil {
			t.Fatalf("%s: render: %v\n%s", target, err, out)
		}
		for _, want := range wants {
			if !strings.Contains(out, want) {
				t.Errorf("%s: render lacks %q:\n%s", target, want, out)
			}
		}
		if strings.Contains(out, `applyTo: "**"`) || strings.Contains(out, "alwaysApply: true") {
			t.Errorf("%s: a paths rule must not load on every file:\n%s", target, out)
		}
	}
}

func TestLintBudget_RulePathsIsNotAlwaysOnCopilotContext(t *testing.T) {
	dir := budgetProject(t, "targets: [copilot, cursor]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "a.md"), "---\nname: a\ndescription: d\npaths: [\"resources/js/**\"]\n---\n\n"+words(300)+"\n")

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := spec.LoadBundle(dir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"copilot", "cursor"} {
		if got := alwaysOnRuleWords(cfg, b, target); got != 0 {
			t.Errorf("%s: always-on rule words = %d, want 0 for a paths rule", target, got)
		}
	}
}

func TestValidate_RejectsRuleWithConflictingPathsAndGlobs(t *testing.T) {
	dir := budgetProject(t, "targets: [cursor]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "both.md"), "---\nname: both\npaths: [\"a/**\"]\nglobs: [\"b/**\"]\n---\nBody.\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "same.md"), "---\nname: same\npaths: [\"a/**\"]\nglobs: \"a/**\"\n---\nBody.\n")

	out, err := runCLI(t, "validate")
	if err == nil || !strings.Contains(out, "both.md") || strings.Contains(out, "same.md") {
		t.Errorf("validate must reject only paths and globs that disagree, got %v:\n%s", err, out)
	}
	out, err = runCLI(t, "lint")
	if err == nil || !strings.Contains(strings.Join(findingLines(out, "LINT013"), "\n"), "both.md") {
		t.Errorf("lint must flag paths and globs that disagree, got %v:\n%s", err, out)
	}
}

func TestRender_RuleEmptyPathsStaysNeverActiveOnClaudeAndCline(t *testing.T) {
	dir := budgetProject(t, "targets: [claude, cline]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "off.md"), "---\nname: off\ndescription: d\npaths: []\n---\n\nOFF rule.\n")

	for _, target := range []string{"claude", "cline"} {
		out, err := runCLI(t, "render", ".agnostic-ai/rules/off.md", "--target", target)
		if err != nil {
			t.Fatalf("%s: render: %v\n%s", target, err, out)
		}
		if !strings.Contains(out, "paths: []") {
			t.Errorf("%s: an empty paths must stay, or the rule loads everywhere:\n%s", target, out)
		}
	}
}

func TestSync_RuleStringPathsKeepsItsFormOnClaudeAndCline(t *testing.T) {
	dir := budgetProject(t, "targets: [claude, cline, copilot]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "go-style.md"), "---\nname: go-style\ndescription: d\npaths: src/**\n---\n\nbody\n")

	for _, target := range []string{"claude", "cline"} {
		out, err := runCLI(t, "render", ".agnostic-ai/rules/go-style.md", "--target", target)
		if err != nil {
			t.Fatalf("%s: render: %v\n%s", target, err, out)
		}
		if !strings.Contains(out, "paths: src/**\n") {
			t.Errorf("%s: a string paths must stay a string:\n%s", target, out)
		}
	}
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync after import must change nothing: %v\n%s", err, out)
	}
	if got := readFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "go-style.md")); !strings.Contains(got, "paths: src/**\n") {
		t.Errorf("import must keep the string paths:\n%s", got)
	}
}
