package augment

import (
	"encoding/json"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// SettingsFilePath is the settings file sync writes Augment's hooks to.
func SettingsFilePath(cfg *config.Config) string {
	return emit.OutputMCPFile(cfg, target, defaultSettingsFile)
}

// HookDoc renders `{"hooks": ...}` as sync writes it for h alone, or nil
// when h produces no handler.
func HookDoc(h spec.Entry) ([]byte, error) {
	block := buildHooksBlock([]spec.Entry{h})
	if block == nil {
		return nil, nil
	}
	return json.Marshal(map[string]any{hooksKey: block})
}
