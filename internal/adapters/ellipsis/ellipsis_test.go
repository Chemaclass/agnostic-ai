package ellipsis

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const reviewFile = ".ellipsis/code-review.yaml"

func reviewer(extra map[string]any) map[string]any {
	r := map[string]any{
		"name":        "project-review",
		"claude_code": map[string]any{"model": "claude-opus-5-5"},
	}
	for k, v := range extra {
		r[k] = v
	}
	return r
}

func settings(x map[string]any) spec.Entry {
	return spec.Entry{Kind: spec.KindSettings, Name: "ellipsis", Path: "settings/ellipsis.yaml", Meta: map[string]any{"x-ellipsis": x}}
}

func review(name, scope, body string) spec.Entry {
	e := spec.Entry{Kind: spec.KindReview, Name: name, Path: "reviews/" + name + ".md", Body: body}
	if scope != "" {
		e.Meta = map[string]any{"scope": scope}
	}
	return e
}

func emitBundle(t *testing.T, cfg *config.Config, entries ...spec.Entry) error {
	t.Helper()
	return New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false)
}

func readReview(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// promptOf returns the reviewer prompt of a generated file.
func promptOf(t *testing.T, file string) string {
	t.Helper()
	var doc struct {
		Reviewers []map[string]any `yaml:"reviewers"`
	}
	if err := yaml.Unmarshal([]byte(file), &doc); err != nil || len(doc.Reviewers) != 1 {
		t.Fatalf("parse generated file: %v\n%s", err, file)
	}
	for _, key := range []string{"claude_code", "codex", "gemini"} {
		if engine, ok := doc.Reviewers[0][key].(map[string]any); ok {
			if p, ok := engine["prompt"].(string); ok {
				return p
			}
		}
	}
	t.Fatalf("no prompt in:\n%s", file)
	return ""
}

func captureNotes(t *testing.T) *strings.Builder {
	t.Helper()
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })
	return buf
}

