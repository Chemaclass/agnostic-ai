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
	visible      bool
	// x and y are where the window is; moves counts SetPosition calls.
	x, y  int
	moves int
	// showsAt, when set, is where the system puts the window when it shows again.
	showsAt *point
}

func newFakeWindow() *fakeWindow {
	return &fakeWindow{visible: true, x: 1000, y: 700}
}

func (w *fakeWindow) Hide() application.Window {
	w.hides++
	w.visible = false
	return w
}

func (w *fakeWindow) Show() application.Window {
	w.shows++
	w.visible = true
	if w.showsAt != nil {
		w.x, w.y = w.showsAt.X, w.showsAt.Y
	}
	return w
}

func (w *fakeWindow) IsVisible() bool      { return w.visible }
func (w *fakeWindow) Position() (int, int) { return w.x, w.y }
func (w *fakeWindow) Size() (int, int)     { return bubbleSide, bubbleSide }
func (w *fakeWindow) SetPosition(x, y int) {
	w.moves++
	w.x, w.y = x, y
}
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

// hideableShell is a shell whose server, once started, is a fake agnostic-ai,
// with stand-in windows. It reports where the stand-in command leaves a mark
// each time it starts.
func hideableShell(t *testing.T) (*shell, *[]appearance, string) {
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
	panel := newFakeWindow()
	panel.visible = false
	s.bubble, s.panel = newFakeWindow(), panel
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

func TestSetHidden_StopsTheServerAndTheReadingAndTellsThePages(t *testing.T) {
	s, told, started := hideableShell(t)
	s.inbox.start()
	waitFor(t, "the first list", func() bool { return len(held(s.inbox)) > 0 })

	if changed := s.setHidden(true); !changed {
		t.Error("hiding a shown companion reported no change")
	}

	if !s.server.resting.Load() || s.server.runningVersion() != "" {
		t.Error("the server still runs while hidden")
	}
	s.inbox.mu.Lock()
	polling := s.inbox.stopPoll != nil
	s.inbox.mu.Unlock()
	if polling {
		t.Error("the poll still runs while hidden")
	}
	if last := (*told)[len(*told)-1]; !last.Hidden {
		t.Errorf("the pages were last told %+v; want hidden", last)
	}
	if !s.appearance().Hidden {
		t.Error("the appearance does not say hidden")
	}
	if n := starts(t, started); n != 1 {
		t.Errorf("the server started %d times; want once, before hiding", n)
	}
	if s.setHidden(true) {
		t.Error("hiding a hidden companion reported a change")
	}
}

func TestSetHidden_TakesTheCircleAndTheListOffTheScreen(t *testing.T) {
	s, _, _ := hideableShell(t)
	bubble, panel := s.bubble.(*fakeWindow), s.panel.(*fakeWindow)

	s.setHidden(true)

	if bubble.visible || panel.visible {
		t.Errorf("circle visible %v, list visible %v; want both off the screen", bubble.visible, panel.visible)
	}
	if bubble.moves != 0 {
		t.Errorf("hiding moved the circle %d times", bubble.moves)
	}
}

func TestSetHidden_ShowingAgainPutsTheCircleWhereItWas(t *testing.T) {
	s, _, _ := hideableShell(t)
	bubble := s.bubble.(*fakeWindow)
	bubble.x, bubble.y = 300, 200

	s.setHidden(true)
	s.setHidden(false)

	if !bubble.visible || bubble.x != 300 || bubble.y != 200 {
		t.Errorf("circle visible %v at %d,%d; want shown at 300,200", bubble.visible, bubble.x, bubble.y)
	}
	if bubble.moves != 0 {
		t.Errorf("the circle was moved %d times though the system kept its place", bubble.moves)
	}
}

func TestSetHidden_ShowingAgainUndoesASystemThatMovedTheCircle(t *testing.T) {
	s, _, _ := hideableShell(t)
	bubble := s.bubble.(*fakeWindow)
	bubble.x, bubble.y = 300, 200
	bubble.showsAt = &point{X: 0, Y: 0}

	s.setHidden(true)
	s.setHidden(false)

	if bubble.x != 300 || bubble.y != 200 {
		t.Errorf("circle at %d,%d; want back at 300,200", bubble.x, bubble.y)
	}
}

func TestSetHidden_IsNeverSaved(t *testing.T) {
	s, _, _ := hideableShell(t)
	s.setHidden(true)
	if err := s.setDimWhenIdle(true); err != nil {
		t.Fatal(err)
	}

	saved, err := readSettings(s.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if saved.appearance().Hidden {
		t.Error("a restart would start hidden")
	}
	data, err := os.ReadFile(s.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "hidden") {
		t.Errorf("the settings file holds the hidden state: %s", data)
	}
}

func TestSetHidden_AListAskedWhileHiddenStartsNoServer(t *testing.T) {
	s, _, started := hideableShell(t)
	s.setHidden(true)
	routes := s.routes(s.inbox, fstest.MapFS{})

	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/inbox", nil))

	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), errResting.Error()) {
		t.Errorf("list while hidden = %d %s", rec.Code, rec.Body)
	}
	if n := starts(t, started); n != 0 {
		t.Errorf("a hidden companion started the server %d times", n)
	}
}

