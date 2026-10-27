package cli

import (
	"path/filepath"
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
