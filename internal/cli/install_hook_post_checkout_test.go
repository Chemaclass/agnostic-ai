package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func postCheckoutHookPath(dir string) string {
	return filepath.Join(dir, ".git", "hooks", "post-checkout")
}

func TestInstallHookPostCheckout_CreatesHook(t *testing.T) {
	dir := setupGitRepo(t)
	testutil.Chdir(t, dir)

	out, err := runInstallHook("--post-checkout")
	if err != nil {
		t.Fatalf("install-hook --post-checkout: %v", err)
	}
	if !strings.Contains(out, "installed") {
		t.Errorf("want the install reported, got %q", out)
	}

	got := readHook(t, postCheckoutHookPath(dir))
	for _, want := range []string{
		"#!/bin/sh\n",
		"# agnostic-ai install-hook --post-checkout\n",
		`[ "$3" = "1" ] || exit 0`,
		"agnostic-ai.yaml",
		"agnostic-ai sync -q",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("hook missing %q, got:\n%s", want, got)
		}
	}
}

func TestInstallHookPostCheckout_Shared(t *testing.T) {
	dir := setupGitRepo(t)
	testutil.Chdir(t, dir)

	if _, err := runInstallHook("--post-checkout", "--shared"); err != nil {
		t.Fatalf("install-hook --post-checkout --shared: %v", err)
	}

	got := readHook(t, filepath.Join(dir, sharedHooksPath, "post-checkout"))
	if !strings.Contains(got, "agnostic-ai sync -q") {
		t.Errorf("shared hook missing the sync call, got:\n%s", got)
	}
	if value := git(t, dir, "config", "core.hooksPath"); value != sharedHooksPath {
		t.Errorf("core.hooksPath = %q, want %q", value, sharedHooksPath)
	}
}

func TestInstallHookPostCheckout_RejectsGlobal(t *testing.T) {
	dir := setupGitRepo(t)
	testutil.Chdir(t, dir)

	if _, err := runInstallHook("--post-checkout", "--global"); err == nil {
		t.Error("--post-checkout with --global must be rejected")
	}
}

// fakeAgnosticAI writes a stub `agnostic-ai` script on a fresh PATH
// entry that appends "<args> cwd=<dir>" to a log file, mirroring the
// fake binary TestInstallHookGlobal_CommitsRunTheChecks uses to prove a
// hook's checks actually run, without needing the real CLI.
func fakeAgnosticAI(t *testing.T) (bin, log string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake agnostic-ai is a sh script")
	}
	bin = t.TempDir()
	log = filepath.Join(t.TempDir(), "calls.log")
	fake := "#!/bin/sh\n" + `echo "$* cwd=$(pwd -P)" >> "` + log + "\"\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "agnostic-ai"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, log
}

// runGitWithFakeCLI runs a git command with the fake agnostic-ai on PATH.
func runGitWithFakeCLI(t *testing.T, dir, bin string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// A branch checkout runs the hook, which runs `agnostic-ai sync -q` from
// the worktree root; a single-file checkout does not (#1330).
func TestInstallHookPostCheckout_RunsSyncOnBranchCheckoutOnly(t *testing.T) {
	dir := setupGitRepo(t)
	testutil.Chdir(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "base")
	git(t, dir, "branch", "other")
	if _, err := runInstallHook("--post-checkout"); err != nil {
		t.Fatal(err)
	}
	bin, log := fakeAgnosticAI(t)

	// A single-file checkout: the hook must not run sync.
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := runGitWithFakeCLI(t, dir, bin, "checkout", "--", "a.txt"); err != nil {
		t.Fatalf("git checkout -- a.txt: %v\n%s", err, out)
	}
	if calls, _ := os.ReadFile(log); len(calls) != 0 {
		t.Errorf("a file checkout must not run sync, got calls:\n%s", calls)
	}

	// A branch checkout: the hook must run sync -q from the repo root.
	if out, err := runGitWithFakeCLI(t, dir, bin, "checkout", "-q", "other"); err != nil {
		t.Fatalf("git checkout other: %v\n%s", err, out)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("read call log: %v", err)
	}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(calls)); got != "sync -q cwd="+root {
		t.Errorf("calls = %q, want %q", got, "sync -q cwd="+root)
	}
}

// After `git worktree add`, the new worktree's post-checkout hook (shared
// through the common git dir) runs sync -q from the new worktree's own
// root, not the main checkout's.
func TestInstallHookPostCheckout_WorktreeAddRunsSyncInTheNewWorktree(t *testing.T) {
	dir := setupGitRepo(t)
	testutil.Chdir(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "base")
	if _, err := runInstallHook("--post-checkout"); err != nil {
		t.Fatal(err)
	}
	bin, log := fakeAgnosticAI(t)

	worktree := filepath.Join(t.TempDir(), "feature")
	if out, err := runGitWithFakeCLI(t, dir, bin, "worktree", "add", "-q", "-b", "feature", worktree); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}

	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("read call log: %v", err)
	}
	root, err := filepath.EvalSymlinks(worktree)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(calls)); got != "sync -q cwd="+root {
		t.Errorf("calls = %q, want %q (the new worktree's own root)", got, "sync -q cwd="+root)
	}
}

