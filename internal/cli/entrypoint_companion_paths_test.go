package cli

import (
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEntryPointPaths_ClaudeCompanionAgents(t *testing.T) {
	setup := func(t *testing.T, claudeMD string) {
		dir := testutil.TempCwd(t)
		t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
		mustWriteFile(t, filepath.Join(dir, "CLAUDE.md"), claudeMD)
		mustWriteFile(t, filepath.Join(dir, "AGENTS.md"), "# My rules\n")
	}

	t.Run("claude only with @AGENTS.md lists the companion", func(t *testing.T) {
		setup(t, "@AGENTS.md\n\nClaude note.\n")
		cfg := &config.Config{Targets: []string{"claude"}}
		got := entryPointPaths(cfg, cfg.Targets)
		want := []string{adapters.AgnosticEntryPointPath, "CLAUDE.md", "AGENTS.md"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("entryPointPaths = %q, want %q", got, want)
		}
	})

	t.Run("claude and codex list AGENTS.md once", func(t *testing.T) {
		setup(t, "@AGENTS.md\n")
		cfg := &config.Config{Targets: []string{"claude", "codex"}}
		got := entryPointPaths(cfg, cfg.Targets)
		count := 0
		for _, p := range got {
			if p == "AGENTS.md" {
				count++
			}
		}
		if count != 1 {
			t.Errorf("entryPointPaths = %q, want AGENTS.md once", got)
		}
	})

	t.Run("claude not importing AGENTS.md keeps the list unchanged", func(t *testing.T) {
		setup(t, "# Claude\n")
		cfg := &config.Config{Targets: []string{"claude"}}
		got := entryPointPaths(cfg, cfg.Targets)
		if slices.Contains(got, "AGENTS.md") {
			t.Errorf("entryPointPaths = %q, want no AGENTS.md", got)
		}
	})

	t.Run("claude outside the run leaves it out", func(t *testing.T) {
		setup(t, "@AGENTS.md\n")
		cfg := &config.Config{Targets: []string{"claude", "gemini"}}
		got := entryPointPaths(cfg, []string{"gemini"})
		if slices.Contains(got, "AGENTS.md") {
			t.Errorf("entryPointPaths = %q, want no AGENTS.md", got)
		}
	})

	t.Run("unmanaged companion is left out", func(t *testing.T) {
		setup(t, "@AGENTS.md\n")
		cfg := &config.Config{Targets: []string{"claude"}}
		cfg.Sync.Unmanaged = []string{"AGENTS.md"}
		got := entryPointPaths(cfg, cfg.Targets)
		if slices.Contains(got, "AGENTS.md") {
			t.Errorf("entryPointPaths = %q, want no AGENTS.md", got)
		}
	})
}
