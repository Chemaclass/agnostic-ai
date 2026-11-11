package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// quittingShell is a shell whose end only counts the calls.
func quittingShell(t *testing.T) (*shell, *int) {
	t.Helper()
	s := shellWith(t, &fakeShortcuts{})
	s.server = newServer()
	ended := 0
	s.end = func() { ended++ }
	return s, &ended
}

func TestQuit_StopsTheServerThenEndsTheApp(t *testing.T) {
	s, ended := quittingShell(t)
	s.end = func() {
		*ended++
		if _, err := s.server.address(); !errors.Is(err, errResting) {
			t.Errorf("the app ended while a server could still start: %v", err)
		}
	}

	s.quit()

	if *ended != 1 {
		t.Errorf("end was called %d times; want once", *ended)
	}
}

func TestQuitRoute_AnswersNoContentAndQuits(t *testing.T) {
	s, ended := quittingShell(t)
	routes := s.routes(&inbox{}, fstest.MapFS{})

	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/quit", nil))

	if rec.Code != http.StatusNoContent {
		t.Errorf("POST /api/quit = %d; want %d", rec.Code, http.StatusNoContent)
	}
	if *ended != 1 {
		t.Errorf("end was called %d times; want once", *ended)
	}
	if _, err := s.server.address(); !errors.Is(err, errResting) {
		t.Errorf("a server can still start after quit: %v", err)
	}
}

func TestBubbleMenu_EndsWithQuitAfterTheSettings(t *testing.T) {
	s, _ := toldShell(t)
	menu := application.NewMenu()

	s.fillBubbleMenu(menu)

	var labels []string
	for i := 0; menu.ItemAt(i) != nil; i++ {
		item := menu.ItemAt(i)
		if item.IsSeparator() {
			labels = append(labels, "---")
			continue
		}
		labels = append(labels, item.Label())
	}
	want := []string{settingsHeader, "Animate companion", "Dim when idle", "---", quitLabel}
	if len(labels) != len(want) {
		t.Fatalf("menu = %q; want %q", labels, want)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Fatalf("menu = %q; want %q", labels, want)
		}
	}
	quit := menu.ItemAt(len(want) - 1)
	if !quit.Enabled() || quit.IsCheckbox() {
		t.Errorf("quit item: enabled %v, checkbox %v; want an enabled plain item", quit.Enabled(), quit.IsCheckbox())
	}
}
