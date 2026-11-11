package cli

import (
	"strings"
	"testing"
)

func TestLint_WarnsOnRulesFileTheTargetDoesNotWrite(t *testing.T) {
	budgetProject(t, "targets: [cline]\noutputs:\n  cline:\n    rules-file: AGENTS.md\n")

	out, err := runCLI(t, "lint")

	if err != nil {
		t.Fatalf("a warning must not fail lint: %v\n%s", err, out)
	}
	lines := findingLines(out, "LINT062")
	if len(lines) != 1 || !strings.Contains(lines[0], "outputs.cline.rules-file has no effect") {
		t.Errorf("want one LINT062 naming outputs.cline.rules-file, got:\n%s", out)
	}
}

func TestLint_AcceptsRulesFileOnATargetThatWritesIt(t *testing.T) {
	budgetProject(t, "targets: [codex]\noutputs:\n  codex:\n    rules-file: AGENTS.md\n")

	out, err := runCLI(t, "lint")

	if err != nil {
		t.Fatalf("lint: %v\n%s", err, out)
	}
	if strings.Contains(out, "LINT062") {
		t.Errorf("codex writes its rules file:\n%s", out)
	}
}
