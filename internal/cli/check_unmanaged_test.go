package cli

import (
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestCollectEntryPointDrift_IgnoresUnmanaged(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeAgnosticFile(t, "# Pointer body\n")
	mustWriteFile(t, filepath.Join(dir, "CLAUDE.md"), "hand-owned\n")
	cfg := &config.Config{Targets: []string{"claude", "codex"}}
	cfg.Sync.Unmanaged = []string{"CLAUDE.md"}

	rep, err := collectEntryPointDrift(cfg, spec.Bundle{}, cfg.Targets)
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range append(rep.Missing, rep.Stale...) {
		if f.Path == "CLAUDE.md" {
			t.Errorf("user-owned CLAUDE.md reported as drift: %+v", rep)
		}
	}
	if len(rep.Missing) != 1 || rep.Missing[0].Path != "AGENTS.md" {
		t.Errorf("managed AGENTS.md should still be missing, got %+v", rep.Missing)
	}
}
