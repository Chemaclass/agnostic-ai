package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func nextRun(t *testing.T, a app) string {
	t.Helper()
	run := filepath.Join(t.TempDir(), "fresh")
	call(t, a, "start", "--out", run, "--builtins-since", "HEAD", "alpha", "beta")
	return run
}

func TestResumeNeedsIndependentFreshCapture(t *testing.T) {
	a, prior := fixture(t)
	fetched(t, a, prior, "changed")
	covered(t, a, prior, false)
	fresh := nextRun(t, a)
	if _, err := a.run([]string{"resume", fresh, prior}); err == nil {
		t.Fatal("reused without a fresh fetch")
	}
	fetched(t, a, fresh, "unchanged")
	call(t, a, "resume", fresh, prior)
	call(t, a, "validate", fresh)
	if err := os.Remove(filepath.Join(prior, "report.md")); err != nil {
		t.Fatal(err)
	}
	call(t, a, "validate", fresh)
	m, err := load(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Reused) != 2 || m.Reused[0].RunID == m.RunID {
		t.Fatalf("missing prior-run provenance: %+v", m.Reused)
	}
	if !strings.HasPrefix(m.Assignments[0].Outcome.Artifact.Path, "resumed/") {
		t.Fatal("outcome was not copied")
	}
}

func TestResumeKeepsIncompleteAndLimitedTargetsPending(t *testing.T) {
	for _, state := range []string{"before-evidence", "after-evidence", "before-page-completion", "limit", "failed"} {
		t.Run(state, func(t *testing.T) {
			a, prior := fixture(t)
			fetched(t, a, prior, "changed")
			covered(t, a, prior, false)
			mutate(t, prior, func(m map[string]any) {
				if state == "before-evidence" || state == "after-evidence" {
					m["assignments"].([]any)[0].(map[string]any)["outcome"] = nil
				}
				if state == "before-evidence" {
					for _, s := range m["sources"].([]any) {
						if s.(map[string]any)["target"] == "alpha" {
							delete(s.(map[string]any), "outcome")
						}
					}
				}
				if state == "before-page-completion" {
					delete(m["sources"].([]any)[1].(map[string]any), "outcome")
				}
				if state == "limit" {
					m["sources"].([]any)[0].(map[string]any)["outcome"].(map[string]any)["disposition"] = "research-limit"
				}
			})
			fresh := nextRun(t, a)
			status := "changed"
			if state == "failed" {
				status = "failed"
			}
			fetched(t, a, fresh, status)
			call(t, a, "resume", fresh, prior)
			m, err := load(fresh)
			if err != nil {
				t.Fatal(err)
			}
			if len(m.Assignments) != 1 {
				t.Fatalf("expected one reused target, got %+v", m.Assignments)
			}
			if _, err := a.run([]string{"validate", fresh}); err == nil {
				t.Fatal("pending target became complete")
			}
		})
	}
}

