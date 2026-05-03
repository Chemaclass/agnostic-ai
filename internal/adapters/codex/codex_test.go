package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestEmit_WritesAgentsMd(t *testing.T) {
	dir := t.TempDir()
	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	_ = os.Chdir(dir)

	a := New()
	if a.Name() != "codex" {
		t.Errorf("expected name codex, got %s", a.Name())
	}

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "rule body"},
		{Kind: spec.KindAgent, Name: "ag1", Body: "agent body"},
		{Kind: spec.KindHook, Name: "h1", Meta: map[string]any{"event": "X"}},
	}
	if err := a.Emit(entries, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "rule body") || !strings.Contains(string(got), "agent body") {
		t.Errorf("missing rule or agent body in output: %s", got)
	}
}

func TestEmit_OutputOverride(t *testing.T) {
	dir := t.TempDir()
	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	_ = os.Chdir(dir)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"codex": {File: "docs/AGENTS.md"}},
	}
	if err := New().Emit(nil, cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "docs", "AGENTS.md")); err != nil {
		t.Error("expected docs/AGENTS.md to exist")
	}
}
