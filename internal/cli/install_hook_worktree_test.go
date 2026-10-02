package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// linkedWorktree makes main a git repository with one commit, adds a
// linked worktree on a new branch, and returns both paths.
func linkedWorktree(t *testing.T) (main, linked string) {
	t.Helper()
	main = setupGitRepo(t)
	git(t, main, "commit", "-q", "--allow-empty", "-m", "base")
	linked = filepath.Join(t.TempDir(), "linked")
	git(t, main, "worktree", "add", "-q", "-b", "feature", linked)
	return main, linked
}

// `--shared` run from a linked worktree must still land the hook where
// every worktree's core.hooksPath resolution actually reads it: the
// main worktree, not the invoking one. Writing it under the linked
// worktree's own toplevel would leave every other worktree (main
// included) with a hooksPath pointing at nothing (review finding 1).
func TestInstallHook_SharedFromLinkedWorktreeWritesUnderMainToplevel(t *testing.T) {
	main, linked := linkedWorktree(t)

	if err := installHook(linked, projectHook, true, io.Discard, io.Discard); err != nil {
		t.Fatalf("install-hook --shared from a linked worktree: %v", err)
	}

	mainHook := filepath.Join(main, sharedHooksPath, "pre-commit")
	if _, err := os.Stat(mainHook); err != nil {
		t.Errorf("expected the shared hook under the main worktree at %s: %v", mainHook, err)
	}
	if _, err := os.Stat(filepath.Join(linked, sharedHooksPath, "pre-commit")); err == nil {
		t.Error("the shared hook must not be written under the invoking (linked) worktree's own directory")
	}
	if got := git(t, linked, "config", "core.hooksPath"); got != sharedHooksPath {
		t.Errorf("core.hooksPath = %q, want %q", got, sharedHooksPath)
	}
	if got := git(t, main, "config", "core.hooksPath"); got != sharedHooksPath {
		t.Errorf("main worktree core.hooksPath = %q, want %q (it is a shared, repository-level setting)", got, sharedHooksPath)
	}
}

// Running `--shared` again from the main worktree, after installing it
// from a linked one, must see the hook as already installed rather than
// writing a second, scattered copy.
func TestInstallHook_SharedIsIdempotentAcrossWorktrees(t *testing.T) {
	main, linked := linkedWorktree(t)
	if err := installHook(linked, projectHook, true, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	if err := installHook(main, projectHook, true, &out, io.Discard); err != nil {
		t.Fatalf("install-hook --shared from the main worktree: %v", err)
	}
	if !strings.Contains(out.String(), "already runs the checks") {
		t.Errorf("want the rerun reported as a no-op, got %q", out.String())
	}
}

// Plain (non-shared) `install-hook` from a linked worktree must find the
// repository's real hooks directory, the common dir's, through git
// rather than a filesystem walk for a `.git` directory: a linked
// worktree's `.git` is a file naming the common dir, not a directory
// (review finding 2).
func TestInstallHook_LocalFromLinkedWorktreeWritesToTheCommonHooksDir(t *testing.T) {
	main, linked := linkedWorktree(t)

	if err := installPreCommitHook(linked, false, io.Discard, io.Discard); err != nil {
		t.Fatalf("install-hook from a linked worktree: %v", err)
	}

	got := readHook(t, filepath.Join(main, ".git", "hooks", "pre-commit"))
	if !strings.Contains(got, "agnostic-ai sync --check --against index || exit 1") {
		t.Errorf("hook missing the check, got:\n%s", got)
	}
}

// Same, for --post-checkout, since it shares the same local-install path.
func TestInstallHookPostCheckout_LocalFromLinkedWorktreeWritesToTheCommonHooksDir(t *testing.T) {
	main, linked := linkedWorktree(t)

	if err := installHook(linked, postCheckoutHook, false, io.Discard, io.Discard); err != nil {
		t.Fatalf("install-hook --post-checkout from a linked worktree: %v", err)
	}

	got := readHook(t, filepath.Join(main, ".git", "hooks", "post-checkout"))
	if !strings.Contains(got, "agnostic-ai sync -q") {
		t.Errorf("hook missing the sync call, got:\n%s", got)
	}
}
