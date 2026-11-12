package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestScheduleRefillsWorkerBeforeOtherTargetFinishes(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "changed")
	call(t, a, "schedule", run, "2", "alpha", "beta")
	if got := call(t, a, "dispatch", run, "Frodo"); !strings.Contains(got, "alpha") {
		t.Errorf("first dispatch: %s", got)
	}
	if _, err := a.run([]string{"dispatch", run, "Frodo"}); err == nil {
		t.Error("busy worker accepted another assignment")
	}
	call(t, a, "dispatch", run, "Sam")
	writeRunFile(t, run, "alpha.md", "finished alpha")
	scheduledCheckpoint(t, a, run, "alpha")
	got := call(t, a, "dispatch", run, "Frodo")
	if got != "" {
		t.Errorf("completed target dispatched again: %s", got)
	}
	m, err := load(run)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Assignments) != 2 {
		t.Errorf("assignments = %d", len(m.Assignments))
	}
}

func TestScheduleRequiresConfirmedStopBeforeRetry(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "changed")
	call(t, a, "schedule", run, "1", "alpha", "beta")
	call(t, a, "dispatch", run, "Frodo")
	if _, err := a.run([]string{"dispatch", run, "Sam"}); err == nil {
		t.Error("capacity exceeded")
	}
	if _, err := a.run([]string{"release", run, "alpha"}); err == nil {
		t.Error("release without stopped confirmation")
	}
	call(t, a, "release", run, "alpha", "--stopped")
	if got := call(t, a, "dispatch", run, "Sam"); !strings.Contains(got, "alpha") {
		t.Errorf("retry = %s", got)
	}
	m, err := load(run)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Assignments) != 1 || m.Assignments[0].Owner != "Sam" {
		t.Errorf("ownership = %+v", m.Assignments)
	}
}

func TestSchedulePrioritizesRecoveryAndRefillsCapacity(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "failed")
	call(t, a, "schedule", run, "1", "alpha", "beta")
	if got := call(t, a, "dispatch", run, "Frodo"); !strings.Contains(got, "beta") {
		t.Errorf("recovery target not first: %s", got)
	}
	writeRunFile(t, run, "beta.md", "recovery remains a limit")
	scheduledCheckpoint(t, a, run, "beta")
	if got := call(t, a, "dispatch", run, "Frodo"); !strings.Contains(got, "alpha") {
		t.Errorf("idle slot not refilled: %s", got)
	}
}

func TestScheduleRejectsCompletedAndDuplicateTargets(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "changed")
	for _, args := range [][]string{{"schedule", run, "0", "alpha"}, {"schedule", run, "2", "alpha", "alpha"}, {"schedule", run, "2", "unknown"}} {
		if _, err := a.run(args); err == nil {
			t.Errorf("accepted invalid schedule %v", args)
		}
	}
	writeRunFile(t, run, "alpha.md", "done")
	scheduledCheckpoint(t, a, run, "alpha")
	if _, err := a.run([]string{"schedule", run, "1", "alpha"}); err == nil {
		t.Error("accepted checkpointed target")
	}
	call(t, a, "schedule", run, "1", "beta")
	if _, err := a.run([]string{"schedule", run, "1", "beta"}); err == nil {
		t.Error("replaced existing schedule")
	}
}

func TestDispatchPreservesOutsideOwnership(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "changed")
	call(t, a, "schedule", run, "1", "alpha")
	call(t, a, "assign", run, "alpha", "existing-worker")
	if _, err := a.run([]string{"dispatch", run, "new-worker"}); err == nil {
		t.Error("stole outside assignment")
	}
	m, err := load(run)
	if err != nil {
		t.Fatal(err)
	}
	if m.Assignments[0].Owner != "existing-worker" {
		t.Errorf("owner changed: %+v", m.Assignments)
	}
}

func TestScheduleFakeWorkersRefillWhileSlowReviewContinues(t *testing.T) {
	a, run := fixtureFor(t, "alpha", "beta", "gamma", "delta")
	m, err := load(run)
	if err != nil {
		t.Fatal(err)
	}
	var rows strings.Builder
	for _, source := range m.Sources {
		path := source.Target + "-" + source.Kind + ".txt"
		writeRunFile(t, run, path, "frozen evidence")
		rows.WriteString(strings.Join([]string{source.Target, source.Kind, source.URL, "200", "text", digest([]byte("frozen evidence")), "2026-11-12", "changed", source.URL, path}, "\t") + "\n")
	}
	if err := os.WriteFile(filepath.Join(run, "docfetch.tsv"), []byte(rows.String()), 0600); err != nil {
		t.Fatal(err)
	}
	call(t, a, "capture", run)
	call(t, a, "schedule", run, "2", "alpha", "beta", "gamma", "delta")
	call(t, a, "dispatch", run, "slow-worker")
	call(t, a, "dispatch", run, "fast-worker")
	// Fake completions at 10, 20 and 30 seconds precede alpha at 100.
	for i, target := range []string{"beta", "gamma", "delta"} {
		writeRunFile(t, run, target+".md", fmt.Sprintf("review at %d", (i+1)*10))
		scheduledCheckpoint(t, a, run, target)
		got := call(t, a, "dispatch", run, "fast-worker")
		if i < 2 && !strings.Contains(got, []string{"gamma", "delta"}[i]) {
			t.Errorf("did not refill after %s: %s", target, got)
		}
	}
	m, err = load(run)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Queue.Attempts) != 4 || m.Queue.Attempts[0].Status != "running" {
		t.Errorf("slow work blocked refills: %+v", m.Queue.Attempts)
	}
	t.Log("Synthetic workload: fixed contiguous batches finish at 110 seconds; target dispatch finishes at 100 seconds. This is not a measured audit speedup.")
}

