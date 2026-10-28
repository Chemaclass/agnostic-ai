package emit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// DropRecordFingerprint identifies the full user-visible diagnostic, including its fix.
func DropRecordFingerprint(record DropRecord) string {
	data, _ := json.Marshal(record)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// SuppressKnownCapabilityWarnings removes known warnings and returns their distinct count.
func SuppressKnownCapabilityWarnings(known map[string]bool) int {
	capabilityWarnState.mu.Lock()
	defer capabilityWarnState.mu.Unlock()
	removed := map[string]bool{}
	seen := map[string]bool{}
	kept := capabilityWarnState.pending[:0]
	for _, p := range capabilityWarnState.pending {
		identity := p.target + "\x00" + string(p.kind)
		if seen[identity] {
			continue
		}
		seen[identity] = true
		r := warningDropRecord(p)
		key := DropRecordFingerprint(r)
		if known[key] {
			removed[key] = true
		} else {
			kept = append(kept, p)
		}
	}
	capabilityWarnState.pending = kept
	return len(removed)
}

// SuppressKnownCoverageNotes removes known notes while retaining omitted-entry tracking.
func SuppressKnownCoverageNotes(known map[string]bool) int {
	coverageNoteState.mu.Lock()
	defer coverageNoteState.mu.Unlock()
	pruneFieldNotesLocked()
	// Pruning must use all target notes before selecting which ones to print.
	coverageNoteState.environmentFields = nil
	removed := map[string]bool{}
	isKnown := func(r DropRecord) bool {
		key := DropRecordFingerprint(r)
		if known[key] {
			removed[key] = true
			return true
		}
		return false
	}
	seen := map[string]bool{}
	gaps := coverageNoteState.pending[:0]
	for _, p := range coverageNoteState.pending {
		identity := "gap\x00" + p.target + "\x00" + string(p.kind) + "\x00" + p.via
		if seen[identity] {
			continue
		}
		seen[identity] = true
		r := gapDropRecord(p)
		if !isKnown(r) {
			gaps = append(gaps, p)
		}
	}
	coverageNoteState.pending = gaps
	fields := coverageNoteState.pendingField[:0]
	for _, p := range coverageNoteState.pendingField {
		identity := "field\x00" + p.target + "\x00" + string(p.kind) + "\x00" + p.field + "\x00" + p.reason
		if seen[identity] {
			continue
		}
		seen[identity] = true
		r := fieldDropRecord(p)
		if !isKnown(r) {
			fields = append(fields, p)
		}
	}
	coverageNoteState.pendingField = fields
	surfaces := coverageNoteState.pendingSurface[:0]
	for _, p := range coverageNoteState.pendingSurface {
		identity := "surface\x00" + p.target + "\x00" + string(p.kind) + "\x00" + p.surface + "\x00" + p.reason
		if seen[identity] {
			continue
		}
		seen[identity] = true
		r := surfaceDropRecord(p)
		if !isKnown(r) {
			surfaces = append(surfaces, p)
		}
	}
	coverageNoteState.pendingSurface = surfaces
	texts := coverageNoteState.pendingText[:0]
	for _, text := range coverageNoteState.pendingText {
		identity := "text\x00" + text
		if seen[identity] {
			continue
		}
		seen[identity] = true
		if !isKnown(DropRecord{Target: projectNoteTarget, Message: text}) {
			texts = append(texts, text)
		}
	}
	coverageNoteState.pendingText = texts
	return len(removed)
}
