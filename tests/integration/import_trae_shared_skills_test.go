package integration

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportTrae_SharedSkillsRoundTrip(t *testing.T) {
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
	project := t.TempDir()
	testutil.Chdir(t, project)
	write := func(path, body string) {
		t.Helper()
		must(t, os.MkdirAll(filepath.Dir(path), 0o755))
		must(t, os.WriteFile(path, []byte(body), 0o644))
	}
	write("agnostic-ai.yaml", "version: 1\ntargets: [trae]\ngitignore:\n  enabled: false\n")
	const shared = "---\nname: deploy\ndescription: Deploy the service.\n---\nRead references/checks.txt.\n"
	const asset = "Check the service status.\n"
	write(".agents/skills/deploy/SKILL.md", shared)
	write(".agents/skills/deploy/references/checks.txt", asset)
	const native = "---\nname: review\ndescription: Review changes.\n---\nNative review guidance.\n"
	write(".trae/skills/review/SKILL.md", native)
	write(".agents/skills/review/SKILL.md", strings.ReplaceAll(native, "Native", "Shared"))
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = project
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	if out := run("import", "trae"); !strings.Contains(out, "2 skills") {
		t.Errorf("import summary:\n%s", out)
	}
	for path, want := range map[string]string{
		".agnostic-ai/skills/deploy/SKILL.md":              shared,
		".agnostic-ai/skills/deploy/references/checks.txt": asset,
		".agnostic-ai/skills/review/SKILL.md":              native,
	} {
		got, err := os.ReadFile(path)
		must(t, err)
		if string(got) != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
	run("sync")
	paths := []string{".trae/skills/deploy/SKILL.md", ".trae/skills/deploy/references/checks.txt", ".trae/skills/review/SKILL.md"}
	before := map[string][]byte{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		must(t, err)
		before[path] = data
	}
	run("sync", "--check")
	run("import", "trae")
	run("sync")
	for _, path := range paths {
		got, err := os.ReadFile(path)
		must(t, err)
		if !bytes.Equal(got, before[path]) {
			t.Errorf("%s changed on re-import:\nbefore: %s\nafter: %s", path, before[path], got)
		}
	}
	run("sync", "--check")
}
