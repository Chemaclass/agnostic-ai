package emit

import (
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestPendingCoverageNotes_OrderDoesNotDependOnAppendOrder(t *testing.T) {
	ResetCoverageNotes()
	t.Cleanup(ResetCoverageNotes)
	NoteSurfaceGap("cursor", spec.KindHook, 1, "Cursor cloud agent", "reason")
	NoteProject("zeta overlaps")
	NoteSurfaceGap("copilot", spec.KindHook, 1, "Copilot cloud agent", "reason")
	NoteProject("alpha overlaps")

	OrderBufferedDropsByTarget([]string{"copilot", "cursor"})
	got := PendingCoverageNotes()

	want := []string{"copilot", "cursor", projectNoteTarget, projectNoteTarget}
	if len(got) != len(want) {
		t.Fatalf("notes = %+v, want %d", got, len(want))
	}
	for i, target := range want {
		if got[i].Target != target {
			t.Errorf("notes[%d].Target = %q, want %q", i, got[i].Target, target)
		}
	}
	if got[2].Message != "alpha overlaps" || got[3].Message != "zeta overlaps" {
		t.Errorf("project notes = %q, %q; want sorted", got[2].Message, got[3].Message)
	}
	if again := PendingCoverageNotes(); len(again) != len(got) {
		t.Errorf("reading must not clear the buffer: got %d, then %d", len(got), len(again))
	}
}
