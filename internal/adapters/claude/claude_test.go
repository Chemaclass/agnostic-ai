package claude

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestEmit_WritesAgent(t *testing.T) {
	dir := t.TempDir()
	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	a := New()
	cfg := &config.Config{}
	entries := []spec.Entry{
		{
			Kind: spec.KindAgent,
			Name: "reviewer",
			Meta: map[string]any{"name": "reviewer", "description": "x"},
			Body: "do reviews",
		},
	}
	if err := a.Emit(entries, cfg, false); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".claude", "agents", "reviewer.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("expected non-empty agent file")
	}
}
