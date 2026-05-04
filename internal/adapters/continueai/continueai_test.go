package continueai

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestEmit_WritesRulesAndAgents(t *testing.T) {
	dir := t.TempDir()
	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	_ = os.Chdir(dir)

	a := New()
	if a.Name() != "continue" {
		t.Errorf("expected continue, got %s", a.Name())
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "rule"},
		{Kind: spec.KindAgent, Name: "ag1", Body: "agent"},
		{Kind: spec.KindSkill, Name: "sk1", Body: "skill"},
	}
	if err := a.Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{".continue/rules/r1.md", ".continue/rules/agent-ag1.md", ".continue/rules/skill-sk1.md"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("missing %s", p)
		}
	}
}
