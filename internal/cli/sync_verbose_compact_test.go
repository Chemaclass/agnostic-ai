package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSyncVerbose_CollapsesRepeatedCoverageAndShowsNewNotes(t *testing.T) {
	dir := testutil.TempCwd(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [cline, codex]\nsync:\n  dropped-summary: true\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "settings", "defaults.yaml"), "model: claude-opus-4-8\n")
	skill := filepath.Join(dir, ".agnostic-ai", "skills", "deploy.md")
	mustWriteFile(t, skill, "---\nname: deploy\nargument-hint: '[version]'\n---\nDeploy.\n")
	silence(t)
	log := captureLog(t)
	diagnostics := captureNotes(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	first := diagnostics.String()
	if !strings.Contains(first, "settings unsupported by cline") || !strings.Contains(first, "`argument-hint`") {
		t.Fatalf("first sync did not show the warning and note:\n%s", first)
	}

	log.Reset()
	diagnostics.Reset()
	verbosity = levelVerbose
	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	if got := diagnostics.String(); strings.Contains(got, "settings unsupported by cline") || strings.Contains(got, "`argument-hint`") {
		t.Errorf("-v repeated unchanged coverage:\n%s", got)
	}
	if got := log.String(); !strings.Contains(got, "capability warning") || !strings.Contains(got, "coverage note") || !strings.Contains(got, "-vv") {
		t.Errorf("-v did not summarize the hidden warnings and notes with a full-detail hint:\n%s", got)
	}

	log.Reset()
	diagnostics.Reset()
	verbosity = levelVerbose + 1
	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	if got := diagnostics.String(); !strings.Contains(got, "settings unsupported by cline") || !strings.Contains(got, "`argument-hint`") {
		t.Errorf("-vv did not replay the warning and note:\n%s", got)
	}

	mustWriteFile(t, skill, "---\nname: deploy\nargument-hint: '[version]'\ndisable-model-invocation: true\n---\nDeploy.\n")
	log.Reset()
	diagnostics.Reset()
	verbosity = levelQuiet
	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	log.Reset()
	diagnostics.Reset()
	verbosity = levelVerbose
	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	if got := diagnostics.String(); !strings.Contains(got, "`disable-model-invocation`") || !strings.Contains(got, "fix:") {
		t.Errorf("-v hid a new actionable coverage note:\n%s", got)
	}
	if got := diagnostics.String(); strings.Contains(got, "settings unsupported by cline") {
		t.Errorf("a new note replayed an unchanged capability warning:\n%s", got)
	}
}

func TestSyncVerbose_LayeredProjectShowsResultBeforeDiagnostics(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	mustWriteFile(t, "agnostic-ai.local.yaml", "version: 1\n")
	silence(t)
	log := captureLog(t)
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-v", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	out := log.String()
	if !strings.HasPrefix(out, "✓ ") || strings.Count(out, "merged 2 config layers") != 1 {
		t.Errorf("expected result before one layer diagnostic:\n%s", out)
	}
}

func TestSyncVerbose_PartialSyncPreservesOtherTargetsDiagnostics(t *testing.T) {
	dir := testutil.TempCwd(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [cline, codex]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "settings", "defaults.yaml"), "model: claude-opus-4-8\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "deploy.md"), "---\nname: deploy\nargument-hint: '[version]'\n---\nDeploy.\n")
	silence(t)
	log := captureLog(t)
	diagnostics := captureNotes(t)
	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	verbosity = levelVerbose
	log.Reset()
	diagnostics.Reset()
	if err := runSyncOnce(".", []string{"cline"}, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	log.Reset()
	diagnostics.Reset()
	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	if got := diagnostics.String(); strings.Contains(got, "`argument-hint`") || strings.Contains(got, "settings unsupported") {
		t.Errorf("full sync replayed unchanged diagnostics after a partial sync:\n%s", got)
	}
}

func TestPartialDropRecordsKeepUnselectedAndProjectEntries(t *testing.T) {
	previous := []string{"legacy-hash", "cline\x00old", "codex\x00keep", "agnostic-ai\x00project"}
	current := []string{"cline\x00new", "agnostic-ai\x00partial-project"}
	got := mergePartialDropRecords(previous, current, []string{"cline"})
	want := []string{"agnostic-ai\x00partial-project", "cline\x00new", "codex\x00keep"}
	if !slices.Equal(got, want) {
		t.Errorf("merged records = %q, want %q", got, want)
	}
	fingerprints := fingerprintSet(append(got, "legacy-hash"))
	for _, hash := range []string{"legacy-hash", "partial-project", "new", "keep"} {
		if !fingerprints[hash] {
			t.Errorf("missing fingerprint %q", hash)
		}
	}
	if fingerprints["old"] || fingerprints["project"] {
		t.Errorf("kept stale fingerprints: %v", fingerprints)
	}
}

func TestSyncVerbose_PartialSyncRemembersNewProjectNote(t *testing.T) {
	dir := testutil.TempCwd(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [codex, claude]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "deploy.md"), "---\nname: deploy\nargument-hint: '[version]'\n---\nDeploy.\n")
	silence(t)
	captureLog(t)
	diagnostics := captureNotes(t)
	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "overlays", "codex.config.toml"), "notify = [\"echo\"]\n")
	verbosity = levelVerbose
	diagnostics.Reset()
	if err := runSyncOnce(".", []string{"codex"}, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	if got := diagnostics.String(); !strings.Contains(got, "sets notify") {
		t.Fatalf("first partial sync did not show new project note:\n%s", got)
	}
	diagnostics.Reset()
	if err := runSyncOnce(".", []string{"codex"}, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	if got := diagnostics.String(); strings.Contains(got, "sets notify") {
		t.Errorf("second partial sync repeated unchanged project note:\n%s", got)
	}
	overlay := filepath.Join(dir, ".agnostic-ai", "overlays", "codex.config.toml")
	if err := os.Remove(overlay); err != nil {
		t.Fatal(err)
	}
	diagnostics.Reset()
	if err := runSyncOnce(".", []string{"codex"}, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, overlay, "notify = [\"echo\"]\n")
	diagnostics.Reset()
	if err := runSyncOnce(".", []string{"codex"}, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	if got := diagnostics.String(); !strings.Contains(got, "sets notify") {
		t.Errorf("partial sync hid reintroduced project note:\n%s", got)
	}
}
