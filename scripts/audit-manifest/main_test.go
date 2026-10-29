package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) (app, string) {
	t.Helper()
	return fixtureFor(t, "alpha", "beta")
}

func fixtureFor(t *testing.T, targets ...string) (app, string) {
	t.Helper()
	root := t.TempDir()
	write := func(path, body string) {
		t.Helper()
		path = filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("scripts/target-facts.sh", "#!/bin/sh\nif [ \"$1\" = --changed ]; then [ -z \"${AUDIT_TEST_DEEP:-}\" ] || printf 'builtin-deep: %s\\n' \"$AUDIT_TEST_DEEP\"; exit 0; fi\nprintf '%s\\n' "+strings.Join(targets, " ")+"\n")
	write("scripts/docfetch.sh", "#!/bin/sh\nshift\nfor t in \"$@\"; do printf '%s\\tdocs\\thttps://vendor.test/shared\\n' \"$t\"; printf '%s\\tcode\\thttps://vendor.test/%s\\n' \"$t\" \"$t\"; done\n")
	write(".agnostic-ai/skills/target-audit/references/sources.md", "source registry\n")
	write("internal/builtins/data/skill.md", "built-in\n")
	write("scripts/target-audit/sources.lock", "# old lock\nalpha\tdocs\thttps://vendor.test/shared\t200\traw\t"+strings.Repeat("a", 64)+"\t2026-09-01\nobsolete\tdocs\thttps://old.test/removed\t200\traw\t"+strings.Repeat("b", 64)+"\t2026-09-01\n")
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"-c", "user.name=Test", "-c", "user.email=test@example.test", "-c", "commit.gpgsign=false", "commit", "-qm", "fixture"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %s: %v", out, err)
		}
	}
	a := app{root: root, lock: filepath.Join(root, "scripts/target-audit/sources.lock")}
	run := filepath.Join(t.TempDir(), "run")
	call(t, a, append([]string{"start", "--out", run, "--builtins-since", "HEAD"}, targets...)...)
	return a, run
}

func call(t *testing.T, a app, args ...string) string {
	t.Helper()
	out, err := a.run(args)
	if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return strings.TrimSpace(out)
}

func fetched(t *testing.T, a app, run string, status string) {
	t.Helper()
	var rows strings.Builder
	for _, target := range []string{"alpha", "beta"} {
		for _, kind := range []string{"docs", "code"} {
			url := "https://vendor.test/shared"
			if kind == "code" {
				url = "https://vendor.test/" + target
			}
			path := "pages/" + target + "-" + kind + ".body"
			body := "fresh " + kind + "\n"
			if err := os.MkdirAll(filepath.Join(run, "pages"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(run, path), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			hash := digest([]byte(body))
			rowStatus := "changed"
			if target == "beta" && kind == "code" {
				rowStatus = status
			}
			if rowStatus == "failed" {
				hash = "-"
			}
			rows.WriteString(strings.Join([]string{target, kind, url, "200", "raw", hash, "2026-10-16", rowStatus, url, path}, "\t") + "\n")
		}
	}
	if err := os.WriteFile(filepath.Join(run, "docfetch.tsv"), []byte(rows.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	call(t, a, "capture", run)
}

func covered(t *testing.T, a app, run string, limit bool) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(run, "report.md"), []byte("coverage\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"alpha", "beta"} {
		call(t, a, "assign", run, target, "reviewer")
		call(t, a, "complete", run, target, "reviewed", "report.md", "checked target")
		for _, kind := range []string{"docs", "code"} {
			url, disposition := "https://vendor.test/shared", "reviewed"
			if kind == "code" {
				url = "https://vendor.test/" + target
			}
			if limit && target == "beta" && kind == "docs" {
				disposition = "research-limit"
			}
			call(t, a, "record", run, target, kind, url, disposition, "report.md", "checked source")
		}
	}
}

func rejectedUnchanged(t *testing.T, a app, run string) {
	t.Helper()
	before, err := os.ReadFile(a.lock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.run([]string{"finalize", run}); err == nil {
		t.Fatal("finalize accepted invalid coverage")
	}
	after, err := os.ReadFile(a.lock)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("rejection changed lock")
	}
}

func mutate(t *testing.T, run string, f func(map[string]any)) {
	t.Helper()
	path := filepath.Join(run, "audit.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	f(m)
	b, err = json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFinalizationRequiresCompleteCoverage(t *testing.T) {
	a, run := fixture(t)
	rejectedUnchanged(t, a, run)
	fetched(t, a, run, "changed")
	rejectedUnchanged(t, a, run)
	covered(t, a, run, false)
	call(t, a, "finalize", run)
	b, err := os.ReadFile(a.lock)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), strings.Repeat("a", 64)) {
		t.Fatal("reviewed hash did not advance")
	}
	if strings.Contains(string(b), "old.test/removed") {
		t.Fatal("removed source was not pruned")
	}
}

func TestResearchLimitPreservesSharedURLWhileOtherPagesAdvance(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "changed")
	covered(t, a, run, true)
	call(t, a, "finalize", run)
	b, err := os.ReadFile(a.lock)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), strings.Repeat("a", 64)) || !strings.Contains(string(b), "https://vendor.test/alpha") {
		t.Fatal("shared limit lost or eligible row not advanced")
	}
}

