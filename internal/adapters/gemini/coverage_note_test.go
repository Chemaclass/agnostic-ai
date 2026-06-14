package gemini

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

func TestEmit_NotesSkillGapWhenOptInUnset(t *testing.T) {
	testutil.TempCwd(t)
	buf := swapNoteWarner(t)
	if err := New().Emit(kitSinkBundle(), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	emit.FlushCoverageNotes()
	want := "3 skills reach gemini only via outputs.gemini.emit-skills-as-commands"
	if !strings.Contains(buf.String(), want) {
		t.Errorf("expected coverage note %q, got: %s", want, buf.String())
	}
}

func TestEmit_NoSkillNoteWhenOptInSet(t *testing.T) {
	testutil.TempCwd(t)
	buf := swapNoteWarner(t)
	cfg := &config.Config{Outputs: map[string]config.Output{
		"gemini": {EmitSkillsAsCommands: true},
	}}
	if err := New().Emit(kitSinkBundle(), cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	emit.FlushCoverageNotes()
	if strings.Contains(buf.String(), "reach gemini") {
		t.Errorf("opt-in set must suppress the skill note, got: %s", buf.String())
	}
}

func TestEmit_NoSkillNoteWhenNoSkills(t *testing.T) {
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
