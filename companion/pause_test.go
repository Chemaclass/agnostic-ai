package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// fakeWindow counts what the shell asks of a window; anything else panics.
type fakeWindow struct {
	application.Window
	hides, shows int
}

func (w *fakeWindow) Hide() application.Window {
	w.hides++
	return w
}

func (w *fakeWindow) Show() application.Window {
	w.shows++
	return w
}

func (w *fakeWindow) Position() (int, int)          { return 1000, 700 }
func (w *fakeWindow) Size() (int, int)              { return bubbleSide, bubbleSide }
func (w *fakeWindow) SetPosition(int, int)          {}
func (w *fakeWindow) Focus()                        {}
func (w *fakeWindow) EmitEvent(string, ...any) bool { return true }
func (w *fakeWindow) GetScreen() (*application.Screen, error) {
	return &application.Screen{WorkArea: application.Rect{Width: 1440, Height: 900}}, nil
}

// held is the list the inbox holds now.
func held(in *inbox) []byte {
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.latest
}

// pausableShell is a shell whose server, once started, is a fake agnostic-ai.
// It reports where the stand-in command leaves a mark each time it starts.
func pausableShell(t *testing.T) (*shell, *[]appearance, string) {
	t.Helper()
	fake := httptest.NewServer(&fakeAgnosticAI{})
	t.Cleanup(fake.Close)
	started := filepath.Join(t.TempDir(), "started")
	fakeCommand(t, `echo x >> "`+started+`"
echo "Serving agnostic-ai at `+fake.URL+`/"
cat > /dev/null`)
	s := shellWith(t, &fakeShortcuts{})
	s.server = newServer()
	s.inbox = newInbox(s.server, "", func() {})
	var mu sync.Mutex
	told := []appearance{}
	s.tell = func(looks appearance) {
		mu.Lock()
		defer mu.Unlock()
		told = append(told, looks)
	}
	t.Cleanup(func() {
		s.inbox.stop()
		s.server.rest()
	})
	return s, &told, started
}

func starts(t *testing.T, mark string) int {
	t.Helper()
	data, err := os.ReadFile(mark)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(data), "x")
}

func TestSetPaused_StopsTheServerAndTheReadingAndTellsThePages(t *testing.T) {
	s, told, started := pausableShell(t)
	s.inbox.start()
	waitFor(t, "the first list", func() bool { return len(held(s.inbox)) > 0 })

	if changed := s.setPaused(true); !changed {
		t.Error("pausing an awake companion reported no change")
	}

	if !s.server.resting.Load() || s.server.runningVersion() != "" {
		t.Error("the server still runs while paused")
	}
	s.inbox.mu.Lock()
	polling := s.inbox.stopPoll != nil
	s.inbox.mu.Unlock()
	if polling {
		t.Error("the poll still runs while paused")
	}
	if last := (*told)[len(*told)-1]; !last.Paused {
		t.Errorf("the pages were last told %+v; want paused", last)
	}
	if !s.appearance().Paused {
		t.Error("the appearance does not say paused")
	}
	if n := starts(t, started); n != 1 {
		t.Errorf("the server started %d times; want once, before the pause", n)
	}
	if s.setPaused(true) {
		t.Error("pausing a paused companion reported a change")
	}
}

func TestSetPaused_AListAskedWhilePausedStartsNoServer(t *testing.T) {
	s, _, started := pausableShell(t)
	s.setPaused(true)
	routes := s.routes(s.inbox, fstest.MapFS{})

	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/inbox", nil))

	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), errResting.Error()) {
		t.Errorf("list while paused = %d %s", rec.Code, rec.Body)
	}
	if n := starts(t, started); n != 0 {
		t.Errorf("a paused companion started the server %d times", n)
	}
}

func TestSetPaused_WakingReadsANewListAndForgetsThePausedMessage(t *testing.T) {
	s, told, _ := pausableShell(t)
	s.setPaused(true)
	routes := s.routes(s.inbox, fstest.MapFS{})
	routes.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/inbox", nil))

	if changed := s.setPaused(false); !changed {
		t.Error("waking a paused companion reported no change")
	}

	s.inbox.mu.Lock()
	problem := s.inbox.problem
	s.inbox.mu.Unlock()
	if problem == errResting.Error() {
		t.Error("the paused message outlived the pause")
	}
	if last := (*told)[len(*told)-1]; last.Paused {
		t.Errorf("the pages were last told %+v; want awake", last)
	}
	waitFor(t, "a new list after waking", func() bool { return len(held(s.inbox)) > 0 })
	if s.setPaused(false) {
		t.Error("waking an awake companion reported a change")
	}
}

