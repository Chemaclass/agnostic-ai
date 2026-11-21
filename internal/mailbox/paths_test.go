package mailbox

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"testing"
)

func TestClaimPaths_RejectsTraversalAndAbsolutePaths(t *testing.T) {
	root, _ := newReviewRepo(t)
	for _, candidate := range []string{"../parent", "dir/../file", "dir//file", filepath.Join(root, "base.txt"), "file/"} {
		t.Run(candidate, func(t *testing.T) {
			requireGuard(t, validateClaimPaths(Gives{Worktree: root, Paths: []string{candidate}}), "format")
		})
	}
}

func TestClaimPaths_ResolvesAliasesAndNewChildrenFromASubfolder(t *testing.T) {
	root, _ := newReviewRepo(t)
	src := filepath.Join(root, "src")
	if err := os.Mkdir(src, 0o700); err != nil {
		t.Fatal(err)
	}
	writeReviewFile(t, src, "file.txt", "existing\n")
	claimSymlink(t, "src", filepath.Join(root, "alias"))
	rootAlias := filepath.Join(t.TempDir(), "root")
	claimSymlink(t, root, rootAlias)
	gives, _, err := normalizeClaimPaths(context.Background(), src, Gives{
		Worktree: rootAlias,
		Paths:    []string{"alias/file.txt", "src/file.txt", "alias/new/child.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if gives.Worktree != realRoot || !slices.Equal(gives.Paths, []string{"src/file.txt", "src/new/child.txt"}) {
		t.Errorf("resolved claim = %#v", gives)
	}
}

func TestClaimPaths_RejectsAnotherRootAndEscapingAliases(t *testing.T) {
	root, _ := newReviewRepo(t)
	outside := t.TempDir()
	_, _, err := normalizeClaimPaths(context.Background(), root, Gives{Worktree: outside, Paths: []string{"."}})
	requireGuard(t, err, "format")

	claimSymlink(t, outside, filepath.Join(root, "outside"))
	_, _, err = normalizeClaimPaths(context.Background(), root, Gives{Worktree: root, Paths: []string{"outside/new/child.txt"}})
	requireGuard(t, err, "format")
}

func TestClaimPaths_CanClaimAFutureSymlinkTarget(t *testing.T) {
	root, _ := newReviewRepo(t)
	claimSymlink(t, filepath.Join("future", "new.txt"), filepath.Join(root, "alias"))
	resolved, _, err := normalizeClaimPaths(t.Context(), root, Gives{Worktree: root, Paths: []string{"alias"}})
	if err != nil || !slices.Equal(resolved.Paths, []string{"future/new.txt"}) {
		t.Errorf("resolved future target = %#v, %v", resolved, err)
	}
}

func TestClaimPaths_RejectsCredentialsInResolvedAliases(t *testing.T) {
	root, _ := newReviewRepo(t)
	writeReviewFile(t, root, "password=regression-secret-marker", "sample\n")
	claimSymlink(t, "password=regression-secret-marker", filepath.Join(root, "alias"))
	_, _, err := normalizeClaimPaths(context.Background(), root, Gives{Worktree: root, Paths: []string{"alias"}})
	requireGuard(t, err, "secret")
}

func TestPathInScope_UsesComponentBoundariesAndFilesystemCase(t *testing.T) {
	for _, test := range []struct {
		scope, candidate string
		fold, want       bool
	}{
		{".", "src/file.go", false, true},
		{"src", "src/file.go", false, true},
		{"src", "src2/file.go", false, false},
		{"SRC", "src/file.go", false, false},
		{"SRC", "src/file.go", true, true},
	} {
		if got := pathInScope(test.scope, test.candidate, test.fold); got != test.want {
			t.Errorf("pathInScope(%q, %q, %t) = %t, want %t", test.scope, test.candidate, test.fold, got, test.want)
		}
	}
}

func TestClaimPaths_UsesTheWorktreeFilesystemCase(t *testing.T) {
	repo, head := newReviewRepo(t)
	linked := filepath.Join(t.TempDir(), "linked")
	reviewGitFixture(t, repo, "worktree", "add", "--detach", linked, head)
	writeReviewFile(t, linked, "Mixed.txt", "local\n")
	_, alternateErr := os.Stat(filepath.Join(linked, "mixed.txt"))
	wantFold := alternateErr == nil
	reviewGitFixture(t, repo, "config", "core.ignorecase", strconv.FormatBool(!wantFold))
	canonical, folded, err := normalizeClaimPaths(context.Background(), linked, Gives{
		Worktree: linked,
		Paths:    []string{"mixed.txt", "Mixed.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantCount := 2
	if wantFold {
		wantCount = 1
	}
	if folded != wantFold || len(canonical.Paths) != wantCount {
		t.Errorf("resolved claim = %#v, fold=%t, want %d paths with fold=%t", canonical, folded, wantCount, wantFold)
	}
}

func claimSymlink(t *testing.T, target, name string) {
	t.Helper()
	if err := os.Symlink(target, name); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlinks are unavailable: %v", err)
		}
		t.Fatal(err)
	}
}
