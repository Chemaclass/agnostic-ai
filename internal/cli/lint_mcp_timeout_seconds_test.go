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
// 30. Sync then writes 1 second, and its rounding note shows only once,
// so lint keeps warning.
func TestLint_MCPTimeoutThatLooksLikeSecondsWarns(t *testing.T) {
	for _, targets := range []string{"targets: [crush]\n", "targets: [pi]\n"} {
		mcpTimeoutLintProject(t, targets, "timeout: 30\n")

		out, err := runCLI(t, "lint")
		if err != nil {
			t.Fatalf("a warning alone must pass: %v\n%s", err, out)
		}
		lines := findingLines(out, "LINT059")
		if len(lines) != 1 || !strings.Contains(lines[0], "multiply it by 1000") {
			t.Errorf("%s: want one LINT059 asking to multiply by 1000:\n%s", targets, out)
		}
	}
}

func TestLint_MCPTimeoutInMillisecondsIsClean(t *testing.T) {
	for _, c := range []struct{ targets, mcp string }{
		{"targets: [crush]\n", "timeout: 30000\n"},
		{"targets: [pi]\n", "timeout: 30\nx-pi:\n  timeout: 30\n"},
		{"targets: [junie]\n", "timeout: 30\n"},
		{"targets: [crush, junie]\n", "timeout: 30\ntargets: [junie]\n"},
	} {
		mcpTimeoutLintProject(t, c.targets, c.mcp)
		if out, err := runCLI(t, "lint"); err != nil || strings.Contains(out, "LINT059") {
			t.Errorf("%s %q: want no LINT059: %v\n%s", c.targets, c.mcp, err, out)
		}
	}
}
