package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// fakeWindow counts what the shell asks of a window; anything else panics.
// mu lets a test watch a window the shell changes from another goroutine.
type fakeWindow struct {
	application.Window
	mu           sync.Mutex
	hides, shows int
	visible      bool
	focused      bool
	// x and y are where the window is; moves counts SetPosition calls.
	x, y  int
	moves int
	// showsAt, when set, is where the system puts the window when it shows again.
	showsAt *point
	// reportedMoves counts the moves the window system reports later.
	reportedMoves int
}

func newFakeWindow() *fakeWindow {
	return &fakeWindow{visible: true, x: 1000, y: 700}
}

func (w *fakeWindow) Hide() application.Window {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.hides++
	w.visible = false
	return w
}

func (w *fakeWindow) Show() application.Window {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.shows++
	w.visible = true
	if w.showsAt != nil {
		w.x, w.y = w.showsAt.X, w.showsAt.Y
		w.reportedMoves++
	}
	return w
}

func (w *fakeWindow) IsVisible() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.visible
}

func (w *fakeWindow) Focus() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.focused = true
}

func (w *fakeWindow) Position() (int, int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.x, w.y
}

func (w *fakeWindow) Size() (int, int) { return bubbleSide, bubbleSide }
func (w *fakeWindow) SetPosition(x, y int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.moves++
	w.reportedMoves++
	w.x, w.y = x, y
}
func (w *fakeWindow) EmitEvent(string, ...any) bool { return true }
func (w *fakeWindow) GetScreen() (*application.Screen, error) {
	return &application.Screen{WorkArea: application.Rect{Width: 1440, Height: 900}}, nil
}

// placedShell is a shell with stand-in windows, the list closed, that
// records what the pages are told.
func placedShell(t *testing.T) (*shell, *[]appearance) {
	t.Helper()
	s := shellWith(t, &fakeShortcuts{})
	s.server = newServer()
	s.inbox = newInbox(s.server, "", func() {})
	panel := newFakeWindow()
	panel.visible = false
	s.bubble, s.panel = newFakeWindow(), panel
	s.screens = func() []*application.Screen {
		return []*application.Screen{
			{IsPrimary: true, WorkArea: application.Rect{Width: 1440, Height: 900}},
			{WorkArea: application.Rect{X: 1440, Width: 1920, Height: 1080}},
		}
	}
	var mu sync.Mutex
	told := []appearance{}
	s.tell = func(looks appearance) {
		mu.Lock()
		defer mu.Unlock()
		told = append(told, looks)
	}
	return s, &told
}

// listWaiting is a list where the conversations named by ids wait for the person.
func listWaiting(count int, ids ...string) []byte {
	rows := []string{`{"tool":"claude","id":"calm","state":"active"}`}
	for _, id := range ids {
		rows = append(rows, `{"tool":"claude","id":"`+id+`","state":"blocked"}`)
	}
	return []byte(`{"needsYou":` + strconv.Itoa(count) + `,"conversations":[` + strings.Join(rows, ",") + `]}`)
}

func TestPresence_StartsShown(t *testing.T) {
	s, _ := placedShell(t)
	if got := s.appearance().Presence; got != shown {
		t.Errorf("presence = %q; want shown", got)
	}
}

func TestHide_TakesTheCircleAndTheListOffTheScreenAndTellsThePages(t *testing.T) {
	s, told := placedShell(t)
	bubble := s.bubble.(*fakeWindow)
	s.panel.Show()

	if !s.setPresence(hidden) {
		t.Error("hiding a shown companion reported no change")
	}

	if bubble.IsVisible() || s.panel.IsVisible() {
		t.Errorf("circle visible %v, list visible %v; want both off the screen", bubble.IsVisible(), s.panel.IsVisible())
	}
	if bubble.moves != 0 {
		t.Errorf("hiding moved the circle %d times", bubble.moves)
	}
	if last := (*told)[len(*told)-1]; last.Presence != hidden {
		t.Errorf("the pages were last told %+v; want hidden", last)
	}
	if s.setPresence(hidden) {
		t.Error("hiding a hidden companion reported a change")
	}
}

