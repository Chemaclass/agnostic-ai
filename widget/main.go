// The agnostic-ai widget: a small circle that floats over every app and
// counts the AI conversations that need the person, with a searchable
// list beside it that opens each one where it lives. It reads the same
// stores and keeps the same marks as `agnostic-ai serve`.
package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
)

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

// instanceID names the widget to the system, so a second start finds the first.
const instanceID = "dev.agnostic-ai.widget"

// Events the pages listen for.
const (
	// eventInbox says the latest list changed; pages read it again.
	eventInbox = "inbox"
	// eventPanelShown says the panel opened; its data is the corner nearest the bubble.
	eventPanelShown = "panel-shown"
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

	settingsPath string

	mu sync.Mutex
	// settings is what is saved; every write saves all of it.
	settings settings
	// shortcutProblem says why the shortcut is not active; empty when it is.
	shortcutProblem string
	// hiddenByFocusAt is when the panel last hid because it lost focus.
	hiddenByFocusAt time.Time
	save            *time.Timer
}

// togglePanel answers a click on the bubble and the shortcut.
func (s *shell) togglePanel() {
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

// screensWait is how long the widget waits at start for Wails to list
// the screens. Wails lists them in its own start handler, which runs
// beside the widget's.
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
		s.mu.Lock()
		s.settings.Bubble = &point{X: x, Y: y}
		saved := s.settings
		s.mu.Unlock()
		if err := writeSettings(s.settingsPath, saved); err != nil {
			log.Print(err)
		}
	})
}

func (s *shell) panelLostFocus() {
	s.mu.Lock()
	s.hiddenByFocusAt = time.Now()
	s.mu.Unlock()
}

// routes serves the widget's pages and the few calls they make.
func (s *shell) routes(in *inbox, pages fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/inbox", in.list)
	mux.HandleFunc("POST /api/open", in.open)
	mux.HandleFunc("POST /api/mark", in.mark)
	mux.HandleFunc("POST /api/panel/toggle", func(w http.ResponseWriter, _ *http.Request) {
		s.togglePanel()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/panel/hide", func(w http.ResponseWriter, _ *http.Request) {
		s.panel.Hide()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/mark-all-seen", in.markAllSeen)
	mux.HandleFunc("GET /api/preferences", s.getPreferences)
	mux.HandleFunc("POST /api/preferences/shortcut", s.setShortcut)
	mux.HandleFunc("POST /api/preferences/start-at-login", s.setStartAtLogin)
	mux.HandleFunc("POST /api/dashboard", dashboard)
	mux.HandleFunc("POST /api/quit", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		s.app.Quit()
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
	path, err := settingsPath()
	if err != nil {
		log.Fatal(err)
	}
	saved, err := readSettings(path)
	if err != nil {
		log.Fatal(err)
	}
	pages, err := fs.Sub(embedded, "frontend")
	if err != nil {
		log.Fatal(err)
	}

	s := &shell{settingsPath: path, settings: saved}
	in := newInbox(conversations.NewScanner(), conversations.SystemLauncher(), func() {
		s.app.Event.Emit(eventInbox)
	})
	s.app = application.New(application.Options{
		Name:        "agnostic-ai widget",
		Description: "The AI conversations that need you, floating over every app.",
		Mac: application.MacOptions{
			// No Dock icon and no menu bar of its own.
			ActivationPolicy: application.ActivationPolicyAccessory,
		},
		Assets: application.AssetOptions{Handler: s.routes(in, pages)},
		// One widget at a time. Starting it again opens the list of the
		// one that runs, instead of a second circle.
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID:               instanceID,
			OnSecondInstanceLaunch: func(application.SecondInstanceData) { s.showPanel() },
		},
	})

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
		HideOnEscape:    true,
	}))

	s.bubble.OnWindowEvent(events.Common.WindowDidMove, func(*application.WindowEvent) { s.bubbleMoved() })
	s.panel.OnWindowEvent(events.Common.WindowLostFocus, func(*application.WindowEvent) { s.panelLostFocus() })

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	s.app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		s.placeBubble(saved.Bubble)
		s.registerShortcut()
		go in.poll(ctx)
	})

	if err := s.app.Run(); err != nil {
		log.Fatal(err)
	}
}
