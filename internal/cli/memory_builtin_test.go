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

func TestRender_ResolvesTheBuiltinSourcesSyncStamps(t *testing.T) {
	setupMemoryProject(t, "claude")

	for selector, want := range map[string]string{
		"builtin:shared-memory-policy": ".claude/rules/shared-memory-policy.md",
		"builtin:shared-memory":        ".claude/skills/shared-memory/SKILL.md",
	} {
		var out strings.Builder
		root := NewRootCmd("test")
		root.SetOut(&out)
		root.SetArgs([]string{"render", selector})
		if err := root.Execute(); err != nil {
			t.Errorf("render %s: %v", selector, err)
			continue
		}
		if !strings.Contains(out.String(), want) {
			t.Errorf("render %s did not render %s:\n%s", selector, want, out.String())
		}
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
	if len(sources) != 1 || !sources["builtin:shared-memory-policy"] {
		t.Errorf("want only builtin:shared-memory-policy, got %v", sources)
	}
}
