package warp

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

func TestEmit_NotesAgentGapWhenWorkflowsDirUnset(t *testing.T) {
	testutil.TempCwd(t)
	buf := swapNoteWarner(t)
	if err := New().Emit(kitSinkBundle(), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	emit.FlushCoverageNotes()
	out := buf.String()
	if !strings.Contains(out, "agents reach warp only via outputs.warp.workflows-dir") {
		t.Errorf("expected agent coverage note, got: %s", out)
	}
	if !strings.Contains(out, "skills reach warp only in the source dir (no native skill surface)") {
		t.Errorf("expected skill source-dir-only note, got: %s", out)
	}
}

func TestEmit_NoAgentNoteWhenWorkflowsDirSet(t *testing.T) {
	testutil.TempCwd(t)
	buf := swapNoteWarner(t)
	cfg := &config.Config{Outputs: map[string]config.Output{
		"warp": {WorkflowsDir: ".warp/workflows"},
	}}
	if err := New().Emit(kitSinkBundle(), cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	emit.FlushCoverageNotes()
	out := buf.String()
	if strings.Contains(out, "agents reach warp") {
		t.Errorf("workflows-dir set must suppress the agent note, got: %s", out)
	}
	// Skills still never reach warp; the source-dir-only note stays.
	if !strings.Contains(out, "skills reach warp only in the source dir (no native skill surface)") {
		t.Errorf("expected skill source-dir-only note even with workflows-dir set, got: %s", out)
	}
}

func TestEmit_NoNotesWhenNoAgentsOrSkills(t *testing.T) {
	testutil.TempCwd(t)
	buf := swapNoteWarner(t)
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "x"},
	})
	if err := New().Emit(b, &config.Config{}, false); err != nil {
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
