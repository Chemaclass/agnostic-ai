package codex

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

func TestEmit_SkillPreservesDirectoryScope(t *testing.T) {
	dir := testutil.TempCwd(t)
	entry := spec.Entry{Kind: spec.KindSkill, Name: "review", Scope: "services/api", Body: "Review API changes."}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "services/api/.agents/skills/review/SKILL.md")); err != nil {
		t.Fatalf("scoped Codex skill missing: %v", err)
	}
}

func TestEmit_SkillRejectsEscapingDirectoryScope(t *testing.T) {
	testutil.TempCwd(t)
	entry := spec.Entry{Kind: spec.KindSkill, Name: "review", Scope: "../outside", Body: "Review API changes."}
	err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), &config.Config{}, false)
	if err == nil || !strings.Contains(err.Error(), "escapes the project") {
		t.Fatalf("Emit() error = %v, want scoped output escape", err)
	}
}
