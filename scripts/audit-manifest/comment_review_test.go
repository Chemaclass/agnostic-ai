package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func commentFixture(t *testing.T) (app, string) {
	t.Helper()
	a, run := commentCaptureFixture(t)
	covered(t, a, run, false)
	return a, run
}

func commentCaptureFixture(t *testing.T) (app, string) {
	t.Helper()
	a, _ := fixture(t)
	codeRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TARGET_FACTS_CODE_ROOT", codeRoot)
	script, err := os.ReadFile("../target-facts.sh")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"docs/site/content/docs/cli-reference/hook-diagnostics.md": "Hook diagnostics\n",
		"scripts/target-facts.sh":                                  string(script),
		"internal/adapters/adapter.go":                             "package adapters\nvar registry = map[string]Adapter{\n \"alpha\": alpha.New(),\n \"beta\": beta.New(),\n}\n",
		"internal/builtins/builtins.go":                            "package builtins\nvar names = []string{}\n",
		"internal/cli/settings.go":                                 "package cli\n// Check the docs link.\nfunc enabled() bool { return true }\n",
	}
	for path, body := range files {
		full := filepath.Join(a.root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.command("git", "add", "."); err != nil {
		t.Fatal(err)
	}
	if _, err := a.command("git", "-c", "user.name=Test", "-c", "user.email=test@example.test", "-c", "commit.gpgsign=false", "commit", "-qm", "real routing baseline"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(a.root, "internal/cli/settings.go")
	if err := os.WriteFile(path, []byte("package cli\n// Check the updated docs link.\nfunc enabled() bool { return true }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.commentSource("HEAD", "internal/cli/settings.go"); err != nil {
		t.Fatalf("canonical Git path must retain comment-only proof: %v", err)
	}
	run := call(t, a, "start", "--out", filepath.Join(t.TempDir(), "run"), "--builtins-since", "HEAD", "alpha", "beta")
	fetched(t, a, run, "unchanged")
	return a, run
}

func TestSharedCommentReviewIsRequiredBeforeFinalization(t *testing.T) {
	a, run := commentFixture(t)
	m, err := load(run)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.BuiltinDeep) != 0 {
		t.Errorf("comment-only targets require runtime review: %v", m.BuiltinDeep)
	}
	if len(m.CommentReviews) != 1 {
		t.Fatalf("comment reviews=%d, want one shared check", len(m.CommentReviews))
	}
	rejectedUnchanged(t, a, run)
	if _, err := readOutcome(m, []string{"fast-path", "report.md", "vendor unchanged"}); err == nil {
		t.Error("fast path accepted pending shared comment review")
	}
	call(t, a, "review-comment", run, m.CommentReviews[0].ID, "report.md", "Checked changed prose against repository and vendor claims")
	call(t, a, "finalize", run)
	// A behavior change after capture must still invalidate the frozen context.
	if err := os.WriteFile(filepath.Join(a.root, "internal/cli/settings.go"), []byte("package cli\nfunc enabled() bool { return false }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	rejectedUnchanged(t, a, run)
}

func TestSharedCommentReviewRejectsMissingAndStaleProof(t *testing.T) {
	a, run := commentFixture(t)
	m, err := load(run)
	if err != nil {
		t.Fatal(err)
	}
	saved := m.CommentReviews
	m.CommentReviews = nil
	if err := save(m); err != nil {
		t.Fatal(err)
	}
	rejectedUnchanged(t, a, run)
	m.CommentReviews = saved
	if err := save(m); err != nil {
		t.Fatal(err)
	}
	check := m.CommentReviews[0]
	if err := os.WriteFile(filepath.Join(run, check.After.Path), []byte("altered source evidence"), 0644); err != nil {
		t.Fatal(err)
	}
	rejectedUnchanged(t, a, run)
}

func TestSharedCommentReviewCannotReuseChangedReviewerEvidence(t *testing.T) {
	a, run := commentFixture(t)
	m, err := load(run)
	if err != nil {
		t.Fatal(err)
	}
	call(t, a, "review-comment", run, m.CommentReviews[0].ID, "report.md", "Reviewed native claim introduced by this prose")
	if err := os.WriteFile(filepath.Join(run, "report.md"), []byte("later correction"), 0644); err != nil {
		t.Fatal(err)
	}
	rejectedUnchanged(t, a, run)
}

func TestSharedCommentReviewResumesOnlyMatchingProof(t *testing.T) {
	a, prior := commentFixture(t)
	m, err := load(prior)
	if err != nil {
		t.Fatal(err)
	}
	call(t, a, "review-comment", prior, m.CommentReviews[0].ID, "report.md", "Reviewed prose; native claims preserved")
	run := call(t, a, "start", "--out", filepath.Join(t.TempDir(), "next"), "--builtins-since", "HEAD", "alpha", "beta")
	fetched(t, a, run, "unchanged")
	call(t, a, "resume", run, prior)
	call(t, a, "finalize", run)
	next, err := load(run)
	if err != nil {
		t.Fatal(err)
	}
	if next.CommentReviews[0].Outcome == nil {
		t.Error("matching prose review not retained")
	}
}

func TestScheduledFastPathsRequireCurrentSharedProseReview(t *testing.T) {
	a, run := commentCaptureFixture(t)
	call(t, a, "schedule", run, "1", "alpha", "beta")
	for _, target := range []string{"alpha", "beta"} {
		call(t, a, "dispatch", run, "Eowyn")
		writeRunFile(t, run, target+".md", "vendor sources unchanged; prose review tracked separately")
		scheduledCheckpoint(t, a, run, target)
		raw := call(t, a, "result-template", run, target)
		var result map[string]any
		if err := json.Unmarshal([]byte(raw), &result); err != nil {
			t.Fatal(err)
		}
		out := result["outcome"].(map[string]any)
		out["disposition"] = "fast-path"
		out["reason"] = "unchanged source evidence and runtime tokens"
		for _, source := range result["sources"].([]any) {
			outcome := source.(map[string]any)["outcome"].(map[string]any)
			outcome["disposition"] = "fast-path"
			outcome["reason"] = "unchanged fetched evidence"
		}
		path := writeResult(t, run, result)
		reject := func() {
			t.Helper()
			before, err := os.ReadFile(filepath.Join(run, "audit.json"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := a.run([]string{"ingest", run, path}); err == nil {
				t.Error("accepted missing or stale prose review")
			}
			after, err := os.ReadFile(filepath.Join(run, "audit.json"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Error("rejected result changed coverage")
			}
		}
		if target == "alpha" {
			reject()
			m, err := load(run)
			if err != nil {
				t.Fatal(err)
			}
			proof := m.CommentReviews
			m.CommentReviews = nil
			if err := save(m); err != nil {
				t.Fatal(err)
			}
			reject()
			m.CommentReviews = proof
			if err := save(m); err != nil {
				t.Fatal(err)
			}
			writeRunFile(t, run, "prose.md", "Checked changed docs link; no changed native behavior claim.")
			call(t, a, "review-comment", run, proof[0].ID, "prose.md", "Checked exact before/after source and fresh vendor evidence")
			writeRunFile(t, run, "prose.md", "changed reviewer evidence")
			reject()
			writeRunFile(t, run, "prose.md", "Checked changed docs link; no changed native behavior claim.")
		}
		call(t, a, "ingest", run, path)
	}
	next := call(t, a, "start", "--out", filepath.Join(t.TempDir(), "resumed"), "--builtins-since", "HEAD", "alpha", "beta")
	fetched(t, a, next, "unchanged")
	call(t, a, "resume", next, run)
	call(t, a, "validate", next)
	call(t, a, "finalize", run)
}

func TestOlderResearchPolicyCannotGainCommentExclusions(t *testing.T) {
	a, prior := commentFixture(t)
	mutate(t, prior, func(m map[string]any) { m["research_version"] = 1; delete(m, "comment_reviews") })
	run := call(t, a, "start", "--out", filepath.Join(t.TempDir(), "next"), "--builtins-since", "HEAD", "alpha", "beta")
	fetched(t, a, run, "unchanged")
	if _, err := a.run([]string{"resume", run, prior}); err == nil {
		t.Error("older policy gained comment exclusions")
	}
	rejectedUnchanged(t, a, run)
}

func TestSharedCommentResumeRequiresFreshMatchingVendorEvidence(t *testing.T) {
	for _, status := range []string{"changed", "failed"} {
		t.Run(status, func(t *testing.T) {
			a, prior := commentFixture(t)
			m, err := load(prior)
			if err != nil {
				t.Fatal(err)
			}
			call(t, a, "review-comment", prior, m.CommentReviews[0].ID, "report.md", "Checked native prose against every affected vendor source")
			run := call(t, a, "start", "--out", filepath.Join(t.TempDir(), "fresh"), "--builtins-since", "HEAD", "alpha", "beta")
			if err := os.MkdirAll(filepath.Join(run, "pages"), 0755); err != nil {
				t.Fatal(err)
			}
			var rows strings.Builder
			for _, source := range m.Sources {
				row := append([]string(nil), source.Fetch.Row...)
				body, err := os.ReadFile(filepath.Join(prior, row[9]))
				if err != nil {
					t.Fatal(err)
				}
				if source.Target == "beta" && source.Kind == "code" {
					row[7] = status
					if status == "failed" {
						row[5] = "-"
					} else {
						body = append(body, []byte("Native behavior changed.\n")...)
						row[5] = digest(body)
					}
				}
				writeRunFile(t, run, row[9], string(body))
				rows.WriteString(strings.Join(row, "\t") + "\n")
			}
			writeRunFile(t, run, "docfetch.tsv", rows.String())
			call(t, a, "capture", run)
			call(t, a, "resume", run, prior)
			fresh, err := load(run)
			if err != nil {
				t.Fatal(err)
			}
			if fresh.CommentReviews[0].Outcome != nil {
				t.Error("reused prose judgment despite changed or unusable vendor evidence")
			}
			if err := requireCommentReviews(fresh); err == nil {
				t.Error("changed vendor evidence did not require fresh prose review")
			}
		})
	}
}

func TestSharedCommentResumePreservesCurrentReviewedCorrection(t *testing.T) {
	a, prior := commentFixture(t)
	m, err := load(prior)
	if err != nil {
		t.Fatal(err)
	}
	call(t, a, "review-comment", prior, m.CommentReviews[0].ID, "report.md", "Original claim assessment")
	run := call(t, a, "start", "--out", filepath.Join(t.TempDir(), "current"), "--builtins-since", "HEAD", "alpha", "beta")
	fetched(t, a, run, "unchanged")
	writeRunFile(t, run, "correction.md", "The earlier assessment missed a native claim; retain this finding.")
	call(t, a, "review-comment", run, m.CommentReviews[0].ID, "correction.md", "Corrected native claim assessment")
	before, err := load(run)
	if err != nil {
		t.Fatal(err)
	}
	want := *before.CommentReviews[0].Outcome
	call(t, a, "resume", run, prior)
	after, err := load(run)
	if err != nil {
		t.Fatal(err)
	}
	if after.CommentReviews[0].Outcome == nil || *after.CommentReviews[0].Outcome != want {
		t.Error("resume replaced the current prose correction with an older assessment")
	}
}
