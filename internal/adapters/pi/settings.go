package pi

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func emitSettings(sess *emit.Session, entries []spec.Entry, path string, dryRun bool) error {
	keys := map[string]any{}
	if model := emit.SettingsModel(entries, target); model != "" {
		keys["defaultModel"] = model
	}
	if effort := emit.SettingsEffortLevel(entries, target, (Adapter{}).SettingsEffortLevels()); effort != "" {
		keys["defaultThinkingLevel"] = effort
	}
	for _, entry := range entries {
		if _, ok := entry.Meta["permissions"]; ok {
			emit.NoteFieldNoOp(target, spec.KindSettings, "permissions", 1, "Pi has no native per-tool permission settings")
		}
	}
	emit.MergeSettingsCustomKeys(keys, entries, target, "defaultProjectTrust")
	if len(keys) == 0 {
		return nil
	}
	return sess.MergeJSONFile(path, keys, dryRun)
}
