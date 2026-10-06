package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func setupMemoryProject(t *testing.T, targets string) string {
	t.Helper()
	dir := t.TempDir()
	cfg := "version: 1\ntargets: [" + targets + "]\nbuiltins: [memory]\n"
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, dir)
	silence(t)
	return dir
}

func TestWhy_CreditsTheSharedMemoryImport(t *testing.T) {
	setupMemoryProject(t, "claude")

	got := runWhyJSON(t, "CLAUDE.md")
	if len(got.Sources) == 0 || got.Sources[0].Mode != "section" {
		t.Fatalf("AGNOSTIC_AI.md should be one section of CLAUDE.md: %+v", got.Sources)
	}
	for _, s := range got.Sources {
		if s.Name == "shared memory import" && s.Builtin != nil && s.Builtin.Name == "memory" {
			return
		}
	}
	t.Errorf("no memory built-in source: %+v", got.Sources)
}

func TestRender_ResolvesTheBuiltinSourceSyncStamps(t *testing.T) {
	setupMemoryProject(t, "claude")

	root := NewRootCmd("test")
	root.SetArgs([]string{"render", "builtin:shared-memory"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "builtin:memory") {
		t.Fatalf("a name shared by the rule and the skill should point at builtin:memory, got %v", err)
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"render", "builtin:memory"})
	if err := root.Execute(); err != nil {
		t.Errorf("render builtin:memory: %v", err)
	}
}

// Cursor gets the built-in rule as an .mdc file and, through Codex, in
// AGENTS.md. Both report one source, and neither marks it not emitted.
func TestExplainFile_ReportsABuiltinRuleUnderOneSource(t *testing.T) {
	setupMemoryProject(t, "cursor, codex")

	got := explainFileJSON(t, "main.go")
	sources := map[string]bool{}
	for _, it := range got.Instructions {
		if strings.Contains(it.Source, "shared-memory") || strings.Contains(it.Source, "builtins") {
			sources[it.Source] = true
			if it.Status == contextNotEmitted {
				t.Errorf("built-in rule reported not emitted: %+v", it)
			}
		}
	}
	if len(sources) != 1 || !sources["builtin:shared-memory"] {
		t.Errorf("want only builtin:shared-memory, got %v", sources)
	}
}
