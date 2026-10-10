package cli

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// isolateGit points git at an empty user config, so a developer's
// core.hooksPath or commit signing cannot leak into a test. It returns
// the user config path.
func isolateGit(t *testing.T) string {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(cfg, []byte("[user]\n\tname = test\n\temail = test@example.com\n[commit]\n\tgpgsign = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	return cfg
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func setupGitRepo(t *testing.T) string {
	t.Helper()
	isolateGit(t)
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	return dir
}

func readHook(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read hook: %v", err)
	}
	return string(data)
}

func TestInstallHook_CreatesPreCommit(t *testing.T) {
	dir := setupGitRepo(t)

	var buf strings.Builder
	if err := installPreCommitHook(dir, false, &buf, io.Discard); err != nil {
		t.Fatalf("installPreCommitHook: %v", err)
	}

	got := readHook(t, filepath.Join(dir, ".git", "hooks", "pre-commit"))
	for _, want := range []string{"#!/bin/sh\n", "# agnostic-ai install-hook\n", "agnostic-ai project --check --against index || exit 1\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("hook missing %q, got:\n%s", want, got)
		}
	}
}

func TestInstallHook_AppendsToExisting(t *testing.T) {
	for name, existing := range map[string]string{
		"plain":                 "#!/bin/sh\necho 'existing hook'\n",
		"env bash":              "#!/usr/bin/env bash\necho 'existing hook'\n",
		"redirecting exec":      "#!/bin/sh\nexec 1>&2\necho 'existing hook'\n",
		"exit in a branch":      "#!/bin/sh\nif false; then\n\texit 1\nfi\n",
		"comment names a check": "#!/bin/sh\n# we used to run agnostic-ai sync --check here\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := setupGitRepo(t)
			hookPath := filepath.Join(dir, ".git", "hooks", "pre-commit")
			if err := os.WriteFile(hookPath, []byte(existing), 0o755); err != nil {
				t.Fatal(err)
			}

			var out strings.Builder
			if err := installPreCommitHook(dir, false, &out, io.Discard); err != nil {
				t.Fatal(err)
			}

			got := readHook(t, hookPath)
			if !strings.HasPrefix(got, existing) || !strings.HasSuffix(got, projectHook.text()) {
				t.Errorf("want the checks appended after the existing content, got:\n%s", got)
			}
			if !strings.Contains(out.String(), "appended the checks to "+hookPath) {
				t.Errorf("want the append reported, got %q", out.String())
			}
		})
	}
}

func TestInstallHook_RefusesHooksThatNeverReachTheEnd(t *testing.T) {
	for name, existing := range map[string]string{
		"python":        "#!/usr/bin/env python3\nprint('hi')\n",
		"no shebang":    "echo hi\n",
		"exit 0":        "#!/bin/sh\necho hi\nexit 0\n",
		"pre-commit":    "#!/usr/bin/env bash\nif [ -x \"$PY\" ]; then\n    exec \"$PY\" -mpre_commit\nelse\n    exec pre-commit\nfi\n",
		"exec at start": "#!/bin/sh\nexec lefthook run pre-commit\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := setupGitRepo(t)
			hookPath := filepath.Join(dir, ".git", "hooks", "pre-commit")
			if err := os.WriteFile(hookPath, []byte(existing), 0o755); err != nil {
				t.Fatal(err)
			}

			err := installPreCommitHook(dir, false, io.Discard, io.Discard)
			if err == nil || !strings.Contains(err.Error(), "would never run") || !strings.Contains(err.Error(), projectHook.checks) {
				t.Fatalf("want a refusal naming the lines to add, got %v", err)
			}
			if got := readHook(t, hookPath); got != existing {
				t.Errorf("refused hook changed:\n%s", got)
			}
		})
	}
}

func TestInstallHook_Idempotent(t *testing.T) {
	dir := setupGitRepo(t)

	for i := 0; i < 3; i++ {
		if err := installPreCommitHook(dir, false, io.Discard, io.Discard); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}

	got := readHook(t, filepath.Join(dir, ".git", "hooks", "pre-commit"))
	if count := strings.Count(got, "agnostic-ai project --check"); count != 1 {
		t.Errorf("expected 1 occurrence of sync --check, got %d:\n%s", count, got)
	}
}