func TestScheduleIngestsCompletedTargetsBeforeRefilling(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "changed")
	call(t, a, "schedule", run, "1", "alpha", "beta")
	for _, target := range []string{"alpha", "beta"} {
		if got := call(t, a, "dispatch", run, "Frodo"); !strings.Contains(got, target) {
			t.Fatalf("dispatch %s: %s", target, got)
		}
		writeRunFile(t, run, target+".md", "checked all sources")
		scheduledCheckpoint(t, a, run, target)
		raw := call(t, a, "result-template", run, target)
		var result map[string]any
		if err := json.Unmarshal([]byte(raw), &result); err != nil {
			t.Fatal(err)
		}
		out := result["outcome"].(map[string]any)
		out["disposition"] = "reviewed"
		out["reason"] = "checked target"
		for _, source := range result["sources"].([]any) {
			o := source.(map[string]any)["outcome"].(map[string]any)
			o["disposition"] = "reviewed"
			o["reason"] = "checked source"
		}
		call(t, a, "ingest", run, writeResult(t, run, result))
	}
	if got := call(t, a, "dispatch", run, "Frodo"); got != "" {
		t.Errorf("finished target requeued: %s", got)
	}
	call(t, a, "validate", run)
	call(t, a, "finalize", run)
}

func TestFinalIngestionClosesScheduledAttemptWithoutDispatch(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "changed")
	call(t, a, "schedule", run, "1", "alpha", "beta")
	for _, target := range []string{"alpha", "beta"} {
		call(t, a, "dispatch", run, "Frodo")
		writeRunFile(t, run, target+".md", "checked all sources")
		scheduledCheckpoint(t, a, run, target)
		var r map[string]any
		if err := json.Unmarshal([]byte(call(t, a, "result-template", run, target)), &r); err != nil {
			t.Fatal(err)
		}
		o := r["outcome"].(map[string]any)
		o["disposition"] = "reviewed"
		o["reason"] = "checked"
		for _, s := range r["sources"].([]any) {
			o := s.(map[string]any)["outcome"].(map[string]any)
			o["disposition"] = "reviewed"
			o["reason"] = "checked"
		}
		call(t, a, "ingest", run, writeResult(t, run, r))
	}
	call(t, a, "finalize", run)
	m, err := load(run)
	if err != nil {
		t.Fatal(err)
	}
	for _, attempt := range m.Queue.Attempts {
		if attempt.Status != "completed" || attempt.End == nil {
			t.Errorf("unfinished recorded attempt: %+v", attempt)
		}
	}
}

func TestStaleCompletedAttemptRequiresConfirmedStop(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "changed")
	call(t, a, "schedule", run, "2", "alpha", "beta")
	call(t, a, "dispatch", run, "Frodo")
	writeRunFile(t, run, "alpha.md", "old report")
	scheduledCheckpoint(t, a, run, "alpha")
	call(t, a, "dispatch", run, "Sam")
	writeRunFile(t, run, "alpha.md", "edited report")
	if got, err := a.run([]string{"dispatch", run, "Gandalf"}); err == nil {
		t.Errorf("stale review reassigned without stop: %s", got)
	}
	call(t, a, "release", run, "alpha", "--stopped")
	if got := call(t, a, "dispatch", run, "Gandalf"); !strings.Contains(got, "alpha") {
		t.Errorf("stopped stale target not retried: %s", got)
	}
}

func TestRetryCannotReuseOldCheckpointAfterRestoringBytes(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "changed")
	call(t, a, "schedule", run, "1", "alpha", "beta")
	call(t, a, "dispatch", run, "Frodo")
	writeRunFile(t, run, "alpha.md", "old report")
	scheduledCheckpoint(t, a, run, "alpha")
	writeRunFile(t, run, "alpha.md", "edited report")
	call(t, a, "release", run, "alpha", "--stopped")
	call(t, a, "dispatch", run, "Sam")
	writeRunFile(t, run, "alpha.md", "old report")
	if got, err := a.run([]string{"dispatch", run, "Gandalf"}); err == nil {
		t.Errorf("old marker freed running retry: %s", got)
	}
}

func scheduledCheckpoint(t *testing.T, a app, run, target string) {
	t.Helper()
	m, err := load(run)
	if err != nil {
		t.Fatal(err)
	}
	if attempt := latestAttempt(m, target); attempt != nil {
		call(t, a, "checkpoint", run, target, "--attempt", strconv.Itoa(attempt.Attempt))
		return
	}
	call(t, a, "checkpoint", run, target)
}

func TestScheduledCheckpointRejectsPreviousAttempt(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "changed")
	call(t, a, "schedule", run, "1", "alpha")
	call(t, a, "dispatch", run, "Frodo")
	writeRunFile(t, run, "alpha.md", "old report")
	scheduledCheckpoint(t, a, run, "alpha")
	writeRunFile(t, run, "alpha.md", "changed report")
	call(t, a, "release", run, "alpha", "--stopped")
	if _, err := os.Stat(filepath.Join(run, "alpha.attempt-1.done")); err != nil {
		t.Errorf("old evidence marker not archived: %v", err)
	}
	call(t, a, "dispatch", run, "Sam")
	if _, err := a.run([]string{"checkpoint", run, "alpha", "--attempt", "1"}); err == nil {
		t.Error("previous worker checkpointed newer attempt")
	}
	if _, err := a.run([]string{"checkpoint", run, "alpha"}); err == nil {
		t.Error("scheduled checkpoint omitted attempt")
	}
	scheduledCheckpoint(t, a, run, "alpha")
	if got := call(t, a, "checkpoints", run, "alpha"); got != "alpha\tdone" {
		t.Errorf("new checkpoint not accepted: %s", got)
	}
}
