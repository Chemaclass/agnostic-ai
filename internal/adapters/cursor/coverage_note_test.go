package cursor

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

func swapNoteWarner(t *testing.T) *strings.Builder {
	t.Helper()
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	return buf
}

// Cursor discovers native skill folders, so a skill's bundled payload
// propagates byte-for-byte instead of being dropped with a coverage
// note (the pre-native behavior from #430).
func TestEmit_SkillBundledAssetsPropagate(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	skillDir := filepath.Join(dir, "skills", "alpha")
	if err := os.MkdirAll(filepath.Join(skillDir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "scripts", "run.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindSkill, Name: "alpha", Path: filepath.Join(skillDir, "SKILL.md"), Body: "body"},
	})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".cursor", "skills", "alpha", "scripts", "run.sh"))
	if err != nil {
		t.Fatalf("bundled asset must propagate to the native skill folder: %v", err)
	}
	if string(got) != "#!/bin/sh\n" {
		t.Errorf("asset not byte-identical: %q", got)
	}

	if n := emit.PendingCoverageNotesCount(); n != 0 {
		t.Errorf("native skill emission must buffer no coverage note, count=%d", n)
	}
	emit.FlushCoverageNotes()
	if buf.Len() != 0 {
		t.Errorf("expected no output, got: %s", buf.String())
	}
}
