package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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
	for _, value := range []string{"feat/a", "feat/b", canonicalDir(a), "removed", head[:7], "dirty", "blocked", "ready", "#17", "2026-10-13T09:00Z"} {
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

func TestHandoffList_HiddenUntrackedCodePreventsRemovalSuggestion(t *testing.T) {
	main, active, _ := handoffListProject(t)
	mustWriteFile(t, filepath.Join(active, "merged.txt"), "merged\n")
	git(t, active, "add", "merged.txt")
	git(t, active, "commit", "-qm", "merged work")
	git(t, main, "merge", "--ff-only", "feat/a")
	git(t, active, "config", "status.showUntrackedFiles", "no")
	path := filepath.Join(main, ".agnostic-ai", "local", "handoffs", "feat-a.md")
	writeListHandoff(t, path, "feat/a", active, git(t, active, "rev-parse", "HEAD"), "ready", "#1")
	before := readText(t, path)
	code := filepath.Join(active, "new-code.go")
	mustWriteFile(t, code, "package probe\n")
	out, err := runRoot(t, "handoff", "list")
	if err != nil {
		t.Fatalf("handoff list: %v %s", err, out)
	}
	if !strings.Contains(out, "feat/a") || !strings.Contains(out, "dirty") {
		t.Errorf("hidden untracked code was not reported dirty: %s", out)
	}
	if strings.Contains(out, "Review ") {
		t.Errorf("untracked code received a removal suggestion: %s", out)
	}
	if got := git(t, active, "config", "status.showUntrackedFiles"); got != "no" {
		t.Errorf("list changed the user's status setting: %q", got)
	}
	if readText(t, code) != "package probe\n" || readText(t, path) != before {
		t.Error("list changed the code or handoff")
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
	if !strings.Contains(out, "feat/bare") || !strings.Contains(out, canonicalDir(linked)) {
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

func TestHandoffList_UsesFileTimeWhenHeaderDatesTie(t *testing.T) {
	main, _, b := handoffListProject(t)
	head := git(t, main, "rev-parse", "HEAD")
	shared := filepath.Join(main, ".agnostic-ai", "local", "handoffs", "feat-b.md")
	local := filepath.Join(b, ".agnostic-ai", "local", "handoffs", "feat-b.md")
	writeListHandoff(t, shared, "feat/b", b, head, "blocked", "#1")
	writeListHandoff(t, local, "feat/b", b, head, "ready", "#6")
	old := time.Unix(100, 0)
	newer := time.Unix(200, 0)
	if err := os.Chtimes(shared, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(local, newer, newer); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, b)
	out, err := runRoot(t, "handoff", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "#6") || strings.Contains(out, "blocked") {
		t.Errorf("newer fallback note lost: %s", out)
	}
}

func TestHandoffList_ComparesMixedHeaderDateFormats(t *testing.T) {
	main, _, b := handoffListProject(t)
	head := git(t, main, "rev-parse", "HEAD")
	shared := filepath.Join(main, ".agnostic-ai", "local", "handoffs", "feat-b.md")
	local := filepath.Join(b, ".agnostic-ai", "local", "handoffs", "feat-b.md")
	writeListHandoff(t, shared, "feat/b", b, head, "blocked", "#1")
	body := strings.Replace(readText(t, shared), "2026-10-13T09:00Z", "2026-10-13T08:00:00Z", 1)
	mustWriteFile(t, shared, body)
	writeListHandoff(t, local, "feat/b", b, head, "ready", "#6")
	older := time.Unix(100, 0)
	newer := time.Unix(200, 0)
	if err := os.Chtimes(shared, newer, newer); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(local, older, older); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, b)
	out, err := runRoot(t, "handoff", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "#6") || strings.Contains(out, "blocked") {
		t.Errorf("newer minute-format note lost to copied RFC3339 note: %s", out)
	}
}

func TestHandoffList_JoinsLegacyDetachedSnapshotWithLiveWorktree(t *testing.T) {
	_, a, _ := handoffListProject(t)
	git(t, a, "checkout", "--detach", "-q", "HEAD")
	head := git(t, a, "rev-parse", "HEAD")
	mustWriteFile(t, filepath.Join(a, ".agnostic-ai", "local", "HANDOFF.auto.md"), "# Automatic handoff\ntool: codex | date: 2026-10-13T09:00Z | branch: detached | head: "+head+"\n\n## Git status\n")
	testutil.Chdir(t, a)
	out, err := runRoot(t, "handoff", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, canonicalDir(a)) || strings.Contains(out, "removed") || !strings.Contains(out, "clean") {
		t.Errorf("live detached snapshot treated as removed: %s", out)
	}
	main := git(t, a, "rev-parse", "--path-format=absolute", "--git-common-dir")
	main = filepath.Dir(main)
	named := filepath.Join(t.TempDir(), "named-detached")
	git(t, main, "worktree", "add", "-q", "-b", "detached", named)
	store := filepath.Join(main, ".agnostic-ai", "local", "handoffs")
	for _, entry := range []struct{ file, path string }{{"detached-" + head + ".auto.md", a}, {"detached.auto.md", named}} {
		mustWriteFile(t, filepath.Join(store, entry.file), "# Automatic handoff\ntool: codex | date: 2026-10-13T10:00:00Z | branch: detached | head: "+head+"\nworktree: "+entry.path+"\n")
	}
	out, err = runRoot(t, "handoff", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, canonicalDir(a)) || !strings.Contains(out, canonicalDir(named)) || !strings.Contains(out, "detached-"+head) {
		t.Errorf("named and detached tasks conflated: %s", out)
	}
}

func TestHandoffList_SkipsAutomaticUpgradeEvenInAnInteractiveRelease(t *testing.T) {
	for _, key := range []string{envNoUpdateCheck, envUpgradeInProgress, "CI", "AGNOSTIC_AI_TARGET"} {
		t.Setenv(key, "")
	}
	root := NewRootCmd("0.83.0")
	command, _, err := root.Find([]string{"handoff", "list"})
	if err != nil {
		t.Fatal(err)
	}
	if automaticUpgradeAllowed(command, "0.83.0", true) {
		t.Error("read-only handoff list can make an automatic network check")
	}
}

func TestHandoffList_ManualAutoSuffixKeepsAuthoritativeTaskFields(t *testing.T) {
	main, _, _ := handoffListProject(t)
	linked := filepath.Join(t.TempDir(), "auto-suffix")
	git(t, main, "worktree", "add", "-q", "-b", "feat.auto", linked)
	head := git(t, linked, "rev-parse", "HEAD")
	store := filepath.Join(main, ".agnostic-ai", "local", "handoffs")
	writeListHandoff(t, filepath.Join(store, "feat.auto.md"), "feat.auto", linked, head, "blocked", "#72")
	mustWriteFile(t, filepath.Join(store, "feat.auto.auto.md"), "# Automatic handoff\ntool: codex | date: 2026-10-13T10:00:00Z | branch: feat.auto | head: "+head+"\nworktree: "+linked+"\n")
	out, err := runRoot(t, "handoff", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "#72") || !strings.Contains(out, "blocked") {
		t.Errorf("manual .auto branch lost to newer snapshot: %s", out)
	}
}

func TestHandoffList_PreservesPipesInBranchesAndWorktreeHeaders(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows paths cannot contain a pipe")
	}
	main, _, _ := handoffListProject(t)
	linked := filepath.Join(t.TempDir(), "wt | pipe")
	git(t, main, "worktree", "add", "-q", "-b", "feat|bar", linked)
	head := git(t, linked, "rev-parse", "HEAD")
	store := filepath.Join(main, ".agnostic-ai", "local", "handoffs")
	writeListHandoff(t, filepath.Join(store, "feat-bar.md"), "feat|bar", linked, head, "active", "none")
	out, err := runRoot(t, "handoff", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "feat|bar") || !strings.Contains(out, canonicalDir(linked)) || strings.Contains(out, "removed") {
		t.Errorf("pipe branch failed live join: %s", out)
	}
	git(t, linked, "checkout", "--detach", "-q", "HEAD")
	named := filepath.Join(t.TempDir(), "named-detached")
	git(t, main, "worktree", "add", "-q", "-b", "detached", named)
	mustWriteFile(t, filepath.Join(linked, ".agnostic-ai", "local", "HANDOFF.auto.md"), "# Automatic handoff\ntool: codex | date: 2026-10-13T10:00:00Z | branch: detached | head: "+head+"\nworktree: "+linked+"\n")
	testutil.Chdir(t, linked)
	out, err = runRoot(t, "handoff", "list")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "detached-"+head+" ") && strings.Contains(line, linked) {
			found = true
		}
	}
	if !found {
		t.Errorf("pipe-containing worktree header failed detached join: %s", out)
	}
}

