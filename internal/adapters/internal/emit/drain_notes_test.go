package emit

import (
	"reflect"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestDrainNotes_ReturnsEveryBufferedShapeAndClears(t *testing.T) {
	buf := swapWarnerForNotes(t)
	ResetCapabilityWarnings()
	t.Cleanup(ResetCapabilityWarnings)

	caps := Capabilities{Target: "zed", Supports: []spec.Kind{spec.KindRule}}
	b := spec.Bundle{Hooks: []spec.Entry{{Kind: spec.KindHook, Name: "h"}}}
	if err := ReportUnsupported(caps, b, OnUnsupportedWarn); err != nil {
		t.Fatal(err)
	}
	NoteCoverageGap("aider", spec.KindAgent, 1, "outputs.aider.rules-file")
	NoteFieldNoOp("cursor", spec.KindAgent, "tools", 1, "no tools field")
	NoteSurfaceGap("cline", spec.KindAgent, 1, "the Cline VS Code extension", "CLI only")
	NoteProject("project-wide text")

	got := DrainNotes()
	want := []Note{
		{Shape: NoteUnsupportedKind, Target: "zed", Kind: spec.KindHook},
		{Shape: NoteGap, Target: "aider", Kind: spec.KindAgent, Reason: "outputs.aider.rules-file"},
		{Shape: NoteField, Target: "cursor", Kind: spec.KindAgent, Field: "tools", Reason: "no tools field"},
		{Shape: NoteSurface, Target: "cline", Kind: spec.KindAgent, Surface: "the Cline VS Code extension", Reason: "CLI only"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DrainNotes() =\n%#v\nwant\n%#v", got, want)
	}
	if n := PendingCoverageNotesCount() + PendingCapabilityWarningsCount(); n != 0 {
		t.Errorf("drain must clear every buffer, %d still pending", n)
	}
	FlushCoverageNotes()
	FlushCapabilityWarnings()
	if buf.Len() != 0 {
		t.Errorf("drained notes must not print on a later flush: %q", buf)
	}
}

func TestSetAsideNotes_DropsCaptureNotesAndRestoresTheRest(t *testing.T) {
	buf := swapWarnerForNotes(t)
	ResetCapabilityWarnings()
	t.Cleanup(ResetCapabilityWarnings)
	NoteCoverageGap("aider", spec.KindAgent, 1, "outputs.aider.rules-file")

	restore := SetAsideNotes()
	NoteCoverageGap("aider", spec.KindAgent, 1, "outputs.aider.rules-file")
	NoteProject("capture text")
	_, _ = Warner.Write([]byte("direct warning\n"))
	restore()

	want := []Note{{Shape: NoteGap, Target: "aider", Kind: spec.KindAgent, Reason: "outputs.aider.rules-file"}}
	if got := DrainNotes(); !reflect.DeepEqual(got, want) {
		t.Errorf("DrainNotes() =\n%#v\nwant\n%#v", got, want)
	}
	if buf.Len() != 0 {
		t.Errorf("a set-aside capture must not print: %q", buf)
	}
}

func TestDrainNotes_EmptyBuffersReturnNil(t *testing.T) {
	swapWarnerForNotes(t)
	ResetCapabilityWarnings()
	if got := DrainNotes(); got != nil {
		t.Errorf("DrainNotes() = %#v, want nil", got)
	}
}

func TestSkipNotes_RecordsNothingUntilRestored(t *testing.T) {
	swapWarnerForNotes(t)
	ResetCapabilityWarnings()
	t.Cleanup(ResetCapabilityWarnings)
	skill := spec.Entry{Kind: spec.KindSkill, Name: "s", Meta: map[string]any{"name": "s", "description": "d", "license": "MIT"}}
	plain := SkillFieldCoverage{Markdown: func(spec.Entry) string { return "---\nname: s\ndescription: d\n---\nbody\n" }}
	raise := func() {
		NoteCoverageGap("aider", spec.KindAgent, 1, "outputs.aider.rules-file")
		NoteFieldNoOp("cursor", spec.KindAgent, "tools", 1, "no tools field")
		NoteSurfaceGap("cline", spec.KindAgent, 1, "the Cline VS Code extension", "CLI only")
		NoteProject("project-wide text")
		NoteDroppedSkillFields("cursor", []spec.Entry{skill}, plain)
		NoteEntryOmitted("aider", spec.KindAgent, "a")
	}

	restore := SkipNotes()
	raise()
	if got := DrainNotes(); got != nil {
		t.Errorf("skipped notes must not buffer, got %#v", got)
	}
	if !OmittedEntry("aider", spec.KindAgent, "a") {
		t.Error("the record of omitted entries must still update while notes are skipped")
	}

	restore()
	raise()
	got := DrainNotes()
	if len(got) != 4 || got[2].Field != "license" {
		t.Errorf("notes after restore = %#v, want the gap, both field no-ops, and the surface gap", got)
	}
}
