package copilot

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// emitSettings merges the portable default model into Copilot CLI's
// repository settings. MergeJSONFile preserves native sibling keys such as
// respectGitignore while making the Settings spec authoritative for model.
func emitSettings(sess *emit.Session, settings []spec.Entry, dryRun bool) error {
	model := emit.LastSettingsModel(settings)
	if model == "" {
		return nil
	}
	return sess.MergeJSONFile(defaultSettingsFile, map[string]any{"model": model}, dryRun)
}