func TestResumeRejectsChangedOrCorruptContext(t *testing.T) {
	for _, change := range []string{"body", "missing", "outcome", "policy", "inventory", "baseline", "embedded", "untracked", "staged", "version", "source-lock", "donor-busy", "fresh-busy"} {
		t.Run(change, func(t *testing.T) {
			a, prior := fixture(t)
			fetched(t, a, prior, "changed")
			covered(t, a, prior, false)
			fresh := nextRun(t, a)
			fetched(t, a, fresh, "changed")
			switch change {
			case "body":
				_ = os.WriteFile(filepath.Join(prior, "pages/alpha-code.body"), []byte("different"), 0o644)
			case "missing":
				_ = os.Remove(filepath.Join(prior, "report.md"))
			case "outcome":
				_ = os.WriteFile(filepath.Join(prior, "report.md"), []byte("changed"), 0o644)
			case "policy":
				_ = os.WriteFile(filepath.Join(a.root, "scripts/target-facts.sh"), []byte("changed"), 0o644)
			case "inventory":
				mutate(t, prior, func(m map[string]any) { m["inventory"].([]any)[0].(map[string]any)["url"] = "https://changed.test" })
			case "baseline":
				mutate(t, prior, func(m map[string]any) { m["builtins_since"] = "different" })
			case "embedded":
				_ = os.WriteFile(filepath.Join(a.root, ".git/info/exclude"), []byte("internal/builtins/data/ignored.md\n"), 0o644)
				_ = os.WriteFile(filepath.Join(a.root, "internal/builtins/data/ignored.md"), []byte("asset"), 0o644)
			case "untracked":
				_ = os.WriteFile(filepath.Join(a.root, "new-input.md"), []byte("input"), 0o644)
			case "staged":
				_, err := a.command("git", "update-index", "--chmod=+x", "scripts/docfetch.sh")
				if err != nil {
					t.Fatal(err)
				}
			case "version":
				mutate(t, prior, func(m map[string]any) { m["research_version"] = 999 })
			case "source-lock":
				mutate(t, prior, func(m map[string]any) { m["lock_baseline_sha256"] = "different" })
			case "donor-busy":
				_ = os.Mkdir(filepath.Join(prior, ".audit-writing"), 0o700)
			case "fresh-busy":
				_ = os.Mkdir(filepath.Join(fresh, ".audit-writing"), 0o700)
			}
			before, err := os.ReadFile(filepath.Join(fresh, "audit.json"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := a.run([]string{"resume", fresh, prior}); err == nil {
				t.Fatal("accepted changed or corrupt inputs")
			}
			after, err := os.ReadFile(filepath.Join(fresh, "audit.json"))
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Fatal("failed resume changed progress")
			}
		})
	}
}

func TestRepeatedFinalizationValidatesWithoutRewritingLock(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "changed")
	covered(t, a, run, false)
	call(t, a, "finalize", run)
	before, err := os.Stat(a.lock)
	if err != nil {
		t.Fatal(err)
	}
	call(t, a, "finalize", run)
	after, err := os.Stat(a.lock)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("repeated finalization rewrote lock")
	}
	_ = os.WriteFile(filepath.Join(run, "report.md"), []byte("changed"), 0o644)
	rejectedUnchanged(t, a, run)
}

func TestFinalizeReconcilesCommittedLockWithMissingMarker(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "changed")
	covered(t, a, run, false)
	before, err := os.ReadFile(filepath.Join(run, "audit.json"))
	if err != nil {
		t.Fatal(err)
	}
	call(t, a, "finalize", run)
	_ = os.WriteFile(filepath.Join(run, "audit.json"), before, 0o644)
	call(t, a, "finalize", run)
	m, err := load(run)
	if err != nil {
		t.Fatal(err)
	}
	if m.Finalized == "" {
		t.Fatal("committed result was not reconciled")
	}
}