func TestHide_KeepsReading(t *testing.T) {
	s, _, _ := hideableShell(t)
	s.inbox.start()
	waitFor(t, "the first list", func() bool { return len(s.inbox.held()) > 0 })

	s.setPresence(hidden)

	if s.server.resting.Load() {
		t.Error("the server stopped when the companion hid")
	}
	s.inbox.mu.Lock()
	polling := s.inbox.stopPoll != nil
	s.inbox.mu.Unlock()
	if !polling || len(s.inbox.held()) == 0 {
		t.Error("the reading stopped when the companion hid")
	}
}

func TestShowingAgain_PutsTheCircleWhereItWas(t *testing.T) {
	s, _ := placedShell(t)
	bubble := s.bubble.(*fakeWindow)
	bubble.x, bubble.y = 300, 200

	s.setPresence(hidden)
	s.comeBack()

	if !bubble.IsVisible() || bubble.x != 300 || bubble.y != 200 {
		t.Errorf("circle visible %v at %d,%d; want shown at 300,200", bubble.IsVisible(), bubble.x, bubble.y)
	}
	if bubble.moves != 0 {
		t.Errorf("the circle was moved %d times though the system kept its place", bubble.moves)
	}
}

func TestShowingAgain_UndoesASystemThatMovedTheCircle(t *testing.T) {
	s, _ := placedShell(t)
	bubble := s.bubble.(*fakeWindow)
	bubble.x, bubble.y = 300, 200
	bubble.showsAt = &point{X: 0, Y: 0}

	s.setPresence(hidden)
	s.comeBack()

	if bubble.x != 300 || bubble.y != 200 {
		t.Errorf("circle at %d,%d; want back at 300,200", bubble.x, bubble.y)
	}
}

func TestPresence_IsNeverSaved(t *testing.T) {
	for _, p := range []presence{hidden, fadedBack} {
		t.Run(string(p), func(t *testing.T) {
			s, _ := placedShell(t)
			s.listRead(listWaiting(0))
			s.setPresence(hidden)
			if p == fadedBack {
				s.listRead(listWaiting(1, "new"))
			}
			if got := s.presenceNow(); got != p {
				t.Fatalf("presence = %q; want %q", got, p)
			}
			if err := s.setDimWhenIdle(true); err != nil {
				t.Fatal(err)
			}

			data, err := os.ReadFile(s.settingsPath)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "presence") || strings.Contains(string(data), string(p)) {
				t.Errorf("the settings file holds the presence: %s", data)
			}
			saved, err := readSettings(s.settingsPath)
			if err != nil {
				t.Fatal(err)
			}
			if saved.appearance().Presence != shown {
				t.Error("a restart would not start shown")
			}
		})
	}
}

func TestHidden_ANewWaitingConversationBringsTheCircleBackFadedWithoutTheList(t *testing.T) {
	s, told := placedShell(t)
	bubble, panel := s.bubble.(*fakeWindow), s.panel.(*fakeWindow)
	s.listRead(listWaiting(1, "old"))
	s.setPresence(hidden)

	s.listRead(listWaiting(2, "old", "new"))

	if got := s.presenceNow(); got != fadedBack {
		t.Fatalf("presence = %q; want faded", got)
	}
	if !bubble.IsVisible() || bubble.focused {
		t.Errorf("circle visible %v, focused %v; want shown without the keys", bubble.IsVisible(), bubble.focused)
	}
	if panel.IsVisible() || panel.focused {
		t.Error("the list opened or took the keys")
	}
	if last := (*told)[len(*told)-1]; last.Presence != fadedBack {
		t.Errorf("the pages were last told %+v; want faded", last)
	}
}

func TestHidden_ConversationsAlreadyWaitingKeepItHidden(t *testing.T) {
	s, _ := placedShell(t)
	s.listRead(listWaiting(2, "a", "b"))
	s.setPresence(hidden)

	s.listRead(listWaiting(2, "a", "b"))
	s.listRead(listWaiting(1, "a"))

	if got := s.presenceNow(); got != hidden {
		t.Errorf("presence = %q; want hidden", got)
	}
}

