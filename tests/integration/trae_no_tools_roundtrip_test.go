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

func TestTraeRoundTrip_PreservesNoToolsRestriction(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
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
	dir := testutil.TempCwd(t)
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte("version: 1\ntargets: [trae]\n"), 0o644))
	native := filepath.Join(dir, ".trae", "agents", "plain.md")
	must(t, os.MkdirAll(filepath.Dir(native), 0o755))
	must(t, os.WriteFile(native, []byte("---\nname: plain\ndescription: Text only\ntools: \"\"\n---\n\nRespond from supplied text.\n"), 0o644))
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	run("import", "trae")
	imported, err := os.ReadFile(filepath.Join(dir, ".agnostic-ai", "agents", "plain.md"))
	must(t, err)
	if !strings.Contains(string(imported), "tools: \"\"") {
		t.Errorf("import lost empty allowlist:\n%s", imported)
	}
	run("sync", "-t", "trae")
	first, err := os.ReadFile(native)
	must(t, err)
	if !strings.Contains(string(first), "tools: \"\"\n") {
		t.Errorf("sync widened tool access:\n%s", first)
	}
	run("sync", "--check", "-t", "trae")
	run("import", "trae")
	run("sync", "-t", "trae")
	second, err := os.ReadFile(native)
	must(t, err)
	if string(second) != string(first) {
		t.Errorf("round trip changed native agent:\n%s", second)
	}
	run("sync", "--check", "-t", "trae")
}
