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

// syncClaudeThenEditSpec syncs claude, then changes the r1 rule spec so
// the next --check sees its output out of date with no hand edit.
func syncClaudeThenEditSpec(t *testing.T, dir string) {
	t.Helper()
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	editRuleSpec(t, dir, "rule body\n\nAlso check rounding.\n")
}

func editRuleSpec(t *testing.T, dir, body string) {
	t.Helper()
	spec := filepath.Join(dir, ".agnostic-ai/rules/r1.md")
	if err := os.WriteFile(spec, []byte("---\nname: r1\n---\n"+body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// checkJSONActions runs `sync --check --json` for claude and maps each
// reported path to its action.
func checkJSONActions(t *testing.T) map[string]string {
	t.Helper()
	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"sync", "-t", "claude", "--check", "--json"})
	_ = root.Execute()
	var result struct {
		Writes []fileRecord `json:"writes"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	actions := map[string]string{}
	for _, w := range result.Writes {
		actions[filepath.ToSlash(w.Path)] = w.Action
	}
	return actions
}

func TestSyncCheck_SpecChangeReportsOutputOutOfDate(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	logBuf := captureLogOut(t)
	syncClaudeThenEditSpec(t, dir)
	logBuf.Reset()

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude", "--check"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected drift error after a spec change")
	}

	got := logBuf.String()
	if !strings.Contains(got, "out of date") || !strings.Contains(got, ".claude/rules/r1.md") {
		t.Errorf("a spec change should list the output as out of date, got:\n%s", got)
	}
	if strings.Contains(got, "edited locally") {
		t.Errorf("a spec change is not a local edit, got:\n%s", got)
	}
}

func TestSyncCheck_HandEditReportsEditedLocally(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	logBuf := captureLogOut(t)
	syncThenEditRule(t, dir)
	logBuf.Reset()

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude", "--check"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected drift error after a hand edit")
	}

	got := logBuf.String()
	if !strings.Contains(got, "edited locally") || !strings.Contains(got, ".claude/rules/r1.md") {
		t.Errorf("a hand edit should be listed as edited locally, got:\n%s", got)
	}
	if strings.Contains(got, "out of date") {
		t.Errorf("a hand edit should not read as out of date, got:\n%s", got)
	}
}

func TestSyncCheckJSON_SeparatesHandEditsFromSpecChanges(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	syncClaudeThenEditSpec(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("hand-edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	actions := checkJSONActions(t)
	if actions[".claude/rules/r1.md"] != "stale" {
		t.Errorf("spec change action = %q, want stale; all: %v", actions[".claude/rules/r1.md"], actions)
	}
	if actions["CLAUDE.md"] != "edited" {
		t.Errorf("hand edit action = %q, want edited; all: %v", actions["CLAUDE.md"], actions)
	}
}

// Without a record of what the last sync wrote there is no proof of a
// hand edit, as after a lost state file or in a fresh CI checkout.
func TestSyncCheckJSON_NoLedgerReportsStale(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	syncThenEditRule(t, dir)
	if err := os.Remove(stateFilePath(dir)); err != nil {
		t.Fatal(err)
	}

	if got := checkJSONActions(t)[".claude/rules/r1.md"]; got != "stale" {
		t.Errorf("action without a ledger = %q, want stale", got)
	}
}

// `doctor --fix` writes outputs as sync does, so a later spec change must
// not read the fixed file as a hand edit.
func TestDoctorFix_RecordsWhatItWrote(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	syncClaudeThenEditSpec(t, dir)

	root := NewRootCmd("test")
	root.SetArgs([]string{"doctor", "-t", "claude", "--fix"})
	if err := root.Execute(); err != nil {
		t.Fatalf("doctor --fix: %v", err)
	}
	editRuleSpec(t, dir, "rule body\n\nRound half to even.\n")

	if got := checkJSONActions(t)[".claude/rules/r1.md"]; got != "stale" {
		t.Errorf("action after doctor --fix and a spec change = %q, want stale", got)
	}
}
