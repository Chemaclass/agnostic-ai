package main

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
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
		if !s.server.resting.Load() || !input.closed {
			t.Errorf("the app ended first: resting %v, input closed %v", s.server.resting.Load(), input.closed)
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
	if !s.server.resting.Load() || !input.closed {
		t.Errorf("the server still runs: resting %v, input closed %v", s.server.resting.Load(), input.closed)
	}
}

func TestRestBriefly_GivesUpWhileTheServerIsStillStarting(t *testing.T) {
	s, input, _ := quittingShell(t)
	s.server.mu.Lock()
	unlocked := false
	t.Cleanup(func() {
		if !unlocked {
			s.server.mu.Unlock()
		}
	})

	started := time.Now()
	s.restBriefly()

	if waited := time.Since(started); waited > quitRestWait+time.Second {
		t.Errorf("restBriefly waited %v for the server to finish starting", waited)
	}
	s.server.mu.Unlock()
	unlocked = true
	// The rest it started still runs once the start lets go of the lock.
	deadline := time.Now().Add(time.Second)
	for {
		s.server.mu.Lock()
		closed := input.closed
		s.server.mu.Unlock()
		if closed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the server never rested after the start let go")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestQuit_TwoAtOnceEndTheAppOnce(t *testing.T) {
	s, _, _ := quittingShell(t)
	var ended atomic.Int32
	s.end = func() { ended.Add(1) }

	var both sync.WaitGroup
	for range 2 {
		both.Add(1)
		go func() {
			defer both.Done()
			s.quit()
		}()
	}
	both.Wait()

	if n := ended.Load(); n != 1 {
		t.Errorf("end was called %d times; want once", n)
	}
}

func TestNativeQuit_AsksInTheListAndEndsNothing(t *testing.T) {
	s, input, ended := quittingShell(t)
	panel := newFakeWindow()
	panel.visible = false
	s.bubble, s.panel = newFakeWindow(), panel
	var asked []string
	s.panel = &askingWindow{fakeWindow: panel, events: &asked}

	s.menuActions()["quit"](false)

	if *ended != 0 || input.closed {
		t.Error("Quit in the right-click menu ended the companion without asking")
	}
	if !panel.IsVisible() || len(asked) != 1 || asked[0] != eventQuitAsked {
		t.Errorf("list visible %v, told %v; want it open on the question", panel.IsVisible(), asked)
	}
}

func TestNativeQuit_BringsAHiddenOrFadedCompanionBackToAsk(t *testing.T) {
	s, _, ended := quittingShell(t)
	panel := newFakeWindow()
	panel.visible = false
	s.bubble, s.panel = newFakeWindow(), panel
	s.setPresence(fadedBack)

	s.menuActions()["quit"](false)

	if s.presenceNow() != shown || *ended != 0 {
		t.Errorf("presence %q, ended %d; want shown and asking", s.presenceNow(), *ended)
	}
}

func TestSecondStart_AskedToQuitEndsWithoutAQuestion(t *testing.T) {
	s, input, ended := quittingShell(t)

	s.secondStart(application.SecondInstanceData{Args: []string{"companion", askQuit}})

	if *ended != 1 || !input.closed {
		t.Errorf("ended %d, server stopped %v; want ended once without asking", *ended, input.closed)
	}
}

// askingWindow records the events the shell sends a window.
type askingWindow struct {
	*fakeWindow
	events *[]string
}

func (w *askingWindow) EmitEvent(name string, _ ...any) bool {
	*w.events = append(*w.events, name)
	return true
}
