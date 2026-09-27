package adapters

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// SettingsModel returns the model sync writes for target from the
// settings entries: the last one that resolves a string for it.
func SettingsModel(entries []spec.Entry, target string) string {
	return emit.SettingsModel(entries, target)
}

// EscapeTOMLBasic escapes s for a TOML basic string.
func EscapeTOMLBasic(s string) string { return emit.EscapeTOMLBasic(s) }

// NoteSettingsFieldNoOp buffers a coverage note that count settings
// specs carry field, which target does not write.
func NoteSettingsFieldNoOp(target, field string, count int, reason string) {
	emit.NoteFieldNoOp(target, spec.KindSettings, field, count, reason)
}
