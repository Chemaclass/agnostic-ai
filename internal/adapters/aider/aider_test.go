package aider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestEmit_WritesConventions(t *testing.T) {
	dir := t.TempDir()
	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	_ = os.Chdir(dir)

	a := New()
	if a.Name() != "aider" {
		t.Errorf("expected aider, got %s", a.Name())
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "rule body"},
	}
	if err := a.Emit(entries, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "CONVENTIONS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "rule body") {
		t.Errorf("missing body: %s", got)
	}
}
