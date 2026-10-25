package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A built-in turned on or off only in agnostic-ai.local.yaml never
// changes a committed file: its extra outputs go to info/exclude.
func TestSync_PersonalBuiltinsLeaveCommittedIgnoresAlone(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	repository, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	name := "agnostic-ai"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(t.TempDir(), name)
	build := exec.Command("go", "build", "-o", bin, "./cmd/agnostic-ai")
	build.Dir = repository
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v: %s", err, output)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("AGNOSTIC_AI_HOME", filepath.Join(home, ".agnostic-ai"))

	run := func(t *testing.T, dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	git := func(t *testing.T, dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false", "-c", "core.autocrlf=false"}, args...)...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	project := func(t *testing.T, builtins string) string {
		t.Helper()
		dir := t.TempDir()
		must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai"), 0o755))
		must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte("version: 1\ntargets: [claude, codex]\nbuiltins: "+builtins+"\ngitignore:\n  enabled: true\n"), 0o644))
		must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), []byte("# demo\n"), 0o644))
		git(t, dir, "init", "-q")
		run(t, dir, "sync")
		git(t, dir, "add", "-A")
		git(t, dir, "commit", "-qm", "init")
		return dir
	}
	clean := func(t *testing.T, dir, step string) {
		t.Helper()
		if status := git(t, dir, "status", "--porcelain", "--untracked-files=all"); status != "" {
			t.Fatalf("%s: git status is not clean:\n%s", step, status)
		}
		run(t, dir, "sync", "--check")
	}
	exclude := func(t *testing.T, dir string) string {
		t.Helper()
		common := strings.TrimSpace(git(t, dir, "rev-parse", "--path-format=absolute", "--git-common-dir"))
		data, err := os.ReadFile(filepath.Join(common, "info", "exclude"))
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		return string(data)
	}
	ignoredBy := func(t *testing.T, dir, path string) string {
		t.Helper()
		return strings.TrimSpace(git(t, dir, "check-ignore", "-v", path))
	}

	t.Run("turning a team built-in off keeps the committed block", func(t *testing.T) {
		dir := project(t, "[handoff]")
		must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.local.yaml"), []byte("builtins: []\n"), 0o644))
		run(t, dir, "sync")
		clean(t, dir, "personal off")
		if _, err := os.Stat(filepath.Join(dir, ".claude", "skills", "handoff", "SKILL.md")); !os.IsNotExist(err) {
			t.Errorf("handoff skill still on disk: %v", err)
		}
		if got := exclude(t, dir); strings.Contains(got, "agnostic-ai (personal)") {
			t.Errorf("info/exclude has a personal block for removed outputs:\n%s", got)
		}
	})

	t.Run("turning a built-in on only for me ignores it through info/exclude", func(t *testing.T) {
		dir := project(t, "[]")
		must(t, os.WriteFile(filepath.Join(dir, ".git", "info", "exclude"), []byte("# mine\n/notes.txt\n"), 0o644))
		must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.local.yaml"), []byte("builtins: [handoff]\n"), 0o644))
		run(t, dir, "sync")
		skill := ".claude/skills/handoff/SKILL.md"
		if _, err := os.Stat(filepath.Join(dir, skill)); err != nil {
			t.Fatalf("handoff skill not written: %v", err)
		}
		clean(t, dir, "personal on")
		if by := ignoredBy(t, dir, skill); !strings.Contains(by, "info/exclude") {
			t.Errorf("%s is ignored by %q, want info/exclude", skill, by)
		}

		must(t, os.Remove(filepath.Join(dir, "agnostic-ai.local.yaml")))
		run(t, dir, "sync")
		clean(t, dir, "personal file removed")
		if got := exclude(t, dir); got != "# mine\n/notes.txt\n" {
			t.Errorf("info/exclude after removing the personal file = %q, want only the user's lines", got)
		}
	})

	t.Run("a linked worktree writes the common info/exclude", func(t *testing.T) {
		main := project(t, "[]")
		worktree := filepath.Join(t.TempDir(), "wt")
		git(t, main, "worktree", "add", "-q", worktree)
		must(t, os.WriteFile(filepath.Join(worktree, "agnostic-ai.local.yaml"), []byte("builtins: [handoff]\n"), 0o644))
		run(t, worktree, "sync")
		clean(t, worktree, "worktree personal on")
		if by := ignoredBy(t, worktree, ".claude/skills/handoff/SKILL.md"); !strings.Contains(by, "info/exclude") {
			t.Errorf("worktree skill is ignored by %q, want info/exclude", by)
		}
		clean(t, main, "main checkout after worktree sync")
	})
}