// Without the binary on PATH, or without agnostic-ai.yaml, the hook
// exits 0 rather than failing the checkout.
func TestInstallHookPostCheckout_SkipsSilentlyWhenNotReady(t *testing.T) {
	dir := setupGitRepo(t)
	testutil.Chdir(t, dir)
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "base")
	git(t, dir, "branch", "other")
	if _, err := runInstallHook("--post-checkout"); err != nil {
		t.Fatal(err)
	}

	// No agnostic-ai.yaml, and no agnostic-ai on PATH: checkout must
	// still succeed. git() itself fails the test on a non-zero exit.
	git(t, dir, "checkout", "-q", "other")
	git(t, dir, "checkout", "-q", "-")

	// agnostic-ai on PATH, but still no config: checkout must succeed
	// and the stub must not be called.
	bin, log := fakeAgnosticAI(t)
	if out, err := runGitWithFakeCLI(t, dir, bin, "checkout", "-q", "other"); err != nil {
		t.Fatalf("checkout with the binary but no config: %v\n%s", err, out)
	}
	if calls, _ := os.ReadFile(log); len(calls) != 0 {
		t.Errorf("without agnostic-ai.yaml, sync must not run, got calls:\n%s", calls)
	}
}

func TestInstallHookPostCheckout_KeepsManualHooksAndInstallsBothOnce(t *testing.T) {
	for _, mode := range []string{"local", "shared", "linked"} {
		t.Run(mode, func(t *testing.T) {
			root := setupGitRepo(t)
			dir := root
			hooks := filepath.Join(root, ".git", "hooks")
			args := []string{"--post-checkout"}
			if mode == "shared" {
				hooks = filepath.Join(root, sharedHooksPath)
				args = append(args, "--shared")
			}
			if mode == "linked" {
				git(t, root, "commit", "-q", "--allow-empty", "-m", "base")
				dir = filepath.Join(t.TempDir(), "linked")
				git(t, root, "worktree", "add", "-q", "-b", "linked", dir)
			}
			testutil.Chdir(t, dir)
			if err := os.MkdirAll(hooks, 0o755); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"post-checkout", "post-merge"} {
				if err := os.WriteFile(filepath.Join(hooks, name), []byte("#!/bin/sh\necho manual-"+name+"\n"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			for range 2 {
				if _, err := runInstallHook(args...); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range []string{"post-checkout", "post-merge"} {
				got := readHook(t, filepath.Join(hooks, name))
				if !strings.Contains(got, "echo manual-"+name) || strings.Count(got, "agnostic-ai sync -q") != 1 {
					t.Errorf("%s lost manual content or lacks one sync: %s", name, got)
				}
			}
		})
	}
}

func TestInstallHookPostCheckout_RefusesOtherHooksPathForBothHooks(t *testing.T) {
	dir := setupGitRepo(t)
	testutil.Chdir(t, dir)
	custom := filepath.Join(t.TempDir(), "custom-hooks")
	if err := os.Mkdir(custom, 0o755); err != nil {
		t.Fatal(err)
	}
	manual := "#!/bin/sh\necho manual\n"
	if err := os.WriteFile(filepath.Join(custom, "post-merge"), []byte(manual), 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "config", "core.hooksPath", custom)
	if _, err := runInstallHook("--post-checkout"); err == nil || !strings.Contains(err.Error(), "core.hooksPath") {
		t.Errorf("want hooksPath refusal, got %v", err)
	}
	if _, err := runInstallHook("--post-checkout", "--shared"); err == nil || !strings.Contains(err.Error(), "core.hooksPath") {
		t.Errorf("want shared hooksPath refusal, got %v", err)
	}
	if got := readHook(t, filepath.Join(custom, "post-merge")); got != manual {
		t.Errorf("custom hook changed: %s", got)
	}
	for _, name := range []string{"post-checkout", "post-merge"} {
		if _, err := os.Stat(filepath.Join(dir, ".git", "hooks", name)); !os.IsNotExist(err) {
			t.Errorf("inactive local %s written: %v", name, err)
		}
	}
}

func TestInstallHookPostMerge_SkipsMissingBinaryOrConfig(t *testing.T) {
	dir := setupGitRepo(t)
	testutil.Chdir(t, dir)
	if _, err := runInstallHook("--post-checkout"); err != nil {
		t.Fatal(err)
	}
	bin, log := fakeAgnosticAI(t)
	hook := filepath.Join(dir, ".git", "hooks", "post-merge")
	run := func(path string) {
		t.Helper()
		cmd := exec.Command("sh", hook, "0")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "PATH="+path)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("post-merge: %v %s", err, out)
		}
	}
	run(bin + string(os.PathListSeparator) + os.Getenv("PATH"))
	if calls, _ := os.ReadFile(log); len(calls) != 0 {
		t.Errorf("missing config ran sync: %s", calls)
	}
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\n")
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	gitOnly := t.TempDir()
	if err := os.Symlink(realGit, filepath.Join(gitOnly, "git")); err != nil {
		t.Fatal(err)
	}
	run(gitOnly)
	if calls, _ := os.ReadFile(log); len(calls) != 0 {
		t.Errorf("missing binary ran sync: %s", calls)
	}
	run(bin + string(os.PathListSeparator) + os.Getenv("PATH"))
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(calls)) != "sync -q cwd="+canonical {
		t.Errorf("post-merge did not sync from root: %s", calls)
	}
}
