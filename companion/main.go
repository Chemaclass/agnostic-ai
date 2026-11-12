// The agnostic-ai companion: a small circle that floats over every app and
// counts the AI conversations that need the person, with a searchable
// list beside it that opens each one where it lives. It is a window on
// `agnostic-ai serve`: the server reads, opens and marks the
// conversations, and the companion asks it.
package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// version is the agnostic-ai release this companion was built for, set
// by `agnostic-ai companion install`; empty in a build made by hand.
var version string

// What `agnostic-ai companion` and its subcommands start the companion
// with, as its first argument.
const (
	// askVersion prints the release this companion was built for.
	askVersion = "--version"
	// askQuit ends the companion that runs.
	askQuit = "--quit"
	// askLoginOff takes the companion out of what starts at login.
	askLoginOff = "--disable-start-at-login"
	// askLoginOn puts the companion in what starts at login. An install
	// uses it to keep start at login on when the program moves.
	askLoginOn = "--enable-start-at-login"
)

// notRunningExit is how a companion started with askQuit ends when no
// other companion runs. One that runs is told to quit, and the asking
// one then ends with 0.
const notRunningExit = 3

// asked returns which of those a companion was started with; empty for
// a plain start. args are the program's own, its path first.
func asked(args []string) string {
	if len(args) < 2 {
		return ""
	}
	switch args[1] {
	case askVersion, askQuit, askLoginOff, askLoginOn:
		return args[1]
	}
	return ""
}

//go:embed frontend
var embedded embed.FS

const (
	// bubbleSide is the bubble window: the 60-point circle and room for
	// its shadow and badge.
	bubbleSide = 84
	// panelWidth and panelHeight are the list window: the card and room
	// for its shadow.
	panelWidth  = 432
	panelHeight = 624
)

// instanceID names the companion to the system, so a second start finds the first.
const instanceID = "dev.agnostic-ai.companion"

// Events the pages listen for.
const (
	// eventInbox says the latest list changed; pages read it again.
	eventInbox = "inbox"
	// eventPanelShown says the panel opened; its data is the corner nearest the bubble.
	eventPanelShown = "panel-shown"
	// eventAppearance says how the pages look changed; its data is the new appearance.
	eventAppearance = "appearance"
	// eventPointer says the pointer entered or left the bubble window; its data is true while it is over it.
	eventPointer = "pointer"
	// eventPreferences says the shortcut or the login setting changed; its data is the new preferences.
	eventPreferences = "preferences"
	// eventSettingsShown says the panel opened on its settings; its data is the corner nearest the bubble.
	eventSettingsShown = "settings-shown"
	// eventQuitAsked says the panel opened on the question whether to quit; its data is the corner nearest the bubble.
	eventQuitAsked = "quit-asked"
)

// reopenGuard is how soon after the panel hid on losing focus a click
// on the bubble counts as that same dismissal. On systems where the
// click itself takes the focus, the panel hides first and the click
// arrives after; without the guard it would open again.
const reopenGuard = 250 * time.Millisecond

// returnSettle is how long after a hidden companion comes back the
// circle's moves leave its list open: the window system may still be
// placing it.
const returnSettle = 500 * time.Millisecond

// savePause is how long the bubble must rest before its place is saved.
const savePause = 500 * time.Millisecond

// shell owns the two windows.
type shell struct {
	app    *application.App
	bubble application.Window
	panel  application.Window
	inbox  *inbox
	server *server

	settingsPath string

	// shortcuts takes and gives back the keys that open the list from anywhere.
	shortcuts shortcuts
	// login is the system's start at login setting.
	login           autostart
	tell            func(appearance)
	tellPreferences func(preferences)
	end             func()
	endOnce         sync.Once
	// placing keeps two presence changes from interleaving their window
	// changes. It guards hiddenAt and waitingSeen.
	placing sync.Mutex
	// hiddenAt is where the circle was when it was hidden.
	hiddenAt point
	// waitingSeen is what waited in the last list read; known is false
	// until one was read.
	waitingSeen      waiting
	waitingSeenKnown bool
	// native holds the right-click menu's items by their id in menu.json.
	// nativeMu keeps two refreshes from interleaving.
	native   map[string]*application.MenuItem
	nativeMu sync.Mutex
	// hideBase is the Hide item's label in menu.json, before the way back.
	// nativeMu guards it too.
	hideBase string

	// mu guards the fields below. A change to the settings holds it
	// through the write to disk, so two changes never write at once and
	// an older snapshot never lands after a newer one.
	mu sync.Mutex
	// settings is what is saved; every write saves all of it.
	settings settings
	// shortcutProblem says why the shortcut is not active; empty when it is.
	shortcutProblem string
	// hiddenByFocusAt is when the panel last hid because it lost focus.
	hiddenByFocusAt time.Time
	save            *time.Timer
	// presence is how the companion sits on the screen; the zero value is shown.
	presence presence
	// shownAgainAt is when a hidden companion last came back.
	shownAgainAt time.Time
}