func TestFinalizationRejectsChangedInputs(t *testing.T) {
	for _, name := range []string{"version", "unknown-field", "missing-assignment", "duplicate-assignment", "extra-target", "unknown-source", "evidence", "artifact", "rows", "sources", "context", "lock"} {
		t.Run(name, func(t *testing.T) {
			a, run := fixture(t)
			fetched(t, a, run, "changed")
			covered(t, a, run, false)
			switch name {
			case "version":
				mutate(t, run, func(m map[string]any) { m["version"] = 99 })
			case "unknown-field":
				mutate(t, run, func(m map[string]any) { m["unexpected"] = true })
			case "missing-assignment":
				mutate(t, run, func(m map[string]any) { m["assignments"] = []any{} })
			case "duplicate-assignment":
				mutate(t, run, func(m map[string]any) { xs := m["assignments"].([]any); m["assignments"] = append(xs, xs[0]) })
			case "extra-target":
				mutate(t, run, func(m map[string]any) { m["targets"] = []any{"alpha", "beta", "gamma"} })
			case "unknown-source":
				mutate(t, run, func(m map[string]any) {
					xs := m["sources"].([]any)
					xs[0].(map[string]any)["url"] = "https://unknown.test"
				})
			case "evidence":
				_ = os.WriteFile(filepath.Join(run, "pages/alpha-docs.body"), []byte("changed bytes"), 0o644)
			case "artifact":
				_ = os.Remove(filepath.Join(run, "report.md"))
			case "rows":
				f, _ := os.OpenFile(filepath.Join(run, "docfetch.tsv"), os.O_APPEND|os.O_WRONLY, 0o644)
				_, _ = f.WriteString("extra row\n")
				_ = f.Close()
			case "sources":
				_ = os.WriteFile(filepath.Join(a.root, ".agnostic-ai/skills/target-audit/references/sources.md"), []byte("changed"), 0o644)
			case "context":
				_ = os.WriteFile(filepath.Join(a.root, "internal/builtins/data/skill.md"), []byte("changed"), 0o644)
			case "lock":
				f, _ := os.OpenFile(a.lock, os.O_APPEND|os.O_WRONLY, 0o644)
				_, _ = f.WriteString("# moved\n")
				_ = f.Close()
			}
			rejectedUnchanged(t, a, run)
		})
	}
}

