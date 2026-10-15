package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestRoundTrip_OpenCodeAgentPermissionOrder(t *testing.T) {
	packageDir, err := os.Getwd()
	must(t, err)
	name := "agnostic-ai"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	build := exec.Command("go", "build", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = filepath.Join(packageDir, "..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	t.Setenv("AGNOSTIC_AI_NO_UPDATE_CHECK", "1")
	dir := testutil.TempCwd(t)
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte("version: 1\ntargets: [opencode]\n"), 0o644))
	path := filepath.Join(dir, ".opencode/agents/reviewer.md")
	must(t, os.MkdirAll(filepath.Dir(path), 0o755))
	permission := "permission:\n  read:\n    secrets/public/*: allow\n    secrets/*: deny\n  bash:\n    git status: allow\n    git *: ask\n  github_create_issue: allow\n  github_*: deny\n"
	native := "---\ndescription: Review project files\n" + permission + "---\nReview.\n"
	must(t, os.WriteFile(path, []byte(native), 0o644))

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	run("import", "opencode")
	imported, err := os.ReadFile(filepath.Join(dir, ".agnostic-ai/agents/reviewer.md"))
	must(t, err)
	if string(imported) != native {
		t.Errorf("import changed native agent:\n%s", imported)
	}
	run("sync")
	got, err := os.ReadFile(path)
	must(t, err)
	if !strings.Contains(string(got), permission) {
		t.Errorf("sync reordered native permissions:\n%s", got)
	}
	run("sync", "--check")
	run("lint")
}