func TestHidden_OneWaitingWhileAnotherStopsStillBringsItBack(t *testing.T) {
	s, _ := placedShell(t)
	s.listRead(listWaiting(1, "a"))
	s.setPresence(hidden)

	s.listRead(listWaiting(1, "b"))

	if got := s.presenceNow(); got != fadedBack {
		t.Errorf("presence = %q; want faded: b started waiting as a stopped", got)
	}
}

// listWith is a list where the conversations named by ids wait, with extra fields such as a note.
func listWith(extra string, ids ...string) []byte {
	list := string(listWaiting(len(ids), ids...))
	return []byte(`{` + extra + `,` + list[1:])
}

func TestHidden_AListThatReadLessIsANewBaselineNotNews(t *testing.T) {
	for name, gap := range map[string]string{
		"marks":       `"marksNote":"could not read the marks"`,
		"cloud":       `"cloudNote":"no claude.ai sign-in"`,
		"cursor read": `"tools":[{"tool":"cursor","found":false,"note":"locked"}]`,
	} {
		t.Run(name, func(t *testing.T) {
			s, _ := placedShell(t)
			s.listRead(listWaiting(1, "a"))
			s.setPresence(hidden)

			s.listRead(listWith(gap))
			s.listRead(listWaiting(1, "a"))

			if got := s.presenceNow(); got != hidden {
				t.Errorf("presence = %q; want hidden: the full list again is nothing new", got)
			}
		})
	}
}

func TestHidden_AStandingNoteStillLetsANewWaitBringItBack(t *testing.T) {
	note := `"cloudNote":"no claude.ai sign-in"`
	s, _ := placedShell(t)
	s.listRead(listWith(note, "a"))
	s.setPresence(hidden)

	s.listRead(listWith(note, "a", "b"))

	if got := s.presenceNow(); got != fadedBack {
		t.Errorf("presence = %q; want faded: b started waiting under the same note", got)
	}
}

func TestHidden_AnUnreadableListChangesNothing(t *testing.T) {
	s, _ := placedShell(t)
	s.listRead(listWaiting(1, "a"))
	s.setPresence(hidden)

	s.listRead(nil)
	s.listRead([]byte(`not json`))
	s.listRead(listWaiting(1, "a"))

	if got := s.presenceNow(); got != hidden {
		t.Errorf("presence = %q; want hidden", got)
	}
}

func TestHidden_TheFirstListAfterAStartDoesNotBringItBack(t *testing.T) {
	s, _ := placedShell(t)
	s.setPresence(hidden)

	s.listRead(listWaiting(3, "a", "b", "c"))

	if got := s.presenceNow(); got != hidden {
		t.Errorf("presence = %q; want hidden: nothing was known to wait before", got)
	}
}

func TestShown_ANewWaitingConversationChangesNothing(t *testing.T) {
	s, _ := placedShell(t)
	s.listRead(listWaiting(0))

	s.listRead(listWaiting(1, "new"))

	if got := s.presenceNow(); got != shown {
		t.Errorf("presence = %q; want shown", got)
	}
}

func TestFaded_AClickOrTheShortcutRestoresFullColorWithTheList(t *testing.T) {
	s, told := placedShell(t)
	s.listRead(listWaiting(0))
	s.setPresence(hidden)
	s.listRead(listWaiting(1, "new"))

	s.togglePanel()

	if got := s.presenceNow(); got != shown {
		t.Errorf("presence = %q; want shown", got)
	}
	if !s.panel.IsVisible() {
		t.Error("the list did not open")
	}
	if last := (*told)[len(*told)-1]; last.Presence != shown {
		t.Errorf("the pages were last told %+v; want shown", last)
	}
	s.listRead(listWaiting(2, "new", "newer"))
	if got := s.presenceNow(); got != shown {
		t.Errorf("presence = %q after more waiting; want it to stay shown", got)
	}
}

func TestFaded_HidingAgainHidesIt(t *testing.T) {
	s, _ := placedShell(t)
	s.listRead(listWaiting(0))
	s.setPresence(hidden)
	s.listRead(listWaiting(1, "new"))

	s.hide()

	if got := s.presenceNow(); got != hidden || s.bubble.IsVisible() {
		t.Errorf("presence = %q, circle visible %v; want hidden", got, s.bubble.IsVisible())
	}
	s.listRead(listWaiting(1, "new"))
	if got := s.presenceNow(); got != hidden {
		t.Errorf("presence = %q; the same waiting brought it back again", got)
	}
}

