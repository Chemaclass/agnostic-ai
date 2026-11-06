package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

// BuildRoots keeps only `{uri, name}` maps with a uri, so any other
// shape reaches no tool. Lint, validate and sync refuse it instead.
func TestMCPRoots_RefusesARootWithoutURI(t *testing.T) {
	for _, tc := range []struct{ roots, problem string }{
		{"roots: [\"file:///tmp/x\"]\n", "roots[0] must be a mapping with `uri`"},
		{"roots:\n  - uri: file:///a\n  - name: docs\n", "roots[1] has no `uri`"},
		{"roots:\n  - uri: 3\n", "roots[0] has no `uri`"},
		{"roots: file:///tmp/x\n", "roots must be a list"},
	} {
		t.Run(tc.roots, func(t *testing.T) {
			dir := budgetProject(t, "targets: [claude]\n")
			mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "fs.yaml"), "name: fs\ncommand: npx\nargs: [x]\n"+tc.roots)

			out, _ := runCLI(t, "lint")
			lines := findingLines(out, "LINT060")
			if len(lines) != 1 || !strings.Contains(lines[0], `MCP server "fs"`) || !strings.Contains(lines[0], tc.problem) {
				t.Errorf("want one LINT060 naming fs and %q:\n%s", tc.problem, out)
			}
			if out, err := runCLI(t, "validate"); err == nil || !strings.Contains(out, tc.problem) {
				t.Errorf("validate must refuse it: %v\n%s", err, out)
			}
			if _, err := runCLI(t, "sync"); err == nil || !strings.Contains(err.Error(), tc.problem) {
				t.Errorf("sync must refuse it: %v", err)
			}
		})
	}
}

func TestMCPRoots_AcceptsURIMappings(t *testing.T) {
	dir := budgetProject(t, "targets: [claude]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "fs.yaml"), "name: fs\ncommand: npx\nargs: [x]\nroots:\n  - uri: file:///tmp/x\n    name: tmp\n  - uri: file:///srv\n")
	if out, _ := runCLI(t, "lint"); strings.Contains(out, "LINT060") {
		t.Errorf("valid roots must lint clean:\n%s", out)
	}
	if out, err := runCLI(t, "sync"); err != nil {
		t.Errorf("valid roots must sync: %v\n%s", err, out)
	}
}

func TestSyncGlobal_RefusesAStringRoot(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "mcps", "fs.yaml"), "name: fs\ncommand: npx\nroots: [\"file:///tmp/x\"]\n")
	if _, _, err := runGlobalAgentTest("--only", "claude"); err == nil || !strings.Contains(err.Error(), "roots[0] must be a mapping") {
		t.Errorf("sync --global must refuse it: %v", err)
	}
}
