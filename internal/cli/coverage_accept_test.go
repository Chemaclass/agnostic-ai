package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const codexToolsNote = "`tools` on 1 agent has no effect on codex"

const acceptCodexTools = `coverage:
  accept:
    - target: codex
      kind: agents
      field: tools
      reason: Codex limits come from sandbox_mode.
`

// setupCoverageAcceptProject writes one agent whose portable `tools` list
// raises a field note on codex, with config appended to the base config.
func setupCoverageAcceptProject(t *testing.T, config string) {
	t.Helper()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\n"+config)
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "rev.md"), "---\nname: rev\ndescription: Reviews code.\ntools: [Read, Grep]\n---\nReview.\n")
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
}

func withVerbosity(t *testing.T, level int) {
	t.Helper()
	prev := verbosity
	verbosity = level
	t.Cleanup(func() { verbosity = prev })
}

func TestSync_AcceptedCoverageNoteStopsPrinting(t *testing.T) {
	setupCoverageAcceptProject(t, acceptCodexTools)
	notes := captureNotes(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(notes.String(), codexToolsNote) {
		t.Errorf("an accepted note still printed:\n%s", notes)
	}
	if strings.Contains(notes.String(), "accepted") {
		t.Errorf("a plain sync should not list accepted notes:\n%s", notes)
	}
}

func TestSync_VerboseListsAcceptedCoverageNoteWithReason(t *testing.T) {
	setupCoverageAcceptProject(t, acceptCodexTools)
	notes := captureNotes(t)
	withVerbosity(t, levelVerbose)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}

	want := "  accepted: " + codexToolsNote + " (reason: Codex limits come from sandbox_mode.)\n"
	if !strings.Contains(notes.String(), want) {
		t.Errorf("sync -v should list the accepted note with its reason, want %q in:\n%s", want, notes)
	}
}

func TestSync_OnUnsupportedErrorIgnoresAcceptedNote(t *testing.T) {
	setupCoverageAcceptProject(t, "on-unsupported: error\n"+acceptCodexTools)
	captureNotes(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatalf("an accepted note must not fail on-unsupported: error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(".codex", "agents", "rev.toml")); err != nil {
		t.Errorf("sync should have written the agent: %v", err)
	}
}

func TestSync_OnUnsupportedErrorFailsOnUnacceptedNote(t *testing.T) {
	setupCoverageAcceptProject(t, `on-unsupported: error
coverage:
  accept:
    - target: codex
      kind: agents
      field: model
      reason: Not the note this sync raises.
`)
	notes := captureNotes(t)

	err := runSyncOnce(".", nil, false, false, "off", 1)

	if err == nil {
		t.Fatal("an unaccepted note must fail on-unsupported: error")
	}
	if !strings.Contains(err.Error(), "coverage.accept") {
		t.Errorf("the error should point at coverage.accept, got %q", err)
	}
	if !strings.Contains(notes.String(), codexToolsNote) {
		t.Errorf("the failing note should print:\n%s", notes)
	}
	if _, statErr := os.Stat(filepath.Join(".codex", "agents", "rev.toml")); !os.IsNotExist(statErr) {
		t.Errorf("a failed sync should roll back its writes: %v", statErr)
	}
}

func TestLint_WarnsOnAcceptEntryThatMatchesNoNote(t *testing.T) {
	setupCoverageAcceptProject(t, acceptCodexTools+`    - target: codex
      kind: agents
      field: model
      reason: Stale entry.
`)

	out, err := runCLI(t, "lint")

	if err != nil {
		t.Fatalf("a stale accept entry is a warning, not an error: %v\n%s", err, out)
	}
	if !strings.Contains(out, "LINT022 [warn] agnostic-ai.yaml: coverage.accept entry codex agents `model` matches no coverage note") {
		t.Errorf("lint should warn on the stale entry:\n%s", out)
	}
	if strings.Contains(out, "`tools` matches no") {
		t.Errorf("lint flagged an entry that matches a note:\n%s", out)
	}
}

func TestLint_AcceptEntryThatMatchesPassesClean(t *testing.T) {
	setupCoverageAcceptProject(t, acceptCodexTools)

	out, err := runCLI(t, "lint", "--strict")

	if err != nil {
		t.Fatalf("a matching accept entry should lint clean: %v\n%s", err, out)
	}
	if strings.Contains(out, "LINT022") {
		t.Errorf("unexpected LINT022:\n%s", out)
	}
}

func TestDoctor_ShowsAcceptedCoverageNoteCount(t *testing.T) {
	setupCoverageAcceptProject(t, acceptCodexTools)
	syncProject(t)

	out, _ := runDoctor(t)

	if !strings.Contains(out, "Coverage notes:\n  1 accepted (sync -v lists them)\n") {
		t.Errorf("doctor should show the accepted note count:\n%s", out)
	}
}
