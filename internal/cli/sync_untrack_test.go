package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// untrackFixture sets up the scenario the issue describes: a project
// that committed a generated file before turning gitignore management
// on. It syncs once with gitignore off (so the file is written and
// nothing added to .gitignore), commits it, then flips gitignore on so
// the next sync's managed block covers the file git already tracks.
func untrackFixture(t *testing.T) (dir string, gitc func(args ...string)) {
	t.Helper()
	dir, gitc = gitRepo(t)
	testutil.Chdir(t, dir)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\ngitignore:\n  enabled: false\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "r1.md"), "---\nname: r1\n---\nrule body\n")
	silence(t)
	captureLogOut(t)
	if err := runSyncArgs(t); err != nil {
		t.Fatal(err)
	}
	gitc("add", "-A")
	gitc("commit", "-q", "-m", "base")
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\ngitignore:\n  enabled: true\n")
	// Materialize the managed .gitignore block on disk: sync only
	// rewrites it during a real run, and doctor never writes at all.
	// The file stays tracked, since only --untrack removes it.
	if err := runSyncArgs(t); err != nil {
		t.Fatal(err)
	}
	return dir, gitc
}

func TestSyncUntrack_ReportsButDoesNotRemoveWithoutTheFlag(t *testing.T) {
	dir, _ := untrackFixture(t)
	logBuf := captureLogOut(t)

	if err := runSyncArgs(t); err != nil {
		t.Fatalf("sync: %v", err)
	}

	rule := filepath.ToSlash(filepath.Join(".claude", "rules", "r1.md"))
	if !strings.Contains(logBuf.String(), "tracked despite being ignored") || !strings.Contains(logBuf.String(), "git rm --cached "+rule) {
		t.Errorf("sync should report the exact untrack command, got:\n%s", logBuf.String())
	}
	if strings.Contains(logBuf.String(), "file(s) to commit") && strings.Contains(logBuf.String(), rule) {
		t.Errorf("the tracked-and-ignored file must not appear in the commit hint:\n%s", logBuf.String())
	}
	if tracked := git(t, dir, "ls-files", "--", rule); tracked == "" {
		t.Error("without --untrack, the file must remain tracked")
	}
}

func TestSyncUntrack_RemovesFromIndexKeepsWorkingTree(t *testing.T) {
	dir, _ := untrackFixture(t)
	logBuf := captureLogOut(t)
	path := filepath.Join(dir, ".claude", "rules", "r1.md")
	before := readFile(t, path)

	if err := runSyncArgs(t, "--untrack"); err != nil {
		t.Fatalf("sync --untrack: %v", err)
	}

	rule := filepath.ToSlash(filepath.Join(".claude", "rules", "r1.md"))
	if !strings.Contains(logBuf.String(), "untracked") {
		t.Errorf("sync --untrack should report the untrack, got:\n%s", logBuf.String())
	}
	if tracked := git(t, dir, "ls-files", "--", rule); tracked != "" {
		t.Errorf("--untrack should remove %s from the index, git still lists: %q", rule, tracked)
	}
	if got := readFile(t, path); got != before {
		t.Errorf("--untrack must not touch the working tree copy, got %q, want %q", got, before)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the file must still exist on disk: %v", err)
	}
}

func TestSyncUntrackJSON_ListsTrackedThenUntracked(t *testing.T) {
	untrackFixture(t)
	silence(t)

	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"sync", "-t", "claude", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync --json: %v", err)
	}
	var result jsonOutput
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	rule := filepath.ToSlash(filepath.Join(".claude", "rules", "r1.md"))
	var tracked bool
	for _, r := range result.Skipped {
		if filepath.ToSlash(r.Path) == rule && r.Action == "tracked" {
			tracked = true
		}
	}
	if !tracked {
		t.Errorf("skipped should list %s as tracked, got %+v", rule, result.Skipped)
	}

	out.Reset()
	root2 := NewRootCmd("test")
	root2.SetOut(&out)
	root2.SetArgs([]string{"sync", "-t", "claude", "--json", "--untrack"})
	if err := root2.Execute(); err != nil {
		t.Fatalf("sync --json --untrack: %v", err)
	}
	var result2 jsonOutput
	if err := json.Unmarshal(out.Bytes(), &result2); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	var untracked bool
	for _, r := range result2.Writes {
		if filepath.ToSlash(r.Path) == rule && r.Action == "untracked" {
			untracked = true
		}
	}
	if !untracked {
		t.Errorf("writes should list %s as untracked, got %+v", rule, result2.Writes)
	}
}

func TestSyncUntrack_RejectsModesThatDoNotWrite(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	for _, flag := range []string{"--check", "--plan", "--dry-run", "--watch", "--global"} {
		err := runSyncArgs(t, "--untrack", flag)
		if err == nil || !strings.Contains(err.Error(), "--untrack") {
			t.Errorf("--untrack %s: err = %v, want a flag conflict naming --untrack", flag, err)
		}
	}
}

func TestDoctor_ReportsTrackedIgnored(t *testing.T) {
	untrackFixture(t)
	silence(t)

	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"doctor"})
	_ = root.Execute()

	rule := filepath.ToSlash(filepath.Join(".claude", "rules", "r1.md"))
	if !strings.Contains(out.String(), "Tracked despite ignored") || !strings.Contains(out.String(), "git rm --cached") || !strings.Contains(out.String(), rule) {
		t.Errorf("doctor should report the tracked-and-ignored file, got:\n%s", out.String())
	}
}
