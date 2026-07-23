package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// captureLogOut redirects the summary sink (logOut) to a buffer for the test
// so printDrift's human table neither leaks to the terminal nor pollutes the
// stdout captured via SetOut. Restored on cleanup.
func captureLogOut(t *testing.T) *bytes.Buffer {
	t.Helper()
	prev := logOut
	buf := &bytes.Buffer{}
	logOut = buf
	t.Cleanup(func() { logOut = prev })
	return buf
}

// syncThenEditRule syncs claude, then overwrites the emitted r1 rule with
// hand-edited content so the next --check sees one stale file.
func syncThenEditRule(t *testing.T, dir string) string {
	t.Helper()
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	rule := filepath.Join(dir, ".claude/rules/r1.md")
	if err := os.WriteFile(rule, []byte("hand-edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return rule
}

func TestSyncCheck_Diff_ShowsChangedLines(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	syncThenEditRule(t, dir)

	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"sync", "-t", "claude", "--check", "--diff"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected drift error from --check --diff")
	}

	got := out.String()
	if !strings.Contains(got, "-hand-edited") {
		t.Errorf("diff should show the removed on-disk line, got:\n%s", got)
	}
	if !strings.Contains(got, "+rule body") {
		t.Errorf("diff should show the would-be content, got:\n%s", got)
	}
	if !strings.Contains(got, "@@") {
		t.Errorf("diff should carry a unified hunk header, got:\n%s", got)
	}
}

func TestSyncCheck_FormatGitHub_EmitsErrorAnnotations(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	syncThenEditRule(t, dir)

	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"sync", "-t", "claude", "--check", "--format=github"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected drift error from --check --format=github")
	}

	got := out.String()
	if !strings.Contains(got, "::error file=.claude/rules/r1.md") {
		t.Errorf("expected a github error annotation for the stale file, got:\n%s", got)
	}
	if !strings.Contains(got, "line=") {
		t.Errorf("stale annotation should carry a line number, got:\n%s", got)
	}
}

func TestSyncCheck_FormatGitHub_ReportsMissingFile(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)

	// No sync first: every emitted file is missing.
	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"sync", "-t", "claude", "--check", "--format=github"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected drift error when files are missing")
	}

	got := out.String()
	if !strings.Contains(got, "::error file=") || !strings.Contains(got, "is missing") {
		t.Errorf("expected a github missing-file annotation, got:\n%s", got)
	}
}

func TestSyncCheck_FixHint_PrintsReconcileCommandOnStderr(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	syncThenEditRule(t, dir)

	var errBuf bytes.Buffer
	root := NewRootCmd("test")
	root.SetErr(&errBuf)
	root.SetArgs([]string{"sync", "-t", "claude", "--check"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected drift error")
	}
	if !strings.Contains(errBuf.String(), "agnostic-ai sync") {
		t.Errorf("stderr should print the reconcile command, got:\n%s", errBuf.String())
	}
	// The drift error itself points at doctor for a full diagnosis.
	if !strings.Contains(err.Error(), "agnostic-ai doctor") {
		t.Errorf("drift error should point at doctor, got: %v", err)
	}
}

func TestSyncCheck_CleanRun_StaysSilent(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	logBuf := captureLogOut(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	logBuf.Reset() // drop the initial sync's summary; only the --check below matters

	var out, errBuf bytes.Buffer
	root = NewRootCmd("test")
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"sync", "-t", "claude", "--check", "--diff", "--format=github"})
	if err := root.Execute(); err != nil {
		t.Fatalf("clean --check should not error, got: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("clean --check should print nothing to stdout, got:\n%s", out.String())
	}
	if errBuf.Len() != 0 {
		t.Errorf("clean --check should print no fix hint, got:\n%s", errBuf.String())
	}
	if logBuf.Len() != 0 {
		t.Errorf("clean --check should print no summary, got:\n%s", logBuf.String())
	}
}

func TestSyncCheck_InvalidFormat_Errors(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude", "--check", "--format=bogus"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for unknown --format value")
	}
	if !strings.Contains(err.Error(), "--format") {
		t.Errorf("error should name the --format flag, got: %v", err)
	}
}
