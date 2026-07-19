package opencode

import (
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

// Skills emit natively under .opencode/skills/ now, so the old
// emit-skills-as-commands coverage note is gone: nothing is dropped.
func TestEmit_NoSkillNote_SkillsEmitNatively(t *testing.T) {
	testutil.TempCwd(t)
	buf := swapNoteWarner(t)
	if err := New().Emit(kitSinkBundle(), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	emit.FlushCoverageNotes()
	if strings.Contains(buf.String(), "reach opencode") {
		t.Errorf("native skill emission must not note a gap, got: %s", buf.String())
	}
}

func TestEmit_NoNotesWhenNoSkills(t *testing.T) {
	testutil.TempCwd(t)
	buf := swapNoteWarner(t)
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindAgent, Name: "alpha", Path: "agents/alpha.md", Body: "x"},
	})
	if err := New().Emit(b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if got := emit.PendingCoverageNotesCount(); got != 0 {
		t.Errorf("no skill specs must buffer no note, count=%d", got)
	}
	emit.FlushCoverageNotes()
	if buf.Len() != 0 {
		t.Errorf("expected no output, got: %s", buf.String())
	}
}
