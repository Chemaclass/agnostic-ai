package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// closer records whether the server's input was closed.
type closer struct{ closed bool }

func (c *closer) Close() error {
	c.closed = true
	return nil
}

// quittingShell is a shell with a running server whose end only counts the calls.
func quittingShell(t *testing.T) (*shell, *closer, *int) {
	t.Helper()
	s := shellWith(t, &fakeShortcuts{})
	input := &closer{}
	s.server = newServer()
	s.server.base, s.server.input = "http://127.0.0.1:1", input
	ended := 0
	s.end = func() { ended++ }
	return s, input, &ended
}

func TestQuit_StopsTheServerThenEndsTheApp(t *testing.T) {
	s, input, ended := quittingShell(t)
	s.end = func() {
		*ended++
		if !s.server.resting || !input.closed {
			t.Errorf("the app ended first: resting %v, input closed %v", s.server.resting, input.closed)
		}
	}

	s.quit()

	if *ended != 1 {
		t.Errorf("end was called %d times; want once", *ended)
	}
}

func TestQuit_EndsTheAppWhileTheServerIsStillStarting(t *testing.T) {
	s, _, ended := quittingShell(t)
	// address holds the lock for the whole start of a server.
	s.server.mu.Lock()
	t.Cleanup(s.server.mu.Unlock)

	done := make(chan struct{})
	go func() {
		s.quit()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(quitRestWait + time.Second):
		t.Fatal("quit waited for the server to finish starting")
	}
	if *ended != 1 {
		t.Errorf("end was called %d times; want once", *ended)
	}
}

func TestQuitRoute_AnswersNoContentAndQuits(t *testing.T) {
	s, input, ended := quittingShell(t)
	routes := s.routes(&inbox{}, fstest.MapFS{})

	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/quit", nil))

	if rec.Code != http.StatusNoContent {
		t.Errorf("POST /api/quit = %d; want %d", rec.Code, http.StatusNoContent)
	}
	if *ended != 1 {
		t.Errorf("end was called %d times; want once", *ended)
	}
	if !s.server.resting || !input.closed {
		t.Errorf("the server still runs: resting %v, input closed %v", s.server.resting, input.closed)
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
