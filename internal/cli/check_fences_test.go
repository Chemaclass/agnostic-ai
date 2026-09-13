package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// A freshly synced fenced body reports no drift, and a hand-edit to
// one target's fenced-out paragraph is caught as stale for that file
// only.
func TestCollectEntryPointDrift_FencedBodyCleanThenStaleAfterHandEdit(t *testing.T) {
	dir := testutil.TempCwd(t)
	body := "Shared.\n\n::target claude\nClaude line.\n::end\n\n::target gemini\nGemini line.\n::end\n"
	writeAgnosticFile(t, body)
	cfg := &config.Config{Targets: []string{"claude", "gemini"}}

	if err := writeAgnosticEntryPoints(adapters.NewSession(), cfg, spec.Bundle{}, cfg.Targets, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rep, err := collectEntryPointDrift(cfg, spec.Bundle{}, cfg.Targets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rep.hasDrift() {
		t.Fatalf("freshly synced fenced tree reports drift: missing=%v stale=%v",
			paths(rep.Missing), paths(rep.Stale))
	}

	geminiPath := filepath.Join(dir, "GEMINI.md")
	if err := os.WriteFile(geminiPath, []byte("hand-edited, no longer matches sync\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rep, err = collectEntryPointDrift(cfg, spec.Bundle{}, cfg.Targets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	stale := paths(rep.Stale)
	if len(stale) != 1 || stale[0] != "GEMINI.md" {
		t.Errorf("expected only GEMINI.md stale, got missing=%v stale=%v", paths(rep.Missing), stale)
	}
}
