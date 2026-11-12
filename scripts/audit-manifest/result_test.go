package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func resultFixture(t *testing.T, a app, run, target string) map[string]any {
	t.Helper()
	writeRunFile(t, run, target+".md", "checked target and sources\n")
	call(t, a, "checkpoint", run, target)
	call(t, a, "assign", run, target, "reviewer")
	m, err := load(run)
	if err != nil {
		t.Fatal(err)
	}
	report, err := getArtifact(m.Directory, target+".md")
	if err != nil {
		t.Fatal(err)
	}
	out := outcome{Disposition: "reviewed", Reason: "checked", Artifact: report}
	sources := []map[string]any{}
	for _, s := range m.Sources {
		if s.Target == target {
			sources = append(sources, map[string]any{"kind": s.Kind, "url": s.URL, "evidence": s.Fetch.Evidence, "outcome": out})
		}
	}
	return map[string]any{"version": 1, "run_id": m.RunID, "target": target, "owner": "reviewer", "outcome": out, "sources": sources}
}
func writeResult(t *testing.T, run string, r map[string]any) string {
	t.Helper()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(run, "result.json")
	if err = os.WriteFile(p, b, 0600); err != nil {
		t.Fatal(err)
	}
	return p
}
func TestIngestCompletesTargetAndRetriesWithoutChangingManifest(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "changed")
	for _, target := range []string{"alpha", "beta"} {
		r := resultFixture(t, a, run, target)
		p := writeResult(t, run, r)
		call(t, a, "ingest", run, p)
		before, _ := os.ReadFile(filepath.Join(run, "audit.json"))
		call(t, a, "ingest", run, p)
		after, _ := os.ReadFile(filepath.Join(run, "audit.json"))
		if string(before) != string(after) {
			t.Error("retry changed manifest")
		}
	}
	call(t, a, "validate", run)
	call(t, a, "finalize", run)
}
func TestIngestRejectsIncompleteResultAtomically(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "changed")
	r := resultFixture(t, a, run, "alpha")
	r["sources"] = []map[string]any{}
	before, _ := os.ReadFile(filepath.Join(run, "audit.json"))
	lock, _ := os.ReadFile(a.lock)
	if _, err := a.run([]string{"ingest", run, writeResult(t, run, r)}); err == nil {
		t.Error("accepted missing sources")
	}
	after, _ := os.ReadFile(filepath.Join(run, "audit.json"))
	afterLock, _ := os.ReadFile(a.lock)
	if string(before) != string(after) || string(lock) != string(afterLock) {
		t.Error("rejection changed state")
	}
}

func TestIngestRejectsInvalidResultsWithoutRecordingCoverage(t *testing.T) {
	cases := []struct {
		name   string
		change func(map[string]any)
	}{
		{"version", func(r map[string]any) { r["version"] = 2 }},
		{"run", func(r map[string]any) { r["run_id"] = "other" }},
		{"owner", func(r map[string]any) { r["owner"] = "other" }},
		{"duplicate source", func(r map[string]any) { s := r["sources"].([]map[string]any); r["sources"] = append(s, s[0]) }},
		{"unknown source", func(r map[string]any) { r["sources"].([]map[string]any)[0]["url"] = "https://unknown.test" }},
		{"evidence", func(r map[string]any) {
			r["sources"].([]map[string]any)[0]["evidence"] = artifact{Path: "missing", SHA256: "bad"}
		}},
		{"artifact", func(r map[string]any) { o := r["outcome"].(outcome); o.Artifact.SHA256 = "bad"; r["outcome"] = o }},
		{"timing", func(r map[string]any) { r["timing"] = map[string]any{"start": 20, "end": 10} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, run := fixture(t)
			fetched(t, a, run, "changed")
			r := resultFixture(t, a, run, "alpha")
			tc.change(r)
			before, _ := os.ReadFile(filepath.Join(run, "audit.json"))
			if _, err := a.run([]string{"ingest", run, writeResult(t, run, r)}); err == nil {
				t.Error("accepted invalid result")
			}
			after, _ := os.ReadFile(filepath.Join(run, "audit.json"))
			if string(before) != string(after) {
				t.Error("changed manifest")
			}
		})
	}
}
func TestIngestRejectsChangedCheckpointAndConflictingRetry(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "changed")
	r := resultFixture(t, a, run, "alpha")
	p := writeResult(t, run, r)
	call(t, a, "ingest", run, p)
	r["timing"] = map[string]any{"start": 10, "end": 20}
	if _, err := a.run([]string{"ingest", run, writeResult(t, run, r)}); err == nil {
		t.Error("accepted conflicting retry")
	}
	writeRunFile(t, run, "alpha.md", "changed report")
	if _, err := a.run([]string{"ingest", run, p}); err == nil {
		t.Error("accepted stale checkpoint")
	}
}
func TestIngestMatchesManualCoverageAndLock(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "changed")
	for _, target := range []string{"alpha", "beta"} {
		r := resultFixture(t, a, run, target)
		if target == "beta" {
			s := r["sources"].([]map[string]any)
			o := s[0]["outcome"].(outcome)
			o.Disposition = "research-limit"
			s[0]["outcome"] = o
		}
		call(t, a, "ingest", run, writeResult(t, run, r))
	}
	call(t, a, "finalize", run)
	got, _ := os.ReadFile(a.lock)
	manual, manualRun := fixture(t)
	fetched(t, manual, manualRun, "changed")
	covered(t, manual, manualRun, true)
	call(t, manual, "finalize", manualRun)
	want, _ := os.ReadFile(manual.lock)
	if string(got) != string(want) {
		t.Errorf("ingestion lock differs from manual recording:\n%s\n%s", got, want)
	}
}
func TestIngestFailedFetchRequiresResearchLimit(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "failed")
	r := resultFixture(t, a, run, "beta")
	if _, err := a.run([]string{"ingest", run, writeResult(t, run, r)}); err == nil {
		t.Error("accepted reviewed failed fetch")
	}
	for _, s := range r["sources"].([]map[string]any) {
		if s["kind"] == "code" {
			o := s["outcome"].(outcome)
			o.Disposition = "research-limit"
			s["outcome"] = o
		}
	}
	call(t, a, "ingest", run, writeResult(t, run, r))
}

