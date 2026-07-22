package aider

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

func TestEmit_NotesAgentAndSkillGapWhenRulesFileUnset(t *testing.T) {
	testutil.TempCwd(t)
	buf := swapNoteWarner(t)
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	emit.FlushCoverageNotes()
	out := buf.String()
	if !strings.Contains(out, "agents reach aider only via outputs.aider.rules-file") {
		t.Errorf("expected agent coverage note, got: %s", out)
	}
	if !strings.Contains(out, "skills reach aider only via outputs.aider.rules-file") {
		t.Errorf("expected skill coverage note, got: %s", out)
	}
}

func TestEmit_NoNotesWhenRulesFileSet(t *testing.T) {
	testutil.TempCwd(t)
	buf := swapNoteWarner(t)
	cfg := &config.Config{Outputs: map[string]config.Output{
		"aider": {RulesFile: "CONVENTIONS.md"},
	}}
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	emit.FlushCoverageNotes()
	if strings.Contains(buf.String(), "reach aider") {
		t.Errorf("rules-file set must suppress the notes, got: %s", buf.String())
	}
}

func TestEmit_NoNotesWhenNoAgentsOrSkills(t *testing.T) {
	testutil.TempCwd(t)
	buf := swapNoteWarner(t)
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "x"},
	})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if got := emit.PendingCoverageNotesCount(); got != 0 {
		t.Errorf("no agent/skill specs must buffer no note, count=%d", got)
	}
	emit.FlushCoverageNotes()
	if buf.Len() != 0 {
		t.Errorf("expected no output, got: %s", buf.String())
	}
}
