package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestScopedRuleUnion_SyncWritesAndRemovesExtraDirectories(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "agnostic-ai")
	build := exec.Command("go", "build", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = filepath.Join(packageDir, "..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}
	dir := t.TempDir()
	for name, body := range map[string]string{
		"agnostic-ai.yaml":               "version: 1\ntargets: [claude, codex, cursor]\non-unsupported: error\n",
		".agnostic-ai/AGNOSTIC_AI.md":    "# Project\n\nRoot guidance.\n",
		".agnostic-ai/rules/module-a.md": "---\nname: module-a\nscope: src/a\nglobs: [tests/a/**]\n---\nModule and test convention.\n",
	} {
		must(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755))
		must(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	run("sync", "--gitignore=off")
	run("sync", "--check", "--gitignore=off")
	for _, name := range []string{"src/a/AGENTS.md", "tests/a/AGENTS.md"} {
		assertContains(t, filepath.Join(dir, name), "Module and test convention.")
	}
	assertContains(t, filepath.Join(dir, ".claude/rules/src/a/module-a.md"), "src/a/**", "tests/a/**")
	assertNoFileContains(t, filepath.Join(dir, "AGENTS.md"), "Module and test convention.")
	assertNoFileContains(t, filepath.Join(dir, "CLAUDE.md"), "Module and test convention.")
	testutil.AssertGoldenTree(t, dir, filepath.Join(packageDir, "fixtures", "scope-union"), "agnostic-ai.yaml")

	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/rules/module-a.md"), []byte("---\nname: module-a\nscope: src/a\n---\nModule and test convention.\n"), 0o644))
	run("sync", "--gitignore=off")
	run("sync", "--check", "--gitignore=off")
	if _, err := os.Stat(filepath.Join(dir, "tests/a/AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("obsolete union destination remains, stat error = %v", err)
	}
	assertContains(t, filepath.Join(dir, "src/a/AGENTS.md"), "Module and test convention.")
}
