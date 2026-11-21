package mailbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/gitquery"
)

func TestResolveReviewRefs_PinsTheWorktreeAndBothRenamePaths(t *testing.T) {
	repo, base := newReviewRepo(t)
	reviewCommit(t, repo, "head.txt")
	reviewGitFixture(t, repo, "mv", "base.txt", "renamed.txt")
	reviewGitFixture(t, repo, "commit", "-m", "rename")
	head := reviewGitFixture(t, repo, "rev-parse", "HEAD")
	sub := filepath.Join(repo, "subfolder")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	reviewGitFixture(t, repo, "mv", "renamed.txt", "dirty-rename.txt")
	writeReviewFile(t, repo, "head.txt", "staged\n")
	reviewGitFixture(t, repo, "add", "head.txt")
	writeReviewFile(t, repo, "head.txt", "unstaged\n")
	writeReviewFile(t, repo, "untracked.txt", "only in the worktree\n")

	refs, err := ResolveReviewRefs(context.Background(), sub, base, "")
	if err != nil {
		t.Fatal(err)
	}
	wantRoot := filepath.Clean(reviewGitFixture(t, repo, "rev-parse", "--show-toplevel"))
	if refs.Worktree != wantRoot || refs.Commit != head || refs.Base != base || refs.Branch != "main" || refs.Uncommitted != 3 {
		t.Errorf("review refs = %+v", refs)
	}
	if want := []string{"base.txt", "head.txt", "renamed.txt"}; !slices.Equal(refs.Paths, want) {
		t.Errorf("paths = %q, want %q", refs.Paths, want)
	}
}

func TestResolveReviewRefs_DefaultAndDetachedLinkedWorktree(t *testing.T) {
	repo, base := newReviewRepo(t)
	head := reviewCommit(t, repo, "changed.txt")
	reviewGitFixture(t, repo, "update-ref", "refs/remotes/origin/main", base)
	reviewGitFixture(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	linked := filepath.Join(t.TempDir(), "linked")
	reviewGitFixture(t, repo, "worktree", "add", "--detach", linked, head)

	refs, err := ResolveReviewRefs(context.Background(), linked, "", "")
	if err != nil {
		t.Fatal(err)
	}
	wantRoot := filepath.Clean(reviewGitFixture(t, linked, "rev-parse", "--show-toplevel"))
	if refs.Worktree != wantRoot || refs.Commit != head || refs.Base != base || refs.Branch != "" {
		t.Errorf("linked review refs = %+v", refs)
	}
}

func TestResolveReviewRefs_RejectsMissingUnknownAndDivergentRevisions(t *testing.T) {
	repo, base := newReviewRepo(t)
	head := reviewCommit(t, repo, "main.txt")
	reviewGitFixture(t, repo, "checkout", "-b", "other", base)
	divergent := reviewCommit(t, repo, "other.txt")
	for _, scenario := range []struct{ name, base, commit string }{
		{"no origin default", "", head},
		{"unknown base", "missing", head},
		{"unknown commit", base, "missing"},
		{"not an ancestor", divergent, head},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			_, err := ResolveReviewRefs(context.Background(), repo, scenario.base, scenario.commit)
			requireGuard(t, err, "refs")
		})
	}
	_, err := ResolveReviewRefs(context.Background(), t.TempDir(), base, "")
	requireGuard(t, err, "refs")
}

func TestResolveReviewRefs_RejectsAnOptionInPlaceOfARevision(t *testing.T) {
	repo, _ := newReviewRepo(t)
	_, err := ResolveReviewRefs(context.Background(), repo, "--quiet", "HEAD")
	requireGuard(t, err, "refs")
	if err != nil && !strings.Contains(err.Error(), "leading dash") {
		t.Errorf("unexpected refusal = %v", err)
	}
}

func TestResolveReviewRefs_RefusesMoreThan200ChangedPaths(t *testing.T) {
	repo, base := newReviewRepo(t)
	for i := range 201 {
		writeReviewFile(t, repo, fmt.Sprintf("part-%03d", i), "new\n")
	}
	reviewGitFixture(t, repo, "add", ".")
	reviewGitFixture(t, repo, "commit", "-m", "many changes")
	_, err := ResolveReviewRefs(context.Background(), repo, base, "")
	requireGuard(t, err, "size")
}

func TestResolveReviewRefs_RefusesSensitiveDerivedPath(t *testing.T) {
	repo, base := newReviewRepo(t)
	reviewCommit(t, repo, "api_key=example-secret-sentinel")
	_, err := ResolveReviewRefs(context.Background(), repo, base, "")
	requireGuard(t, err, "secret")
}

func TestResolveReviewRefs_PreservesWhitespaceInPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows file names cannot contain newlines")
	}
	repo, base := newReviewRepo(t)
	const name = "leading and trailing \n "
	reviewCommit(t, repo, name)
	refs, err := ResolveReviewRefs(context.Background(), repo, base, "")
	if err != nil || !slices.Equal(refs.Paths, []string{name}) {
		t.Errorf("white-space paths = %q, %v", refs.Paths, err)
	}
}

func TestReviewStatus_DisablesConfiguredFiltersForTheReadOnlyCommand(t *testing.T) {
	repo, _ := newReviewRepo(t)
	keys := []string{"filter.accept=spaces.clean", "filter..process", "filter.accept=spaces.required"}
	for _, key := range keys {
		reviewGitFixture(t, repo, "config", key, "true")
	}
	ctx := context.Background()
	overrides, err := reviewFilterOverrides(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		out, err := reviewGit(ctx, repo, append(slices.Clone(overrides), "config", "--get", key)...)
		if got := strings.TrimSpace(string(out)); err != nil || got != "" {
			t.Errorf("effective %s = %q, %v; want disabled", key, got, err)
		}
		if got := reviewGitFixture(t, repo, "config", "--get", key); got != "true" {
			t.Errorf("saved %s = %q; want the original setting", key, got)
		}
	}
}

func newReviewRepo(t *testing.T) (string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git is unavailable")
	}
	root := t.TempDir()
	reviewGitFixture(t, root, "init", "--initial-branch=main", "--quiet")
	return root, reviewCommit(t, root, "base.txt")
}

func reviewCommit(t *testing.T, repo, file string) string {
	t.Helper()
	writeReviewFile(t, repo, file, "committed content\n")
	reviewGitFixture(t, repo, "add", "--", file)
	reviewGitFixture(t, repo, "commit", "-m", "record a change")
	return reviewGitFixture(t, repo, "rev-parse", "HEAD")
}

func writeReviewFile(t *testing.T, repo, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func reviewGitFixture(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{
		"-C", root, "-c", "user.name=Mailbox Test", "-c", "user.email=mailbox@example.invalid",
		"-c", "commit.gpgsign=false", "-c", "init.templateDir=", "-c", "core.hooksPath=.test-hooks",
	}, args...)...)
	cmd.Env = append(gitquery.IsolatedEnv(os.Environ()), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %q: %v: %s", args[0], err, out)
	}
	return strings.TrimSuffix(strings.TrimSuffix(string(out), "\n"), "\r")
}
