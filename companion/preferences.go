package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// preferences is what the list's settings menu shows.
type preferences struct {
	// Shortcut is the keys that open the list from anywhere.
	Shortcut string `json:"shortcut"`
	// ShortcutProblem says why the shortcut is not active, such as
	// another app holding the same keys; empty when it works.
	ShortcutProblem string `json:"shortcutProblem,omitempty"`
	StartAtLogin    bool   `json:"startAtLogin"`
	// StartAtLoginProblem says why the login setting could not be read.
	StartAtLoginProblem string `json:"startAtLoginProblem,omitempty"`
}

// preferences reads the current settings; the login setting comes from
// the system, which owns it.
func (s *shell) preferences() preferences {
	s.mu.Lock()
	prefs := preferences{Shortcut: s.settings.shortcut(), ShortcutProblem: s.shortcutProblem}
	s.mu.Unlock()
	enabled, err := s.login.IsEnabled()
	if err != nil {
		prefs.StartAtLoginProblem = err.Error()
		return prefs
	}
	prefs.StartAtLogin = enabled
	return prefs
}

// autostart starts the companion when the person logs in; the system keeps it.
type autostart interface {
	Enable() error
	Disable() error
	IsEnabled() (bool, error)
}

// changeStartAtLogin turns starting at login on or off, then shows the
// value in effect in both menus.
func (s *shell) changeStartAtLogin(on bool) error {
	change := s.login.Disable
	if on {
		change = s.login.Enable
	}
	err := change()
	s.preferencesChanged()
	if err != nil {
		return fmt.Errorf("change start at login: %w", err)
	}
	return nil
}

// preferencesChanged tells the list and the right-click menu the
// preferences in effect.
func (s *shell) preferencesChanged() {
	s.tellPreferences(s.preferences())
	s.refreshNative()
}

// shortcuts takes keys away from every other app so they open the
// list, and gives them back. The window system refuses keys another
// app already holds.
type shortcuts interface {
	Register(keys string, pressed func()) error
	Unregister(keys string) error
}

// registerShortcut makes the saved shortcut open the list. A shortcut
// the system refuses is kept in the settings and its reason shown, so
// the person can pick another or try the same keys again later.
func (s *shell) registerShortcut() {
	s.mu.Lock()
	keys := s.settings.shortcut()
	s.mu.Unlock()
	err := s.shortcuts.Register(keys, s.togglePanel)
	s.mu.Lock()
	s.shortcutProblem = ""
	if err != nil {
		s.shortcutProblem = fmt.Sprintf("%s is not active: %v", keys, err)
	}
	s.mu.Unlock()
	s.refreshNative()
}

// changeShortcut swaps the shortcut for keys. The new keys are taken
// first, so a refused shortcut leaves the old one working. Keys whose
// last registration was refused are not held, so choosing them again
// tries again: the problem clears only once the system takes them.
func (s *shell) changeShortcut(keys string) error {
	s.mu.Lock()
	old := s.settings.shortcut()
	oldIsHeld := s.shortcutProblem == ""
	s.mu.Unlock()
	if keys == old && oldIsHeld {
		return nil
	}
	if err := s.shortcuts.Register(keys, s.togglePanel); err != nil {
		return fmt.Errorf("%s cannot be used: %w", keys, err)
	}
	if keys != old && oldIsHeld {
		if err := s.shortcuts.Unregister(old); err != nil {
			return fmt.Errorf("release %s: %w", old, err)
		}
	}
	s.mu.Lock()
	s.shortcutProblem = ""
	s.mu.Unlock()
	return s.changeSettings(func(saved *settings) { saved.Shortcut = keys })
}

func readBody[T any](w http.ResponseWriter, r *http.Request, shape string) (T, bool) {
	var body T
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "send " + shape})
		return body, false
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "send one JSON object"})
		return body, false
	}
	return body, true
}

// getPreferences answers GET /api/preferences.
// The list asks each time its menu opens, which also catches a login
// change made in the system's settings for the right-click menu.
func (s *shell) getPreferences(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.preferences())
	s.refreshNative()
}

// setShortcut answers POST /api/preferences/shortcut.
func (s *shell) setShortcut(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody[struct {
		Shortcut string `json:"shortcut"`
	}](w, r, "{shortcut}")
	if !ok {
		return
	}
	if body.Shortcut == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "send {shortcut}"})
		return
	}
	if err := s.changeShortcut(body.Shortcut); err != nil {
		writeJSON(w, http.StatusConflict, apiError{Error: err.Error()})
		return
	}
	s.preferencesChanged()
	writeJSON(w, http.StatusOK, s.preferences())
}

// setStartAtLogin answers POST /api/preferences/start-at-login.
func (s *shell) setStartAtLogin(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody[struct {
		Enabled *bool `json:"enabled"`
	}](w, r, "{enabled}")
	if !ok {
		return
	}
	if body.Enabled == nil {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "send {enabled}"})
		return
	}
	if err := s.changeStartAtLogin(*body.Enabled); err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, s.preferences())
}
