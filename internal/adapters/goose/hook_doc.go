package goose

import (
	"encoding/json"
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// HooksFilePath is the plugin hooks file sync writes Goose's hooks to.
func HooksFilePath(cfg *config.Config) string {
	return emit.OutputHooksFile(cfg, target, defaultHooksFile)
}

// PluginRoot is the plugin directory Goose sets PLUGIN_ROOT to.
func PluginRoot(cfg *config.Config) string {
	return filepath.ToSlash(filepath.Dir(filepath.Dir(HooksFilePath(cfg))))
}

// HookDoc renders the hooks file sync writes for h alone, or nil when h
// produces no handler.
func HookDoc(cfg *config.Config, h spec.Entry) ([]byte, error) {
	doc := buildHooks([]spec.Entry{h}, filepath.ToSlash(filepath.Dir(HooksFilePath(cfg))))
	if doc == nil {
		return nil, nil
	}
	return json.Marshal(doc)
}
