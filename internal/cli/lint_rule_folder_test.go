package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A rule folder named for a project directory reads as its scope, so a
// frontmatter scope elsewhere is worth a warning. A folder named for no
// directory only groups rules (#1430).
func TestLint_WarnsWhenARuleFolderAndItsScopeDisagree(t *testing.T) {
	dir := budgetProject(t, "targets: [claude]\n")
	if err := os.MkdirAll(filepath.Join(dir, "backend", "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	rules := filepath.Join(dir, ".agnostic-ai", "rules")
	mustWriteFile(t, filepath.Join(rules, "backend", "moved.md"), "---\nname: moved\ndescription: M\nscope: src/b\n---\nM\n")
	mustWriteFile(t, filepath.Join(rules, "backend", "narrower.md"), "---\nname: narrower\ndescription: N\nscope: backend/api\n---\nN\n")
	mustWriteFile(t, filepath.Join(rules, "backend", "same.md"), "---\nname: same\ndescription: S\nscope: ./backend\n---\nS\n")
	mustWriteFile(t, filepath.Join(rules, "backend", "dot-narrower.md"), "---\nname: dot-narrower\ndescription: D\nscope: ./backend/api\n---\nD\n")
	mustWriteFile(t, filepath.Join(rules, "modules", "grouped.md"), "---\nname: grouped\ndescription: G\nscope: src/a\n---\nG\n")

	out, _ := runCLI(t, "lint")
	lines := findingLines(out, "LINT020")
	if len(lines) != 1 || !strings.Contains(lines[0], "moved.md") || !strings.Contains(lines[0], "src/b") {
		t.Errorf("expected one LINT020 line for moved.md, got:\n%s", out)
	}
}