// A hook an older version wrote checked the working tree, which passed
// a commit that left regenerated outputs unstaged (#1592). Installing
// again moves it to the staged state.
func TestInstallHook_UpdatesAHookFromAnOlderVersion(t *testing.T) {
	for _, old := range []string{"agnostic-ai sync --check", "# agnostic-ai install-hook\nagnostic-ai sync --check || exit 1"} {
		dir := setupGitRepo(t)
		hookPath := filepath.Join(dir, ".git", "hooks", "pre-commit")
		if err := os.WriteFile(hookPath, []byte("#!/bin/sh\n"+old+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}

		var out strings.Builder
		if err := installPreCommitHook(dir, false, &out, io.Discard); err != nil {
			t.Fatal(err)
		}
		got := readHook(t, hookPath)
		if !strings.Contains(got, "agnostic-ai project --check --against index || exit 1\n") || strings.Count(got, "project --check") != 1 {
			t.Errorf("hook from an older version not updated:\n%s", got)
		}
		if !strings.Contains(out.String(), "updated the checks") {
			t.Errorf("want the update reported, got %q", out.String())
		}
	}
}

func TestInstallHook_Shared_CreatesGithooksDir(t *testing.T) {
	dir := setupGitRepo(t)

	if err := installPreCommitHook(dir, true, io.Discard, io.Discard); err != nil {
		t.Fatalf("shared install: %v", err)
	}

	got := readHook(t, filepath.Join(dir, ".githooks", "pre-commit"))
	if !strings.Contains(got, "agnostic-ai project --check") {
		t.Errorf("hook missing sync --check, got:\n%s", got)
	}
	if hooksPath := git(t, dir, "config", "core.hooksPath"); hooksPath != ".githooks" {
		t.Errorf("core.hooksPath = %q, want .githooks", hooksPath)
	}
}

func TestInstallHook_SharedFlag(t *testing.T) {
	dir := setupGitRepo(t)
	testutil.Chdir(t, dir)

	if _, err := runInstallHook("--shared"); err != nil {
		t.Fatalf("install-hook --shared: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".githooks", "pre-commit")); err != nil {
		t.Errorf(".githooks/pre-commit not created: %v", err)
	}
	if hooksPath := git(t, dir, "config", "core.hooksPath"); hooksPath != ".githooks" {
		t.Errorf("core.hooksPath = %q, want .githooks", hooksPath)
	}
}

func TestInstallHook_Shared_WritesAtTheWorktreeRoot(t *testing.T) {
	dir := setupGitRepo(t)
	sub := filepath.Join(dir, "pkg", "inner")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := installPreCommitHook(sub, true, io.Discard, io.Discard); err != nil {
		t.Fatalf("shared install from a subdirectory: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".githooks", "pre-commit")); err != nil {
		t.Errorf("want the hook at the worktree root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sub, ".githooks")); !os.IsNotExist(err) {
		t.Errorf("wrote .githooks in the subdirectory: %v", err)
	}
	if hooksPath := git(t, dir, "config", "core.hooksPath"); hooksPath != ".githooks" {
		t.Errorf("core.hooksPath = %q, want .githooks", hooksPath)
	}
}

func TestInstallHook_Shared_RefusesAnotherHooksPath(t *testing.T) {
	dir := setupGitRepo(t)
	git(t, dir, "config", "core.hooksPath", ".husky")

	err := installPreCommitHook(dir, true, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "core.hooksPath is already .husky (set in .git/config)") {
		t.Fatalf("want the hooksPath refusal, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".githooks")); !os.IsNotExist(err) {
		t.Errorf("refused install wrote .githooks: %v", err)
	}
	if hooksPath := git(t, dir, "config", "core.hooksPath"); hooksPath != ".husky" {
		t.Errorf("core.hooksPath = %q, want .husky kept", hooksPath)
	}
}

func TestInstallHook_Shared_WarnsAboutHooksThatStopRunning(t *testing.T) {
	dir := setupGitRepo(t)
	commitMsg := filepath.Join(dir, ".git", "hooks", "commit-msg")
	if err := os.WriteFile(commitMsg, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	var warn strings.Builder
	if err := installPreCommitHook(dir, true, io.Discard, &warn); err != nil {
		t.Fatal(err)
	}
	got := warn.String()
	// Git reports the long path where t.TempDir may hold an 8.3 short name (Windows).
	named := filepath.Join(filepath.Base(dir), ".git", "hooks", "commit-msg")
	if !strings.Contains(got, "git no longer runs these hooks") || !strings.Contains(got, named) {
		t.Errorf("want a warning naming %s, got %q", named, got)
	}
	if strings.Contains(got, ".sample") {
		t.Errorf("warning lists sample hooks: %q", got)
	}

	warn.Reset()
	if err := installPreCommitHook(dir, true, io.Discard, &warn); err != nil {
		t.Fatal(err)
	}
	if warn.Len() != 0 {
		t.Errorf("rerun warns again: %q", warn.String())
	}
}

func TestInstallHook_Shared_OutsideGitWritesNothing(t *testing.T) {
	isolateGit(t)
	dir := t.TempDir()

	err := installPreCommitHook(dir, true, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "is not inside a git work tree") {
		t.Fatalf("want the git refusal, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".githooks")); !os.IsNotExist(err) {
		t.Errorf("wrote .githooks outside git: %v", err)
	}
}
