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

func TestImportWindsurf_RemovedTriggersLiftOnlyWindsurfRestriction(t *testing.T) {
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
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = project
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	write("agnostic-ai.yaml", "version: 1\ntargets: [windsurf, codex]\noutputs:\n  windsurf:\n    skills-dir: .devin/skills\n")
	write(".agnostic-ai/skills/ship/SKILL.md", "---\nname: ship\ndescription: Ships.\ndisable-model-invocation: true\n---\nShip it.\n")
	run("sync")
	const native = ".devin/skills/ship/SKILL.md"
	data, err := os.ReadFile(native)
	must(t, err)
	if !strings.Contains(string(data), "triggers:\n  - user\n") {
		t.Fatalf("missing initial restriction:\n%s", data)
	}
	write(native, strings.Replace(string(data), "triggers:\n  - user\n", "", 1))
	run("import", "windsurf", "--overwrite")
	run("sync")
	assertAbsent(t, native, "triggers:")
	assertContains(t, ".agents/skills/ship/agents/openai.yaml", "allow_implicit_invocation: false")
	run("sync", "--check")
	run("lint")
}
