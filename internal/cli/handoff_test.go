package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func handoffListProject(t *testing.T) (string, string, string) {
	t.Helper()
	main := setupGitRepo(t)
	mustWriteFile(t, filepath.Join(main, ".gitignore"), "**/.agnostic-ai/local/\n")
	git(t, main, "add", ".gitignore")
	git(t, main, "commit", "-qm", "fixture")
	git(t, main, "branch", "-M", "main")
	a, b := filepath.Join(t.TempDir(), "wt-a"), filepath.Join(t.TempDir(), "wt-b")
	git(t, main, "worktree", "add", "-q", "-b", "feat/a", a)
	git(t, main, "worktree", "add", "-q", "-b", "feat/b", b)
	testutil.Chdir(t, main)
	return main, a, b
}

func writeListHandoff(t *testing.T, path, branch, worktree, head, status, pr string) {
	t.Helper()
	mustWriteFile(t, path, fmt.Sprintf("# Handoff\ntool: codex | date: 2026-10-13T09:00Z | branch: %s | head: %s\nworktree: %s | status: %s | pr: %s\n\n## Goal\nBody is never listed.\n", branch, head, worktree, status, pr))
}

func TestHandoffList_JoinsLiveAndRemovedWorktreesWithoutDuplicatingSnapshots(t *testing.T) {
	main, a, b := handoffListProject(t)
	head := git(t, main, "rev-parse", "HEAD")
	store := filepath.Join(main, ".agnostic-ai", "local", "handoffs")
	writeListHandoff(t, filepath.Join(store, "feat-a.md"), "feat/a", a, head, "blocked", "#17")
	writeListHandoff(t, filepath.Join(store, "feat-b.md"), "feat/b", b, head, "ready", "none")
	auto := fmt.Sprintf("# Automatic handoff\ntool: claude | date: 2026-10-13T10:00Z | branch: feat/a | head: %s\nworktree: %s\n\n## Git status\n", head, a)
	mustWriteFile(t, filepath.Join(store, "feat-a.auto.md"), auto)
	mustWriteFile(t, filepath.Join(a, ".agnostic-ai", "local", "HANDOFF.auto.md"), auto)
	mustWriteFile(t, filepath.Join(a, "dirty.txt"), "scratch\n")
	git(t, main, "worktree", "remove", b)
	testutil.Chdir(t, a)
	out, err := runRoot(t, "handoff", "list")
	if err != nil {
		t.Fatalf("handoff list: %v %s", err, out)
	}
	for _, value := range []string{"feat/a", "feat/b", a, "removed", head[:7], "dirty", "blocked", "ready", "#17", "2026-10-13T09:00Z"} {
		if !strings.Contains(out, value) {
			t.Errorf("missing %q: %s", value, out)
		}
	}
	if strings.Count(out, "feat/a") != 1 {
		t.Errorf("snapshot duplicated task: %s", out)
	}
	if strings.Contains(out, "Body is never listed") {
		t.Errorf("listed body: %s", out)
	}
	if _, err := os.Stat(filepath.Join(store, "feat-b.md")); err != nil {
		t.Errorf("removed handoff changed: %v", err)
	}
}

