package grok

import (
	"os"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_CapabilityMatrixCoversEveryDeclaredKind(t *testing.T) {
	testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), visibleConfig(), false); err != nil {
		t.Fatal(err)
	}
	paths := map[spec.Kind]string{spec.KindRule: ".grok/rules/style.md", spec.KindAgent: ".grok/agents/reviewer.md", spec.KindSkill: ".grok/skills/review/SKILL.md", spec.KindCommand: ".grok/commands/check.md", spec.KindHook: ".grok/hooks/guard.json", spec.KindMCP: ".grok/config.toml", spec.KindSettings: ".grok/config.toml"}
	for _, kind := range caps.Supports {
		data, err := os.ReadFile(paths[kind])
		if err != nil || len(data) == 0 {
			t.Errorf("%s has no content: %v", kind, err)
		}
	}
}