func TestPauseRoute_AnswersNoContentAndPauses(t *testing.T) {
	s, _, _ := pausableShell(t)
	panel, bubble := &fakeWindow{}, &fakeWindow{}
	s.panel, s.bubble = panel, bubble
	routes := s.routes(s.inbox, fstest.MapFS{})

	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/pause", nil))

	if rec.Code != http.StatusNoContent || !s.appearance().Paused {
		t.Errorf("POST /api/pause = %d, paused %v", rec.Code, s.appearance().Paused)
	}
	if panel.hides != 1 || bubble.hides != 0 {
		t.Errorf("hid the list %d times and the circle %d times; want the list once and the circle never", panel.hides, bubble.hides)
	}
}

func TestGetAppearance_SaysWhenPaused(t *testing.T) {
	s, _, _ := pausableShell(t)
	s.setPaused(true)

	rec := httptest.NewRecorder()
	s.getAppearance(rec, httptest.NewRequest(http.MethodGet, "/api/appearance", nil))

	if body := strings.TrimSpace(rec.Body.String()); body != `{"animate":true,"dimWhenIdle":false,"paused":true}` {
		t.Errorf("appearance = %s", body)
	}
}

// slowServer is a stand-in agnostic-ai that marks its start, takes a
// moment to listen, and marks its end once its input closes.
func slowServer(t *testing.T) (began, stopped string) {
	t.Helper()
	folder := t.TempDir()
	began, stopped = filepath.Join(folder, "began"), filepath.Join(folder, "stopped")
	fakeCommand(t, `touch "`+began+`"
sleep 0.3
echo "Serving agnostic-ai at http://127.0.0.1:1/"
cat > /dev/null
touch "`+stopped+`"`)
	return began, stopped
}

func exists(path string) func() bool {
	return func() bool {
		_, err := os.Stat(path)
		return err == nil
	}
}

func TestServer_APauseWhileAServerStartsStopsThatServer(t *testing.T) {
	began, stopped := slowServer(t)
	server := newServer()
	answered := make(chan error, 1)
	go func() {
		_, err := server.address()
		answered <- err
	}()
	waitFor(t, "the start to begin", exists(began))

	rested := make(chan struct{})
	go func() {
		server.rest()
		close(rested)
	}()

	if err := <-answered; err != errResting {
		t.Errorf("address during a pause = %v; want %v", err, errResting)
	}
	<-rested
	waitFor(t, "the new server to stop", exists(stopped))
}

func TestServer_AWakeWhileAPauseWaitsKeepsTheNewServer(t *testing.T) {
	began, _ := slowServer(t)
	server := newServer()
	t.Cleanup(server.rest)
	answered := make(chan error, 1)
	go func() {
		_, err := server.address()
		answered <- err
	}()
	waitFor(t, "the start to begin", exists(began))
	rested := make(chan struct{})
	go func() {
		server.rest()
		close(rested)
	}()
	waitFor(t, "the pause to be asked", server.resting.Load)

	server.resume()

	if err := <-answered; err != nil {
		t.Fatalf("address after a wake = %v", err)
	}
	<-rested
	if address, err := server.address(); err != nil || address != "http://127.0.0.1:1" {
		t.Errorf("the pause that waited stopped the new server: %q, %v", address, err)
	}
}

func TestInbox_StartReadsANewListNotOneHeldFromBefore(t *testing.T) {
	// A list answered after a pause stays held; the wake must not send its tag.
	in, _ := connected(t, &fakeAgnosticAI{})
	var changes atomic.Int32
	in.changed = func() { changes.Add(1) }
	in.latest, in.tag = []byte(`{"needsYou":9}`), `"v1"`
	t.Cleanup(in.stop)

	in.start()

	waitFor(t, "the pages to be told of a new list", func() bool { return changes.Load() > 0 })
	if got := string(held(in)); got != `{"needsYou":2}` {
		t.Errorf("held %s; want a new read", got)
	}
}

func TestShowRoute_WakesAPausedCompanion(t *testing.T) {
	// The list shows itself again after an open failed; over a paused
	// companion every call would answer that it is paused.
	s, told, _ := pausableShell(t)
	panel, bubble := &fakeWindow{}, &fakeWindow{}
	s.panel, s.bubble = panel, bubble
	s.setPaused(true)
	routes := s.routes(s.inbox, fstest.MapFS{})

	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/panel/show", nil))

	if rec.Code != http.StatusNoContent || panel.shows != 1 {
		t.Errorf("POST /api/panel/show = %d, list shown %d times", rec.Code, panel.shows)
	}
	if s.appearance().Paused || s.server.resting.Load() {
		t.Error("the list showed over a paused companion")
	}
	if last := (*told)[len(*told)-1]; last.Paused {
		t.Errorf("the pages were last told %+v; want awake", last)
	}
}