func TestHideRoute_AnswersNoContentAndHides(t *testing.T) {
	s, _ := placedShell(t)
	routes := s.routes(s.inbox, fstest.MapFS{})

	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/hide", nil))

	if rec.Code != http.StatusNoContent || s.presenceNow() != hidden {
		t.Errorf("POST /api/hide = %d, presence %q", rec.Code, s.presenceNow())
	}
	if s.bubble.IsVisible() || s.panel.IsVisible() {
		t.Error("the circle or the list stayed on the screen")
	}
}

func TestShortcut_BringsAHiddenCompanionBackWithItsList(t *testing.T) {
	s, _ := placedShell(t)
	s.setPresence(hidden)

	s.togglePanel()

	if s.presenceNow() != shown || !s.bubble.IsVisible() || !s.panel.IsVisible() {
		t.Errorf("after the shortcut: presence %q, circle %v, list %v; want shown with the list", s.presenceNow(), s.bubble.IsVisible(), s.panel.IsVisible())
	}
}

func TestSecondStart_BringsAHiddenCompanionBack(t *testing.T) {
	s, _ := placedShell(t)
	s.setPresence(hidden)

	s.secondStart(application.SecondInstanceData{Args: []string{"companion"}})

	if s.presenceNow() != shown || !s.bubble.IsVisible() || !s.panel.IsVisible() {
		t.Errorf("after a second start: presence %q, circle %v, list %v; want shown with the list", s.presenceNow(), s.bubble.IsVisible(), s.panel.IsVisible())
	}
}

func TestShortcut_KeepsTheListOpenThoughTheReturningCircleReportsMoves(t *testing.T) {
	s, _ := placedShell(t)
	bubble := s.bubble.(*fakeWindow)
	bubble.showsAt = &point{X: 0, Y: 0}
	s.setPresence(hidden)

	s.togglePanel()
	for range bubble.reportedMoves {
		s.bubbleMoved()
	}

	if !s.panel.IsVisible() {
		t.Error("the circle's return closed the list the shortcut opened")
	}
}

func TestBubbleMoved_StillClosesTheListOnceTheReturnSettled(t *testing.T) {
	s, _ := placedShell(t)
	s.setPresence(hidden)
	s.togglePanel()
	s.mu.Lock()
	s.shownAgainAt = time.Now().Add(-returnSettle)
	s.mu.Unlock()

	s.bubbleMoved()

	if s.panel.IsVisible() {
		t.Error("dragging the circle left the list open")
	}
}

func TestAppReopened_BringsAHiddenCompanionBackInsteadOfTheSystem(t *testing.T) {
	s, _ := placedShell(t)
	s.setPresence(hidden)
	cancelled := false

	s.appReopened(func() { cancelled = true })

	if !cancelled {
		t.Error("the system was left to show the windows itself")
	}
	waitFor(t, "the companion to come back", func() bool { return s.presenceNow() == shown })
	waitFor(t, "the list to open", s.panel.IsVisible)
}

func TestAppReopened_LeavesAShownCompanionToTheSystem(t *testing.T) {
	s, _ := placedShell(t)
	cancelled := false

	s.appReopened(func() { cancelled = true })

	if cancelled {
		t.Error("a reopen of a shown companion was cancelled")
	}
}

func TestGetAppearance_SaysThePresence(t *testing.T) {
	s, _ := placedShell(t)
	s.setPresence(hidden)

	rec := httptest.NewRecorder()
	s.getAppearance(rec, httptest.NewRequest(http.MethodGet, "/api/appearance", nil))

	if body := strings.TrimSpace(rec.Body.String()); body != `{"animate":true,"dimWhenIdle":false,"presence":"hidden"}` {
		t.Errorf("appearance = %s", body)
	}
}

