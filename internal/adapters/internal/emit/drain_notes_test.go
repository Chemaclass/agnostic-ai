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

func TestDrainNotes_EmptyBuffersReturnNil(t *testing.T) {
	swapWarnerForNotes(t)
	ResetCapabilityWarnings()
	if got := DrainNotes(); got != nil {
		t.Errorf("DrainNotes() = %#v, want nil", got)
	}
}
