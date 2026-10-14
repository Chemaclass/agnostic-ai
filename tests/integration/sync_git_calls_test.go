package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A no-op sync reads the Git layout once and asks about each file once.
// 0.83.0 started no Git process for sync --check; 0.84.0-rc.1 started 18.
func TestSync_NoOpStartsFewGitProcesses(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Git shim is a shell script")
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}
	repository, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "agnostic-ai")
	build := exec.Command("go", "build", "-o", bin, "./cmd/agnostic-ai")
	build.Dir = repository
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v: %s", err, output)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("AGNOSTIC_AI_HOME", filepath.Join(home, ".agnostic-ai"))
	dir := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte("version: 1\nbuiltins: [handoff, memory]\ntargets: [claude, codex, cursor]\ngitignore:\n  enabled: true\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), []byte("# demo\n"), 0o644))
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false", "commit", "-qm", "init"}} {
		git := exec.Command(realGit, args...)
		git.Dir = dir
		if output, err := git.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	shims := t.TempDir()
	log := filepath.Join(shims, "calls")
	shim := "#!/bin/sh\necho \"$*\" >> '" + log + "'\nexec '" + realGit + "' \"$@\"\n"
	must(t, os.WriteFile(filepath.Join(shims, "git"), []byte(shim), 0o755))
	t.Setenv("PATH", shims+string(os.PathListSeparator)+os.Getenv("PATH"))

	run := func(args ...string) []string {
		must(t, os.WriteFile(log, nil, 0o644))
		cmd := exec.Command(bin, args...)
		cmd.Dir = dir
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v: %s", args, err, output)
		}
		data, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Split(strings.TrimSpace(string(data)), "\n")
	}
	run("sync", "-q")
	for _, tc := range []struct {
		args []string
		max  int
	}{{[]string{"sync", "--check"}, 3}, {[]string{"sync", "-q"}, 6}} {
		if calls := run(tc.args...); len(calls) > tc.max {
			t.Errorf("%v started %d git processes, want at most %d:\n%s", tc.args, len(calls), tc.max, strings.Join(calls, "\n"))
		}
	}
}
