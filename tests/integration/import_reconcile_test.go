package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestImportReconcile_ReportsCommittedChangesWithoutApplying(t *testing.T) {
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
	dir := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "missing-config"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(name, content string) {
		t.Helper()
		file := filepath.Join(dir, name)
		must(t, os.MkdirAll(filepath.Dir(file), 0o755))
		must(t, os.WriteFile(file, []byte(content), 0o644))
	}
	git("init", "-q")
	write("agnostic-ai.yaml", "version: 1\ntargets: [cursor]\nsources:\n  skills: specs/skills\n")
	write("native/example/SKILL.md", "original")
	git("add", ".")
	git("commit", "-qm", "base")
	base := git("rev-parse", "HEAD")
	must(t, os.Mkdir(filepath.Join(dir, "specs"), 0o755))
	must(t, os.Rename(filepath.Join(dir, "native"), filepath.Join(dir, "specs/skills")))
	git("add", "-A")
	git("commit", "-qm", "migrate")
	migrated := git("rev-parse", "HEAD")
	git("checkout", "-q", base)
	write("native/example/SKILL.md", "updated")
	git("add", ".")
	git("commit", "-qm", "upstream")
	upstream := git("rev-parse", "HEAD")
	git("checkout", "-q", migrated)
	cmd := exec.Command(binary, "import", "reconcile", "--base", base, "--migrated", migrated, "--upstream", upstream, "--map", "native=specs/skills", "--json")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	var plan struct {
		Entries []struct{ Action, Source, Destination string }
	}
	if err := json.Unmarshal(out, &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Entries) != 1 || plan.Entries[0].Action != "update" || plan.Entries[0].Destination != "specs/skills/example/SKILL.md" {
		t.Errorf("plan: %s", out)
	}
	if status := git("status", "--porcelain"); status != "" {
		t.Errorf("changed project: %s", status)
	}
	data, err := os.ReadFile(filepath.Join(dir, "specs/skills/example/SKILL.md"))
	must(t, err)
	if string(data) != "original" {
		t.Errorf("applied update: %q", data)
	}
}
