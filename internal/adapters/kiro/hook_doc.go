package kiro

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// HookFilePath is the file sync writes the hook spec named name to.
func HookFilePath(cfg *config.Config, name string) string {
	return filepath.Join(emit.OutputHooksDir(cfg, target, defaultHooksDir), name+".json")
}

// HookDoc renders the hook file sync writes for h, or nil when h
// produces no entry.
func HookDoc(h spec.Entry) ([]byte, error) {
	entries, err := buildHookEntries(h)
	if err != nil {
		return nil, fmt.Errorf("hook %s: %w", h.Name, err)
	}
	if len(entries) == 0 {
		return nil, nil
	}
	return json.Marshal(hooksFile{Version: "v1", Hooks: entries})
}