func TestHandoffList_UsesConfiguredSubprojectAndLegacyFallback(t *testing.T) {
	main, _, b := handoffListProject(t)
	for _, root := range []string{main, b} {
		mustWriteFile(t, filepath.Join(root, "app", "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\n")
	}
	head := git(t, main, "rev-parse", "HEAD")
	writeListHandoff(t, filepath.Join(main, "app", ".agnostic-ai", "local", "handoffs", "feat-b.md"), "feat/b", b, head, "active", "#4")
	writeListHandoff(t, filepath.Join(main, ".agnostic-ai", "local", "handoffs", "wrong.md"), "wrong/project", main, head, "blocked", "none")
	testutil.Chdir(t, filepath.Join(b, "app"))
	out, err := runRoot(t, "handoff", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "feat/b") || strings.Contains(out, "wrong/project") {
		t.Errorf("wrong project store: %s", out)
	}
	legacy := filepath.Join(b, "app", ".agnostic-ai", "local", "HANDOFF.md")
	mustWriteFile(t, legacy, "# Handoff\ntool: codex | date: 2026-10-12T09:00Z | branch: legacy/task | head: abc1234\n\nLegacy body.\n")
	out, err = runRoot(t, "handoff", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "legacy/task") || !strings.Contains(out, "abc1234") {
		t.Errorf("legacy omitted: %s", out)
	}
}

func TestHandoffList_SuggestsOnlyBranchesProvenMergedAndNeverDeletes(t *testing.T) {
	main, a, b := handoffListProject(t)
	mustWriteFile(t, filepath.Join(a, "a.txt"), "merged\n")
	git(t, a, "add", "a.txt")
	git(t, a, "commit", "-qm", "a")
	mustWriteFile(t, filepath.Join(b, "b.txt"), "unmerged\n")
	git(t, b, "add", "b.txt")
	git(t, b, "commit", "-qm", "b")
	git(t, main, "merge", "--ff-only", "feat/a")
	store := filepath.Join(main, ".agnostic-ai", "local", "handoffs")
	path := filepath.Join(store, "feat-a.md")
	writeListHandoff(t, path, "feat/a", a, git(t, a, "rev-parse", "HEAD"), "ready", "#1")
	writeListHandoff(t, filepath.Join(store, "feat-b.md"), "feat/b", b, git(t, b, "rev-parse", "HEAD"), "ready", "#2")
	before := readText(t, path)
	out, err := runRoot(t, "handoff", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "feat/a is merged into main") {
		t.Errorf("missing ancestry suggestion: %s", out)
	}
	if strings.Contains(out, "feat/b is merged") {
		t.Errorf("unproven branch suggested: %s", out)
	}
	if after := readText(t, path); after != before {
		t.Error("list changed handoff")
	}
}

func TestHandoffList_EmptyOutsideGitDoesNotCreateAStore(t *testing.T) {
	root := testutil.TempCwd(t)
	out, err := runRoot(t, "handoff", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "No task handoffs") {
		t.Errorf("empty message: %s", out)
	}
	if _, err := os.Stat(filepath.Join(root, ".agnostic-ai")); !os.IsNotExist(err) {
		t.Errorf("list created state: %v", err)
	}
}

func TestHandoffList_FallsBackWhenMainStoreIsNotIgnoredAndSkipsLinks(t *testing.T) {
	main, _, b := handoffListProject(t)
	head := git(t, main, "rev-parse", "HEAD")
	mustWriteFile(t, filepath.Join(main, ".gitignore"), "")
	writeListHandoff(t, filepath.Join(main, ".agnostic-ai", "local", "handoffs", "public.md"), "public/unsafe", main, head, "active", "none")
	store := filepath.Join(b, ".agnostic-ai", "local", "handoffs")
	writeListHandoff(t, filepath.Join(store, "feat-b.md"), "feat/b", b, head, "active", "none")
	outside := filepath.Join(t.TempDir(), "outside.md")
	writeListHandoff(t, outside, "do-not-read", b, head, "active", "none")
	if err := os.Symlink(outside, filepath.Join(store, "linked.md")); err != nil {
		t.Skipf("symlink: %v", err)
	}
	testutil.Chdir(t, b)
	out, err := runRoot(t, "handoff", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "feat/b") || strings.Contains(out, "public/unsafe") || strings.Contains(out, "do-not-read") {
		t.Errorf("unsafe store/file read: %s", out)
	}
}

func TestHandoffList_BareMainUsesCurrentCheckoutStore(t *testing.T) {
	main, _, _ := handoffListProject(t)
	bare := filepath.Join(t.TempDir(), "bare.git")
	git(t, main, "clone", "-q", "--bare", main, bare)
	linked := filepath.Join(t.TempDir(), "linked")
	git(t, bare, "worktree", "add", "-q", "-b", "feat/bare", linked)
	head := git(t, linked, "rev-parse", "HEAD")
	writeListHandoff(t, filepath.Join(linked, ".agnostic-ai", "local", "handoffs", "feat-bare.md"), "feat/bare", linked, head, "ready", "none")
	testutil.Chdir(t, linked)
	out, err := runRoot(t, "handoff", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "feat/bare") || !strings.Contains(out, linked) {
		t.Errorf("bare fallback missing: %s", out)
	}
}

func TestHandoffList_ReadsOnlyBoundedHeaders(t *testing.T) {
	main, _, _ := handoffListProject(t)
	path := filepath.Join(main, ".agnostic-ai", "local", "handoffs", "main.md")
	writeListHandoff(t, path, "main", main, git(t, main, "rev-parse", "HEAD"), "active", "none")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(strings.Repeat("body-must-not-be-printed\n", 4000)); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := runRoot(t, "handoff", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "main") || strings.Contains(out, "body-must-not-be-printed") {
		t.Errorf("body leaked: %s", out)
	}
}

func TestHandoffList_ReadsIndividuallyIgnoredLegacyFileWithoutTaskStore(t *testing.T) {
	main, _, _ := handoffListProject(t)
	mustWriteFile(t, filepath.Join(main, ".gitignore"), "/.agnostic-ai/local/HANDOFF.md\n")
	mustWriteFile(t, filepath.Join(main, ".agnostic-ai", "local", "HANDOFF.md"), "# Handoff\ntool: codex | date: 2026-10-13T09:00Z | branch: main | head: abc1234\n")
	out, err := runRoot(t, "handoff", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "abc1234") {
		t.Errorf("individually ignored legacy missing: %s", out)
	}
}