// presenceNow is the companion's presence, shown before any change.
func (s *shell) presenceNow() presence {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.presenceLocked()
}

func (s *shell) presenceLocked() presence {
	if s.presence == "" {
		return shown
	}
	return s.presence
}

// hide takes the circle and the list off the screen, until the shortcut
// or a second start brings them back, or something new waits.
func (s *shell) hide() {
	s.setPresence(hidden)
}

// setPresence puts the windows where to says and tells the pages. It
// reports whether the presence changed.
func (s *shell) setPresence(to presence) bool {
	return s.changePresence("", to)
}

// changePresence is setPresence, only while the presence is from; an
// empty from means any.
func (s *shell) changePresence(from, to presence) bool {
	s.placing.Lock()
	defer s.placing.Unlock()
	s.mu.Lock()
	was := s.presenceLocked()
	if was == to || (from != "" && was != from) {
		s.mu.Unlock()
		return false
	}
	s.presence = to
	if was == hidden {
		s.shownAgainAt = time.Now()
	}
	looks := s.settings.appearance()
	looks.Presence = to
	s.mu.Unlock()
	s.tell(looks)
	switch {
	case to == hidden:
		x, y := s.bubble.Position()
		s.hiddenAt = point{X: x, Y: y}
		s.panel.Hide()
		s.bubble.Hide()
	case was == hidden:
		s.bubble.Show()
		s.placeReturningBubble()
	}
	s.refreshNative()
	return true
}

// placeReturningBubble puts a circle shown again where it was hidden.
// Some window systems place a window again when it shows; a place no
// screen shows any more, such as on a display unplugged meanwhile,
// gives way to the default one.
func (s *shell) placeReturningBubble() {
	area := s.workArea()
	if !onScreen(rect{X: s.hiddenAt.X, Y: s.hiddenAt.Y, Width: bubbleSide, Height: bubbleSide}, []rect{area}) {
		x, y := defaultBubblePosition(area, bubbleSide)
		s.bubble.SetPosition(x, y)
		return
	}
	if x, y := s.bubble.Position(); x != s.hiddenAt.X || y != s.hiddenAt.Y {
		s.bubble.SetPosition(s.hiddenAt.X, s.hiddenAt.Y)
	}
}

// listRead notes what waits in a new list. While hidden, something new
// waiting brings the circle back faded, without the list or the keys.
func (s *shell) listRead(list json.RawMessage) {
	now, ok := waitingIn(list)
	if !ok {
		return
	}
	s.placing.Lock()
	before, known := s.waitingSeen, s.waitingSeenKnown
	s.waitingSeen, s.waitingSeenKnown = now, true
	s.placing.Unlock()
	if known && now.gained(before) {
		s.changePresence(hidden, fadedBack)
	}
}

// quitRestWait is how long the companion waits for its server to stop
// before it ends. A server still starting holds it longer; it ends with
// this process anyway.
const quitRestWait = time.Second

// restBriefly stops the server, waiting for it at most quitRestWait.
func (s *shell) restBriefly() {
	rested := make(chan struct{})
	go func() {
		s.server.rest()
		close(rested)
	}()
	select {
	case <-rested:
	case <-time.After(quitRestWait):
	}
}

// quit stops the server, then ends the app once, however many ask.
func (s *shell) quit() {
	s.restBriefly()
	s.endOnce.Do(s.end)
}

