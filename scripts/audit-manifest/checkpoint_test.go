package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRunFile(t *testing.T, run, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(run, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestStoppedAuditorLeavesCheckpointedTargetsThatARelaunchSkips(t *testing.T) {
	a, run := fixtureFor(t, "alpha", "beta", "gamma", "delta")
	for _, target := range []string{"alpha", "beta"} {
		writeRunFile(t, run, target+".md", "### Clean\n- "+target+": verified\n")
		call(t, a, "checkpoint", run, target)
	}
	writeRunFile(t, run, "gamma.md", "half-written report without a marker\n")

	got := call(t, a, "checkpoints", run)
	want := "alpha\tdone\nbeta\tdone\ngamma\tpending\ndelta\tpending"
	if got != want {
		t.Fatalf("checkpoints after a stopped batch:\n%s\nwant:\n%s", got, want)
	}
	if pending := call(t, a, "checkpoints", "--pending", run); pending != "gamma delta" {
		t.Fatalf("relaunch assignment = %q, want only the unfinished targets", pending)
	}
	if pending := call(t, a, "checkpoints", "--pending", run, "alpha", "gamma"); pending != "gamma" {
		t.Errorf("batch relaunch = %q, want only that batch's unfinished target", pending)
	}
	if _, err := a.run([]string{"checkpoints", run, "other"}); err == nil {
		t.Error("checkpoints accepted a target outside the run")
	}

	call(t, a, "assign", run, "alpha", "batch-1")
	call(t, a, "complete", run, "alpha", "reviewed", "alpha.md", "checkpointed before the batch stopped")
	if got := call(t, a, "checkpoints", run); !strings.HasPrefix(got, "alpha\tcompleted\nbeta\tdone\n") {
		t.Fatalf("recorded checkpoint not reported as completed:\n%s", got)
	}
}

func TestCheckpointMarkerGoesStaleWhenTheReportChanges(t *testing.T) {
	a, run := fixtureFor(t, "alpha", "beta")
	writeRunFile(t, run, "alpha.md", "first report\n")
	call(t, a, "checkpoint", run, "alpha")
	writeRunFile(t, run, "alpha.md", "edited after the checkpoint\n")
	if got := call(t, a, "checkpoints", run); got != "alpha\tstale\nbeta\tpending" {
		t.Fatalf("edited report kept its checkpoint:\n%s", got)
	}
	if pending := call(t, a, "checkpoints", "--pending", run); pending != "alpha beta" {
		t.Fatalf("stale checkpoint was not reassigned: %q", pending)
	}
}

func TestCheckpointRequiresARequestedTargetWithAReport(t *testing.T) {
	a, run := fixtureFor(t, "alpha", "beta")
	if _, err := a.run([]string{"checkpoint", run, "alpha"}); err == nil {
		t.Error("checkpoint accepted a target without a report")
	}
	writeRunFile(t, run, "other.md", "report\n")
	if _, err := a.run([]string{"checkpoint", run, "other"}); err == nil {
		t.Error("checkpoint accepted a target outside the run")
	}
	if _, err := os.Stat(filepath.Join(run, "alpha.done")); !os.IsNotExist(err) {
		t.Errorf("rejected checkpoint left a marker: %v", err)
	}
}

func TestCheckpointDoesNotWaitForTheCoordinatorWriteGuard(t *testing.T) {
	a, run := fixtureFor(t, "alpha", "beta")
	if err := os.Mkdir(filepath.Join(run, ".audit-writing"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeRunFile(t, run, "alpha.md", "report\n")
	call(t, a, "checkpoint", run, "alpha")
	if got := call(t, a, "checkpoints", run); got != "alpha\tdone\nbeta\tpending" {
		t.Fatalf("checkpoint during a coordinator write:\n%s", got)
	}
}

func TestCheckpointRejectsAFinalizedRun(t *testing.T) {
	a, run := fixture(t)
	mutate(t, run, func(m map[string]any) { m["finalized"] = "2026-10-29T00:00:00Z" })
	writeRunFile(t, run, "alpha.md", "report\n")
	if _, err := a.run([]string{"checkpoint", run, "alpha"}); err == nil {
		t.Error("checkpoint wrote into a finalized run")
	}
}
