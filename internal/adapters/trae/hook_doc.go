package trae

import (
	"encoding/json"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// HooksFilePath is the file sync writes Trae's hooks to.
func HooksFilePath(cfg *config.Config) string {
	return emit.OutputHooksFile(cfg, target, defaultHooksFile)
}

// HookDoc renders the hooks file sync writes for h alone, or nil when h
// produces no handler.
func HookDoc(h spec.Entry) ([]byte, error) {
	doc := buildHooks([]spec.Entry{h})
	if doc == nil {
		return nil, nil
	}
	return json.Marshal(doc)
}