func TestEmit_RendersRootAndScopedGuidanceIntoOneReviewer(t *testing.T) {
	testutil.TempCwd(t)
	err := emitBundle(t, &config.Config{},
		review("general", "", "Flag any handler that talks to the database directly."),
		review("contracts", "services/api", "Reject a change that removes a field of a public response."),
		settings(map[string]any{
			"pull_requests": map[string]any{"draft": false},
			"reviewer":      reviewer(nil),
			"budget":        map[string]any{"session": 3},
		}))
	if err != nil {
		t.Fatal(err)
	}
	want := header.Line(header.FormatYAML) + `ellipsis:
  kind: code_review
  name: code-review
pull_requests:
  draft: false
reviewers:
  - name: project-review
    claude_code:
      prompt: |-
        Flag any handler that talks to the database directly.

        ## Checks for changes under services/api/

        Apply the checks in this section only when the pull request changes a file under ` + "`services/api/`" + `. Skip them otherwise.

        Reject a change that removes a field of a public response.
      model: claude-opus-5-5
budget:
  session: 3
`
	if got := readReview(t, reviewFile); got != want {
		t.Errorf("code-review.yaml mismatch\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

func TestEmit_ScopedChecksStayConditional(t *testing.T) {
	testutil.TempCwd(t)
	err := emitBundle(t, &config.Config{},
		review("api", "services/api", "API check."),
		review("web", "./services/web/", "Web check."),
		review("nested", "services/api/v2", "V2 check."),
		settings(map[string]any{"reviewer": reviewer(nil)}))
	if err != nil {
		t.Fatal(err)
	}
	got := promptOf(t, readReview(t, reviewFile))
	for _, scope := range []string{"services/api/", "services/api/v2/", "services/web/"} {
		section := "## Checks for changes under " + scope + "\n\nApply the checks in this section only when the pull request changes a file under `" + scope + "`. Skip them otherwise."
		if !strings.Contains(got, section) {
			t.Errorf("missing conditional section for %s:\n%s", scope, got)
		}
	}
	if strings.Index(got, "API check.") > strings.Index(got, "V2 check.") || strings.Index(got, "V2 check.") > strings.Index(got, "Web check.") {
		t.Errorf("scoped sections are not in sorted scope order:\n%s", got)
	}
	if file := readReview(t, reviewFile); strings.Count(file, "reviewers:") != 1 || strings.Count(file, "  - name:") != 1 {
		t.Errorf("scoped guidance must stay inside one reviewer:\n%s", file)
	}
}

func TestEmit_ScopedOnlyGuidanceHasNoUnconditionalText(t *testing.T) {
	testutil.TempCwd(t)
	if err := emitBundle(t, &config.Config{},
		review("api", "services/api", "API check."),
		settings(map[string]any{"reviewer": reviewer(nil)})); err != nil {
		t.Fatal(err)
	}
	got := readReview(t, reviewFile)
	if !strings.Contains(got, "      prompt: |-\n        ## Checks for changes under services/api/\n") {
		t.Errorf("prompt should open with the conditional section:\n%s", got)
	}
}

func TestEmit_EscapingScopeIsSkipped(t *testing.T) {
	testutil.TempCwd(t)
	if err := emitBundle(t, &config.Config{},
		review("root", "", "Root check."),
		review("bad", "../outside", "Outside check."),
		settings(map[string]any{"reviewer": reviewer(nil)})); err != nil {
		t.Fatal(err)
	}
	if got := readReview(t, reviewFile); strings.Contains(got, "Outside check.") {
		t.Errorf("a scope outside the project must not reach the file:\n%s", got)
	}
}

func TestEmit_OutputOverrideMovesTheFile(t *testing.T) {
	dir := testutil.TempCwd(t)
	cfg := &config.Config{Outputs: map[string]config.Output{"ellipsis": {ReviewFile: "ci/ellipsis.yaml"}}}
	if err := emitBundle(t, cfg, review("r", "", "Check."), settings(map[string]any{"reviewer": reviewer(nil)})); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readReview(t, "ci/ellipsis.yaml"), "Check.") {
		t.Error("override file lacks the review")
	}
	if _, err := os.Stat(filepath.Join(dir, reviewFile)); !os.IsNotExist(err) {
		t.Errorf("default path must stay empty when the override is set (%v)", err)
	}
}

func TestEmit_TargetFilterKeepsAReviewOutOfTheFile(t *testing.T) {
	testutil.TempCwd(t)
	only := review("cursor-only", "", "Cursor only.")
	only.Meta = map[string]any{"targets": []any{"cursor"}}
	both := review("shared", "", "Shared check.")
	b := spec.NewBundle([]spec.Entry{only, both, settings(map[string]any{"reviewer": reviewer(nil)})}).For("ellipsis")
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readReview(t, reviewFile)
	if strings.Contains(got, "Cursor only.") || !strings.Contains(got, "Shared check.") {
		t.Errorf("target filter not honored:\n%s", got)
	}
}

func TestEmit_NoReviewsWritesNothingAndSaysNothing(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := captureNotes(t)
	if err := emitBundle(t, &config.Config{}, settings(map[string]any{"reviewer": reviewer(nil)})); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if paths := testutil.WalkRel(t, dir); len(paths) != 0 {
		t.Errorf("no reviews must write no file, got %v", paths)
	}
	if buf.Len() != 0 {
		t.Errorf("no reviews must raise no note, got:\n%s", buf)
	}
}

func TestEmit_NeverInfersAReviewerFromAnAgent(t *testing.T) {
	dir := testutil.TempCwd(t)
	agent := spec.Entry{Kind: spec.KindAgent, Name: "reviewer", Path: "agents/reviewer.md", Body: "Review code.",
		Meta: map[string]any{"model": "claude-opus-5-5"}}
	if err := emitBundle(t, &config.Config{}, agent, review("r", "", "Check.")); err != nil {
		t.Fatal(err)
	}
	if paths := testutil.WalkRel(t, dir); len(paths) != 0 {
		t.Errorf("an agent spec must not become a reviewer, got %v", paths)
	}
}

func TestEmit_WritesNoOutputBlockUnlessConfigured(t *testing.T) {
	testutil.TempCwd(t)
	if err := emitBundle(t, &config.Config{}, review("r", "", "Check."), settings(map[string]any{"reviewer": reviewer(nil)})); err != nil {
		t.Fatal(err)
	}
	got := readReview(t, reviewFile)
	for _, absent := range []string{"output:", "approve", "pull_requests:", "budget:"} {
		if strings.Contains(got, absent) {
			t.Errorf("%q must come only from explicit x-ellipsis settings:\n%s", absent, got)
		}
	}
}

func TestEmit_AutomaticApprovalNeedsExplicitOutput(t *testing.T) {
	testutil.TempCwd(t)
	x := map[string]any{
		"reviewer": reviewer(nil),
		"output":   map[string]any{"on_pass": "approve", "on_fail": "request_changes", "summary": true},
	}
	if err := emitBundle(t, &config.Config{}, review("r", "", "Check."), settings(x)); err != nil {
		t.Fatal(err)
	}
	want := "output:\n  on_fail: request_changes\n  on_pass: approve\n  summary: true\n"
	if got := readReview(t, reviewFile); !strings.Contains(got, want) {
		t.Errorf("output block not copied as written:\n%s", got)
	}
}

func TestEmit_KeepsNativeReviewerFieldsInNativeOrder(t *testing.T) {
	testutil.TempCwd(t)
	x := map[string]any{
		"ellipsis": map[string]any{"name": "nightly-review"},
		"reviewer": map[string]any{
			"comments":    map[string]any{"max": 5},
			"permissions": map[string]any{"github": map[string]any{"permissions": "read_only"}},
			"environment": "api-environment",
			"codex":       map[string]any{"model": "gpt-6-sol"},
			"paths":       []any{"services/**"},
			"name":        "project-review",
			"budget":      map[string]any{"session": 5},
		},
	}
	if err := emitBundle(t, &config.Config{}, review("r", "", "Check."), settings(x)); err != nil {
		t.Fatal(err)
	}
	want := header.Line(header.FormatYAML) + `ellipsis:
  kind: code_review
  name: nightly-review
reviewers:
  - name: project-review
    paths:
      - services/**
    codex:
      prompt: Check.
      model: gpt-6-sol
    environment: api-environment
    permissions:
      github:
        permissions: read_only
    budget:
      session: 5
    comments:
      max: 5
`
	if got := readReview(t, reviewFile); got != want {
		t.Errorf("mismatch\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

func TestEmit_LaterSettingsSpecReplacesEachTopLevelBlock(t *testing.T) {
	testutil.TempCwd(t)
	first := settings(map[string]any{"reviewer": reviewer(nil), "budget": map[string]any{"session": 3}})
	second := settings(map[string]any{"budget": map[string]any{"session": 9}})
	second.Name = "later"
	if err := emitBundle(t, &config.Config{}, review("r", "", "Check."), first, second); err != nil {
		t.Fatal(err)
	}
	got := readReview(t, reviewFile)
	if !strings.Contains(got, "budget:\n  session: 9\n") || !strings.Contains(got, "project-review") {
		t.Errorf("later spec must win per key and keep the others:\n%s", got)
	}
}

func TestEmit_DryRunWritesNothing(t *testing.T) {
	dir := testutil.TempCwd(t)
	b := spec.NewBundle([]spec.Entry{review("r", "", "Check."), settings(map[string]any{"reviewer": reviewer(nil)})})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, true); err != nil {
		t.Fatal(err)
	}
	if paths := testutil.WalkRel(t, dir); len(paths) != 0 {
		t.Errorf("dry run wrote %v", paths)
	}
}

func TestEmit_NameIsRegistered(t *testing.T) {
	if New().Name() != "ellipsis" {
		t.Errorf("Name() = %q", New().Name())
	}
	got := New().Capabilities()
	want := []spec.Kind{spec.KindSettings, spec.KindReview, spec.KindEnvironment, spec.KindMCP}
	if len(got) != len(want) {
		t.Fatalf("Capabilities() = %v, want %v", got, want)
	}
	for _, k := range want {
		if !slices.Contains(got, k) {
			t.Errorf("Capabilities() = %v lacks %s", got, k)
		}
	}
}

func TestEmit_PassesThroughEveryEllipsisBlockKey(t *testing.T) {
	testutil.TempCwd(t)
	x := map[string]any{
		"reviewer": reviewer(nil),
		"ellipsis": map[string]any{
			"name":        "api-review",
			"description": "Review the API",
			"enabled":     false,
			"metadata":    map[string]any{"team": "api"},
		},
	}
	if err := emitBundle(t, &config.Config{}, review("r", "", "Check."), settings(x)); err != nil {
		t.Fatal(err)
	}
	want := "ellipsis:\n  kind: code_review\n  name: api-review\n  description: Review the API\n  enabled: false\n  metadata:\n    team: api\nreviewers:\n"
	if got := readReview(t, reviewFile); !strings.Contains(got, want) {
		t.Errorf("ellipsis block not written as set:\n%s", got)
	}
}

func TestEmit_WritesAGeminiReviewer(t *testing.T) {
	testutil.TempCwd(t)
	x := map[string]any{"reviewer": map[string]any{
		"name":   "project-review",
		"gemini": map[string]any{"model": "gemini-4-argon"},
	}}
	if err := emitBundle(t, &config.Config{}, review("r", "", "Check."), settings(x)); err != nil {
		t.Fatal(err)
	}
	want := header.Line(header.FormatYAML) + `ellipsis:
  kind: code_review
  name: code-review
reviewers:
  - name: project-review
    gemini:
      prompt: Check.
      model: gemini-4-argon
`
	if got := readReview(t, reviewFile); got != want {
		t.Errorf("mismatch\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}