func TestSetHidden_ShowingAgainReadsANewListAndForgetsTheHiddenMessage(t *testing.T) {
	s, told, _ := hideableShell(t)
	s.setHidden(true)
	routes := s.routes(s.inbox, fstest.MapFS{})
	routes.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/inbox", nil))

	if changed := s.setHidden(false); !changed {
		t.Error("showing a hidden companion reported no change")
	}

	s.inbox.mu.Lock()
	problem := s.inbox.problem
	s.inbox.mu.Unlock()
	if problem == errResting.Error() {
		t.Error("the hidden message outlived the hiding")
	}
	if last := (*told)[len(*told)-1]; last.Hidden {
		t.Errorf("the pages were last told %+v; want shown", last)
	}
	waitFor(t, "a new list after showing again", func() bool { return len(held(s.inbox)) > 0 })
	if s.setHidden(false) {
		t.Error("showing a shown companion reported a change")
	}
}

func TestHideRoute_AnswersNoContentAndHides(t *testing.T) {
	s, _, _ := hideableShell(t)
	routes := s.routes(s.inbox, fstest.MapFS{})

	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/hide", nil))

	if rec.Code != http.StatusNoContent || !s.appearance().Hidden {
		t.Errorf("POST /api/hide = %d, hidden %v", rec.Code, s.appearance().Hidden)
	}
	if s.bubble.IsVisible() || s.panel.IsVisible() {
		t.Error("the circle or the list stayed on the screen")
	}
}

func TestShortcut_BringsAHiddenCompanionBackWithItsList(t *testing.T) {
	s, _, _ := hideableShell(t)
	s.setHidden(true)

	s.togglePanel()

	if s.appearance().Hidden || !s.bubble.IsVisible() || !s.panel.IsVisible() {
		t.Errorf("after the shortcut: hidden %v, circle %v, list %v; want shown with the list", s.appearance().Hidden, s.bubble.IsVisible(), s.panel.IsVisible())
	}
	if s.server.resting.Load() {
		t.Error("the server still rests after showing again")
	}
}

func TestSecondStart_BringsAHiddenCompanionBack(t *testing.T) {
	s, _, _ := hideableShell(t)
	s.setHidden(true)

	s.secondStart(application.SecondInstanceData{Args: []string{"companion"}})

	if s.appearance().Hidden || !s.bubble.IsVisible() || !s.panel.IsVisible() {
		t.Errorf("after a second start: hidden %v, circle %v, list %v; want shown with the list", s.appearance().Hidden, s.bubble.IsVisible(), s.panel.IsVisible())
	}
}

func TestOpenSettings_BringsAHiddenCompanionBack(t *testing.T) {
	s, _, _ := hideableShell(t)
	s.setHidden(true)

	s.openSettings()

	if s.appearance().Hidden || !s.bubble.IsVisible() || !s.panel.IsVisible() {
		t.Error("opening the settings left the companion hidden")
	}
}

func TestGetAppearance_SaysWhenHidden(t *testing.T) {
	s, _, _ := hideableShell(t)
	s.setHidden(true)

	rec := httptest.NewRecorder()
	s.getAppearance(rec, httptest.NewRequest(http.MethodGet, "/api/appearance", nil))

	if body := strings.TrimSpace(rec.Body.String()); body != `{"animate":true,"dimWhenIdle":false,"hidden":true}` {
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

func TestServer_ARestWhileAServerStartsStopsThatServer(t *testing.T) {
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
		t.Errorf("address during a rest = %v; want %v", err, errResting)
	}
	<-rested
	waitFor(t, "the new server to stop", exists(stopped))
}

func TestServer_AResumeWhileARestWaitsKeepsTheNewServer(t *testing.T) {
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
	waitFor(t, "the rest to be asked", server.resting.Load)

	server.resume()

	if err := <-answered; err != nil {
		t.Fatalf("address after a resume = %v", err)
	}
	<-rested
	if address, err := server.address(); err != nil || address != "http://127.0.0.1:1" {
		t.Errorf("the rest that waited stopped the new server: %q, %v", address, err)
	}
}

func TestInbox_StartReadsANewListNotOneHeldFromBefore(t *testing.T) {
	// A list answered after hiding stays held; showing again must not send its tag.
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

func TestShowRoute_LeavesAHiddenCompanionHidden(t *testing.T) {
	// The list asks to show itself again when an open fails, which can be
	// long after hiding. Only the shortcut, a second start or the settings item brings it back.
	s, told, _ := hideableShell(t)
	panel := s.panel.(*fakeWindow)
	s.setHidden(true)
	toldBefore := len(*told)
	routes := s.routes(s.inbox, fstest.MapFS{})

	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/panel/show", nil))

	if rec.Code != http.StatusNoContent || panel.shows != 0 || s.bubble.IsVisible() {
		t.Errorf("POST /api/panel/show = %d, list shown %d times; want 204 and no list", rec.Code, panel.shows)
	}
	if !s.appearance().Hidden || !s.server.resting.Load() || len(*told) != toldBefore {
		t.Error("showing the list brought a hidden companion back")
	}
}
