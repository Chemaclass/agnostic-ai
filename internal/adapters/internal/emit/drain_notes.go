package emit

import "github.com/chemaclass/agnostic-ai/internal/spec"

// NoteShape names which buffered report a Note came from.
type NoteShape string

const (
	// NoteUnsupportedKind is a capability warning: the target has no
	// native surface for the spec kind at all.
	NoteUnsupportedKind NoteShape = "unsupported-kind"
	// NoteGap is a coverage gap: the entry reaches the target only via
	// Reason (a config key or a source-dir-only explanation).
	NoteGap NoteShape = "gap"
	// NoteField is a field no-op: the entry emits, Field has no effect.
	NoteField NoteShape = "field"
	// NoteSurface is a surface gap: the entry emits and one of the
	// target's own surfaces ignores it.
	NoteSurface NoteShape = "surface"
)

// Note is one buffered capability warning or coverage note in
// structured form, for callers that attribute notes to a single spec
// instead of printing the grouped sync summary.
type Note struct {
	Shape   NoteShape
	Target  string
	Kind    spec.Kind
	Field   string
	Surface string
	Reason  string
}

// DrainNotes returns every buffered capability warning and coverage
// note in recording order, then clears all buffers so nothing prints
// on a later flush. Project-wide notes belong to no target and are
// discarded. Returns nil when nothing is pending.
func DrainNotes() []Note {
	var out []Note
	capabilityWarnState.mu.Lock()
	for _, p := range capabilityWarnState.pending {
		out = append(out, Note{Shape: NoteUnsupportedKind, Target: p.target, Kind: p.kind})
	}
	capabilityWarnState.pending = nil
	capabilityWarnState.mu.Unlock()

	coverageNoteState.mu.Lock()
	defer coverageNoteState.mu.Unlock()
	for _, p := range coverageNoteState.pending {
		out = append(out, Note{Shape: NoteGap, Target: p.target, Kind: p.kind, Reason: p.via})
	}
	for _, p := range coverageNoteState.pendingField {
		out = append(out, Note{Shape: NoteField, Target: p.target, Kind: p.kind, Field: p.field, Reason: p.reason})
	}
	for _, p := range coverageNoteState.pendingSurface {
		out = append(out, Note{Shape: NoteSurface, Target: p.target, Kind: p.kind, Surface: p.surface, Reason: p.reason})
	}
	coverageNoteState.pending = nil
	coverageNoteState.pendingField = nil
	coverageNoteState.pendingSurface = nil
	coverageNoteState.pendingText = nil
	return out
}
