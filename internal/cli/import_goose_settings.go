package cli

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

// goosePluginSettings is the plugin switch list of one Goose settings
// file (`enabledPlugins` / `disabledPlugins`).
type goosePluginSettings struct {
	Enabled  []string `json:"enabledPlugins"`
	Disabled []string `json:"disabledPlugins"`
}

// gooseSettingsLayers reads the Goose settings files that decide which
// plugins load, highest precedence first: project-local, project, user
// (`crates/goose/src/plugins/discovery.rs`, `is_enabled`). An absent or
// unreadable file is skipped, as Goose skips it with a warning.
func gooseSettingsLayers(root string) []goosePluginSettings {
	paths := []string{
		filepath.Join(root, ".config", "goose", "settings.local.json"),
		filepath.Join(root, ".config", "goose", "settings.json"),
	}
	if user := gooseUserSettingsPath(); user != "" {
		paths = append(paths, user)
	}
	var layers []goosePluginSettings
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		var s goosePluginSettings
		if err != nil || json.Unmarshal(data, &s) != nil {
			keptf("skipping unreadable Goose settings %s\n", p)
			continue
		}
		layers = append(layers, s)
	}
	return layers
}

// gooseUserSettingsPath honors `GOOSE_PATH_ROOT` when absolute, like
// Goose, and falls back to the home directory.
func gooseUserSettingsPath() string {
	base := os.Getenv("GOOSE_PATH_ROOT")
	if !filepath.IsAbs(base) {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = home
	}
	return filepath.Join(base, ".config", "goose", "settings.json")
}

// gooseEnabled applies Goose's rule: the first layer that names the
// plugin decides, a disabled listing beating an enabled one in the same
// layer; a plugin no layer names is enabled.
func gooseEnabled(layers []goosePluginSettings, plugin string) bool {
	for _, l := range layers {
		if slices.Contains(l.Disabled, plugin) {
			return false
		}
		if slices.Contains(l.Enabled, plugin) {
			return true
		}
	}
	return true
}
