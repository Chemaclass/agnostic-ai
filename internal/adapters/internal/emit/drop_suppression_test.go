package emit

import (
	"bytes"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestSuppressKnownCoverageNotesKeepsNewNotesAndFixes(t *testing.T) {
	ResetCoverageNotes()
	t.Cleanup(ResetCoverageNotes)
	NoteFieldNoOp("codex", spec.KindAgent, "can", 1, "unsupported")
	prior := PendingCoverageNotes()
	known := map[string]bool{DropRecordFingerprint(prior[0]): true}
	NoteFieldNoOp("codex", spec.KindSkill, "argument-hint", 1, "no field")
	coverageNoteState.pendingField[len(coverageNoteState.pendingField)-1].suggestion = "add guidance"
	if n := SuppressKnownCoverageNotes(known); n != 1 {
		t.Errorf("suppressed %d notes, want 1", n)
	}
	var out bytes.Buffer
	prev := Warner
	Warner = &out
	t.Cleanup(func() { Warner = prev })
	FlushCoverageNotes()
	if strings.Contains(out.String(), "`can`") || !strings.Contains(out.String(), "`argument-hint`") || !strings.Contains(out.String(), "fix:") {
		t.Errorf("unexpected output: %s", out.String())
	}
}

func TestSuppressKnownWarningsKeepsChangedCount(t *testing.T) {
	ResetCapabilityWarnings()
	t.Cleanup(ResetCapabilityWarnings)
	capabilityWarnState.pending = []pendingWarn{{target: "codex", kind: spec.KindSkill, count: 1}}
	known := map[string]bool{DropRecordFingerprint(PendingCapabilityWarnings()[0]): true}
	capabilityWarnState.pending = append(capabilityWarnState.pending, pendingWarn{target: "codex", kind: spec.KindSkill, count: 1})
	capabilityWarnState.pending = append(capabilityWarnState.pending, pendingWarn{target: "claude", kind: spec.KindAgent, count: 2})
	if n := SuppressKnownCapabilityWarnings(known); n != 1 {
		t.Errorf("suppressed %d warnings, want 1", n)
	}
	got := PendingCapabilityWarnings()
	if len(got) != 1 || got[0].Target != "claude" {
		t.Errorf("unexpected remaining warnings: %+v", got)
	}
	capabilityWarnState.pending = []pendingWarn{{target: "codex", kind: spec.KindSkill, count: 2}}
	if n := SuppressKnownCapabilityWarnings(known); n != 0 || len(PendingCapabilityWarnings()) != 1 {
		t.Error("a changed count was suppressed")
	}
}

func TestSuppressKnownCoverageNotesUsesSameRecordsAsSnapshot(t *testing.T) {
	ResetCoverageNotes()
	t.Cleanup(ResetCoverageNotes)
	NoteCoverageGap("zed", spec.KindAgent, 3, "outputs.zed.rules-file")
	NoteCoverageGap("zed", spec.KindAgent, 3, "outputs.zed.rules-file")
	NoteFieldNoOp("codex", spec.KindAgent, "can", 2, "no allowlist")
	NoteSurfaceGap("copilot", spec.KindHook, 4, "cloud", "not read there")
	NoteProject("project warning")
	records := PendingCoverageNotes()
	known := make(map[string]bool, len(records))
	for _, r := range records {
		known[DropRecordFingerprint(r)] = true
	}
	if got := SuppressKnownCoverageNotes(known); got != len(records) {
		t.Errorf("suppressed %d, want %d", got, len(records))
	}
	if got := PendingCoverageNotes(); len(got) != 0 {
		t.Errorf("records remained: %+v", got)
	}
}
