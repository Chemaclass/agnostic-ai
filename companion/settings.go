package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
)

// defaultShortcut opens the list from anywhere until the person picks
// another. Almost nothing else uses it; a global shortcut takes its keys
// away from every other app.
const defaultShortcut = "Ctrl+Alt+Space"

// defaultSendShortcut opens the list to send the copied text, beside
// the system's own paste keys.
const defaultSendShortcut = "Ctrl+Alt+V"

// settings is what the companion remembers between runs.
type settings struct {
	// Bubble is where the person left the bubble; nil before they moved it.
	Bubble *point `json:"bubble,omitempty"`
	// Shortcut is the keys that open the list from anywhere, as Wails
	// writes them, such as "Ctrl+Alt+Space". Empty means defaultShortcut.
	Shortcut string `json:"shortcut,omitempty"`
	// SendShortcut is the keys that open the list to send the copied
	// text. Empty means defaultSendShortcut.
	SendShortcut string `json:"sendShortcut,omitempty"`
	// Animate is whether the circle and the list move; nil means they do.
	Animate *bool `json:"animate,omitempty"`
	// DimWhenIdle fades the circle while nothing needs the person.
	DimWhenIdle bool `json:"dimWhenIdle,omitempty"`
	// SmallCompanion shows a 40-point circle instead of the 60-point one.
	SmallCompanion bool `json:"smallCompanion,omitempty"`
}

// shortcut is the saved shortcut, or the default before one was saved.
func (s settings) shortcut() string {
	if s.Shortcut == "" {
		return defaultShortcut
	}
	return s.Shortcut
}

// sendShortcut is the saved send shortcut, or the default before one was saved.
func (s settings) sendShortcut() string {
	if s.SendShortcut == "" {
		return defaultSendShortcut
	}
	return s.SendShortcut
}

// bubbleSide is the bubble window's width and height for the saved circle size.
func (s settings) bubbleSide() int {
	return bubbleSideFor(s.SmallCompanion)
}

func (s settings) animate() bool {
	return s.Animate == nil || *s.Animate
}

type point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// settingsPath is the companion's file in the user's configuration folder,
// beside the conversation marks.
func settingsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate the companion settings: %w", err)
	}
	return filepath.Join(dir, "agnostic-ai", "companion.json"), nil
}

// readSettings loads the file; a missing one is empty settings. A file
// that does not parse is moved aside to companion.json.bad, so the
// companion still starts, with empty settings.
func readSettings(path string) (settings, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return settings{}, nil
	}
	if err != nil {
		return settings{}, fmt.Errorf("read the companion settings: %w", err)
	}
	var s settings
	if err := json.Unmarshal(raw, &s); err != nil {
		aside := path + ".bad"
		if moveErr := os.Rename(path, aside); moveErr != nil {
			log.Printf("companion settings %s: %v; could not move it aside: %v", path, err, moveErr)
		} else {
			log.Printf("companion settings %s: %v; moved it to %s and started with the defaults", path, err, aside)
		}
		return settings{}, nil
	}
	return s, nil
}

// writeSettings replaces the file in one step.
func writeSettings(path string, s settings) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("write the companion settings: %w", err)
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("write the companion settings: %w", err)
	}
	temp := path + ".tmp"
	if err := writeSynced(temp, append(raw, '\n')); err != nil {
		return fmt.Errorf("write the companion settings: %w", err)
	}
	if err := os.Rename(temp, path); err != nil {
		return fmt.Errorf("write the companion settings: %w", err)
	}
	return nil
}

// writeSynced writes the file and flushes it to disk, so a rename after
// it never leaves an empty file behind a power cut.
func writeSynced(path string, raw []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(raw); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