func TestHandoffList_ExplicitDetachedMarkerSurvivesNamedBranchReuse(t *testing.T) {
	_, linked, _ := handoffListProject(t)
	git(t, linked, "checkout", "--detach", "-q", "HEAD")
	head := git(t, linked, "rev-parse", "HEAD")
	mustWriteFile(t, filepath.Join(linked, ".agnostic-ai", "local", "HANDOFF.auto.md"), "# Automatic handoff\ntool: codex | date: 2026-10-13T10:00:00Z | branch: detached | head: "+head+"\nworktree: "+linked+" | detached: true\n")
	git(t, linked, "checkout", "-q", "-b", "detached")
	testutil.Chdir(t, linked)
	out, err := runRoot(t, "handoff", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "detached-"+head) || !strings.Contains(out, "removed") {
		t.Errorf("explicit detached snapshot joined a reused named branch: %s", out)
	}
}

func TestHandoffList_PreservesMetadataLabelsInsideWorktreePaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows paths cannot contain a pipe or colon")
	}
	main, _, _ := handoffListProject(t)
	linked := filepath.Join(t.TempDir(), "wt | branch: bogus | status: misleading")
	git(t, main, "worktree", "add", "-q", "-b", "path-labels", linked)
	head := git(t, linked, "rev-parse", "HEAD")
	store := filepath.Join(main, ".agnostic-ai", "local", "handoffs")
	writeListHandoff(t, filepath.Join(store, "path-labels.md"), "path-labels", linked, head, "blocked", "#73")
	out, err := runRoot(t, "handoff", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "path-labels") || !strings.Contains(out, canonicalDir(linked)) || !strings.Contains(out, "blocked") || !strings.Contains(out, "#73") || strings.Contains(out, "removed") {
		t.Errorf("path metadata text changed task identity or path: %s", out)
	}
	git(t, linked, "checkout", "--detach", "-q", "HEAD")
	mustWriteFile(t, filepath.Join(linked, ".agnostic-ai", "local", "HANDOFF.auto.md"), "# Automatic handoff\ntool: codex | date: 2026-10-13T10:00:00Z | branch: detached | head: "+head+"\nworktree: "+linked+" | detached: true\n")
	testutil.Chdir(t, linked)
	out, err = runRoot(t, "handoff", "list")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "detached-"+head+" ") && strings.Contains(line, linked) {
			found = true
		}
	}
	if !found {
		t.Errorf("path metadata text changed detached snapshot: %s", out)
	}
}

