package warp

import (
	"os"
	"path/filepath"
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
		t.Fatalf("scoped Warp skill missing: %v", err)
	}
}
