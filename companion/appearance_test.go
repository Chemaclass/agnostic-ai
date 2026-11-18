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

	if body := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK || body != `{"animate":false,"dimWhenIdle":false,"smallCompanion":false,"presence":"shown"}` {
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

func TestSetSmallCompanion_SavesAndTellsThePages(t *testing.T) {
	s, told := toldShell(t)
	s.settings.DimWhenIdle = true

	if err := s.setSmallCompanion(true); err != nil {
		t.Fatal(err)
	}

	saved, err := readSettings(s.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.SmallCompanion || !saved.DimWhenIdle {
		t.Errorf("saved %+v; want the small circle and dimming kept", saved)
	}
	if len(*told) != 1 || !(*told)[0].SmallCompanion {
		t.Errorf("the pages were told %+v; want the small circle once", *told)
	}
}

func TestSetSmallCompanion_KeepsTheOldChoiceWhenTheSaveFails(t *testing.T) {
	s, told := placedShell(t)
	notAFolder := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(notAFolder, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	s.settingsPath = filepath.Join(notAFolder, "companion.json")

	err := s.setSmallCompanion(true)

	if err == nil || !strings.HasPrefix(err.Error(), "save the circle size: ") {
		t.Errorf("error = %v", err)
	}
	if s.appearance().SmallCompanion || (*told)[0].SmallCompanion {
		t.Errorf("the unsaved choice took effect: told %+v", *told)
	}
	if width, _ := s.bubble.Size(); width != bubbleSide {
		t.Errorf("window side = %d; want it kept at %d", width, bubbleSide)
	}
}

func TestSetSmallCompanion_ShrinksTheWindowAtTheEdgesItSitsBy(t *testing.T) {
	s, _ := placedShell(t)
	bubble := s.bubble.(*fakeWindow)
	work := rect{Width: 1440, Height: 900}
	bubble.x, bubble.y = defaultBubblePosition(work, bubbleSide)

	if err := s.setSmallCompanion(true); err != nil {
		t.Fatal(err)
	}

	wantX, wantY := defaultBubblePosition(work, smallBubbleSide)
	if bubble.side != smallBubbleSide || bubble.x != wantX || bubble.y != wantY {
		t.Errorf("window %d wide at %d,%d; want %d wide at %d,%d", bubble.side, bubble.x, bubble.y, smallBubbleSide, wantX, wantY)
	}

	if err := s.setSmallCompanion(false); err != nil {
		t.Fatal(err)
	}
	wantX, wantY = defaultBubblePosition(work, bubbleSide)
	if bubble.side != bubbleSide || bubble.x != wantX || bubble.y != wantY {
		t.Errorf("back to regular: window %d wide at %d,%d; want %d wide at %d,%d", bubble.side, bubble.x, bubble.y, bubbleSide, wantX, wantY)
	}
}

func TestSetSmallCompanion_KeepsTheOpenListBesideTheCircle(t *testing.T) {
	s, _ := placedShell(t)
	s.togglePanel()

	if err := s.setSmallCompanion(true); err != nil {
		t.Fatal(err)
	}
	bubble := s.bubble.(*fakeWindow)
	for range bubble.reportedMoves {
		s.bubbleMoved()
	}

	if !s.panel.IsVisible() {
		t.Fatal("the size change closed the list it was made from")
	}
	wantX, wantY, _ := placePanel(windowRect(s.bubble), rect{Width: 1440, Height: 900}, panelWidth, panelHeight)
	if x, y := s.panel.Position(); x != wantX || y != wantY {
		t.Errorf("list at %d,%d; want %d,%d beside the small circle", x, y, wantX, wantY)
	}
}

func TestSetSmallCompanion_SavesTheResizedPlace(t *testing.T) {
	s, _ := placedShell(t)
	bubble := s.bubble.(*fakeWindow)
	bubble.x, bubble.y = 1200, 700

	if err := s.setSmallCompanion(true); err != nil {
		t.Fatal(err)
	}

	saved, err := readSettings(s.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if want := (point{X: 1228, Y: 728}); saved.Bubble == nil || *saved.Bubble != want {
		t.Errorf("saved place %+v; want %+v, where the small circle went", saved.Bubble, want)
	}
}

func TestSetSmallCompanion_KeepsTheListOnItsSideNearTheMiddle(t *testing.T) {
	s, _ := placedShell(t)
	s.settings.SmallCompanion = true
	bubble := s.bubble.(*fakeWindow)
	bubble.side = smallBubbleSide
	// The small circle's middle sits right of the screen's middle; the
	// regular one, kept at the same right edge, sits left of it.
	bubble.x, bubble.y = 700, 700
	s.togglePanel()
	beforeX, _ := s.panel.Position()

	if err := s.setSmallCompanion(false); err != nil {
		t.Fatal(err)
	}
	for range bubble.reportedMoves {
		s.bubbleMoved()
	}

	if bubble.x != 672 {
		t.Errorf("circle at x %d; want 672, its right edge kept", bubble.x)
	}
	if x, _ := s.panel.Position(); x != 672-panelGap-panelWidth || beforeX != 700-panelGap-panelWidth {
		t.Errorf("list at x %d (was %d); want it still left of the circle at %d", x, beforeX, 672-panelGap-panelWidth)
	}
}

func TestBubbleMoved_ADragAfterAResizePlacesTheListByTheNewPlace(t *testing.T) {
	s, _ := placedShell(t)
	s.settings.SmallCompanion = true
	bubble := s.bubble.(*fakeWindow)
	bubble.side = smallBubbleSide
	bubble.x, bubble.y = 700, 700
	s.togglePanel()
	if err := s.setSmallCompanion(false); err != nil {
		t.Fatal(err)
	}

	bubble.SetPosition(200, 700)
	s.bubbleMoved()

	wantX, wantY, _ := placePanel(windowRect(s.bubble), rect{Width: 1440, Height: 900}, panelWidth, panelHeight)
	if x, y := s.panel.Position(); x != wantX || y != wantY {
		t.Errorf("list at %d,%d; want %d,%d beside the dragged circle", x, y, wantX, wantY)
	}
}
