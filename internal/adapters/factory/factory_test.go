package factory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestName(t *testing.T) {
	if got := New().Name(); got != "factory" {
		t.Errorf("Name() = %q, want %q", got, "factory")
	}
}

// The project-root AGENTS.md is written centrally by sync, never by
// this adapter: Droid CLI has no per-rule surface of its own.
func TestEmit_NoRootAGENTSMd_ByDefault(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("adapter should not write AGENTS.md, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".factory")); !os.IsNotExist(err) {
		t.Errorf("a rule-only bundle should not create .factory/, err=%v", err)
	}
}

func TestEmit_Agent_WritesDroidFile(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent, Name: "release-manager",
			Meta: map[string]any{"description": "Ships releases.", "model": "opus", "tools": []any{"Read", "Bash"}},
			Body: "Run the release checklist.",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".factory/droids/release-manager.md"))
	if !strings.HasPrefix(got, "---\n") {
		t.Fatalf("frontmatter must be first, got:\n%s", got)
	}
	for _, want := range []string{
		"name: release-manager",
		"description: Ships releases.",
		"model: opus",
		"tools:",
		"- Read",
		"- Bash",
		"Run the release checklist.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestEmit_Agent_DescriptionFallsBackToName(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "no-desc", Body: "body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".factory/droids/no-desc.md"))
	if !strings.Contains(got, "description: no-desc") {
		t.Errorf("expected description fallback to agent name, got:\n%s", got)
	}
	if strings.Contains(got, "model:") || strings.Contains(got, "tools:") {
		t.Errorf("expected no model/tools keys when absent from meta, got:\n%s", got)
	}
}

// Arbitrary x-factory keys pass through so the full droid schema is
// reachable without waiting on this adapter's allowlist.
func TestEmit_Agent_XFactoryPassthrough(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent, Name: "alpha",
			Meta: map[string]any{"description": "d", "x-factory": map[string]any{"reasoning_effort": "high"}},
			Body: "body",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".factory/droids/alpha.md"))
	if !strings.Contains(got, "reasoning_effort: high") {
		t.Errorf("expected x-factory key to pass through, got:\n%s", got)
	}
}

func TestEmit_AgentsDirOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{Outputs: map[string]config.Output{"factory": {AgentsDir: "custom/droids"}}}
	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "a1", Body: "body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/droids/a1.md")); err != nil {
		t.Errorf("expected override dir to hold the droid file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".factory/droids/a1.md")); !os.IsNotExist(err) {
		t.Errorf("expected no output at the default droids dir, err=%v", err)
	}
}

func TestEmit_EmptyBundle_WritesNothing(t *testing.T) {
	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".factory")); !os.IsNotExist(err) {
		t.Errorf("expected no .factory/ for an empty bundle, err=%v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
