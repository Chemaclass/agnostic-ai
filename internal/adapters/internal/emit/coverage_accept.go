package emit

import (
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// AcceptedNote is a buffered coverage note that a coverage.accept entry
// took out of the sync output.
type AcceptedNote struct {
	// Text is the note sentence sync would have printed, without the
	// target's own reason.
	Text string
	// Reason is the accept entry's reason.
	Reason string
}

// AcceptCoverageNotes takes every buffered coverage note an entry of
// accept matches out of the buffers, so it no longer prints, feeds the
// digest, or fails on-unsupported: error. An entry with a field matches
// the target's field no-op notes for that field; one without matches
// the target's gap and surface notes for the kind. Project notes name no
// target and stay. It returns one AcceptedNote per distinct note, and
// the entries that matched none.
func AcceptCoverageNotes(accept []config.CoverageAccept) (accepted []AcceptedNote, unmatched []config.CoverageAccept) {
	if len(accept) == 0 {
		return nil, nil
	}
	coverageNoteState.mu.Lock()
	defer coverageNoteState.mu.Unlock()
	pruneFieldNotesLocked()
	// The prune is final here. Run again once a note is accepted, it would
	// read the target as having noted nothing, and drop another target's
	// note on the same field.
	coverageNoteState.environmentFields = nil
	used := make([]bool, len(accept))
	match := func(target string, kind spec.Kind, field string) (string, bool) {
		for i, a := range accept {
			if a.Target == target && spec.Kind(a.SpecKind()) == kind && a.Field == field {
				used[i] = true
				return a.Reason, true
			}
		}
		return "", false
	}
	seen := map[string]bool{}
	add := func(text, reason string) {
		if !seen[text] {
			seen[text] = true
			accepted = append(accepted, AcceptedNote{Text: text, Reason: reason})
		}
	}

	gaps := coverageNoteState.pending[:0]
	for _, p := range coverageNoteState.pending {
		if reason, ok := match(p.target, p.kind, ""); ok {
			add(gapNoteText(p.kind, p.count, p.via, []string{p.target}), reason)
			continue
		}
		gaps = append(gaps, p)
	}
	coverageNoteState.pending = gaps

	fields := coverageNoteState.pendingField[:0]
	for _, p := range coverageNoteState.pendingField {
		if reason, ok := match(p.target, p.kind, p.field); ok {
			add(fieldNoteText(p.kind, p.field, p.count, []string{p.target}), reason)
			continue
		}
		fields = append(fields, p)
	}
	coverageNoteState.pendingField = fields

	surfaces := coverageNoteState.pendingSurface[:0]
	for _, p := range coverageNoteState.pendingSurface {
		if reason, ok := match(p.target, p.kind, ""); ok {
			add(surfaceNoteText(p.kind, p.count, p.surface, []string{p.target}), reason)
			continue
		}
		surfaces = append(surfaces, p)
	}
	coverageNoteState.pendingSurface = surfaces

	for i, a := range accept {
		if !used[i] {
			unmatched = append(unmatched, a)
		}
	}
	return accepted, unmatched
}

// PrintAcceptedNotes prints one line per accepted note with the reason
// the project gave for it.
func PrintAcceptedNotes(accepted []AcceptedNote) {
	for _, a := range accepted {
		_, _ = fmt.Fprintf(Warner, "  accepted: %s (reason: %s)\n", a.Text, a.Reason)
	}
}

// PendingTargetCoverageNotesCount returns how many distinct coverage
// gap, field no-op, and surface gap notes are buffered. Project notes
// are left out: they name no target, so no coverage.accept entry can
// match them.
func PendingTargetCoverageNotesCount() int {
	coverageNoteState.mu.Lock()
	defer coverageNoteState.mu.Unlock()
	pruneFieldNotesLocked()
	return len(targetNoteKeysLocked())
}
