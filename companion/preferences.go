package main

import (
	"context"
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
	// SendShortcut is the keys that open the list to send the copied text.
	SendShortcut        string `json:"sendShortcut"`
	SendShortcutProblem string `json:"sendShortcutProblem,omitempty"`
	StartAtLogin        bool   `json:"startAtLogin"`
	// StartAtLoginProblem says why the login setting could not be read.
	StartAtLoginProblem string `json:"startAtLoginProblem,omitempty"`
	CheckForUpdates     bool   `json:"checkForUpdates"`
}

// preferences reads the current settings; the login setting comes from
// the system, which owns it.
func (s *shell) preferences() preferences {
	s.mu.Lock()
	prefs := preferences{
		Shortcut:            s.settings.shortcut(),
		ShortcutProblem:     s.shortcutProblem,
		SendShortcut:        s.settings.sendShortcut(),
		SendShortcutProblem: s.sendShortcutProblem,
		CheckForUpdates:     s.settings.checksForUpdates(),
	}
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

// changeCheckForUpdates saves whether the companion checks for updates on
// its own, and checks now when turned on.
func (s *shell) changeCheckForUpdates(on bool) error {
	s.mu.Lock()
	before := s.settings
	s.settings.CheckForUpdates = &on
	err := writeSettings(s.settingsPath, s.settings)
	if err != nil {
		s.settings = before
	}
	s.mu.Unlock()
	s.preferencesChanged()
	if err != nil {
		return fmt.Errorf("save check for updates: %w", err)
	}
	if on && s.appCommands != nil {
		go s.appCommands.checkUpdates(context.Background())
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

// shortcutSlot is one global shortcut: where its keys are saved, where
// the reason it is not active is kept, and what pressing it does.
type shortcutSlot struct {
	// does says what the keys do, for a refusal: "opens the list".
	does    string
	keys    func(settings) string
	save    func(saved *settings, keys string)
	problem *string
	pressed func()
}

// shortcutSlots are the shortcut that opens the list and the one that
// opens it to send the copied text.
func (s *shell) shortcutSlots() (open, send shortcutSlot) {
	open = shortcutSlot{
		does:    "opens the list",
		keys:    settings.shortcut,
		save:    func(saved *settings, keys string) { saved.Shortcut = keys },
		problem: &s.shortcutProblem,
		pressed: s.togglePanel,
	}
	send = shortcutSlot{
		does:    "sends the copied text",
		keys:    settings.sendShortcut,
		save:    func(saved *settings, keys string) { saved.SendShortcut = keys },
		problem: &s.sendShortcutProblem,
		pressed: s.showPanelToSend,
	}
	return open, send
}

// registerShortcuts makes the saved shortcuts work. A shortcut the
// system refuses is kept in the settings and its reason shown, so the
// person can pick another or try the same keys again later. Keys saved
// for both, such as an open shortcut set to the send default before it
// existed, stay with the open shortcut.
func (s *shell) registerShortcuts() {
	open, send := s.shortcutSlots()
	s.registerShortcut(open, nil)
	s.registerShortcut(send, &open)
	s.refreshNative()
}

func (s *shell) registerShortcut(slot shortcutSlot, first *shortcutSlot) {
	s.mu.Lock()
	keys := slot.keys(s.settings)
	clash := first != nil && keys == first.keys(s.settings)
	s.mu.Unlock()
	var err error
	if clash {
		err = fmt.Errorf("it already %s", first.does)
	} else {
		err = s.shortcuts.Register(keys, slot.pressed)
	}
	s.mu.Lock()
	*slot.problem = ""
	if err != nil {
		*slot.problem = fmt.Sprintf("%s is not active: %v", keys, err)
	}
	s.mu.Unlock()
}

func (s *shell) changeOpenShortcut(keys string) error {
	open, send := s.shortcutSlots()
	return s.changeShortcut(open, send, keys)
}

func (s *shell) changeSendShortcut(keys string) error {
	open, send := s.shortcutSlots()
	return s.changeShortcut(send, open, keys)
}

// changeShortcut swaps slot's keys for keys. The new keys are taken
// first, so a refused shortcut leaves the old one working; a failed
// release or save gives them back and keeps the old ones. Keys whose
// last registration was refused are not held, so choosing them again
// tries again: the problem clears only once the system takes them. The
// other shortcut's keys are refused: one press cannot do both. Keys the
// other shortcut lost to this one at start are tried for it again once
// this one lets them go.
func (s *shell) changeShortcut(slot, other shortcutSlot, keys string) error {
	s.changingShortcut.Lock()
	defer s.changingShortcut.Unlock()
	s.mu.Lock()
	old := slot.keys(s.settings)
	oldIsHeld := *slot.problem == ""
	clash := keys == other.keys(s.settings)
	s.mu.Unlock()
	if clash {
		return fmt.Errorf("%s already %s; choose other keys", keys, other.does)
	}
	if keys == old && oldIsHeld {
		return nil
	}
	if err := s.shortcuts.Register(keys, slot.pressed); err != nil {
		return fmt.Errorf("%s cannot be used: %w", keys, err)
	}
	if keys == old {
		s.mu.Lock()
		*slot.problem = ""
		s.mu.Unlock()
		return nil
	}
	if oldIsHeld {
		if err := s.shortcuts.Unregister(old); err != nil {
			_ = s.shortcuts.Unregister(keys)
			return fmt.Errorf("release %s: %w", old, err)
		}
	}
	if err := s.changeSettings(func(saved *settings) { slot.save(saved, keys) }); err != nil {
		_ = s.shortcuts.Unregister(keys)
		if oldIsHeld {
			s.registerShortcut(slot, nil)
		}
		return err
	}
	s.mu.Lock()
	*slot.problem = ""
	freed := keys != old && *other.problem != "" && other.keys(s.settings) == old
	s.mu.Unlock()
	if freed {
		s.registerShortcut(other, &slot)
	}
	return nil
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

// setShortcut answers POST /api/preferences/shortcut and
// /api/preferences/send-shortcut, each with its own change.
func (s *shell) setShortcut(change func(keys string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		if err := change(body.Shortcut); err != nil {
			writeJSON(w, http.StatusConflict, apiError{Error: err.Error()})
			return
		}
		s.preferencesChanged()
		writeJSON(w, http.StatusOK, s.preferences())
	}
}

// setPreferenceSwitch answers a POST that turns one preference on or off.
func (s *shell) setPreferenceSwitch(change func(bool) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		if err := change(*body.Enabled); err != nil {
			writeJSON(w, http.StatusInternalServerError, apiError{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, s.preferences())
	}
}
