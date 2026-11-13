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

// GlobalHooksFile renders the user-level plugin hooks file, or nil when no
// hook produces a handler. Commands reference scripts in scriptsDir.
func GlobalHooksFile(hooks []spec.Entry, scriptsDir string) ([]byte, error) {
	doc := buildHooksWith(hooks, func(command string, meta map[string]any) string {
		return emit.RewriteGlobalHookPath(command, target, scriptsDir, meta)
	}, func(path string) string {
		return emit.RewriteGlobalHookPath(path, target, scriptsDir)
	})
	if doc == nil {
		return nil, nil
	}
	body, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}

// GlobalPluginManifest renders the manifest for the user-level plugin
// named by the directory root.
func GlobalPluginManifest(root string) ([]byte, error) {
	body, err := json.MarshalIndent(map[string]string{
		"name":        filepath.Base(root),
		"version":     "1.0.0",
		"description": "Managed by agnostic-ai",
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}
