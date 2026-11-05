package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func mcpTimeoutLintProject(t *testing.T, targets, mcp string) {
	t.Helper()
	dir := budgetProject(t, targets)
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "slow.yaml"), "name: slow\ntype: http\nurl: https://x.example/mcp\n"+mcp)
}

// A spec imported from Crush or Pi before #2997 holds seconds, such as
// 30. Sync then writes 1 second to Crush, 0.03 to Pi and 30 ms to Junie,
// below Junie's 1000 ms floor, and sync's notes show only once, so lint
// keeps warning and names each tool the value reaches.
func TestLint_MCPTimeoutThatLooksLikeSecondsWarns(t *testing.T) {
	for targets, named := range map[string]string{
		"targets: [crush]\n":            "crush",
		"targets: [pi]\n":               "pi",
		"targets: [junie]\n":            "junie",
		"targets: [crush, junie]\n":     "crush and junie",
		"targets: [crush, pi, junie]\n": "crush, pi and junie",
	} {
		mcpTimeoutLintProject(t, targets, "timeout: 30\n")

		out, err := runCLI(t, "lint")
		if err != nil {
			t.Fatalf("a warning alone must pass: %v\n%s", err, out)
		}
		lines := findingLines(out, "LINT059")
		if len(lines) != 1 || !strings.Contains(lines[0], "multiply it by 1000") || !strings.Contains(lines[0], named+" get") {
			t.Errorf("%s: want one LINT059 naming %s and asking to multiply by 1000:\n%s", targets, named, out)
		}
	}
}

// Sync reads a whole-number float such as 500.0 as 500 ms, so lint must too.
func TestLint_MCPTimeoutWholeFloatWarns(t *testing.T) {
	mcpTimeoutLintProject(t, "targets: [pi]\n", "timeout: 500.0\n")
	out, err := runCLI(t, "lint")
	if err != nil || len(findingLines(out, "LINT059")) != 1 {
		t.Errorf("want one LINT059: %v\n%s", err, out)
	}
}

func TestLint_MCPTimeoutInMillisecondsIsClean(t *testing.T) {
	for _, c := range []struct{ targets, mcp string }{
		{"targets: [crush]\n", "timeout: 30000\n"},
		{"targets: [pi]\n", "timeout: 30\nx-pi:\n  timeout: 30\n"},
		{"targets: [junie]\n", "timeout: 30\nx-junie:\n  timeout: 30000\n"},
		{"targets: [crush, junie]\n", "timeout: 30\ntargets: [kiro]\n"},
	} {
		mcpTimeoutLintProject(t, c.targets, c.mcp)
		if out, err := runCLI(t, "lint"); err != nil || strings.Contains(out, "LINT059") {
			t.Errorf("%s %q: want no LINT059: %v\n%s", c.targets, c.mcp, err, out)
		}
	}
}
