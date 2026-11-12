package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeShortcuts stands in for the window system's global shortcuts.
type fakeShortcuts struct {
	// refused are the keys another app holds.
	refused map[string]bool
	held    map[string]bool
	calls   []string
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
	s.registerShortcut()
	if s.shortcutProblem != "Ctrl+Alt+Space is not active: another app holds these keys" {
		t.Fatalf("problem after a refused start = %q", s.shortcutProblem)
	}

	// The other app still holds them: choosing them again must not report success.
	err := s.changeShortcut(defaultShortcut)
	if err == nil || err.Error() != "Ctrl+Alt+Space cannot be used: another app holds these keys" {
		t.Errorf("choosing refused keys again: %v", err)
	}
	if s.shortcutProblem == "" || keys.held[defaultShortcut] {
		t.Errorf("the problem cleared (%q) though the keys are not held", s.shortcutProblem)
	}

	// The other app let go: the same keys now register and the problem clears.
	keys.refused = nil
	if err := s.changeShortcut(defaultShortcut); err != nil {
		t.Fatal(err)
	}
	if s.shortcutProblem != "" || !keys.held[defaultShortcut] {
		t.Errorf("problem %q, held %v", s.shortcutProblem, keys.held)
	}
	if got := strings.Join(keys.calls, "; "); got != "register Ctrl+Alt+Space; register Ctrl+Alt+Space; register Ctrl+Alt+Space" {
		t.Errorf("calls = %s", got)
	}
}

func TestChangeShortcut_KeepsTheOldKeysWhenTheNewOnesAreRefused(t *testing.T) {
	keys := &fakeShortcuts{refused: map[string]bool{"Cmd+Shift+K": true}}
	s := shellWith(t, keys)
	s.registerShortcut()

	if err := s.changeShortcut("Cmd+Shift+K"); err == nil {
		t.Fatal("refused keys were accepted")
	}
	if !keys.held[defaultShortcut] || s.settings.shortcut() != defaultShortcut {
		t.Errorf("held %v, saved %q; want the old shortcut still working", keys.held, s.settings.shortcut())
	}

	if err := s.changeShortcut("Ctrl+Alt+J"); err != nil {
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
