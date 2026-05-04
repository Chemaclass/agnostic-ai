package gemini

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestEmit_WritesGeminiMd(t *testing.T) {
	dir := t.TempDir()
	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	_ = os.Chdir(dir)

	a := New()
	if a.Name() != "gemini" {
		t.Errorf("expected gemini, got %s", a.Name())
	}

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "rule"},
		{Kind: spec.KindAgent, Name: "ag1", Body: "agent"},
		{Kind: spec.KindSkill, Name: "sk1", Path: "skills/sk1.md", Body: "skill body"},
	}
	if err := a.Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "GEMINI.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Agent: ag1", "## Skills", "### sk1"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}