func TestResultTemplateDoesNotAssumeCoverage(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "changed")
	resultFixture(t, a, run, "alpha")
	template := call(t, a, "result-template", run, "alpha")
	var r targetResult
	if err := json.Unmarshal([]byte(template), &r); err != nil {
		t.Fatal(err)
	}
	if r.Owner != "reviewer" || len(r.Sources) != 2 || r.Outcome.Disposition != "" {
		t.Errorf("invalid template: %+v", r)
	}
	for _, s := range r.Sources {
		if s.Outcome.Disposition != "" || s.Outcome.Reason != "" {
			t.Error("template inferred coverage")
		}
	}
	writeRunFile(t, run, "template.json", template)
	if _, err := a.run([]string{"ingest", run, filepath.Join(run, "template.json")}); err == nil {
		t.Error("accepted unfinished template")
	}
}

func TestIngestedCoverageRemainsReusable(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "changed")
	r := resultFixture(t, a, run, "alpha")
	call(t, a, "ingest", run, writeResult(t, run, r))
	fresh := filepath.Join(t.TempDir(), "run")
	call(t, a, "start", "--out", fresh, "--builtins-since", "HEAD", "alpha", "beta")
	fetched(t, a, fresh, "changed")
	call(t, a, "resume", fresh, run)
	m, err := load(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Assignments) != 1 || m.Assignments[0].Target != "alpha" || m.Assignments[0].Outcome == nil {
		t.Error("ingested target coverage was not reused")
	}
	if len(m.Results) != 0 {
		t.Error("old timing was presented as a new observation")
	}
}

func TestStoredResultsRejectConflictingCoverageEverywhere(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "changed")
	for _, target := range []string{"alpha", "beta"} {
		r := resultFixture(t, a, run, target)
		if target == "alpha" {
			s := r["sources"].([]map[string]any)
			o := s[0]["outcome"].(outcome)
			o.Disposition = "research-limit"
			s[0]["outcome"] = o
		}
		call(t, a, "ingest", run, writeResult(t, run, r))
	}
	mutate(t, run, func(m map[string]any) {
		r := m["results"].([]any)[0].(map[string]any)
		s := r["sources"].([]any)[0].(map[string]any)
		s["outcome"].(map[string]any)["disposition"] = "reviewed"
	})
	fresh := filepath.Join(t.TempDir(), "run")
	call(t, a, "start", "--out", fresh, "--builtins-since", "HEAD", "alpha", "beta")
	fetched(t, a, fresh, "changed")
	lock, _ := os.ReadFile(a.lock)
	for _, args := range [][]string{{"result-report", run}, {"validate", run}, {"ingest", run, filepath.Join(run, "result.json")}, {"resume", fresh, run}, {"finalize", run}} {
		if _, err := a.run(args); err == nil {
			t.Errorf("accepted inconsistent stored coverage: %v", args)
		}
	}
	after, _ := os.ReadFile(a.lock)
	if string(lock) != string(after) {
		t.Error("inconsistent stored result advanced lock")
	}
}

func TestStoredResultsRejectMalformedRecords(t *testing.T) {
	cases := []struct {
		name   string
		change func(map[string]any)
	}{
		{"version", func(m map[string]any) { m["results"].([]any)[0].(map[string]any)["version"] = 2 }},
		{"duplicate target", func(m map[string]any) { r := m["results"].([]any); m["results"] = append(r, r[0]) }},
		{"missing assignment", func(m map[string]any) { m["assignments"] = []any{} }},
		{"assignment owner", func(m map[string]any) { m["assignments"].([]any)[0].(map[string]any)["owner"] = "different" }},
		{"missing source", func(m map[string]any) { m["results"].([]any)[0].(map[string]any)["sources"] = []any{} }},
		{"negative timing", func(m map[string]any) {
			m["results"].([]any)[0].(map[string]any)["timing"] = map[string]any{"start": -1}
		}},
		{"reversed timing", func(m map[string]any) {
			m["results"].([]any)[0].(map[string]any)["timing"] = map[string]any{"start": 20, "end": 10}
		}},
		{"short fetch row", func(m map[string]any) {
			m["sources"].([]any)[0].(map[string]any)["fetch"].(map[string]any)["row"] = []any{}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, run := fixture(t)
			fetched(t, a, run, "changed")
			r := resultFixture(t, a, run, "alpha")
			call(t, a, "ingest", run, writeResult(t, run, r))
			mutate(t, run, tc.change)
			if _, err := a.run([]string{"result-report", run}); err == nil {
				t.Error("reported malformed stored result")
			}
		})
	}
}
