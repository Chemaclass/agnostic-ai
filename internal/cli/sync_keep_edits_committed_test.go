package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Without a ledger entry, a file git tracks is compared with its committed
// version (#1397): a fresh linked worktree has no .sync-state yet.
func TestSyncKeepEdits_NoLedgerKeepsEditToCommittedOutput(t *testing.T) {
	dir := committedClaudeOutputsWithoutLedger(t)
	logBuf := captureLogOut(t)
	entry := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(entry, []byte("uncommitted hand edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	editRuleSpec(t, dir, "rule body\n\nAlso check rounding.\n")
	logBuf.Reset()

	if err := runSyncArgs(t, "--keep-edits"); err != nil {
		t.Fatalf("sync --keep-edits: %v", err)
	}

	if got := readFile(t, entry); got != "uncommitted hand edit\n" {
		t.Errorf("CLAUDE.md = %q, want the hand edit kept", got)
	}
	if !strings.Contains(logBuf.String(), "kept CLAUDE.md") {
		t.Errorf("the run should name the kept file, got:\n%s", logBuf.String())
	}
	if got := readFile(t, filepath.Join(dir, ".claude/rules/r1.md")); !strings.Contains(got, "Also check rounding.") {
		t.Errorf("a file that matches HEAD should be rewritten from the spec:\n%s", got)
	}
	if got := checkJSONActions(t)["CLAUDE.md"]; got != "edited" {
		t.Errorf("check after --keep-edits: CLAUDE.md action = %q, want edited", got)
	}
}

// An untracked file has no committed version to keep it by, so sync stops
// instead of writing over the hand-written text (#1611).
func TestSyncKeepEdits_NoLedgerStopsOnAnUntrackedHandWrittenFile(t *testing.T) {
	dir := committedClaudeOutputsWithoutLedger(t)
	entry := filepath.Join(dir, "CLAUDE.md")
	git(t, dir, "rm", "-q", "--cached", "CLAUDE.md")
	git(t, dir, "commit", "-q", "-m", "untrack")
	if err := os.WriteFile(entry, []byte("hand edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runSyncArgs(t, "--keep-edits"); err == nil || !strings.Contains(err.Error(), "agnostic-ai import claude") {
		t.Errorf("sync --keep-edits: err = %v, want a stop", err)
	}
	if got := readFile(t, entry); got != "hand edit\n" {
		t.Errorf("CLAUDE.md = %q, want the hand edit kept", got)
	}
}

func TestSyncKeepEdits_NoLedgerOutsideGitStopsOnAHandWrittenFile(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	syncClaudeThenEditSpec(t, dir)
	entry := filepath.Join(dir, "CLAUDE.md")
	if err := os.Remove(filepath.Join(dir, ".agnostic-ai/.sync-state")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte("hand edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runSyncArgs(t, "--keep-edits"); err == nil || !strings.Contains(err.Error(), "agnostic-ai import claude") {
		t.Errorf("sync --keep-edits: err = %v, want a stop", err)
	}
	if got := readFile(t, entry); got != "hand edit\n" {
		t.Errorf("CLAUDE.md = %q, want the hand edit kept", got)
	}
}

// committedClaudeOutputsWithoutLedger syncs claude, commits every output,
// and removes the ledger: the state of a fresh linked worktree.
func committedClaudeOutputsWithoutLedger(t *testing.T) string {
	t.Helper()
	dir := setupFixture(t)
	isolateGit(t)
	git(t, dir, "init", "-q")
	testutil.Chdir(t, dir)
	silence(t)
	if err := runSyncArgs(t); err != nil {
		t.Fatalf("sync: %v", err)
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "base")
	if err := os.Remove(filepath.Join(dir, ".agnostic-ai/.sync-state")); err != nil {
		t.Fatal(err)
	}
	return dir
}
