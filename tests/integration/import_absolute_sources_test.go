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

func TestImportClaude_AbsoluteSourcesPreviewAndSync(t *testing.T) {
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
	project := t.TempDir()
	testutil.Chdir(t, project)
	source := filepath.Join(t.TempDir(), "portable", "skills")
	path := filepath.Join(source, "review", "SKILL.md")
	native := filepath.Join(project, ".claude", "skills", "review", "SKILL.md")
	const skill = "---\nname: review\ndescription: Review changes.\n---\nNative guidance.\n"
	must(t, os.WriteFile("agnostic-ai.yaml", []byte("version: 1\ntargets: [claude]\nsources:\n  skills: "+filepath.ToSlash(source)+"\n"), 0o644))
	must(t, os.MkdirAll(filepath.Dir(native), 0o755))
	must(t, os.WriteFile(native, []byte(skill), 0o644))
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
	for _, flags := range [][]string{{"--dry-run"}, {"--dry-run", "--diff"}} {
		out := run(append([]string{"import", "claude"}, flags...)...)
		if !strings.Contains(filepath.ToSlash(out), filepath.ToSlash(path)) {
			t.Errorf("preview lacks absolute destination %s:\n%s", path, out)
		}
		if _, err := os.Stat(source); !os.IsNotExist(err) {
			t.Errorf("preview wrote absolute source: %v", err)
		}
		if _, err := os.Stat(".agnostic-ai"); !os.IsNotExist(err) {
			t.Errorf("preview wrote project sources or state: %v", err)
		}
	}
	run("import", "claude")
	assertContains(t, path, "Native guidance.")
	mirror := filepath.Join(project, strings.TrimLeft(strings.TrimPrefix(source, filepath.VolumeName(source)), `/\`))
	if _, err := os.Stat(mirror); !os.IsNotExist(err) {
		t.Errorf("import created a project mirror of its absolute source: %v", err)
	}
	must(t, os.WriteFile(path, []byte(strings.ReplaceAll(skill, "Native", "Portable")), 0o644))
	run("sync")
	assertContains(t, native, "Portable guidance.")
	run("sync", "--check")
}
