package cli

import (
	"path/filepath"
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

// A key set only in agnostic-ai.local.yaml is reported against that file.
func TestLint_NamesTheLocalFileForAnOutputKeySetThere(t *testing.T) {
	dir := budgetProject(t, "targets: [cline, codex]\n")
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.local.yaml"), "outputs:\n  cline:\n    rules-file: AGENTS.md\n  codex:\n    rules-mode: import\n")

	out, _ := runCLI(t, "lint")

	for _, code := range []string{"LINT061", "LINT062"} {
		lines := findingLines(out, code)
		if len(lines) != 1 || !strings.Contains(lines[0], "agnostic-ai.local.yaml:") {
			t.Errorf("want one %s naming agnostic-ai.local.yaml, got:\n%s", code, out)
		}
	}
}
