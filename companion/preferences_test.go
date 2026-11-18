package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeShortcuts stands in for the window system's global shortcuts.
type fakeShortcuts struct {
	// refused are the keys another app holds.
	refused map[string]bool
	// stuck are the keys the system fails to give back.
	stuck map[string]bool
	held  map[string]bool
	calls []string
}

func (f *fakeShortcuts) Register(keys string, _ func()) error {
	f.calls = append(f.calls, "register "+keys)
	if f.refused[keys] {
		return errors.New("another app holds these keys")
	}
	if f.held == nil {
		f.held = map[string]bool{}
	}
	f.held[keys] = true
	return nil
}

func (f *fakeShortcuts) Unregister(keys string) error {
	f.calls = append(f.calls, "unregister "+keys)
	if f.stuck[keys] {
		return errors.New("the system kept these keys")
	}
	delete(f.held, keys)
	return nil
}

func shellWith(t *testing.T, keys *fakeShortcuts) *shell {
	t.Helper()
	return &shell{
		shortcuts:       keys,
		login:           &fakeAutostart{},
		settingsPath:    filepath.Join(t.TempDir(), "companion.json"),
		tell:            func(appearance) {},
		tellPreferences: func(preferences) {},
	}
}

func TestChangeShortcut_TriesTheSameKeysAgainAfterTheyWereRefused(t *testing.T) {
	keys := &fakeShortcuts{refused: map[string]bool{defaultShortcut: true}}
	s := shellWith(t, keys)
	s.registerShortcuts()
	if s.shortcutProblem != "Ctrl+Alt+Space is not active: another app holds these keys" {
		t.Fatalf("problem after a refused start = %q", s.shortcutProblem)
	}

	// The other app still holds them: choosing them again must not report success.
	err := s.changeOpenShortcut(defaultShortcut)
	if err == nil || err.Error() != "Ctrl+Alt+Space cannot be used: another app holds these keys" {
		t.Errorf("choosing refused keys again: %v", err)
	}
	if s.shortcutProblem == "" || keys.held[defaultShortcut] {
		t.Errorf("the problem cleared (%q) though the keys are not held", s.shortcutProblem)
	}

	// The other app let go: the same keys now register and the problem clears.
	keys.refused = nil
	if err := s.changeOpenShortcut(defaultShortcut); err != nil {
		t.Fatal(err)
	}
	if s.shortcutProblem != "" || !keys.held[defaultShortcut] {
		t.Errorf("problem %q, held %v", s.shortcutProblem, keys.held)
	}
	if got := strings.Join(keys.calls, "; "); got != "register Ctrl+Alt+Space; register Ctrl+Alt+V; register Ctrl+Alt+Space; register Ctrl+Alt+Space" {
		t.Errorf("calls = %s", got)
	}
}

