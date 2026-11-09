package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// defaultShortcut opens the list from anywhere until the person picks
// another. Almost nothing else uses it; a global shortcut takes its keys
// away from every other app.
const defaultShortcut = "Ctrl+Alt+Space"

// settings is what the widget remembers between runs.
type settings struct {
	// Bubble is where the person left the bubble; nil before they moved it.
	Bubble *point `json:"bubble,omitempty"`
	// Shortcut is the keys that open the list from anywhere, as Wails
	// writes them, such as "Ctrl+Alt+Space". Empty means defaultShortcut.
	Shortcut string `json:"shortcut,omitempty"`
}

// shortcut is the saved shortcut, or the default before one was saved.
func (s settings) shortcut() string {
	if s.Shortcut == "" {
		return defaultShortcut
	}
	return s.Shortcut
}

type point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// settingsPath is the widget's file in the user's configuration folder,
// beside the conversation marks.
func settingsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate the widget settings: %w", err)
	}
	return filepath.Join(dir, "agnostic-ai", "widget.json"), nil
}

// readSettings loads the file; a missing one is empty settings.
func readSettings(path string) (settings, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return settings{}, nil
	}
	if err != nil {
		return settings{}, fmt.Errorf("read the widget settings: %w", err)
	}
	var s settings
	if err := json.Unmarshal(raw, &s); err != nil {
		return settings{}, fmt.Errorf("widget settings %s: %w", path, err)
	}
	return s, nil
}

// writeSettings replaces the file in one step.
func writeSettings(path string, s settings) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("write the widget settings: %w", err)
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("write the widget settings: %w", err)
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("write the widget settings: %w", err)
	}
	if err := os.Rename(temp, path); err != nil {
		return fmt.Errorf("write the widget settings: %w", err)
	}
	return nil
}
