package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

// fakeGitFailingRmCached writes a `git` shim on a fresh PATH entry that
// refuses any `rm --cached` invocation and forwards everything else to
// the real git, so a sync's other git calls (ls-files, status) keep
// working normally.
func fakeGitFailingRmCached(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake git is a sh script")
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	fake := "#!/bin/sh\n" +
		"for a in \"$@\"; do\n" +
		"  if [ \"$a\" = \"--cached\" ]; then echo fake-git: rm --cached refused >&2; exit 1; fi\n" +
		"done\n" +
		"exec \"" + realGit + "\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

// When git rm --cached fails, sync --untrack must exit non-zero,
// report the file as still tracked (never falsely as untracked), and
// leave the rest of the sync's writes in place rather than rolling
// them back (#1330 review).
func TestSyncUntrack_FailureReturnsErrorKeepsAccurateReportAndDoesNotRollBack(t *testing.T) {
	dir, _ := untrackFixture(t)
	logBuf := captureLogOut(t)
	entry := filepath.Join(dir, "CLAUDE.md")
	rule := filepath.Join(dir, ".claude", "rules", "r1.md")
	entryBefore := readFile(t, entry)
	ruleBefore := readFile(t, rule)

	bin := fakeGitFailingRmCached(t)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	err := runSyncArgs(t, "--untrack")

	if err == nil {
		t.Fatal("sync --untrack must exit non-zero when git rm --cached fails")
	}
	out := logBuf.String()
	if !strings.Contains(out, "tracked despite being ignored") {
		t.Errorf("the file should still be reported as tracked, got:\n%s", out)
	}
	if strings.Contains(out, "untracked") {
		t.Errorf("nothing should be reported as untracked when the removal failed, got:\n%s", out)
	}
	if got := readFile(t, entry); got != entryBefore {
		t.Errorf("CLAUDE.md must not be rolled back: got %q, want %q", got, entryBefore)
	}
	if got := readFile(t, rule); got != ruleBefore {
		t.Errorf(".claude/rules/r1.md must not be rolled back: got %q, want %q", got, ruleBefore)
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
