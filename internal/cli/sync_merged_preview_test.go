package cli

import (
	"bytes"
	"encoding/json"
	"maps"
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const mergedPreviewHook = "name: status\ndescription: show status\non: session-start\ncommand: git status --short\n"

// syncedHookProject syncs claude with one hook, so .claude/settings.json
// holds only sync's hook, then lets edit change it and drops the hook spec.
func syncedHookProject(t *testing.T, edit func(string) string) {
	t.Helper()
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	testutil.TempCwd(t)
	writeFile(t, ".agnostic-ai/hooks/status.yaml", mergedPreviewHook)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	captureLogOut(t)
	if out, err := runCLI(t, "sync", "--all"); err != nil {
		t.Fatalf("first sync: %v\n%s", err, out)
	}
	data, err := os.ReadFile(".claude/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, ".claude/settings.json", edit(string(data)))
	if err := os.Remove(".agnostic-ai/hooks/status.yaml"); err != nil {
		t.Fatal(err)
	}
}

func dryRunJSONAction(t *testing.T, path string) string {
	t.Helper()
	out, err := runSyncStdout(t, "sync", "--dry-run", "--json")
	if err != nil {
		t.Fatalf("--dry-run --json: %v\n%s", err, out)
	}
	var got struct {
		Writes  []fileRecord `json:"writes"`
		Skipped []fileRecord `json:"skipped"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, r := range append(got.Writes, got.Skipped...) {
		if r.Path == path {
			return r.Action
		}
	}
	return ""
}

func TestSync_DryRunDiffShowsAStrippedSettingsFileAsAnUpdate(t *testing.T) {
	syncedHookProject(t, func(s string) string {
		return strings.Replace(s, "{", "{\n  \"myKey\": 1,", 1)
	})
	before := snapshotTree(t, ".")

	out, err := runSyncStdout(t, "sync", "--dry-run", "--diff")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "would remove .claude/settings.json") {
		t.Errorf("sync keeps the file, so the preview must not remove it:\n%s", out)
	}
	for _, want := range []string{"--- .claude/settings.json (on disk)", "-  \"hooks\": {"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if got := dryRunJSONAction(t, ".claude/settings.json"); got != "update" {
		t.Errorf("--dry-run --json action = %q, want update", got)
	}
	plan, err := runSyncStdout(t, "sync", "--plan")
	if err != nil || strings.Contains(plan, "removed:") || !strings.Contains(plan, "changed: 1") {
		t.Errorf("--plan counts the stripped file as changed, got err = %v:\n%s", err, plan)
	}
	if after := snapshotTree(t, "."); !maps.Equal(before, after) {
		t.Error("previews must change no files")
	}

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	data, err := os.ReadFile(".claude/settings.json")
	if err != nil || strings.Contains(string(data), "hooks") || !strings.Contains(string(data), "myKey") {
		t.Errorf("sync strips its hook and keeps the file, got %q, %v", data, err)
	}
}

func TestSync_DryRunDiffKeepsASettingsFileWithAnEditedValue(t *testing.T) {
	syncedHookProject(t, func(s string) string {
		return strings.Replace(s, "git status --short", "git status", 1)
	})

	out, err := runSyncStdout(t, "sync", "--dry-run", "--diff")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "would remove .claude/settings.json") {
		t.Errorf("sync keeps an edited value, so the preview must not remove it:\n%s", out)
	}
	if got := dryRunJSONAction(t, ".claude/settings.json"); got == "delete" {
		t.Errorf("--dry-run --json must not delete an edited file")
	}
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if _, err := os.Stat(".claude/settings.json"); err != nil {
		t.Errorf("sync keeps the edited file: %v", err)
	}
}

func TestSync_DryRunDiffStillRemovesASettingsFileSyncOwns(t *testing.T) {
	syncedHookProject(t, func(s string) string { return s })

	out, err := runSyncStdout(t, "sync", "--dry-run", "--diff")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "would remove .claude/settings.json") {
		t.Errorf("sync deletes a file it created and fully owns:\n%s", out)
	}
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if _, err := os.Stat(".claude/settings.json"); err == nil {
		t.Error("sync removes the file it fully owns")
	}
}

func runSyncStdout(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func checkJSONAction(t *testing.T, path string) string {
	t.Helper()
	out, err := runSyncStdout(t, "sync", "--check", "--json")
	if err == nil {
		t.Fatalf("--check --json exits non-zero on drift:\n%s", out)
	}
	var got struct {
		Writes []fileRecord `json:"writes"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, r := range got.Writes {
		if r.Path == path {
			return r.Action
		}
	}
	return ""
}

func TestSyncCheck_ReportsAStrippedSettingsFileAsStale(t *testing.T) {
	syncedHookProject(t, func(s string) string {
		return strings.Replace(s, "{", "{\n  \"myKey\": 1,", 1)
	})
	if got := checkJSONAction(t, ".claude/settings.json"); got != "stale" {
		t.Errorf("--check --json action = %q, want stale", got)
	}
	logs := captureLogOut(t)
	if _, err := runSyncStdout(t, "sync", "--check"); err == nil {
		t.Error("--check exits non-zero on drift")
	}
	if got := logs.String(); !strings.Contains(got, "out of date") || strings.Contains(got, "no longer generated") {
		t.Errorf("--check lists the stripped file as out of date:\n%s", got)
	}
}

// Sync strips its other keys and keeps the hook the user edited, so
// check reports a change, not a removal.
func TestSyncCheck_ReportsAnEditedSettingsFileAsStale(t *testing.T) {
	syncedHookProject(t, func(s string) string {
		return strings.Replace(s, "git status --short", "git status", 1)
	})
	if got := checkJSONAction(t, ".claude/settings.json"); got != "stale" {
		t.Errorf("--check --json action = %q, want stale", got)
	}
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	data, err := os.ReadFile(".claude/settings.json")
	if err != nil || !strings.Contains(string(data), "git status") {
		t.Errorf("sync keeps the edited hook, got %q, %v", data, err)
	}
}

func TestSync_PlainDryRunPrintsAStrippedSettingsFile(t *testing.T) {
	syncedHookProject(t, func(s string) string {
		return strings.Replace(s, "{", "{\n  \"myKey\": 1,", 1)
	})
	before := snapshotTree(t, ".")
	var err error
	out := captureStdout(t, func() { _, err = runSyncStdout(t, "sync", "--dry-run") })
	if err != nil {
		t.Fatal(err)
	}
	head := strings.Index(out, "--- .claude/settings.json ---")
	if head < 0 || !strings.Contains(out[head:], "myKey") || strings.Contains(out[head:], "SessionStart") {
		t.Errorf("plain --dry-run prints the stripped file as sync writes it:\n%s", out)
	}
	if after := snapshotTree(t, "."); !maps.Equal(before, after) {
		t.Error("previews must change no files")
	}
}
