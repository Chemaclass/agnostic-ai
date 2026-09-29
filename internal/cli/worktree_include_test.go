package cli

import (
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
