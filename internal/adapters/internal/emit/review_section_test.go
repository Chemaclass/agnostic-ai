package emit

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func reviewBundle() spec.Bundle {
	return spec.NewBundle([]spec.Entry{
		{Kind: spec.KindReview, Name: "api", Body: "Flag raw SQL in handlers.\n", Meta: map[string]any{"scope": "services/api"}},
		{Kind: spec.KindReview, Name: "root", Body: "Check error wrapping.\n"},
		{Kind: spec.KindReview, Name: "bugbot-only", Body: "Cursor only.\n", Meta: map[string]any{"targets": []any{"cursor"}}},
	})
}

// Codex code review reads `## Code Review Rules` from the root and the
// nearest AGENTS.md (learn.chatgpt.com/docs/third-party/github) (#1341).
func TestReviewSections_OneSectionPerScopeForCodex(t *testing.T) {
	got := ReviewSections(reviewBundle(), &config.Config{Targets: []string{"codex"}})
	if len(got) != 2 {
		t.Fatalf("got %d sections, want root and services/api: %v", len(got), got)
	}
	api := got["services/api"]
	if !strings.Contains(api, "## Code Review Rules\n\nFlag raw SQL in handlers.") {
		t.Errorf("scoped section:\n%s", api)
	}
	if strings.Contains(got[""], "Cursor only.") {
		t.Errorf("a review targeting cursor reached the Codex section:\n%s", got[""])
	}
	if ReviewSections(reviewBundle(), &config.Config{Targets: []string{"cursor"}}) != nil {
		t.Error("sections rendered without codex in the project")
	}
}

func TestStripReviewSection_RoundTrips(t *testing.T) {
	body := "Intro.\n"
	with := AppendReviewSection(body, RenderReviewSection([]spec.Entry{{Body: "Rule."}}))
	if got := StripGeneratedAppendices(with); got != body {
		t.Errorf("strip = %q, want %q", got, body)
	}
}

// A codex scope with reviews and no rules still gets an AGENTS.md; a
// reader that writes AGENTS.md only for rules, such as cursor, does not
// create one, and a scope with both keeps rules then reviews.
func TestPrepareScopedDocuments_WritesReviewSection(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	cfg := &config.Config{Targets: []string{"codex", "cursor"}}
	sections := ReviewSections(reviewBundle(), cfg)
	path := filepath.Join("services", "api", "AGENTS.md")

	_, files, err := PrepareScopedDocuments(spec.Bundle{}, cfg, "codex", sections)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path != path || !strings.Contains(files[0].Content, "Flag raw SQL") {
		t.Fatalf("codex files = %+v", files)
	}
	if _, files, _ = PrepareScopedDocuments(spec.Bundle{}, cfg, "cursor", sections); len(files) != 0 {
		t.Errorf("cursor wrote a review-only AGENTS.md: %+v", files)
	}

	rules := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "api", Body: "Use the repo layer.", Meta: map[string]any{"scope": "services/api"}}})
	_, files, err = PrepareScopedDocuments(rules, cfg, "codex", sections)
	if err != nil {
		t.Fatal(err)
	}
	content := files[0].Content
	if r, s := strings.Index(content, "Use the repo layer."), strings.Index(content, "## Code Review Rules"); r < 0 || s < r {
		t.Errorf("want rules then reviews:\n%s", content)
	}
	shared := map[string]CapturedFile{path: files[0]}
	bundles := map[string]spec.Bundle{"codex": rules, "cursor": rules}
	if err := CheckScopeReaders(bundles, shared, []string{"codex", "cursor"}, sections); err != nil {
		t.Errorf("readers disagree on the shared file: %v", err)
	}
}

func TestCheckScopeReaders_AcceptsReviewOnlyFile(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	cfg := &config.Config{Targets: []string{"codex", "cursor"}}
	sections := ReviewSections(reviewBundle(), cfg)
	_, files, err := PrepareScopedDocuments(spec.Bundle{}, cfg, "codex", sections)
	if err != nil {
		t.Fatal(err)
	}
	shared := map[string]CapturedFile{files[0].Path: files[0]}
	if err := CheckScopeReaders(map[string]spec.Bundle{}, shared, []string{"codex", "cursor"}, sections); err != nil {
		t.Errorf("review-only AGENTS.md rejected: %v", err)
	}
}
