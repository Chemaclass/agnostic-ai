package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestSettings_CompanionSwitchKeepsTheOtherFields(t *testing.T) {
	h, _ := settingsFixture(t, "")
	dir, _ := h.companionDir()
	path := filepath.Join(dir, companionSettingsFile)
	writeFile(t, path, `{"bubble":{"x":3,"y":4},"shortcut":"Cmd+K","futureField":[1]}`)

	_, doc := getSettings(t, h, "")
	if app := doc.CompanionApp; app == nil || !app.Animate || app.SmallCompanion || !app.CheckForUpdates || app.Running || app.Live != nil {
		t.Fatalf("companionApp = %+v", doc.CompanionApp)
	}

	// Cosmetic: no proof needed.
	rec, reply := postSetting(t, h, `{"key":"companion.smallCompanion","value":true,"expected":false}`, nil)
	if rec.Code != http.StatusOK || !reply.Settings.CompanionApp.SmallCompanion || len(reply.Changed) != 1 {
		t.Fatalf("write = %d %s", rec.Code, rec.Body)
	}
	var saved map[string]any
	raw, _ := os.ReadFile(path)
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if saved["smallCompanion"] != true || saved["shortcut"] != "Cmd+K" || saved["bubble"] == nil || saved["futureField"] == nil {
		t.Errorf("saved %s", raw)
	}

	rec, _ = postSetting(t, h, `{"key":"companion.smallCompanion","value":false,"expected":false}`, nil)
	if rec.Code != http.StatusConflict {
		t.Errorf("stale expected = %d %s", rec.Code, rec.Body)
	}
	rec, _ = postSetting(t, h, `{"key":"companion.shortcut","value":"Ctrl+Q"}`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("shortcut write = %d %s; it is read only", rec.Code, rec.Body)
	}

	writeFile(t, path, `not json`)
	rec, _ = postSetting(t, h, `{"key":"companion.animate","value":false}`, nil)
	if raw, _ := os.ReadFile(path); rec.Code != http.StatusConflict || string(raw) != "not json" {
		t.Errorf("broken file: %d %s; file %q", rec.Code, rec.Body, raw)
	}
}

func TestSettings_CompanionLiveRowsOnlyWhileItRuns(t *testing.T) {
	h, _ := settingsFixture(t, "")
	dir, _ := h.companionDir()
	state := filepath.Join(dir, companionStateFile)
	writeFile(t, state, `{"pid":`+jsonInt(os.Getpid())+`,"shortcut":"Ctrl+Alt+Space","sendShortcut":"Ctrl+Alt+V","startAtLogin":true}`)
	_, doc := getSettings(t, h, "")
	if app := doc.CompanionApp; !app.Running || app.Live == nil || app.Live.Shortcut != "Ctrl+Alt+Space" || !app.Live.StartAtLogin {
		t.Errorf("running companion = %+v", app)
	}
	writeFile(t, state, `{"pid":2147483646,"shortcut":"Ctrl+Alt+Space"}`)
	if _, doc := getSettings(t, h, ""); doc.CompanionApp.Running {
		t.Errorf("a state file left by a crash counts as running: %+v", doc.CompanionApp)
	}
	if err := os.Remove(state); err != nil {
		t.Fatal(err)
	}
	if _, doc := getSettings(t, h, ""); doc.CompanionApp.Running || doc.CompanionApp.Live != nil {
		t.Errorf("after quit = %+v", doc.CompanionApp)
	}
}

func jsonInt(n int) string {
	raw, _ := json.Marshal(n)
	return string(raw)
}

func TestSettings_CompanionSwitchWaitsForTheCompanionsLock(t *testing.T) {
	h, _ := settingsFixture(t, "")
	dir, _ := h.companionDir()
	held, err := lockCompanionSettings(filepath.Join(dir, companionSettingsFile))
	if err != nil {
		t.Fatal(err)
	}
	defer held()
	rec, _ := postSetting(t, h, `{"key":"companion.dimWhenIdle","value":true}`, nil)
	if _, statErr := os.Stat(filepath.Join(dir, companionSettingsFile)); rec.Code != http.StatusConflict || statErr == nil {
		t.Errorf("write under a held lock = %d %s; file %v", rec.Code, rec.Body, statErr)
	}
}
