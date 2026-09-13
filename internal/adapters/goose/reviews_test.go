package goose

import (
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_ReviewsPreserveRootAndNestedScopes(t *testing.T) {
	testutil.TempCwd(t)
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindReview, Name: "root", Body: "Check every change."},
		{Kind: spec.KindReview, Name: "root-alias", Meta: map[string]any{"scope": "."}, Body: "Check root aliases."},
		{Kind: spec.KindReview, Name: "api", Meta: map[string]any{"scope": "api"}, Body: "Check API compatibility."},
		{Kind: spec.KindReview, Name: "auth", Meta: map[string]any{"scope": "./api"}, Body: "Check authentication."},
		{Kind: spec.KindReview, Name: "v2", Meta: map[string]any{"scope": "api/v2"}, Body: "Check the v2 response."},
	})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		".agents/REVIEW.md":        "Check every change.\n\nCheck root aliases.\n",
		"api/.agents/REVIEW.md":    "Check API compatibility.\n\nCheck authentication.\n",
		"api/v2/.agents/REVIEW.md": "Check the v2 response.\n",
	} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(string(got), want) || strings.Contains(string(got), "scope:") {
			t.Errorf("%s = %q, want review body %q", path, got, want)
		}
	}
}

func TestEmit_ReviewsHonorOutputOverride(t *testing.T) {
	testutil.TempCwd(t)
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindReview, Name: "review", Body: "Check the change."}})
	cfg := &config.Config{Outputs: map[string]config.Output{"goose": {ReviewFile: "custom/REVIEW.md"}}}
	if err := New().Emit(emit.NewSession(), b, cfg, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("custom/REVIEW.md")
	if err != nil || !strings.Contains(string(data), "Check the change.") {
		t.Errorf("review override = %q (%v)", data, err)
	}
}
