package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// toldShell is a shell whose pages' news is recorded in told.
func toldShell(t *testing.T) (*shell, *[]appearance) {
	t.Helper()
	s := shellWith(t, &fakeShortcuts{})
	told := []appearance{}
	s.tell = func(looks appearance) { told = append(told, looks) }
	return s, &told
}

func TestSetAnimate_SavesAndTellsThePages(t *testing.T) {
	s, told := toldShell(t)
	s.settings.Shortcut = "Cmd+Shift+K"

	if err := s.setAnimate(false); err != nil {
		t.Fatal(err)
	}

	saved, err := readSettings(s.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if saved.animate() || saved.Shortcut != "Cmd+Shift+K" {
		t.Errorf("saved %+v; want animation off and the shortcut kept", saved)
	}
	if len(*told) != 1 || (*told)[0].Animate {
		t.Errorf("the pages were told %+v; want animation off once", *told)
	}

	if err := s.setAnimate(true); err != nil {
		t.Fatal(err)
	}
	if !s.appearance().Animate || len(*told) != 2 || !(*told)[1].Animate {
		t.Errorf("turning it back on: appearance %+v, told %+v", s.appearance(), *told)
	}
}

func TestSetAnimate_KeepsTheOldChoiceWhenTheSaveFails(t *testing.T) {
	s, told := toldShell(t)
	notAFolder := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(notAFolder, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	s.settingsPath = filepath.Join(notAFolder, "companion.json")

	err := s.setAnimate(false)

	if err == nil || !strings.HasPrefix(err.Error(), "save the animation choice: ") {
		t.Errorf("error = %v", err)
	}
	if !s.appearance().Animate {
		t.Error("the unsaved choice took effect")
	}
	if len(*told) != 1 || !(*told)[0].Animate {
		t.Errorf("the pages were told %+v; want the old appearance, animation on", *told)
	}
}

func TestSetDimWhenIdle_SavesAndTellsThePages(t *testing.T) {
	s, told := toldShell(t)
	s.settings.Shortcut = "Cmd+Shift+K"

	if err := s.setDimWhenIdle(true); err != nil {
		t.Fatal(err)
	}

	saved, err := readSettings(s.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.DimWhenIdle || saved.Shortcut != "Cmd+Shift+K" {
		t.Errorf("saved %+v; want dimming on and the shortcut kept", saved)
	}
	if len(*told) != 1 || !(*told)[0].DimWhenIdle || !(*told)[0].Animate {
		t.Errorf("the pages were told %+v; want dimming on and animation on, once", *told)
	}
}

func TestSetDimWhenIdle_KeepsTheOldChoiceWhenTheSaveFails(t *testing.T) {
	s, told := toldShell(t)
	notAFolder := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(notAFolder, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	s.settingsPath = filepath.Join(notAFolder, "companion.json")

	err := s.setDimWhenIdle(true)

	if err == nil || !strings.HasPrefix(err.Error(), "save the dim choice: ") {
		t.Errorf("error = %v", err)
	}
	if s.appearance().DimWhenIdle {
		t.Error("the unsaved choice took effect")
	}
	if len(*told) != 1 || (*told)[0].DimWhenIdle {
		t.Errorf("the pages were told %+v; want the old appearance, dimming off", *told)
	}
}

func TestSetAnimate_LeavesDimAsItWas(t *testing.T) {
	s, told := toldShell(t)
	if err := s.setDimWhenIdle(true); err != nil {
		t.Fatal(err)
	}

	if err := s.setAnimate(false); err != nil {
		t.Fatal(err)
	}

	saved, err := readSettings(s.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.DimWhenIdle || saved.animate() {
		t.Errorf("saved %+v; want dimming on and animation off", saved)
	}
	if last := (*told)[len(*told)-1]; !last.DimWhenIdle || last.Animate {
		t.Errorf("the pages were last told %+v; want dimming on and animation off", last)
	}
}

func TestSetDimWhenIdle_LeavesAnimationAsItWas(t *testing.T) {
	s, told := toldShell(t)
	if err := s.setAnimate(false); err != nil {
		t.Fatal(err)
	}

	if err := s.setDimWhenIdle(true); err != nil {
		t.Fatal(err)
	}

	saved, err := readSettings(s.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.DimWhenIdle || saved.animate() {
		t.Errorf("saved %+v; want dimming on and animation off", saved)
	}
	if last := (*told)[len(*told)-1]; !last.DimWhenIdle || last.Animate {
		t.Errorf("the pages were last told %+v; want dimming on and animation off", last)
	}
}

func TestGetAppearance_AnswersTheSavedValues(t *testing.T) {
	s, _ := toldShell(t)
	off := false
	s.settings.Animate = &off

	rec := httptest.NewRecorder()
	s.getAppearance(rec, httptest.NewRequest(http.MethodGet, "/api/appearance", nil))

	if body := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK || body != `{"animate":false,"dimWhenIdle":false,"presence":"shown"}` {
		t.Errorf("appearance = %d %s", rec.Code, body)
	}
}

func TestSettingsHeader_LeadsTheMenuAndCannotBeClicked(t *testing.T) {
	menu := application.NewMenu()

	addSettingsHeader(menu)
	menu.AddCheckbox("Animate companion", true)

	first := menu.ItemAt(0)
	if first == nil || first.Label() != settingsHeader || first.Enabled() {
		t.Fatalf("first item = %+v; want a disabled %q", first, settingsHeader)
	}
	if second := menu.ItemAt(1); second == nil || second.Label() != "Animate companion" {
		t.Errorf("the header was added twice or the checkbox moved: second item = %+v", second)
	}
}