func TestShowRoute_LeavesAHiddenOrFadedCompanionAlone(t *testing.T) {
	// The list asks to show itself again when an open fails, which can be
	// long after hiding. Only the shortcut, a click or a second start brings it back.
	s, told := placedShell(t)
	s.setPresence(hidden)
	toldBefore := len(*told)
	routes := s.routes(s.inbox, fstest.MapFS{})

	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/panel/show", nil))

	if rec.Code != http.StatusNoContent || s.panel.IsVisible() || s.bubble.IsVisible() {
		t.Errorf("POST /api/panel/show = %d, list %v; want 204 and nothing shown", rec.Code, s.panel.IsVisible())
	}
	if s.presenceNow() != hidden || len(*told) != toldBefore {
		t.Error("showing the list brought a hidden companion back")
	}
}

func TestWaiting_Gained(t *testing.T) {
	ab := waiting{ids: map[string]bool{"a": true, "b": true}}
	cases := []struct {
		name string
		now  waiting
		want bool
	}{
		{"same", ab, false},
		{"one fewer", waiting{ids: map[string]bool{"a": true}}, false},
		{"one more", waiting{ids: map[string]bool{"a": true, "b": true, "c": true}}, true},
		{"one swapped", waiting{ids: map[string]bool{"a": true, "c": true}}, true},
		{"one more, read differently", waiting{ids: map[string]bool{"a": true, "b": true, "c": true}, gaps: "cloud:down"}, false},
	}
	for _, tc := range cases {
		if got := tc.now.gained(ab); got != tc.want {
			t.Errorf("%s: gained = %v; want %v", tc.name, got, tc.want)
		}
	}
}

func TestWaitingIn_ReadsBlockedAndUnreadRows(t *testing.T) {
	got, ok := waitingIn([]byte(`{"needsYou":5,"conversations":[{"tool":"claude","id":"a","state":"blocked"},{"tool":"codex","id":"b","state":"unread"},{"tool":"claude","id":"c","state":"working"}]}`))
	if !ok || len(got.ids) != 2 || !got.ids["claude:a"] || !got.ids["codex:b"] {
		t.Errorf("waitingIn = %+v, %v", got, ok)
	}
}

func TestShowingAgain_KeepsACircleHiddenOnASecondScreenThere(t *testing.T) {
	s, _ := placedShell(t)
	bubble := s.bubble.(*fakeWindow)
	bubble.x, bubble.y = 2000, 400

	s.setPresence(hidden)
	s.comeBack()

	if bubble.x != 2000 || bubble.y != 400 {
		t.Errorf("circle at %d,%d; want back at 2000,400 on the second screen", bubble.x, bubble.y)
	}
}

func TestShowingAgain_PutsACircleNoScreenShowsAtTheDefaultPlace(t *testing.T) {
	s, _ := placedShell(t)
	bubble := s.bubble.(*fakeWindow)
	bubble.x, bubble.y = 9000, 9000

	s.setPresence(hidden)
	s.comeBack()

	wantX, wantY := defaultBubblePosition(rect{Width: 1440, Height: 900}, bubbleSide)
	if bubble.x != wantX || bubble.y != wantY {
		t.Errorf("circle at %d,%d; want the default place %d,%d", bubble.x, bubble.y, wantX, wantY)
	}
}

func TestAppReopened_BringsAFadedCompanionBackToFullColor(t *testing.T) {
	s, _ := placedShell(t)
	s.listRead(listWaiting(0))
	s.setPresence(hidden)
	s.listRead(listWaiting(1, "new"))

	s.appReopened(func() {})

	waitFor(t, "full color", func() bool { return s.presenceNow() == shown })
	waitFor(t, "the list to open", s.panel.IsVisible)
}

// hideableShell is a placedShell whose server, once started, is a fake agnostic-ai.
func hideableShell(t *testing.T) (*shell, *[]appearance, string) {
	t.Helper()
	fake := httptest.NewServer(&fakeAgnosticAI{})
	t.Cleanup(fake.Close)
	started := t.TempDir() + "/started"
	fakeCommand(t, `echo x >> "`+started+`"
echo "Serving agnostic-ai at `+fake.URL+`/"
cat > /dev/null`)
	s, told := placedShell(t)
	t.Cleanup(func() {
		s.inbox.stop()
		s.server.rest()
	})
	return s, told, started
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
