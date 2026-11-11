// The agnostic-ai companion: a small circle that floats over every app and
// counts the AI conversations that need the person, with a searchable
// list beside it that opens each one where it lives. It is a window on
// `agnostic-ai serve`: the server reads, opens and marks the
// conversations, and the companion asks it.
package main

import (
	"embed"
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
)

// reopenGuard is how soon after the panel hid on losing focus a click
// on the bubble counts as that same dismissal. On systems where the
// click itself takes the focus, the panel hides first and the click
// arrives after; without the guard it would open again.
const reopenGuard = 250 * time.Millisecond

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
	tell      func(appearance)

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
	// hidden is set while the companion is off the screen: only the
	// shortcut listens then, and nothing reads or asks anything.
	hidden bool
}

// hide takes the companion off the screen and stops its server, until
// the shortcut or a second start brings it back.
func (s *shell) hide() {
	s.mu.Lock()
	s.hidden = true
	s.mu.Unlock()
	s.panel.Hide()
	s.bubble.Hide()
	s.inbox.stop()
	s.server.rest()
}

// comeBack puts the bubble back and reads a new list. It reports
// whether the companion was hidden.
func (s *shell) comeBack() bool {
	s.mu.Lock()
	wasHidden := s.hidden
	s.hidden = false
	s.mu.Unlock()
	if !wasHidden {
		return false
	}
	s.server.resume()
	s.inbox.start()
	s.bubble.Show()
	return true
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
	x, y, nearest := placePanel(windowRect(s.bubble), s.workArea(), panelWidth, panelHeight)
	s.panel.SetPosition(x, y)
	s.panel.Show()
	s.panel.Focus()
	s.panel.EmitEvent(eventPanelShown, string(nearest))
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
	if s.panel.IsVisible() {
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
	mux.HandleFunc("POST /api/panel/toggle", func(w http.ResponseWriter, _ *http.Request) {
		s.togglePanel()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/panel/hide", func(w http.ResponseWriter, _ *http.Request) {
		s.panel.Hide()
		w.WriteHeader(http.StatusNoContent)
	})
	// Unlike toggle, show skips the reopen guard: the page hid the panel itself.
	mux.HandleFunc("POST /api/panel/show", func(w http.ResponseWriter, _ *http.Request) {
		s.showPanel()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/preferences", s.getPreferences)
	mux.HandleFunc("POST /api/preferences/shortcut", s.setShortcut)
	mux.HandleFunc("POST /api/preferences/start-at-login", s.setStartAtLogin)
	mux.HandleFunc("GET /api/appearance", s.getAppearance)
	mux.HandleFunc("POST /api/dashboard", dashboard)
	mux.HandleFunc("POST /api/hide", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		s.hide()
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
			UniqueID: instanceID,
			OnSecondInstanceLaunch: func(second application.SecondInstanceData) {
				switch asked(second.Args) {
				case askQuit:
					s.app.Quit()
				case askLoginOff:
					// The companion that asked ended inside New and cannot do it.
					if err := s.app.Autostart.Disable(); err != nil {
						log.Print(err)
					}
				case askLoginOn:
					if err := s.app.Autostart.Enable(); err != nil {
						log.Print(err)
					}
				default:
					s.comeBack()
					s.showPanel()
				}
			},
		},
	})
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
	s.bubbleMenu()
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
	s.server.rest()
	if err != nil {
		log.Fatal(err)
	}
}