// comeBack puts a hidden or faded companion back at full color. It
// reports whether it was hidden or faded.
func (s *shell) comeBack() bool {
	return s.setPresence(shown)
}

// secondStart answers a companion started while this one runs, which
// hands over its arguments and ends.
func (s *shell) secondStart(second application.SecondInstanceData) {
	switch asked(second.Args) {
	case askQuit:
		s.quit()
	case askLoginOff:
		// The companion that asked ended inside New and cannot do it.
		if err := s.changeStartAtLogin(false); err != nil {
			log.Print(err)
		}
	case askLoginOn:
		if err := s.changeStartAtLogin(true); err != nil {
			log.Print(err)
		}
	default:
		s.bringBack()
	}
}

// bringBack shows the companion at full color and opens its list.
func (s *shell) bringBack() {
	s.comeBack()
	s.showPanel()
}

// appReopened answers macOS reopening the running app, from Finder or
// `open`. Wails would show the hidden windows itself, behind the shell's back.
func (s *shell) appReopened(cancel func()) {
	if s.presenceNow() == shown {
		return
	}
	cancel()
	go s.bringBack()
}

// togglePanel answers a click on the bubble and the shortcut.
func (s *shell) togglePanel() {
	if s.comeBack() {
		s.showPanel()
		return
	}
	if s.panel.IsVisible() {
		s.panel.Hide()
		return
	}
	s.mu.Lock()
	justDismissed := time.Since(s.hiddenByFocusAt) < reopenGuard
	s.mu.Unlock()
	if justDismissed {
		return
	}
	s.showPanel()
}

// showPanel opens the list beside the bubble and gives it the keys.
func (s *shell) showPanel() {
	s.showPanelOn(eventPanelShown)
}

// showPanelOn opens the panel beside the bubble and tells it with event,
// whose data is the corner nearest the bubble.
func (s *shell) showPanelOn(event string) {
	x, y, nearest := placePanel(windowRect(s.bubble), s.workArea(), panelWidth, panelHeight)
	s.panel.SetPosition(x, y)
	s.panel.Show()
	s.panel.Focus()
	s.panel.EmitEvent(event, string(nearest))
}

func windowRect(w application.Window) rect {
	x, y := w.Position()
	width, height := w.Size()
	return rect{X: x, Y: y, Width: width, Height: height}
}

// workArea is the usable part of the screen the bubble is on, or of the
// primary screen when the bubble's screen is unknown.
func (s *shell) workArea() rect {
	screen, err := s.bubble.GetScreen()
	if err != nil || screen == nil {
		screen = s.app.Screen.GetPrimary()
	}
	return rect{X: screen.WorkArea.X, Y: screen.WorkArea.Y, Width: screen.WorkArea.Width, Height: screen.WorkArea.Height}
}

// screensWait is how long the companion waits at start for Wails to list
// the screens. Wails lists them in its own start handler, which runs
// beside the companion's.
const screensWait = 2 * time.Second

// workAreas waits for the screens and returns the usable part of each,
// the primary screen first. It is empty when none was listed in time.
func (s *shell) workAreas() []rect {
	deadline := time.Now().Add(screensWait)
	for len(s.app.Screen.GetAll()) == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	var primary, others []rect
	for _, screen := range s.app.Screen.GetAll() {
		work := rect{X: screen.WorkArea.X, Y: screen.WorkArea.Y, Width: screen.WorkArea.Width, Height: screen.WorkArea.Height}
		if screen.IsPrimary {
			primary = append(primary, work)
		} else {
			others = append(others, work)
		}
	}
	return append(primary, others...)
}

// placeBubble puts the bubble where the person left it, when a screen
// still shows that place, otherwise at the lower right of the first
// screen. With no screen listed it leaves the bubble where the system
// opened it.
func (s *shell) placeBubble(saved *point) {
	workAreas := s.workAreas()
	if len(workAreas) == 0 {
		log.Print("no screen was listed in time; the bubble stays where the system opened it")
		return
	}
	if saved != nil && onScreen(rect{X: saved.X, Y: saved.Y, Width: bubbleSide, Height: bubbleSide}, workAreas) {
		s.bubble.SetPosition(saved.X, saved.Y)
		return
	}
	x, y := defaultBubblePosition(workAreas[0], bubbleSide)
	s.bubble.SetPosition(x, y)
}

