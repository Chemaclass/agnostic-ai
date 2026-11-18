package goose

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_ReviewsAreUnsupported(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCapabilityWarnings()
	t.Cleanup(emit.ResetCapabilityWarnings)
	var warnings bytes.Buffer
	previous := emit.Warner
	emit.Warner = &warnings
	t.Cleanup(func() { emit.Warner = previous })
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindReview, Name: "root", Body: "Check every change."},
		{Kind: spec.KindReview, Name: "api", Meta: map[string]any{"scope": "api"}, Body: "Check API compatibility."},
	})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCapabilityWarnings()
	if got := warnings.String(); !strings.Contains(got, "2 reviews unsupported by goose") {
		t.Errorf("missing unsupported review warning: %q", got)
	}
	for _, path := range []string{".agents/REVIEW.md", "api/.agents/REVIEW.md"} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("unsupported review output %s exists: %v", path, err)
		}
	}
}

func TestEmit_ReviewsPreserveUserFilesAndAcceptedOverride(t *testing.T) {
	testutil.TempCwd(t)
	const body = "User-owned review instructions.\n"
	for _, path := range []string{".agents/REVIEW.md", "api/.agents/REVIEW.md", "custom/REVIEW.md", "api/custom/REVIEW.md"} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindReview, Name: "root", Body: "Check every change."},
		{Kind: spec.KindReview, Name: "api", Meta: map[string]any{"scope": "api"}, Body: "Check API compatibility."},
	})
	cfg := &config.Config{OnUnsupported: "silent", Outputs: map[string]config.Output{"goose": {ReviewFile: "custom/REVIEW.md"}}}
	if err := New().Emit(emit.NewSession(), b, cfg, false); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{".agents/REVIEW.md", "api/.agents/REVIEW.md", "custom/REVIEW.md", "api/custom/REVIEW.md"} {
		if got := readFile(t, path); got != body {
			t.Errorf("%s changed to %q", path, got)
		}
	}
}

func TestEmit_ReviewsRespectUnsupportedError(t *testing.T) {
	testutil.TempCwd(t)
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindReview, Name: "review", Body: "Check the change."}})
	err := New().Emit(emit.NewSession(), b, &config.Config{OnUnsupported: "error"}, false)
	if err == nil || !strings.Contains(err.Error(), "target does not support review") {
		t.Errorf("expected unsupported review error, got %v", err)
	}
}
