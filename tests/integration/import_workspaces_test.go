package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// An import of a skill that lists `workspaces` found the workspace copy
// Cursor holds and appended the key a second time on every run. The
// spec must stay as written, and lint must warn about a repeated key,
// failing only under --strict (#2202).
func TestImportCursor_WorkspacesKeyIsNotAppendedAgain(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	write := func(path, body string) {
		t.Helper()
		must(t, os.MkdirAll(filepath.Dir(path), 0o755))
		must(t, os.WriteFile(path, []byte(body), 0o644))
	}
	write("agnostic-ai.yaml", "version: 1\ntargets: [claude, codex, cursor]\nsync:\n  shared-skills: true\n")
	write(".agnostic-ai/AGNOSTIC_AI.md", "# demo\n")
	write("apps/web/index.txt", "x\n")
	const skill = "---\nname: web-skill\n# Cursor opens the web app on its own.\nworkspaces: [apps/web]\ndescription: Web skill.\n---\n\nBody.\n"
	const spec = ".agnostic-ai/skills/web-skill/SKILL.md"
	write(spec, skill)
	runCmd(t, "sync")
	if info, err := os.Lstat("apps/web/.cursor/skills/web-skill"); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Skipf("sync wrote no linked workspace copy: %v", err)
	}

	for run := 1; run <= 3; run++ {
		runCmd(t, "import", "cursor")
		got, err := os.ReadFile(spec)
		must(t, err)
		if string(got) != skill {
			t.Fatalf("import run %d changed the spec:\n%s", run, got)
		}
	}
	runCmd(t, "sync", "--check")
	runCmd(t, "lint")

	repeated := strings.Replace(skill, "description: Web skill.\n", "description: Web skill.\nworkspaces: [apps/web]\n", 1)
	write(spec, repeated)
	runCmd(t, "lint")
	runCmdExpectErr(t, "lint", "--strict")
}
