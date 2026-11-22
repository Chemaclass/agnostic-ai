package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"time"
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
	// CheckForUpdates is whether the companion checks for updates on its own; nil means it does.
	CheckForUpdates *bool `json:"checkForUpdates,omitempty"`
	// KeptShells are the stuck shells the person chose to keep running for now.
	KeptShells []keptShell `json:"keptShells,omitempty"`
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

func (s settings) checksForUpdates() bool {
	return s.CheckForUpdates == nil || *s.CheckForUpdates
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

// webviewDataPath is where the Windows web view keeps its data: inside
// the companion folder that companion uninstall removes, given the
// settings file beside it.
func webviewDataPath(settingsPath string) string {
	return filepath.Join(filepath.Dir(settingsPath), "companion", "webview")
}

// readSettings loads the file; a missing one is empty settings. A value
// of the wrong type is skipped and the rest kept; a file that is not
// JSON at all is an error.
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
		if _, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
			log.Printf("companion settings %s: %v; kept the other settings", path, err)
			return fieldsThatParse(raw), nil
		}
		return settings{}, &unreadableSettings{path: path, cause: err}
	}
	return s, nil
}

// fieldsThatParse reads each top-level field of raw on its own and keeps
// those that parse. A field that fails leaves its default: decoding the
// whole file would leave a pointer such as Bubble half filled.
func fieldsThatParse(raw []byte) settings {
	var s settings
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return s
	}
	for name, value := range fields {
		one, err := json.Marshal(map[string]json.RawMessage{name: value})
		if err != nil {
			continue
		}
		var probe settings
		if json.Unmarshal(one, &probe) != nil {
			continue
		}
		_ = json.Unmarshal(one, &s)
	}
	return s
}

// unreadableSettings is a settings file that is not JSON, such as one
// cut short.
type unreadableSettings struct {
	path  string
	cause error
}

func (e *unreadableSettings) Error() string {
	return fmt.Sprintf("companion settings %s: %v", e.path, e.cause)
}

func (e *unreadableSettings) Unwrap() error { return e.cause }

// loadSettings is readSettings for a companion that starts: a file that
// is not JSON moves aside, to companion.json.bad or, when that is taken,
// a name with the time, and the companion starts with empty settings.
func loadSettings(path string) (settings, error) {
	s, err := readSettings(path)
	if _, ok := errors.AsType[*unreadableSettings](err); !ok {
		return s, err
	}
	aside := path + ".bad"
	if _, statErr := os.Lstat(aside); statErr == nil {
		aside = path + ".bad-" + time.Now().Format("20060102T150405.000000000")
	}
	if moveErr := os.Rename(path, aside); moveErr != nil {
		log.Printf("%v; could not move it aside: %v", err, moveErr)
	} else {
		log.Printf("%v; moved it to %s and started with the defaults", err, aside)
	}
	return settings{}, nil
}

// settingsLockWait bounds how long a save waits for another writer of
// the settings file, which holds the lock for one read and one write.
const settingsLockWait = 2 * time.Second

// lockSettings holds the lock file beside the settings file. Every
// writer of the file, the companion and agnostic-ai serve, takes it.
func lockSettings(path string) (release func(), err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("lock the companion settings: %w", err)
	}
	file, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("lock the companion settings: %w", err)
	}
	deadline := time.Now().Add(settingsLockWait)
	for {
		err := tryLockFile(file)
		if err == nil {
			return func() { _ = file.Close() }, nil
		}
		if !lockHeldElsewhere(err) || time.Now().After(deadline) {
			_ = file.Close()
			return nil, fmt.Errorf("lock the companion settings: %w", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// saveSettingsChange applies change to the settings file under its lock.
// It reads the file again first, so a value another process wrote since
// stays. A file that is missing or not JSON starts from base, the
// companion's own copy before the change.
func saveSettingsChange(path string, base settings, change func(*settings)) error {
	release, err := lockSettings(path)
	if err != nil {
		return err
	}
	defer release()
	saved := base
	if _, statErr := os.Stat(path); statErr == nil {
		read, err := readSettings(path)
		_, unreadable := errors.AsType[*unreadableSettings](err)
		switch {
		case err == nil:
			saved = read
		case !unreadable:
			return err
		}
	}
	change(&saved)
	return writeSettings(path, saved)
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