// bubbleMoved closes the panel, which no longer sits beside the bubble,
// and saves the bubble's new place once it rests.
func (s *shell) bubbleMoved() {
	s.mu.Lock()
	settled := time.Since(s.shownAgainAt) >= returnSettle
	s.mu.Unlock()
	// A circle shown again can report moves after the list opened beside it.
	if settled && s.panel.IsVisible() {
		s.panel.Hide()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.save != nil {
		s.save.Stop()
	}
	s.save = time.AfterFunc(savePause, func() {
		x, y := s.bubble.Position()
		if err := s.changeSettings(func(saved *settings) { saved.Bubble = &point{X: x, Y: y} }); err != nil {
			log.Print(err)
		}
	})
}

// changeSettings applies change to the settings and saves them, as one
// step: the lock is held from the change through the write.
func (s *shell) changeSettings(change func(saved *settings)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	change(&s.settings)
	return writeSettings(s.settingsPath, s.settings)
}

func (s *shell) panelLostFocus() {
	s.mu.Lock()
	s.hiddenByFocusAt = time.Now()
	s.mu.Unlock()
}

// routes serves the companion's pages and the few calls they make.
func (s *shell) routes(in *inbox, pages fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/inbox", in.list)
	mux.HandleFunc("POST /api/open", in.pass(serverOpenPath))
	mux.HandleFunc("POST /api/mark", in.pass(serverMarkPath))
	mux.HandleFunc("POST /api/rename", in.pass(serverRenamePath))
	mux.HandleFunc("POST /api/delete", in.pass(serverDeletePath))
	mux.HandleFunc("POST /api/panel/toggle", func(w http.ResponseWriter, _ *http.Request) {
		s.togglePanel()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/panel/hide", func(w http.ResponseWriter, _ *http.Request) {
		s.panel.Hide()
		w.WriteHeader(http.StatusNoContent)
	})
	// Unlike toggle, show skips the reopen guard: the page hid the panel itself.
	// The page asks after an open fails, which can come long after a hide;
	// only the shortcut or a second start brings it back.
	mux.HandleFunc("POST /api/panel/show", func(w http.ResponseWriter, _ *http.Request) {
		if s.presenceNow() != shown {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		s.showPanel()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/preferences", s.getPreferences)
	mux.HandleFunc("POST /api/preferences/shortcut", s.setShortcut)
	mux.HandleFunc("POST /api/preferences/start-at-login", s.setStartAtLogin)
	mux.HandleFunc("GET /api/appearance", s.getAppearance)
	mux.HandleFunc("POST /api/appearance/animate", s.setAppearanceSwitch(s.setAnimate))
	mux.HandleFunc("POST /api/appearance/dim-when-idle", s.setAppearanceSwitch(s.setDimWhenIdle))
	mux.HandleFunc("POST /api/dashboard", dashboard)
	mux.HandleFunc("POST /api/hide", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		s.hide()
	})
	mux.HandleFunc("POST /api/quit", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		s.quit()
	})
	mux.Handle("/", application.BundledAssetFileServer(pages))
	return mux
}

// floating makes a window a frameless, see-through panel that stays
// above other apps, on every desktop, without taking over the menu bar
// or the taskbar.
func floating(options application.WebviewWindowOptions) application.WebviewWindowOptions {
	options.Frameless = true
	options.AlwaysOnTop = true
	options.DisableResize = true
	options.BackgroundType = application.BackgroundTypeTransparent
	options.BackgroundColour = application.NewRGBA(0, 0, 0, 0)
	options.DefaultContextMenuDisabled = true
	options.Mac = application.MacWindow{
		Backdrop:      application.MacBackdropTransparent,
		DisableShadow: true,
		CornerType:    application.MacWindowCornerTypeSquare,
		WindowClass:   application.MacWindowClassPanel,
		PanelPreferences: application.MacPanelPreferences{
			NonActivating:          true,
			BecomesKeyOnlyIfNeeded: true,
		},
		WindowLevel: application.MacWindowLevelFloating,
		CollectionBehavior: application.MacWindowCollectionBehaviorCanJoinAllSpaces |
			application.MacWindowCollectionBehaviorFullScreenAuxiliary,
	}
	options.Windows = application.WindowsWindow{
		HiddenOnTaskbar:                   true,
		DisableFramelessWindowDecorations: true,
		BackdropType:                      application.None,
	}
	return options
}

func main() {
	ask := asked(os.Args)
	if ask == askVersion {
		fmt.Println(version)
		return
	}
	path, err := settingsPath()
	if err != nil {
		log.Fatal(err)
	}
	saved, err := readSettings(path)
	// A companion asked to quit or to change the login needs no settings,
	// and must answer even when the file cannot be read.
	if err != nil && ask == "" {
		log.Fatal(err)
	}
	pages, err := fs.Sub(embedded, "frontend")
	if err != nil {
		log.Fatal(err)
	}

	takeFirstClick()
	s := &shell{settingsPath: path, settings: saved, server: newServer()}
	s.inbox = newInbox(s.server, version, func() {
		s.app.Event.Emit(eventInbox)
		s.listRead(s.inbox.held())
	})
	s.tell = func(looks appearance) {
		s.app.Event.Emit(eventAppearance, looks)
	}
	in := s.inbox
	s.app = application.New(application.Options{
		Name:        "agnostic-ai companion",
		Description: "The AI conversations that need you, floating over every app.",
		Mac: application.MacOptions{
			// No Dock icon and no menu bar of its own.
			ActivationPolicy: application.ActivationPolicyAccessory,
		},
		Assets: application.AssetOptions{Handler: s.routes(in, pages)},
		// One companion at a time. Starting it again brings back the one
		// that runs and opens its list, instead of a second circle.
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID:               instanceID,
			OnSecondInstanceLaunch: s.secondStart,
		},
	})
	s.end = s.app.Quit
	// A companion started while another runs hands over its arguments and
	// ends inside New, so from here on this is the only one.
	switch ask {
	case askQuit:
		os.Exit(notRunningExit)
	case askLoginOff, askLoginOn:
		change := s.app.Autostart.Disable
		if ask == askLoginOn {
			change = s.app.Autostart.Enable
		}
		if err := change(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	s.shortcuts = s.app.GlobalShortcut
	s.login = s.app.Autostart
	s.tellPreferences = func(prefs preferences) {
		s.app.Event.Emit(eventPreferences, prefs)
	}
	if err := s.bubbleMenu(); err != nil {
		log.Fatal(err)
	}
	s.bubble = s.app.Window.NewWithOptions(floating(application.WebviewWindowOptions{
		Name:   "bubble",
		Title:  "agnostic-ai",
		Width:  bubbleSide,
		Height: bubbleSide,
		// The bubble is index.html: Wails serves pages only from a folder that has one.
		URL: "/",
	}))
	s.panel = s.app.Window.NewWithOptions(floating(application.WebviewWindowOptions{
		Name:            "panel",
		Title:           "agnostic-ai conversations",
		Width:           panelWidth,
		Height:          panelHeight,
		URL:             "/panel.html",
		Hidden:          true,
		HideOnFocusLost: true,
		// The page handles Escape: it first closes its menu or a rename.
	}))

	s.bubble.OnWindowEvent(events.Common.WindowDidMove, func(*application.WindowEvent) { s.bubbleMoved() })
	s.panel.OnWindowEvent(events.Common.WindowLostFocus, func(*application.WindowEvent) { s.panelLostFocus() })

	s.app.Event.RegisterApplicationEventHook(events.Mac.ApplicationShouldHandleReopen, func(event *application.ApplicationEvent) {
		s.appReopened(event.Cancel)
	})
	s.app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		s.placeBubble(saved.Bubble)
		watchPointer(s.bubble.NativeWindow(), func(over bool) {
			s.app.Event.Emit(eventPointer, over)
		})
		s.registerShortcut()
		in.start()
	})

	err = s.app.Run()
	// The server also stops by itself when this process ends; this stops it sooner.
	s.restBriefly()
	if err != nil {
		log.Fatal(err)
	}
}
