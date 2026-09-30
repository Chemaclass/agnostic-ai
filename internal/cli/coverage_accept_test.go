package cli

import (
	"encoding/json"
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

const failOnNotes = `coverage:
  fail-on-notes: true
`

const failOnNotesAcceptingTools = `coverage:
  fail-on-notes: true
  accept:
    - target: [codex]
      kind: agents
      field: tools
      reason: Codex limits come from sandbox_mode.
`

var codexAgentFile = filepath.Join(".codex", "agents", "rev.toml")

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

	want := "  accepted: " + codexToolsNote + " (Codex uses tools as a configuration table, not a Claude-style allowlist; set x-codex.tools for Codex-native tool settings)\n    reason: Codex limits come from sandbox_mode.\n"
	if !strings.Contains(notes.String(), want) {
		t.Errorf("sync -v should list the accepted note with its reason, want %q in:\n%s", want, notes)
	}
}

func TestSync_OnUnsupportedErrorStillIgnoresCoverageNotes(t *testing.T) {
	setupCoverageAcceptProject(t, "on-unsupported: error\n")
	captureNotes(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatalf("on-unsupported: error never failed on a coverage note: %v", err)
	}
}

func TestSync_FailOnNotesIgnoresAcceptedNote(t *testing.T) {
	setupCoverageAcceptProject(t, failOnNotesAcceptingTools)
	captureNotes(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatalf("an accepted note must not fail coverage.fail-on-notes: %v", err)
	}
	if _, err := os.Stat(codexAgentFile); err != nil {
		t.Errorf("sync should have written the agent: %v", err)
	}
}

func TestSync_FailOnNotesFailsOnUnacceptedNote(t *testing.T) {
	setupCoverageAcceptProject(t, failOnNotes)
	notes := captureNotes(t)

	err := runSyncOnce(".", nil, false, false, "off", 1)

	if err == nil || !strings.Contains(err.Error(), "coverage.fail-on-notes") || !strings.Contains(err.Error(), "coverage.accept") {
		t.Fatalf("an unaccepted note must fail coverage.fail-on-notes and point at coverage.accept, got %v", err)
	}
	if !strings.Contains(notes.String(), codexToolsNote) {
		t.Errorf("the failing note should print:\n%s", notes)
	}
	if _, statErr := os.Stat(codexAgentFile); !os.IsNotExist(statErr) {
		t.Errorf("a failed sync should roll back its writes: %v", statErr)
	}
}

func TestSyncJSON_FailOnNotesFailsAndRollsBack(t *testing.T) {
	setupCoverageAcceptProject(t, failOnNotes)
	captureNotes(t)

	out, err := runCLI(t, "sync", "--json")

	if err == nil || !strings.Contains(err.Error(), "coverage.fail-on-notes") {
		t.Fatalf("sync --json must fail on an unaccepted note, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "coverage.fail-on-notes") {
		t.Errorf("the JSON errors should name the failure:\n%s", out)
	}
	if _, statErr := os.Stat(codexAgentFile); !os.IsNotExist(statErr) {
		t.Errorf("a failed sync --json should roll back its writes: %v", statErr)
	}
}

func TestSyncJSON_FailOnNotesPassesWhenAccepted(t *testing.T) {
	setupCoverageAcceptProject(t, failOnNotesAcceptingTools)
	captureNotes(t)

	if out, err := runCLI(t, "sync", "--json"); err != nil {
		t.Fatalf("an accepted note must not fail sync --json: %v\n%s", err, out)
	}
	if _, err := os.Stat(codexAgentFile); err != nil {
		t.Errorf("sync --json should have written the agent: %v", err)
	}
}

func TestSyncCheck_FailOnNotesFailsOnUnacceptedNote(t *testing.T) {
	setupCoverageAcceptProject(t, failOnNotesAcceptingTools)
	notes := captureNotes(t)
	syncProject(t)
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Fatalf("an in-sync project with every note accepted should pass --check: %v\n%s", err, out)
	}
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n"+failOnNotes)

	for _, args := range [][]string{{"sync", "--check"}, {"sync", "--check", "--json"}} {
		notes.Reset()
		out, err := runCLI(t, args...)
		if err == nil || !strings.Contains(err.Error(), "coverage.fail-on-notes") {
			t.Errorf("%v must fail on an unaccepted note, got %v\n%s", args, err, out)
		}
		if !strings.Contains(notes.String(), codexToolsNote) {
			t.Errorf("%v should print the failing note:\n%s", args, notes)
		}
	}
	out, _ := runCLI(t, "sync", "--check", "--json")
	var report map[string]any
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Errorf("sync --check --json should still print its report: %v\n%s", err, out)
	}
}

func TestSync_RejectsUnknownAcceptTarget(t *testing.T) {
	setupCoverageAcceptProject(t, `coverage:
  accept:
    - target: [codex, codx]
      kind: agents
      field: tools
      reason: Typo.
`)

	_, err := runCLI(t, "sync")

	if err == nil || !strings.Contains(err.Error(), `coverage.accept[0]: unknown target "codx" (did you mean codex?)`) {
		t.Errorf("an unknown accept target should fail config loading, got %v", err)
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
	if !strings.Contains(out, "LINT024 [warn] agnostic-ai.yaml: coverage.accept entry codex agents `model` matches no coverage note") {
		t.Errorf("lint should warn on the stale entry:\n%s", out)
	}
	if strings.Contains(out, "`tools` matches no") {
		t.Errorf("lint flagged an entry that matches a note:\n%s", out)
	}
}

func TestLint_TargetThatFailsToEmitIsAFindingNotAFailure(t *testing.T) {
	setupCoverageAcceptProject(t, "")
	failingAdapterOnPath(t, "flaky")
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex, flaky]\n"+acceptCodexTools+`    - target: flaky
      kind: agents
      field: tools
      reason: Unchecked.
`)

	out, err := runCLI(t, "lint")

	if err != nil {
		t.Fatalf("an emit error must not abort lint: %v\n%s", err, out)
	}
	if !strings.Contains(out, "LINT024 [warn] agnostic-ai.yaml: coverage.accept entries for flaky were not checked") {
		t.Errorf("lint should report the target it could not check:\n%s", out)
	}
	if strings.Contains(out, "matches no coverage note") {
		t.Errorf("an entry on a target that failed to emit is not stale:\n%s", out)
	}
}

func TestLint_AcceptEntryThatMatchesPassesClean(t *testing.T) {
	setupCoverageAcceptProject(t, acceptCodexTools)

	out, err := runCLI(t, "lint", "--strict")

	if err != nil {
		t.Fatalf("a matching accept entry should lint clean: %v\n%s", err, out)
	}
	if strings.Contains(out, "LINT024") {
		t.Errorf("unexpected LINT024:\n%s", out)
	}
}

func TestDoctor_ShowsAcceptedCoverageNoteCount(t *testing.T) {
	setupCoverageAcceptProject(t, acceptCodexTools)
	syncProject(t)

	out, _ := runDoctor(t)

	if !strings.Contains(out, "Coverage notes:\n  1 accepted (sync -v lists them)\n") {
		t.Errorf("doctor should show the accepted note count:\n%s", out)
	}

	out, _ = runDoctor(t, "--json")
	var report struct {
		CoverageAccepted int `json:"coverage_accepted"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, out)
	}
	if report.CoverageAccepted != 1 {
		t.Errorf("doctor --json coverage_accepted = %d, want 1", report.CoverageAccepted)
	}
}