func TestHandoffList_FinalFalseMarkerPreservesLiteralDetachedPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows paths cannot contain a pipe or colon")
	}
	main, _, _ := handoffListProject(t)
	linked := filepath.Join(t.TempDir(), "wt | detached: true")
	git(t, main, "worktree", "add", "-q", "-b", "detached", linked)
	head := git(t, linked, "rev-parse", "HEAD")
	store := filepath.Join(main, ".agnostic-ai", "local", "handoffs")
	mustWriteFile(t, filepath.Join(store, "detached.auto.md"), "# Automatic handoff\ntool: codex | date: 2026-10-13T10:00:00Z | branch: detached | head: "+head+"\nworktree: "+linked+" | detached: false\n")
	out, err := runRoot(t, "handoff", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, canonicalDir(linked)) || strings.Contains(out, "detached-"+head) || strings.Contains(out, "removed") || strings.Contains(out, " | detached: false") {
		t.Errorf("final false marker lost named branch or literal path: %s", out)
	}
	git(t, linked, "checkout", "--detach", "-q", "HEAD")
	out, err = runRoot(t, "handoff", "list")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "detached-"+head) || !strings.Contains(out, "removed") {
		t.Errorf("explicit false marker became a detached snapshot after checkout reuse: %s", out)
	}
}

func TestHandoffList_CanonicalizesLiveWorktreeAliases(t *testing.T) {
	main, active, _ := handoffListProject(t)
	alias := filepath.Join(t.TempDir(), "checkout-alias")
	if err := os.Symlink(active, alias); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	path := filepath.Join(main, ".agnostic-ai", "local", "handoffs", "feat-a.md")
	writeListHandoff(t, path, "feat/a", alias, git(t, active, "rev-parse", "HEAD"), "blocked", "#17")
	testutil.Chdir(t, alias)
	out, err := runRoot(t, "handoff", "list")
	if err != nil {
		t.Fatalf("handoff list from alias: %v %s", err, out)
	}
	if !strings.Contains(out, "feat/a") || !strings.Contains(out, canonicalDir(active)) || !strings.Contains(out, "blocked") || !strings.Contains(out, "#17") {
		t.Errorf("canonical worktree metadata missing: %s", out)
	}
	if strings.Contains(out, alias) || strings.Contains(out, "removed") {
		t.Errorf("alias was not joined to the live worktree: %s", out)
	}
}
