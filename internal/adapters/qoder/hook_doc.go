package qoder

import (
	"encoding/json"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// SettingsFilePath is the file sync merges Qoder's hooks into.
func SettingsFilePath(cfg *config.Config) string {
	return emit.OutputMCPFile(cfg, target, defaultMCPFile)
}

// HookDoc renders the `{"hooks": ...}` block sync merges into
// `.qoder/settings.json` for h alone, or nil when h produces no handler.
func HookDoc(h spec.Entry) ([]byte, error) {
	block := buildHooksBlock([]spec.Entry{h})
	if block == nil {
		return nil, nil
	}
	return json.Marshal(map[string]any{qoderHooksKey: block})
}
