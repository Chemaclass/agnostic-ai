package main

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// TestTakeOutsideSettings_AppliesSwitchesAndKeepsTheShortcuts: a switch
// the web page saved reaches the pages; a shortcut written from outside
// is put back.
func TestTakeOutsideSettings_AppliesSwitchesAndKeepsTheShortcuts(t *testing.T) {
	s, told := toldShell(t)
	s.settings.Shortcut = "Cmd+Shift+K"
	if err := writeSettings(s.settingsPath, s.settings); err != nil {
		t.Fatal(err)
	}
	s.takeOutsideSettings()
	if len(*told) != 0 {
		t.Fatalf("an unchanged file told the pages %+v", *told)
	}

	outside := s.settings
	outside.SmallCompanion = true
	outside.Shortcut = "Ctrl+Q"
	if err := writeSettings(s.settingsPath, outside); err != nil {
		t.Fatal(err)
	}
	s.takeOutsideSettings()
	if !s.appearance().SmallCompanion || len(*told) != 1 || !(*told)[0].SmallCompanion {
		t.Errorf("appearance %+v, told %+v; want the small circle", s.appearance(), *told)
	}
	saved, err := readSettings(s.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Shortcut != "Cmd+Shift+K" || !saved.SmallCompanion {
		t.Errorf("saved %+v; want the shortcut in use back and the small circle kept", saved)
	}

	// A drag saved afterwards keeps the switch the page turned on.
	if err := s.changeSettings(func(saved *settings) { saved.Bubble = &point{X: 5, Y: 6} }); err != nil {
		t.Fatal(err)
	}
	if saved, _ := readSettings(s.settingsPath); !saved.SmallCompanion || saved.Bubble == nil {
		t.Errorf("after a drag: %+v", saved)
	}
}

// TestTakeOutsideSettings_DragBeforeThePollKeepsTheSwitch: the page
// turns Small companion on, the person drags the circle before the
// companion reads the file, and the switch stays on.
func TestTakeOutsideSettings_DragBeforeThePollKeepsTheSwitch(t *testing.T) {
	s, told := toldShell(t)
	if err := writeSettings(s.settingsPath, settings{SmallCompanion: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.changeSettings(func(saved *settings) { saved.Bubble = &point{X: 1, Y: 2} }); err != nil {
		t.Fatal(err)
	}
	s.takeOutsideSettings()
	saved, _ := readSettings(s.settingsPath)
	if !saved.SmallCompanion || saved.Bubble == nil || !s.appearance().SmallCompanion || len(*told) != 1 {
		t.Errorf("saved %+v, appearance %+v, told %+v", saved, s.appearance(), *told)
	}
}

func TestTakeOutsideSettings_IgnoresAMissingFileAndTakesCheckForUpdates(t *testing.T) {
	s, told := toldShell(t)
	s.settings.SmallCompanion = true
	s.takeOutsideSettings()
	if !s.appearance().SmallCompanion || len(*told) != 0 {
		t.Errorf("a missing file reset the switches: %+v", s.appearance())
	}
	told2 := []preferences{}
	s.tellPreferences = func(p preferences) { told2 = append(told2, p) }
	off := false
	if err := writeSettings(s.settingsPath, settings{SmallCompanion: true, CheckForUpdates: &off}); err != nil {
		t.Fatal(err)
	}
	s.takeOutsideSettings()
	if s.preferences().CheckForUpdates || len(told2) != 1 || told2[0].CheckForUpdates {
		t.Errorf("check for updates not taken: %+v", told2)
	}
}

func TestWriteState_HoldsTheProcessAndItsPreferences(t *testing.T) {
	s := shellWith(t, &fakeShortcuts{})
	s.shortcutProblem = "Ctrl+Alt+Space is not active: another app holds these keys"
	s.writeState()
	raw, err := os.ReadFile(statePath(s.settingsPath))
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		PID             int    `json:"pid"`
		Shortcut        string `json:"shortcut"`
		ShortcutProblem string `json:"shortcutProblem"`
	}
	if err := json.Unmarshal(raw, &state); err != nil || state.PID != os.Getpid() || state.Shortcut != defaultShortcut || state.ShortcutProblem == "" {
		t.Errorf("state = %s, %v", raw, err)
	}
	s.removeState()
	if _, err := os.Stat(statePath(s.settingsPath)); !os.IsNotExist(err) {
		t.Errorf("state file after quit: %v", err)
	}
}

// TestSaveSettingsChange_KeepsFieldsItDoesNotKnow: the write-back of a
// shortcut and a drag patch their own fields, so a field a newer
// version or the server wrote stays, and a file holding null works.
func TestSaveSettingsChange_KeepsFieldsItDoesNotKnow(t *testing.T) {
	s, _ := toldShell(t)
	if err := os.WriteFile(s.settingsPath, []byte(`{"shortcut":"Ctrl+Q","futureField":{"a":1},"smallCompanion":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s.takeOutsideSettings()
	if err := s.changeSettings(func(saved *settings) { saved.Bubble = &point{X: 7, Y: 8} }); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(s.settingsPath)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	var future bytes.Buffer
	_ = json.Compact(&future, fields["futureField"])
	if future.String() != `{"a":1}` || fields["bubble"] == nil || fields["shortcut"] != nil || string(fields["smallCompanion"]) != "true" {
		t.Errorf("saved %s; want futureField kept, the bubble saved and the outside shortcut removed", raw)
	}

	if err := os.WriteFile(s.settingsPath, []byte("null"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.changeSettings(func(saved *settings) { saved.DimWhenIdle = true }); err != nil {
		t.Fatal(err)
	}
	if saved, err := readSettings(s.settingsPath); err != nil || !saved.DimWhenIdle {
		t.Errorf("after a null file: %+v, %v", saved, err)
	}
}

func TestTouchState_RefreshesOrWritesTheFileUntilQuit(t *testing.T) {
	s := shellWith(t, &fakeShortcuts{})
	path := statePath(s.settingsPath)
	s.touchState()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("a missing state file was not written: %v", err)
	}
	old := time.Now().Add(-time.Minute)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	s.touchState()
	if info, _ := os.Stat(path); time.Since(info.ModTime()) > 10*time.Second {
		t.Errorf("state file not touched: %v", info.ModTime())
	}
	s.removeState()
	s.quitting = true
	s.touchState()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("state file came back after quit: %v", err)
	}
}

func TestSaveSettingsChange_ClearsAFieldAndKeepsTheRest(t *testing.T) {
	s, _ := toldShell(t)
	if err := os.WriteFile(s.settingsPath, []byte(`{"dimWhenIdle":true,"futureField":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s.settings.DimWhenIdle = true
	if err := s.setDimWhenIdle(false); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(s.settingsPath)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	if _, dim := fields["dimWhenIdle"]; dim || string(fields["futureField"]) != "1" {
		t.Errorf("saved %s; want dimWhenIdle removed and futureField kept", raw)
	}
}
