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

func runSyncArgs(t *testing.T, args ...string) error {
	t.Helper()
	root := NewRootCmd("test")
	root.SetArgs(append([]string{"sync", "-t", "claude"}, args...))
	return root.Execute()
}

func TestSyncKeepEdits_KeepsHandEditUpdatesTheRestAndNamesIt(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	logBuf := captureLogOut(t)
	syncClaudeThenEditSpec(t, dir)
	entry := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(entry, []byte("hand edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	logBuf.Reset()

	if err := runSyncArgs(t, "--keep-edits"); err != nil {
		t.Fatalf("sync --keep-edits: %v", err)
	}

	if got := readFile(t, entry); got != "hand edit\n" {
		t.Errorf("CLAUDE.md = %q, want the hand edit kept", got)
	}
	if got := readFile(t, filepath.Join(dir, ".claude/rules/r1.md")); !strings.Contains(got, "Also check rounding.") {
		t.Errorf("rule output was not updated from the spec:\n%s", got)
	}
	if out := logBuf.String(); !strings.Contains(out, "kept CLAUDE.md") {
		t.Errorf("the run should name the kept file, got:\n%s", out)
	}
	// The next check still sees the edit, and a plain sync overwrites it.
	if got := checkJSONActions(t)["CLAUDE.md"]; got != "edited" {
		t.Errorf("check after --keep-edits: CLAUDE.md action = %q, want edited", got)
	}
	if err := runSyncArgs(t); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, entry); got == "hand edit\n" {
		t.Error("a plain sync should overwrite the hand edit")
	}
}

// An output the specs no longer produce is swept by a plain sync even
// when it carries a hand edit; --keep-edits keeps it.
func TestSyncKeepEdits_KeepsEditedOrphan(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	logBuf := captureLogOut(t)
	// Only a sync of every configured target sweeps orphans.
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte("version: 1\ntargets: [claude]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := filepath.Join(dir, ".agnostic-ai/rules/r2.md")
	if err := os.WriteFile(spec, []byte("---\nname: r2\n---\nsecond rule"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runSyncArgs(t); err != nil {
		t.Fatal(err)
	}
	// The edit keeps the provenance header, which alone lets the sweep
	// remove the file.
	out := filepath.Join(dir, ".claude/rules/r2.md")
	edited := readFile(t, out) + "\nhand edit\n"
	if err := os.WriteFile(out, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(spec); err != nil {
		t.Fatal(err)
	}
	logBuf.Reset()

	if err := runSyncArgs(t, "--keep-edits"); err != nil {
		t.Fatalf("sync --keep-edits: %v", err)
	}
	if got := readFile(t, out); got != edited {
		t.Errorf("edited orphan = %q, want it kept", got)
	}
	if !strings.Contains(logBuf.String(), "kept orphan") {
		t.Errorf("the run should name the kept orphan, got:\n%s", logBuf.String())
	}
}

// --quiet suppresses routine output, but a kept edit still needs to reach
// whoever runs sync from a hook, so it prints on stderr instead.
func TestSyncKeepEdits_QuietStillPrintsKeptEditOnStderr(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	syncClaudeThenEditSpec(t, dir)
	entry := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(entry, []byte("hand edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout string
	stderr := captureStderr(t, func() {
		stdout = captureStdout(t, func() {
			if err := runSyncArgs(t, "--keep-edits", "--quiet"); err != nil {
				t.Fatalf("sync --keep-edits --quiet: %v", err)
			}
		})
	})
	if !strings.Contains(stderr, "kept CLAUDE.md") {
		t.Errorf("--quiet should still report the kept file on stderr, got:\n%s", stderr)
	}
	if strings.Contains(stdout, "kept") {
		t.Errorf("--quiet should not duplicate the kept line on stdout, got:\n%s", stdout)
	}
}

// Same as above for an edited orphan: --quiet must still name it on stderr.
func TestSyncKeepEdits_QuietStillPrintsKeptOrphanOnStderr(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte("version: 1\ntargets: [claude]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := filepath.Join(dir, ".agnostic-ai/rules/r2.md")
	if err := os.WriteFile(spec, []byte("---\nname: r2\n---\nsecond rule"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runSyncArgs(t); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, ".claude/rules/r2.md")
	edited := readFile(t, out) + "\nhand edit\n"
	if err := os.WriteFile(out, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(spec); err != nil {
		t.Fatal(err)
	}

	stderr := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if err := runSyncArgs(t, "--keep-edits", "--quiet"); err != nil {
				t.Fatalf("sync --keep-edits --quiet: %v", err)
			}
		})
	})
	if !strings.Contains(stderr, "kept orphan") {
		t.Errorf("--quiet should still report the kept orphan on stderr, got:\n%s", stderr)
	}
}

func TestSyncKeepEditsJSON_ListsKeptFileAsSkippedEdited(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	syncClaudeThenEditSpec(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("hand edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"sync", "-t", "claude", "--json", "--keep-edits"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync --json --keep-edits: %v", err)
	}
	var result jsonOutput
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	var kept bool
	for _, r := range result.Skipped {
		if filepath.ToSlash(r.Path) == "CLAUDE.md" && r.Action == "edited" {
			kept = true
		}
	}
	if !kept {
		t.Errorf("skipped should list CLAUDE.md as edited, got %+v", result.Skipped)
	}
	for _, r := range result.Writes {
		if filepath.ToSlash(r.Path) == "CLAUDE.md" {
			t.Errorf("CLAUDE.md should not be in writes: %+v", r)
		}
	}
}

func TestSyncKeepEdits_RejectsModesThatDoNotWrite(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	for _, flag := range []string{"--check", "--watch", "--global"} {
		err := runSyncArgs(t, "--keep-edits", flag)
		if err == nil || !strings.Contains(err.Error(), "--keep-edits") {
			t.Errorf("--keep-edits %s: err = %v, want a flag conflict naming --keep-edits", flag, err)
		}
	}
}
