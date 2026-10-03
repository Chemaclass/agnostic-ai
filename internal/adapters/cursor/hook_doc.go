package cursor

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// HooksFilePath is the file sync writes Cursor's hooks to.
func HooksFilePath(cfg *config.Config) string {
	return emit.OutputHooksFile(cfg, target, defaultHooksFile)
}

// HookDoc renders the hooks file sync writes for h alone, or nil when h
// produces no handler.
func HookDoc(h spec.Entry) ([]byte, error) {
	byEvent := buildHooks([]spec.Entry{h})
	if len(byEvent) == 0 {
		return nil, nil
	}
	return emit.MarshalJSONIndent(hooksDoc{Version: 1, Hooks: byEvent})
}
