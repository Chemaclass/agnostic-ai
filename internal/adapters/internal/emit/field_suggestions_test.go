package emit

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestFieldSuggestions_KeepGroupedNotesAndCounts(t *testing.T) {
	buf := swapWarner(t)
	ResetCoverageNotes()
	t.Cleanup(ResetCoverageNotes)
	for _, target := range []string{"codex", "gemini", "codex"} {
		NoteFieldNoOp(target, spec.KindSkill, "argument-hint", 2, "the skill file has no argument-hint field")
	}
	digest := CoverageNotesDigest()
	if records := PendingCoverageNotes(); len(records) != 2 || records[0].Count != 2 || records[0].Suggestion == "" {
		t.Errorf("suggestions must preserve records and spec counts: %+v", records)
	}
	if PendingCoverageNotesCount() != 2 || CoverageNotesDigest() != digest {
		t.Error("reading suggestions changed the note count or digest")
	}
	FlushCoverageNotes()
	if strings.Count(buf.String(), "`argument-hint` on 2 skills has no effect on codex, gemini") != 1 || strings.Count(buf.String(), "fix:") != 1 {
		t.Errorf("want one grouped note and fix: %s", buf.String())
	}
	before := buf.String()
	FlushCoverageNotes()
	if buf.String() != before || PendingCoverageNotesCount() != 0 {
		t.Error("flush did not clear suggestions with their notes")
	}
}

func TestFieldSuggestions_OnlyKnownWarningShapes(t *testing.T) {
	for _, tc := range []struct {
		kind   spec.Kind
		field  string
		reason string
	}{
		{spec.KindAgent, "metadata", "the skill file has no metadata field"},
		{spec.KindSkill, "argument-hint", "another reason"},
		{spec.KindAgent, "sandbox_mode", "another sandbox restriction"},
		{spec.KindSettings, "readonly", "the agent file has no read-only mode"},
	} {
		if got := fieldNoteSuggestion(tc.kind, tc.field, tc.reason, false); got != "" {
			t.Errorf("unrelated shape got guidance: %+v: %s", tc, got)
		}
	}
}
