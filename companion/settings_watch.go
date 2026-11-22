package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"time"
)

// stateFile sits beside the settings file. It holds what only this
// process knows, such as whether the system took a shortcut, so the
// web page can show those values read only while the companion runs.
const stateFile = "companion-state.json"

// companionState is what the companion writes to stateFile.
type companionState struct {
	PID int `json:"pid"`
	preferences
}

func statePath(settingsPath string) string {
	return filepath.Join(filepath.Dir(settingsPath), stateFile)
}

// watchSettings writes the state file, then every pollEvery takes a
// change another program made to the settings file, such as the web
// page, until ctx ends.
func (s *shell) watchSettings(ctx context.Context) {
	s.writeState()
	ticker := time.NewTicker(pollEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.takeOutsideSettings()
			s.touchState()
		}
	}
}

// takeOutsideSettings applies the switches the file holds when they
// differ from the companion's copy. Shortcuts are not changed from
// outside: the system must take the keys first, so the file gets the
// keys in use back.
func (s *shell) takeOutsideSettings() {
	s.mu.Lock()
	// Own writes hold mu through the save, so the file read here is never
	// older than the companion's copy. A missing file is not a change: the
	// next save writes it again.
	read, _, found, err := readSettingsFields(s.settingsPath)
	if err != nil || !found {
		s.mu.Unlock()
		return
	}
	held := s.settings
	looks := read.animate() != held.animate() || read.DimWhenIdle != held.DimWhenIdle || read.SmallCompanion != held.SmallCompanion
	resized := read.SmallCompanion != held.SmallCompanion
	updates := read.checksForUpdates() != held.checksForUpdates()
	shortcuts := read.shortcut() != held.shortcut() || read.sendShortcut() != held.sendShortcut()
	if !looks && !updates && !shortcuts {
		s.mu.Unlock()
		return
	}
	s.settings.Animate, s.settings.DimWhenIdle, s.settings.SmallCompanion, s.settings.CheckForUpdates = read.Animate, read.DimWhenIdle, read.SmallCompanion, read.CheckForUpdates
	if shortcuts {
		if err := saveSettingsChange(s.settingsPath, s.settings, func(saved *settings) {
			saved.Shortcut, saved.SendShortcut = held.Shortcut, held.SendShortcut
		}); err != nil {
			log.Printf("put the shortcuts in use back in the settings: %v", err)
		}
	}
	appearance := s.settings.appearance()
	appearance.Presence = s.presenceLocked()
	checks := s.settings.checksForUpdates()
	s.mu.Unlock()
	if looks {
		s.tell(appearance)
		s.refreshNative()
	}
	if resized {
		s.fitBubble()
	}
	if updates {
		s.preferencesChanged()
		if checks && s.appCommands != nil {
			go s.appCommands.checkUpdates(context.Background())
		}
	}
}

// writeState saves the preferences in effect and this process's id.
func (s *shell) writeState() {
	if s.settingsPath == "" {
		return
	}
	s.writingState.Lock()
	defer s.writingState.Unlock()
	if err := writeCompanionState(statePath(s.settingsPath), companionState{PID: os.Getpid(), preferences: s.preferences()}); err != nil {
		log.Print(err)
	}
}

// touchState marks the state file fresh. agnostic-ai serve treats one
// untouched for 30 seconds as left by a companion that is gone, even when
// another process took its id.
func (s *shell) touchState() {
	if s.settingsPath == "" {
		return
	}
	now := time.Now()
	s.writingState.Lock()
	err := os.Chtimes(statePath(s.settingsPath), now, now)
	s.writingState.Unlock()
	if err != nil {
		s.writeState()
	}
}

func writeCompanionState(path string, state companionState) error {
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("write the companion state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("write the companion state: %w", err)
	}
	temp := path + ".tmp"
	if err := writeSynced(temp, append(raw, '\n')); err != nil {
		return fmt.Errorf("write the companion state: %w", err)
	}
	if err := os.Rename(temp, path); err != nil {
		return fmt.Errorf("write the companion state: %w", err)
	}
	return nil
}

// removeState removes the state file as the companion quits, so the web
// page stops showing values only a running companion knows.
func (s *shell) removeState() {
	if s.settingsPath == "" {
		return
	}
	s.writingState.Lock()
	defer s.writingState.Unlock()
	if err := os.Remove(statePath(s.settingsPath)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		log.Printf("remove the companion state: %v", err)
	}
}