func TestFreshRunsRejectReusedDirectoriesAndDuplicateTargets(t *testing.T) {
	a, run := fixture(t)
	for _, args := range [][]string{{"start", "--out", run, "alpha"}, {"start", "alpha", "alpha"}, {"start", "unknown"}} {
		if _, err := a.run(args); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
}

func TestAssignmentsAndSourceRecordsAreExactOnce(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "changed")
	covered(t, a, run, false)
	for _, args := range [][]string{{"assign", run, "alpha", "another-owner"}, {"record", run, "alpha", "docs", "https://vendor.test/shared", "reviewed", "report.md", "again"}, {"complete", run, "alpha", "reviewed", "report.md", "again"}, {"record", run, "alpha", "docs", "https://unknown.test", "reviewed", "report.md", "unknown"}} {
		if _, err := a.run(args); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
	rejected := filepath.Join(run, ".audit-interrupted-temp")
	if err := os.WriteFile(rejected, []byte("half a write"), 0o644); err != nil {
		t.Fatal(err)
	}
	call(t, a, "finalize", run)
}

func TestCaptureRejectsMissingDuplicateAndUnknownFetchRows(t *testing.T) {
	for _, name := range []string{"missing", "duplicate", "unknown"} {
		t.Run(name, func(t *testing.T) {
			a, run := fixture(t)
			fetched(t, a, run, "changed")
			mutate(t, run, func(m map[string]any) {
				delete(m, "rows_sha256")
				for _, s := range m["sources"].([]any) {
					delete(s.(map[string]any), "fetch")
				}
			})
			path := filepath.Join(run, "docfetch.tsv")
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			rows := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
			switch name {
			case "missing":
				rows = rows[1:]
			case "duplicate":
				rows[0] = rows[1]
			case "unknown":
				rows[0] = strings.Replace(rows[0], "vendor.test/shared", "unknown.test/shared", 1)
			}
			if err = os.WriteFile(path, []byte(strings.Join(rows, "\n")+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err = a.run([]string{"capture", run}); err == nil {
				t.Fatal("capture accepted incomplete sources")
			}
			rejectedUnchanged(t, a, run)
		})
	}
}

func TestFailedSourceNeedsResearchLimitAndKeepsOldHash(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "failed")
	covered(t, a, run, false)
	rejectedUnchanged(t, a, run)
	mutate(t, run, func(m map[string]any) {
		for _, item := range m["sources"].([]any) {
			s := item.(map[string]any)
			if s["target"] == "beta" && s["kind"] == "code" {
				s["outcome"].(map[string]any)["disposition"] = "research-limit"
			}
		}
	})
	call(t, a, "finalize", run)
	b, err := os.ReadFile(a.lock)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "https://vendor.test/beta") {
		t.Fatal("failed source advanced")
	}
}

func TestFastPathNeedsJustificationAndSavedArtifact(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "changed")
	covered(t, a, run, false)
	mutate(t, run, func(m map[string]any) {
		for _, item := range m["sources"].([]any) {
			item.(map[string]any)["outcome"].(map[string]any)["disposition"] = "fast-path"
		}
		for _, item := range m["assignments"].([]any) {
			item.(map[string]any)["outcome"].(map[string]any)["disposition"] = "fast-path"
		}
	})
	call(t, a, "validate", run)
	mutate(t, run, func(m map[string]any) {
		m["sources"].([]any)[0].(map[string]any)["outcome"].(map[string]any)["reason"] = ""
	})
	rejectedUnchanged(t, a, run)
}

