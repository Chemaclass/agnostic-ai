package emit

import (
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// DropRecord is one buffered capability warning or coverage note for one
// target, with the sentence a plain sync prints for it.
type DropRecord struct {
	Target  string
	Kind    spec.Kind
	Count   int
	Message string
}

// projectNoteTarget names the source of a project-wide note, which
// belongs to no target.
const projectNoteTarget = "agnostic-ai"

// PendingCapabilityWarnings returns one record per buffered (target,
// kind) warning, in buffer order, without clearing the buffer.
func PendingCapabilityWarnings() []DropRecord {
	capabilityWarnState.mu.Lock()
	defer capabilityWarnState.mu.Unlock()
	var out []DropRecord
	seen := map[string]bool{}
	for _, p := range capabilityWarnState.pending {
		key := p.target + "\x00" + string(p.kind)
		if seen[key] {
			continue
		}
		seen[key] = true
		message := fmt.Sprintf("%d %s unsupported by %s", p.count, pluralizeKind(p.kind, p.count), p.target)
		out = append(out, DropRecord{Target: p.target, Kind: p.kind, Count: p.count, Message: message})
	}
	return out
}

// PendingCoverageNotes returns one record per buffered coverage note,
// deduplicated as the flush does, without clearing the buffers. A
// project-wide note carries no kind or count.
func PendingCoverageNotes() []DropRecord {
	coverageNoteState.mu.Lock()
	defer coverageNoteState.mu.Unlock()
	pruneFieldNotesLocked()
	var out []DropRecord
	seen := map[string]bool{}
	add := func(key string, r DropRecord) {
		if !seen[key] {
			seen[key] = true
			out = append(out, r)
		}
	}
	for _, p := range coverageNoteState.pending {
		add("gap\x00"+p.target+"\x00"+string(p.kind)+"\x00"+p.via, DropRecord{
			Target: p.target, Kind: p.kind, Count: p.count,
			Message: gapNoteText(p.kind, p.count, p.via, []string{p.target}),
		})
	}
	for _, p := range coverageNoteState.pendingField {
		add("field\x00"+p.target+"\x00"+string(p.kind)+"\x00"+p.field+"\x00"+p.reason, DropRecord{
			Target: p.target, Kind: p.kind, Count: p.count,
			Message: fmt.Sprintf("%s (%s)", fieldNoteText(p.kind, p.field, p.count, []string{p.target}), p.reason),
		})
	}
	for _, p := range coverageNoteState.pendingSurface {
		add("surface\x00"+p.target+"\x00"+string(p.kind)+"\x00"+p.surface+"\x00"+p.reason, DropRecord{
			Target: p.target, Kind: p.kind, Count: p.count,
			Message: fmt.Sprintf("%s (%s)", surfaceNoteText(p.kind, p.count, p.surface, []string{p.target}), p.reason),
		})
	}
	for _, text := range coverageNoteState.pendingText {
		add("text\x00"+text, DropRecord{Target: projectNoteTarget, Message: text})
	}
	return out
}