func replaceCapturedBody(t *testing.T, a app, run, body, comparison string) {
	t.Helper()
	path := filepath.Join(run, "pages/alpha-code.body")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
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
			f[5] = comparison
			rows[i] = strings.Join(f, "\t")
		}
	}
	if err = os.WriteFile(rowsPath, []byte(strings.Join(rows, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mutate(t, run, func(m map[string]any) { delete(m, "rows_sha256") })
	call(t, a, "capture", run)
}

func TestResumeDoesNotTrustNormalizedSchemaHash(t *testing.T) {
	a, prior := fixture(t)
	fetched(t, a, prior, "changed")
	normalized := "{\"a\":1}\n"
	replaceCapturedBody(t, a, prior, "{\n  \"a\": 1\n}\n", digest([]byte(normalized)))
	covered(t, a, prior, false)
	fresh := nextRun(t, a)
	fetched(t, a, fresh, "changed")
	replaceCapturedBody(t, a, fresh, normalized, digest([]byte(normalized)))
	call(t, a, "resume", fresh, prior)
	m, err := load(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Reused) != 1 || m.Reused[0].Target != "beta" {
		t.Fatalf("normalized hash reused different exact schema bytes: %+v", m.Reused)
	}
	rejectedUnchanged(t, a, fresh)
}

func TestResumeChangedSuccessfulSourceKeepsTargetPending(t *testing.T) {
	a, prior := fixture(t)
	fetched(t, a, prior, "changed")
	covered(t, a, prior, false)
	fresh := nextRun(t, a)
	fetched(t, a, fresh, "changed")
	body := "new vendor behavior\n"
	replaceCapturedBody(t, a, fresh, body, digest([]byte(body)))
	call(t, a, "resume", fresh, prior)
	m, err := load(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Reused) != 1 || m.Reused[0].Target != "beta" {
		t.Fatalf("changed source reused target: %+v", m.Reused)
	}
	rejectedUnchanged(t, a, fresh)
}

func TestResumeRetryAfterInterruptedArtifactCopy(t *testing.T) {
	a, prior := fixture(t)
	fetched(t, a, prior, "changed")
	covered(t, a, prior, false)
	fresh := nextRun(t, a)
	fetched(t, a, fresh, "changed")
	m, err := load(prior)
	if err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(fresh, "resumed", m.RunID, "report.md")
	if err = os.MkdirAll(blocked, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err = a.run([]string{"resume", fresh, prior}); err == nil {
		t.Fatal("accepted incomplete artifact copy")
	}
	current, err := load(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Assignments) != 0 || len(current.Reused) != 0 {
		t.Fatal("copy failure committed partial progress")
	}
	if err = os.Remove(blocked); err != nil {
		t.Fatal(err)
	}
	call(t, a, "resume", fresh, prior)
	call(t, a, "validate", fresh)
}

func TestRepeatedFinalizationRejectsDifferentCurrentLock(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "changed")
	covered(t, a, run, false)
	call(t, a, "finalize", run)
	if err := os.WriteFile(a.lock, []byte("# later lock\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rejectedUnchanged(t, a, run)
}

func TestResumePreservesAmbiguousPublicationForManualReconciliation(t *testing.T) {
	a, prior := fixture(t)
	fetched(t, a, prior, "changed")
	covered(t, a, prior, false)
	mutate(t, prior, func(m map[string]any) {
		body := "Audit finding: alpha-native-path-2026-10\nIssue creation timed out. Remote state must be refreshed. Publication uncertain.\n"
		if err := os.WriteFile(filepath.Join(prior, "report.md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, as := range m["assignments"].([]any) {
			as.(map[string]any)["outcome"].(map[string]any)["artifact"].(map[string]any)["sha256"] = digest([]byte(body))
		}
		for _, s := range m["sources"].([]any) {
			s.(map[string]any)["outcome"].(map[string]any)["artifact"].(map[string]any)["sha256"] = digest([]byte(body))
		}
	})
	fresh := nextRun(t, a)
	fetched(t, a, fresh, "changed")
	call(t, a, "resume", fresh, prior)
	m, err := load(fresh)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(fresh, m.Assignments[0].Outcome.Artifact.Path))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "alpha-native-path-2026-10") || !strings.Contains(string(b), "Publication uncertain") {
		t.Fatal("resume lost stable identity or publication uncertainty")
	}
}

func TestResumePreservesIndependentlyRequestedTargetOrder(t *testing.T) {
	a, prior := fixture(t)
	fetched(t, a, prior, "changed")
	covered(t, a, prior, false)
	fresh := filepath.Join(t.TempDir(), "reordered")
	call(t, a, "start", "--out", fresh, "--builtins-since", "HEAD", "beta", "alpha")
	fetched(t, a, fresh, "changed")
	call(t, a, "resume", fresh, prior)
	m, err := load(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(m.Targets, ",") != "beta,alpha" || m.RunID == m.Reused[0].RunID {
		t.Fatal("resume adopted donor invocation identity")
	}
	call(t, a, "validate", fresh)
}

func TestResumeChecksCopiedProvenanceAndArtifacts(t *testing.T) {
	for _, name := range []string{"report", "manifest"} {
		t.Run(name, func(t *testing.T) {
			a, prior := fixture(t)
			fetched(t, a, prior, "changed")
			covered(t, a, prior, false)
			fresh := nextRun(t, a)
			fetched(t, a, fresh, "changed")
			call(t, a, "resume", fresh, prior)
			m, err := load(fresh)
			if err != nil {
				t.Fatal(err)
			}
			path := m.Reused[0].Manifest.Path
			if name == "report" {
				path = m.Assignments[0].Outcome.Artifact.Path
			}
			if err = os.WriteFile(filepath.Join(fresh, path), []byte("changed"), 0o644); err != nil {
				t.Fatal(err)
			}
			rejectedUnchanged(t, a, fresh)
		})
	}
}

func TestFinalizationRejectsCorruptReceiptAndChangedReconciliationEvidence(t *testing.T) {
	for _, name := range []string{"receipt", "evidence", "lock", "baseline"} {
		t.Run(name, func(t *testing.T) {
			a, run := fixture(t)
			fetched(t, a, run, "changed")
			covered(t, a, run, false)
			before, err := os.ReadFile(filepath.Join(run, "audit.json"))
			if err != nil {
				t.Fatal(err)
			}
			call(t, a, "finalize", run)
			if err = os.WriteFile(filepath.Join(run, "audit.json"), before, 0o644); err != nil {
				t.Fatal(err)
			}
			switch name {
			case "receipt":
				_ = os.WriteFile(filepath.Join(run, "finalization.json"), []byte("{"), 0o644)
			case "evidence":
				_ = os.WriteFile(filepath.Join(run, "report.md"), []byte("changed"), 0o644)
			case "lock":
				_ = os.WriteFile(a.lock, []byte("# unknown lock\n"), 0o644)
			case "baseline":
				mutate(t, run, func(m map[string]any) { m["lock_baseline_sha256"] = "different" })
			}
			rejectedUnchanged(t, a, run)
		})
	}
}

func TestFinalizationReceiptBeforeCommitRequiresFreshValidation(t *testing.T) {
	a, run := fixture(t)
	fetched(t, a, run, "changed")
	covered(t, a, run, false)
	m, err := load(run)
	if err != nil {
		t.Fatal(err)
	}
	if err = writeFinalizationReceipt(m, strings.Repeat("c", 64)); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(run, "report.md"), []byte("changed after receipt"), 0o644); err != nil {
		t.Fatal(err)
	}
	rejectedUnchanged(t, a, run)
}

func TestResumeSelectsOnePriorRunEvenWhenEverythingIsPending(t *testing.T) {
	a, prior := fixture(t)
	fetched(t, a, prior, "changed")
	fresh := nextRun(t, a)
	fetched(t, a, fresh, "changed")
	call(t, a, "resume", fresh, prior)
	if _, err := a.run([]string{"resume", fresh, prior}); err == nil {
		t.Fatal("accepted a second prior-run selection")
	}
}

func TestResumeRejectsUnsafeCopyBeforeCreatingDirectories(t *testing.T) {
	for _, name := range []string{"donor-traversal", "destination-symlink"} {
		t.Run(name, func(t *testing.T) {
			a, _ := fixture(t)
			workspace := t.TempDir()
			prior := filepath.Join(workspace, "a", "b", "c", "prior")
			call(t, a, "start", "--out", prior, "--builtins-since", "HEAD", "alpha", "beta")
			fetched(t, a, prior, "changed")
			covered(t, a, prior, false)
			fresh := filepath.Join(workspace, "fresh")
			call(t, a, "start", "--out", fresh, "--builtins-since", "HEAD", "alpha", "beta")
			fetched(t, a, fresh, "changed")
			m, err := load(prior)
			if err != nil {
				t.Fatal(err)
			}
			var outside string
			if name == "donor-traversal" {
				alias := "audit-resume-" + filepath.Base(workspace)
				if err = os.Symlink(prior, filepath.Join(workspace, alias)); err != nil {
					t.Fatal(err)
				}
				escape := filepath.Join("..", "..", "..", "..", alias, "report.md")
				mutate(t, prior, func(m map[string]any) {
					m["assignments"].([]any)[0].(map[string]any)["outcome"].(map[string]any)["artifact"].(map[string]any)["path"] = escape
				})
				outside = filepath.Join(filepath.Dir(workspace), alias)
				t.Cleanup(func() { _ = os.Remove(outside) })
			} else {
				outsideRoot := t.TempDir()
				if err = os.Symlink(outsideRoot, filepath.Join(fresh, "resumed")); err != nil {
					t.Fatal(err)
				}
				outside = filepath.Join(outsideRoot, m.RunID)
			}
			if _, err = os.Stat(outside); !os.IsNotExist(err) {
				t.Fatalf("outside path must start absent: %s: %v", outside, err)
			}
			before, err := os.ReadFile(filepath.Join(fresh, "audit.json"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err = a.run([]string{"resume", fresh, prior}); err == nil {
				t.Fatal("accepted unsafe artifact destination")
			}
			if _, err = os.Stat(outside); !os.IsNotExist(err) {
				t.Errorf("resume created an outside directory before rejecting: %s: %v", outside, err)
			}
			after, err := os.ReadFile(filepath.Join(fresh, "audit.json"))
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Error("unsafe artifact copy changed progress")
			}
		})
	}
}