func TestSharedURLConflictRejectsWithoutChangingLock(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "changed")
	path := filepath.Join(run, "pages/beta-docs.body")
	body := []byte("different fetched bytes\n")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	rowsPath := filepath.Join(run, "docfetch.tsv")
	b, err := os.ReadFile(rowsPath)
	if err != nil {
		t.Fatal(err)
	}
	var rows []string
	for _, line := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		f := strings.Split(line, "\t")
		if f[0] == "beta" && f[1] == "docs" {
			f[5] = digest(body)
		}
		rows = append(rows, strings.Join(f, "\t"))
	}
	if err = os.WriteFile(rowsPath, []byte(strings.Join(rows, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mutate(t, run, func(m map[string]any) { delete(m, "rows_sha256") })
	call(t, a, "capture", run)
	covered(t, a, run, false)
	rejectedUnchanged(t, a, run)
}

func TestJSONComparisonHashDiffersFromExactEvidenceHash(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Fatal("jq is needed to test docfetch JSON hashing")
	}
	a, run := fixture(t)
	fetched(t, a, run, "changed")
	body := []byte("{\n  \"a\": 1\n}\n")
	path := filepath.Join(run, "pages/alpha-code.body")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	rowsPath := filepath.Join(run, "docfetch.tsv")
	b, err := os.ReadFile(rowsPath)
	if err != nil {
		t.Fatal(err)
	}
	rows := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	for i, line := range rows {
		f := strings.Split(line, "\t")
		if f[0] == "alpha" && f[1] == "code" {
			f[5] = digest([]byte("{\"a\":1}\n"))
			rows[i] = strings.Join(f, "\t")
		}
	}
	if err = os.WriteFile(rowsPath, []byte(strings.Join(rows, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mutate(t, run, func(m map[string]any) { delete(m, "rows_sha256") })
	call(t, a, "capture", run)
	covered(t, a, run, false)
	call(t, a, "validate", run)
	if err = os.WriteFile(path, []byte("{\"a\":1}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rejectedUnchanged(t, a, run)
}

func TestFailedHTTPFetchWithoutBodySavesFailureEvidence(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "failed")
	rowsPath := filepath.Join(run, "docfetch.tsv")
	b, err := os.ReadFile(rowsPath)
	if err != nil {
		t.Fatal(err)
	}
	var rows []string
	for _, line := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		f := strings.Split(line, "\t")
		if f[0] == "beta" && f[1] == "code" {
			f[3] = "503"
			f[9] = ""
		}
		rows = append(rows, strings.Join(f, "\t"))
	}
	if err = os.WriteFile(rowsPath, []byte(strings.Join(rows, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mutate(t, run, func(m map[string]any) { delete(m, "rows_sha256") })
	call(t, a, "capture", run)
	covered(t, a, run, false)
	mutate(t, run, func(m map[string]any) {
		for _, item := range m["sources"].([]any) {
			s := item.(map[string]any)
			if s["target"] == "beta" && s["kind"] == "code" {
				s["outcome"].(map[string]any)["disposition"] = "research-limit"
			}
		}
	})
	call(t, a, "finalize", run)
}

func TestSharedOwnerOutsideRequestedScopeRetainsPriorHash(t *testing.T) {
	a, original := fixture(t)
	fetched(t, a, original, "changed")
	run := filepath.Join(t.TempDir(), "partial")
	call(t, a, "start", "--out", run, "--builtins-since", "HEAD", "alpha")
	b, err := os.ReadFile(filepath.Join(original, "docfetch.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	var rows []string
	for _, line := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		f := strings.Split(line, "\t")
		if f[0] != "alpha" {
			continue
		}
		rows = append(rows, line)
		body, readErr := os.ReadFile(filepath.Join(original, f[9]))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if err = os.MkdirAll(filepath.Join(run, "pages"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(run, f[9]), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err = os.WriteFile(filepath.Join(run, "docfetch.tsv"), []byte(strings.Join(rows, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(run, "alpha.md"), []byte("reviewed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	call(t, a, "capture", run)
	call(t, a, "assign", run, "alpha", "reviewer")
	call(t, a, "complete", run, "alpha", "reviewed", "alpha.md", "checked")
	call(t, a, "record", run, "alpha", "docs", "https://vendor.test/shared", "reviewed", "alpha.md", "checked")
	call(t, a, "record", run, "alpha", "code", "https://vendor.test/alpha", "reviewed", "alpha.md", "checked")
	call(t, a, "finalize", run)
	b, err = os.ReadFile(a.lock)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), strings.Repeat("a", 64)) {
		t.Fatal("unrequested owner's prior hash was lost")
	}
}

func TestInterruptedStateRejectsWithoutChangingLock(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "changed")
	covered(t, a, run, false)
	if err := os.Mkdir(filepath.Join(run, ".audit-writing"), 0o700); err != nil {
		t.Fatal(err)
	}
	rejectedUnchanged(t, a, run)
	if err := os.Remove(filepath.Join(run, ".audit-writing")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a.lock+".audit-finalizing", []byte("interrupted\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rejectedUnchanged(t, a, run)
}

func TestBuiltinDeepRejectsFastPath(t *testing.T) {
	t.Setenv("AUDIT_TEST_DEEP", "alpha")
	a, run := fixture(t)
	fetched(t, a, run, "changed")
	covered(t, a, run, false)
	mutate(t, run, func(m map[string]any) {
		for _, item := range m["sources"].([]any) {
			s := item.(map[string]any)
			if s["target"] == "alpha" {
				s["outcome"].(map[string]any)["disposition"] = "fast-path"
			}
		}
	})
	rejectedUnchanged(t, a, run)
}

func TestMalformedManifestRejectsWithoutChangingLock(t *testing.T) {
	a, run := fixture(t)
	if err := os.WriteFile(filepath.Join(run, "audit.json"), []byte(`{"version":1,`), 0o644); err != nil {
		t.Fatal(err)
	}
	rejectedUnchanged(t, a, run)
}

func obstructFinalizationMarker(t *testing.T, run string) *manifest {
	t.Helper()
	m, err := load(run)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(run, "audit.json")
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestFinalizationReportsCommittedLockWhenManifestMarkerFails(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "changed")
	covered(t, a, run, false)
	m := obstructFinalizationMarker(t, run)
	before, err := os.ReadFile(a.lock)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.finalize(m)
	if err == nil {
		t.Fatal("finalization accepted an unwritable manifest destination")
	}
	after, readErr := os.ReadFile(a.lock)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(before) == string(after) || !strings.Contains(string(after), "https://vendor.test/alpha") {
		t.Fatal("valid coverage did not commit the lock before the marker failed")
	}
	for _, text := range []string{"lock committed; manifest finalization marker failed", digest(after), a.lock, "scripts/target-audit/manifest.md"} {
		if !strings.Contains(err.Error(), text) {
			t.Errorf("error %q does not contain %q", err, text)
		}
	}
}

func TestInvalidCoveragePreservesLockWhenManifestMarkerCannotBeSaved(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "changed")
	m := obstructFinalizationMarker(t, run)
	before, err := os.ReadFile(a.lock)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.finalize(m)
	if err == nil || strings.Contains(err.Error(), "lock committed") {
		t.Fatalf("invalid coverage must reject before committing: %v", err)
	}
	after, readErr := os.ReadFile(a.lock)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(before) != string(after) {
		t.Fatal("invalid coverage changed the lock")
	}
}
