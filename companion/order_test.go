package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"
)

// orderWait is how long a test waits for a step that a wrong order
// would block forever.
const orderWait = 5 * time.Second

// settleWait gives a goroutine a fair chance to run ahead when nothing
// holds it back.
const settleWait = 50 * time.Millisecond

func finished(t *testing.T, done <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(orderWait):
		t.Fatalf("%s did not finish within %s", what, orderWait)
	}
}

func runAsync(f func()) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()
	return done
}

// heldPanelPosition stops the list on its way to the screen, until released.
type heldPanelPosition struct {
	*fakeWindow
	entered, release chan struct{}
}

func (w *heldPanelPosition) SetPosition(x, y int) {
	close(w.entered)
	<-w.release
	w.fakeWindow.SetPosition(x, y)
}

func TestHide_WinsOverAListOpeningAtTheSameTime(t *testing.T) {
	opens := map[string]func(*shell){
		"bring back":    (*shell).bringBack,
		"toggle":        (*shell).togglePanel,
		"settings":      (*shell).openSettings,
		"quit question": (*shell).askQuit,
	}
	for name, open := range opens {
		t.Run(name, func(t *testing.T) {
			s, _ := placedShell(t)
			panel := &heldPanelPosition{fakeWindow: s.panel.(*fakeWindow), entered: make(chan struct{}), release: make(chan struct{})}
			s.panel = panel
			opened := runAsync(func() { open(s) })
			finished(t, panel.entered, "opening the list")

			hideDone := runAsync(s.hide)
			select {
			case <-hideDone:
			case <-time.After(settleWait):
			}
			close(panel.release)
			finished(t, opened, "opening the list")
			finished(t, hideDone, "hiding")

			if s.presenceNow() != hidden || s.panel.IsVisible() || s.bubble.IsVisible() {
				t.Fatalf("presence = %s, circle shown = %v, list shown = %v; want hidden, with both windows off the screen",
					s.presenceNow(), s.bubble.IsVisible(), s.panel.IsVisible())
			}
		})
	}
}

func TestPanelShow_OpensNothingWhileHidden(t *testing.T) {
	s, _ := placedShell(t)
	s.hide()
	rec := httptest.NewRecorder()
	s.routes(s.inbox, fstest.MapFS{}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/panel/show", nil))
	if s.panel.IsVisible() {
		t.Fatal("the list opened while the companion is hidden")
	}
}

type answerFunc func(*http.Request) (*http.Response, error)

func (f answerFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// heldFirstList answers the first list request only once released, with
// nobody waiting and the tag "a"; later ones answer with one waiting
// chat and the tag "b", or "unchanged" to a request naming "b".
func heldFirstList(s *server) (started, release chan struct{}, calls *atomic.Int32) {
	started, release, calls = make(chan struct{}), make(chan struct{}), &atomic.Int32{}
	s.base = "http://companion.test"
	s.client = &http.Client{Transport: answerFunc(func(r *http.Request) (*http.Response, error) {
		answer := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Request: r, Body: http.NoBody}
		if calls.Add(1) == 1 {
			close(started)
			<-release
			answer.Header.Set("ETag", `"a"`)
			answer.Body = io.NopCloser(strings.NewReader(string(listWaiting(0))))
			return answer, nil
		}
		if r.Header.Get("If-None-Match") == `"b"` {
			answer.StatusCode = http.StatusNotModified
			return answer, nil
		}
		answer.Header.Set("ETag", `"b"`)
		answer.Body = io.NopCloser(strings.NewReader(string(listWaiting(1, "already-waiting"))))
		return answer, nil
	})}
	return started, release, calls
}

func TestInbox_AnOlderAnswerNeverReplacesANewerOne(t *testing.T) {
	s, _ := placedShell(t)
	started, release, _ := heldFirstList(s.server)
	s.inbox = newInbox(s.server, "", func() { s.listRead(s.inbox.held()) })

	first := runAsync(func() { s.inbox.read(context.Background()) })
	<-started
	second := runAsync(func() { s.inbox.read(context.Background()) })
	select {
	case <-second:
	case <-time.After(settleWait):
	}
	close(release)
	finished(t, first, "the first read")
	finished(t, second, "the second read")

	if got := string(s.inbox.held()); got != string(listWaiting(1, "already-waiting")) {
		t.Fatalf("held list = %s; want the newer answer", got)
	}
	s.hide()
	s.inbox.read(context.Background())
	if got := s.presenceNow(); got != hidden {
		t.Fatalf("presence = %s; a chat that waited before Hide brought the circle back", got)
	}
}

func TestInbox_APageFirstReadWaitsForThePollersRead(t *testing.T) {
	s, _ := placedShell(t)
	started, release, calls := heldFirstList(s.server)
	in := newInbox(s.server, "", func() {})

	polled := runAsync(func() { in.read(context.Background()) })
	<-started
	var status int
	asked := runAsync(func() { status, _ = getInbox(t, in, "") })
	select {
	case <-asked:
	case <-time.After(settleWait):
	}
	close(release)
	finished(t, polled, "the poller's read")
	finished(t, asked, "the page's read")

	if status != http.StatusOK || calls.Load() != 1 {
		t.Fatalf("page answered %d after %d list requests; want 200 after the poller's one", status, calls.Load())
	}
}
