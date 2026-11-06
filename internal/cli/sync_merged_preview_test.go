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

// The user took out every key sync set, so sync has nothing to do and
// check must agree before and after it runs.
func TestSyncCheck_IgnoresASettingsFileWithNoSyncKeysLeft(t *testing.T) {
	syncedHookProject(t, func(string) string { return "{\n  \"myKey\": 1\n}\n" })
	before := snapshotTree(t, ".")
	_, checkErr := runSyncStdout(t, "sync", "--check")
	plan, err := runSyncStdout(t, "sync", "--plan")
	if err != nil || strings.Contains(plan, "orphaned") {
		t.Errorf("--plan has nothing to do with the file, got err = %v:\n%s", err, plan)
	}
	if got := dryRunJSONAction(t, ".claude/settings.json"); got != "" {
		t.Errorf("--dry-run --json lists the file as %q", got)
	}
	if after := snapshotTree(t, "."); !maps.Equal(before, after) {
		t.Error("previews must change no files")
	}
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	_, afterErr := runSyncStdout(t, "sync", "--check")
	if (checkErr == nil) != (afterErr == nil) {
		t.Errorf("--check before sync = %v, after = %v; sync changed nothing", checkErr, afterErr)
	}
}

// markMergedUnrecorded rewrites the ledger as an older sync left it,
// with no record of which keys in path are sync's.
func markMergedUnrecorded(t *testing.T, path string) {
	t.Helper()
	const state = ".agnostic-ai/.sync-state"
	data, err := os.ReadFile(state)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	doc["merged"].(map[string]any)[path] = map[string]any{"unrecorded": true}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, state, string(out))
}

func TestSync_DryRunDiffKeepsAnUnrecordedSettingsFile(t *testing.T) {
	syncedHookProject(t, func(s string) string {
		return strings.Replace(s, "{", "{\n  \"myKey\": 1,", 1)
	})
	markMergedUnrecorded(t, ".claude/settings.json")
	out, err := runSyncStdout(t, "sync", "--dry-run", "--diff")
	if err != nil || strings.Contains(out, "would remove .claude/settings.json") {
		t.Errorf("sync keeps an unrecorded file, got err = %v:\n%s", err, out)
	}
	if got := dryRunJSONAction(t, ".claude/settings.json"); got != "orphan" {
		t.Errorf("--dry-run --json action = %q, want orphan", got)
	}
	logs := captureLogOut(t)
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if _, err := os.Stat(".claude/settings.json"); err != nil || !strings.Contains(logs.String(), "kept orphan") {
		t.Errorf("sync keeps the file as an orphan, got %v:\n%s", err, logs.String())
	}
}

// With --keep-edits the sweep leaves a settings file edited since the
// last sync alone, so the dry run prints no rewrite of it.
func TestSync_PlainDryRunKeepEditsLeavesAnEditedSettingsFile(t *testing.T) {
	syncedHookProject(t, func(s string) string {
		return strings.Replace(s, "{", "{\n  \"myKey\": 1,", 1)
	})
	var err error
	out := captureStdout(t, func() { _, err = runSyncStdout(t, "sync", "--dry-run", "--keep-edits") })
	if err != nil || strings.Contains(out, "--- .claude/settings.json ---") {
		t.Errorf("--keep-edits leaves the edited file, got err = %v:\n%s", err, out)
	}
	before, _ := os.ReadFile(".claude/settings.json")
	if out, err := runCLI(t, "sync", "--keep-edits"); err != nil {
		t.Fatalf("sync --keep-edits: %v\n%s", err, out)
	}
	if after, _ := os.ReadFile(".claude/settings.json"); string(after) != string(before) {
		t.Errorf("sync --keep-edits changed the file:\n%s", after)
	}
}

// A -t run covers one target, so the real run releases no other
// target's merged file, and the dry run must not print one.
func TestSync_PlainDryRunForOneTargetLeavesOtherMergedFiles(t *testing.T) {
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	testutil.TempCwd(t)
	writeFile(t, ".agnostic-ai/hooks/status.yaml", mergedPreviewHook)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cursor]\n")
	captureLogOut(t)
	if out, err := runCLI(t, "sync", "--all"); err != nil {
		t.Fatalf("first sync: %v\n%s", err, out)
	}
	data, err := os.ReadFile(".cursor/hooks.json")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, ".cursor/hooks.json", strings.Replace(string(data), "{", "{\n  \"myKey\": 1,", 1))
	for _, args := range [][]string{{"-t", "claude"}, {"--only", "claude"}, {"--except", "cursor"}} {
		var err error
		out := captureStdout(t, func() { _, err = runSyncStdout(t, append([]string{"sync", "--dry-run"}, args...)...) })
		if err != nil || strings.Contains(out, "--- .cursor/hooks.json ---") {
			t.Errorf("%v leaves cursor's file, got err = %v:\n%s", args, err, out)
		}
	}
	before, _ := os.ReadFile(".cursor/hooks.json")
	if out, err := runCLI(t, "sync", "-t", "claude"); err != nil {
		t.Fatalf("sync -t claude: %v\n%s", err, out)
	}
	if after, _ := os.ReadFile(".cursor/hooks.json"); string(after) != string(before) {
		t.Errorf("sync -t claude changed cursor's file:\n%s", after)
	}
}

func TestDoctor_ReportsStrippedSettingsWithoutChangingRepairOwnership(t *testing.T) {
	syncedHookProject(t, func(s string) string {
		return strings.Replace(s, "{", "{\n  \"myKey\": 1,", 1)
	})
	before := snapshotTree(t, ".")
	logs := captureLogOut(t)
	if _, err := runCLI(t, "doctor"); err == nil {
		t.Error("doctor must report drift")
	}
	if got := logs.String(); !strings.Contains(got, "removes unchanged managed keys") || !strings.Contains(got, "keeps your keys") {
		t.Errorf("doctor must explain the shared file update:\n%s", got)
	}
	out, err := runCLI(t, "doctor", "--json")
	if err == nil || !strings.Contains(out, `"action": "stale"`) {
		t.Errorf("doctor JSON must report stale: %v\n%s", err, out)
	}
	if after := snapshotTree(t, "."); !maps.Equal(before, after) {
		t.Error("read-only doctor changed files")
	}
	if out, err := runCLI(t, "doctor", "--fix"); err != nil {
		t.Fatalf("doctor --fix: %v\n%s", err, out)
	}
	if _, ok := readStateFile(".").Merged[".claude/settings.json"]; ok {
		t.Error("doctor --fix must release merged ownership")
	}
	data, err := os.ReadFile(".claude/settings.json")
	if err != nil || !strings.Contains(string(data), "myKey") || strings.Contains(string(data), "hooks") {
		t.Errorf("doctor --fix must retain only user keys: %q, %v", data, err)
	}
}