func TestChangeShortcut_KeepsTheOldKeysWhenTheNewOnesAreRefused(t *testing.T) {
	keys := &fakeShortcuts{refused: map[string]bool{"Cmd+Shift+K": true}}
	s := shellWith(t, keys)
	s.registerShortcuts()

	if err := s.changeOpenShortcut("Cmd+Shift+K"); err == nil {
		t.Fatal("refused keys were accepted")
	}
	if !keys.held[defaultShortcut] || s.settings.shortcut() != defaultShortcut {
		t.Errorf("held %v, saved %q; want the old shortcut still working", keys.held, s.settings.shortcut())
	}

	if err := s.changeOpenShortcut("Ctrl+Alt+J"); err != nil {
		t.Fatal(err)
	}
	saved, err := readSettings(s.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Shortcut != "Ctrl+Alt+J" || keys.held[defaultShortcut] || !keys.held["Ctrl+Alt+J"] {
		t.Errorf("saved %q, held %v", saved.Shortcut, keys.held)
	}
}

func TestChangeSettings_OverlappingChangesAllReachTheFile(t *testing.T) {
	s := shellWith(t, &fakeShortcuts{})
	var wait sync.WaitGroup
	failed := make(chan error, 80)
	for i := range 40 {
		wait.Go(func() {
			if err := s.changeSettings(func(saved *settings) { saved.Bubble = &point{X: i, Y: i} }); err != nil {
				failed <- err
			}
		})
		wait.Go(func() {
			if err := s.changeSettings(func(saved *settings) { saved.Shortcut = fmt.Sprintf("Ctrl+Alt+%d", i) }); err != nil {
				failed <- err
			}
		})
	}
	wait.Wait()
	close(failed)
	for err := range failed {
		t.Fatalf("a save failed while another ran: %v", err)
	}

	saved, err := readSettings(s.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Bubble == nil || saved.Shortcut == "" || *saved.Bubble != *s.settings.Bubble || saved.Shortcut != s.settings.Shortcut {
		t.Errorf("the file holds %+v, the companion %+v: the last change did not land last", saved, s.settings)
	}
}

func TestRegisterShortcuts_TakesTheOpenAndTheSendKeys(t *testing.T) {
	keys := &fakeShortcuts{}
	s := shellWith(t, keys)

	s.registerShortcuts()

	if !keys.held[defaultShortcut] || !keys.held[defaultSendShortcut] {
		t.Errorf("held %v; want both defaults", keys.held)
	}
	prefs := s.preferences()
	if prefs.SendShortcut != "Ctrl+Alt+V" || prefs.SendShortcutProblem != "" || prefs.ShortcutProblem != "" {
		t.Errorf("preferences = %+v", prefs)
	}
}

func TestRegisterShortcuts_ARefusedSendShortcutSaysWhyAndLeavesTheOpenOne(t *testing.T) {
	keys := &fakeShortcuts{refused: map[string]bool{defaultSendShortcut: true}}
	s := shellWith(t, keys)

	s.registerShortcuts()

	prefs := s.preferences()
	if prefs.SendShortcutProblem != "Ctrl+Alt+V is not active: another app holds these keys" {
		t.Errorf("send problem = %q", prefs.SendShortcutProblem)
	}
	if prefs.ShortcutProblem != "" || !keys.held[defaultShortcut] {
		t.Errorf("the open shortcut suffered: problem %q, held %v", prefs.ShortcutProblem, keys.held)
	}
}

func TestRegisterShortcuts_KeysSavedForBothStayWithTheOpenShortcut(t *testing.T) {
	keys := &fakeShortcuts{}
	s := shellWith(t, keys)
	s.settings.Shortcut = defaultSendShortcut

	s.registerShortcuts()

	if got := strings.Join(keys.calls, "; "); got != "register Ctrl+Alt+V" {
		t.Errorf("calls = %s; want the keys taken once", got)
	}
	if prefs := s.preferences(); prefs.SendShortcutProblem != "Ctrl+Alt+V is not active: it already opens the list" || prefs.ShortcutProblem != "" {
		t.Errorf("preferences = %+v", prefs)
	}
}

func TestChangeShortcut_GivesKeysItLetsGoToTheShortcutThatLostThem(t *testing.T) {
	keys := &fakeShortcuts{}
	s := shellWith(t, keys)
	s.settings.Shortcut = defaultSendShortcut
	s.registerShortcuts()

	if err := s.changeOpenShortcut("Ctrl+Alt+J"); err != nil {
		t.Fatal(err)
	}

	if prefs := s.preferences(); prefs.SendShortcutProblem != "" || prefs.ShortcutProblem != "" {
		t.Errorf("preferences = %+v", prefs)
	}
	if !keys.held["Ctrl+Alt+J"] || !keys.held[defaultSendShortcut] {
		t.Errorf("held %v; want the new open keys and the send keys", keys.held)
	}
}

func TestChangeShortcut_RefusesTheKeysOfTheOtherShortcut(t *testing.T) {
	keys := &fakeShortcuts{}
	s := shellWith(t, keys)
	s.registerShortcuts()
	keys.calls = nil

	err := s.changeOpenShortcut(defaultSendShortcut)
	if err == nil || err.Error() != "Ctrl+Alt+V already sends the copied text; choose other keys" {
		t.Errorf("open set to the send keys: %v", err)
	}
	err = s.changeSendShortcut(defaultShortcut)
	if err == nil || err.Error() != "Ctrl+Alt+Space already opens the list; choose other keys" {
		t.Errorf("send set to the open keys: %v", err)
	}
	if len(keys.calls) != 0 || s.settings.Shortcut != "" || s.settings.SendShortcut != "" {
		t.Errorf("calls %v, settings %+v; want nothing changed", keys.calls, s.settings)
	}
}

func TestChangeSendShortcut_SavesTheNewKeysAndReleasesTheOld(t *testing.T) {
	keys := &fakeShortcuts{}
	s := shellWith(t, keys)
	s.registerShortcuts()

	if err := s.changeSendShortcut("Cmd+Shift+B"); err != nil {
		t.Fatal(err)
	}

	saved, err := readSettings(s.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if saved.SendShortcut != "Cmd+Shift+B" || saved.Shortcut != "" || keys.held[defaultSendShortcut] || !keys.held["Cmd+Shift+B"] || !keys.held[defaultShortcut] {
		t.Errorf("saved %+v, held %v", saved, keys.held)
	}
}

func TestChangeShortcut_GivesTheNewKeysBackWhenTheOldOnesStayHeld(t *testing.T) {
	keys := &fakeShortcuts{stuck: map[string]bool{defaultShortcut: true}}
	s := shellWith(t, keys)
	s.registerShortcuts()

	err := s.changeOpenShortcut("Cmd+Shift+K")

	if err == nil || err.Error() != "release Ctrl+Alt+Space: the system kept these keys" {
		t.Errorf("change = %v", err)
	}
	if keys.held["Cmd+Shift+K"] {
		t.Error("the new keys stay taken though the change failed")
	}
	if s.settings.shortcut() != defaultShortcut {
		t.Errorf("shortcut = %s; want the old one kept", s.settings.shortcut())
	}
	keys.stuck = nil
	if err := s.changeOpenShortcut("Cmd+Shift+K"); err != nil {
		t.Errorf("choosing the same keys again: %v", err)
	}
}

func TestChangeShortcut_KeepsTheOldKeysWhenTheSettingsCannotBeSaved(t *testing.T) {
	keys := &fakeShortcuts{}
	s := shellWith(t, keys)
	s.registerShortcuts()
	blocker := filepath.Join(t.TempDir(), "a file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	s.settingsPath = filepath.Join(blocker, "companion.json")

	err := s.changeOpenShortcut("Cmd+Shift+K")

	if err == nil {
		t.Fatal("a change whose save failed reported success")
	}
	if s.settings.shortcut() != defaultShortcut || s.shortcutProblem != "" {
		t.Errorf("shortcut = %s, problem %q; want the old one, working", s.settings.shortcut(), s.shortcutProblem)
	}
	if keys.held["Cmd+Shift+K"] || !keys.held[defaultShortcut] {
		t.Errorf("held = %v; want only the old keys", keys.held)
	}
}
