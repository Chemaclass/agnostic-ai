package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func codexMCPEnvironmentProject(t *testing.T, targets, mcp string) {
	t.Helper()
	dir := budgetProject(t, targets)
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "fs.yaml"), "name: fs\ncommand: npx\n"+mcp)
}

func TestLint_CodexMCPExperimentalEnvironmentWarns(t *testing.T) {
	for mcp, want := range map[string]string{
		"experimental_environment: local\n":                          "migrate --only mcps",
		"experimental_environment: remote\n":                         "set environment_id to a configured executor ID",
		"environment_id: staging\nexperimental_environment: local\n": "remove experimental_environment",
	} {
		codexMCPEnvironmentProject(t, "targets: [codex]\n", mcp)

		out, err := runCLI(t, "lint")
		if err != nil {
			t.Fatalf("a warning alone must pass: %v\n%s", err, out)
		}
		lines := findingLines(out, "LINT058")
		if len(lines) != 1 || !strings.Contains(lines[0], want) {
			t.Errorf("%q: want one LINT058 with %q:\n%s", mcp, want, out)
		}
	}
}

func TestLint_CodexMCPEnvironmentIDIsClean(t *testing.T) {
	codexMCPEnvironmentProject(t, "targets: [codex]\n", "environment_id: staging\n")
	if out, err := runCLI(t, "lint"); err != nil || strings.Contains(out, "LINT0") {
		t.Errorf("want a clean lint: %v\n%s", err, out)
	}
}

func TestLint_CodexMCPExperimentalEnvironmentWithoutCodexIsClean(t *testing.T) {
	codexMCPEnvironmentProject(t, "targets: [claude]\n", "experimental_environment: remote\n")
	if out, _ := runCLI(t, "lint"); len(findingLines(out, "LINT058")) != 0 {
		t.Errorf("want no LINT058:\n%s", out)
	}
}
