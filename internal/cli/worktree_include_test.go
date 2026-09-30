package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func syncGitignored(t *testing.T, config string) string {
	t.Helper()
	dir, _ := gitRepo(t)
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), config)
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), "---\nname: style\n---\nKeep it short.\n")
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	return dir
}

// With claude as a target and outputs ignored, `.worktreeinclude` lists
// the managed gitignore block's paths but the ledger, so Claude Code
// copies the generated files and the local layer into each new worktree.
func TestSync_WorktreeIncludeMirrorsTheManagedBlock(t *testing.T) {
	dir := syncGitignored(t, "version: 1\ntargets: [claude]\ngitignore:\n  enabled: true\n")

	data, err := os.ReadFile(filepath.Join(dir, ".worktreeinclude"))
	if err != nil {
		t.Fatalf("no .worktreeinclude: %v", err)
	}
	gitignore, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{gitignoreBlockStart, "/CLAUDE.md", "/.claude/rules/", "/.agnostic-ai/local/"} {
		if !strings.Contains(string(data), want) {
			t.Errorf(".worktreeinclude lacks %q:\n%s", want, data)
		}
	}
	if strings.Contains(string(data), ".sync-state") || strings.Contains(string(data), "Not committed") {
		t.Errorf(".worktreeinclude carries the ledger or the gitignore hint:\n%s", data)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "/") && !strings.Contains(string(gitignore), line+"\n") {
			t.Errorf("%s is not in the .gitignore block", line)
		}
	}
}

// Opting out removes a block an earlier sync wrote and keeps the user's
// own lines; without claude, sync never creates the file.
func TestSync_WorktreeIncludeOptOutAndOtherTargets(t *testing.T) {
	dir := syncGitignored(t, "version: 1\ntargets: [claude]\ngitignore:\n  enabled: true\n")
	path := filepath.Join(dir, ".worktreeinclude")
	data, _ := os.ReadFile(path)
	mustWriteFile(t, path, "secrets.local\n"+string(data))
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\ngitignore:\n  enabled: true\n  worktree-include: false\n")
	if err := runSyncArgs(t); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); strings.TrimSpace(string(got)) != "secrets.local" {
		t.Errorf("opt-out left:\n%s", got)
	}

	dir = syncGitignored(t, "version: 1\ntargets: [codex]\ngitignore:\n  enabled: true\n")
	if _, err := os.Stat(filepath.Join(dir, ".worktreeinclude")); !os.IsNotExist(err) {
		t.Errorf("a codex-only project got a .worktreeinclude: %v", err)
	}
}

func TestSync_IgnoresManagedWorktreeIncludeWhenConfigured(t *testing.T) {
	dir := syncGitignored(t, "version: 1\ntargets: [claude]\ngitignore:\n  enabled: true\n  ignore-worktree-include: true\n")
	data, err := os.ReadFile(filepath.Join(dir, ".worktreeinclude"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "/.claude/rules/\n") {
		t.Errorf("worktreeinclude is not managed: %s", data)
	}
	ignored, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ignored), "/.worktreeinclude\n") {
		t.Errorf("managed worktreeinclude is not ignored: %s", ignored)
	}
	if pending := gitPending(dir, []string{worktreeIncludeFile}); len(pending) != 0 {
		t.Errorf("ignored worktreeinclude appears in files to commit: %v", pending)
	}
}

func TestSync_WorktreeIncludeIgnoreRequiresManagement(t *testing.T) {
	dir := syncGitignored(t, "version: 1\ntargets: [claude]\ngitignore:\n  enabled: true\n  ignore-worktree-include: true\n  worktree-include: false\n")
	ignored, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(ignored), "/.worktreeinclude\n") {
		t.Errorf("unmanaged worktreeinclude is ignored: %s", ignored)
	}
}

func trackedWorktreeIncludeProject(t *testing.T) string {
	t.Helper()
	dir := syncGitignored(t, "version: 1\ntargets: [claude]\ngitignore:\n  enabled: true\n")
	git(t, dir, "add", worktreeIncludeFile)
	git(t, dir, "commit", "-q", "-m", "worktree include")
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\ngitignore:\n  enabled: true\n  ignore-worktree-include: true\n")
	return dir
}

func TestSync_ReportsIgnoredTrackedWorktreeIncludeWithoutCommitHint(t *testing.T) {
	trackedWorktreeIncludeProject(t)
	out := captureLogOut(t)
	if err := runSyncArgs(t); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "git rm --cached .worktreeinclude") {
		t.Errorf("missing tracked-and-ignored guidance: %s", out)
	}
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.Contains(line, "to commit:") && strings.Contains(line, worktreeIncludeFile) {
			t.Errorf("tracked ignored file appears in commit hint: %s", line)
		}
	}
}

func TestSync_UntracksIgnoredWorktreeIncludeAndKeepsManagingIt(t *testing.T) {
	dir := trackedWorktreeIncludeProject(t)
	if err := runSyncArgs(t, "--untrack"); err != nil {
		t.Fatal(err)
	}
	if tracked := git(t, dir, "ls-files", "--", worktreeIncludeFile); tracked != "" {
		t.Errorf("worktree include remains tracked: %s", tracked)
	}
	if got := readFile(t, filepath.Join(dir, worktreeIncludeFile)); !strings.Contains(got, "/.claude/rules/\n") {
		t.Errorf("worktree include lost its managed block: %s", got)
	}
}

func TestSyncJSON_ReportsIgnoredTrackedWorktreeInclude(t *testing.T) {
	trackedWorktreeIncludeProject(t)
	out, err := runCLI(t, "sync", "--json")
	if err != nil {
		t.Fatalf("sync: %v: %s", err, out)
	}
	var got jsonOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	for _, file := range got.Skipped {
		if file.Path == worktreeIncludeFile && file.Action == "tracked" {
			return
		}
	}
	t.Errorf("JSON lacks tracked-and-ignored worktreeinclude: %s", out)
}
