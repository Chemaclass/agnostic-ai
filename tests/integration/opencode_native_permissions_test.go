package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOpencodeNativePermissions_SyncImportSyncKeepsLimits(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "agnostic-ai")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}
	dir := t.TempDir()
	home := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "AGNOSTIC_AI_HOME="+home)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte("version: 1\ntargets: [opencode]\n"), 0o644))
	project := `{"permissions":[{"action":"r*","resource":"private/*","effect":"ask"},{"action":"read","resource":"secrets/*","effect":"deny"}]}`
	must(t, os.WriteFile(filepath.Join(dir, "opencode.json"), []byte(project), 0o644))
	source := filepath.Join(dir, ".agnostic-ai/agents/reviewer.md")
	must(t, os.MkdirAll(filepath.Dir(source), 0o755))
	must(t, os.WriteFile(source, []byte("---\nname: reviewer\ndescription: Review files\ncan: [read]\n---\nReview.\n"), 0o644))
	run("sync", "--all")
	path := filepath.Join(dir, ".opencode/agents/reviewer.md")
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"secrets/*: deny", "private/*: ask"} {
		if !strings.Contains(string(first), want) {
			t.Errorf("missing %q in %s", want, first)
		}
	}
	run("sync", "--check")
	run("sync", "--dry-run")
	must(t, os.Remove(source))
	run("import", "opencode")
	run("sync")
	run("sync", "--check")
	run("lint")
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("agent changed after import and sync:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	got, err := os.ReadFile(filepath.Join(dir, "opencode.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != project {
		t.Errorf("native project rules changed across import and sync: %s", got)
	}
}
