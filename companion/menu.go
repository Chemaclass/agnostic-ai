package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"runtime"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// bubbleMenuName is the circle's right-click menu; bubble.css names it in
// --custom-contextmenu.
const bubbleMenuName = "bubble"

const settingsHeader = "Settings"

// menuItem is one entry of frontend/menu.json, the list both menus show.
type menuItem struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Kind is switch, shortcut or action.
	Kind string `json:"kind"`
	Hint string `json:"hint,omitempty"`
}

// readMenu reads the shared menu list from the companion's pages.
func readMenu(files fs.FS) ([]menuItem, error) {
	data, err := fs.ReadFile(files, "frontend/menu.json")
	if err != nil {
		return nil, fmt.Errorf("read the menu list: %w", err)
	}
	var items []menuItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("read the menu list: %w", err)
	}
	return items, nil
}

// menuActions is what each item of the right-click menu does; a switch
// is given its new value.
func (s *shell) menuActions() map[string]func(on bool) {
	return map[string]func(on bool){
		"animate": func(on bool) {
			if err := s.setAnimate(on); err != nil {
				log.Print(err)
			}
		},
		"dimWhenIdle": func(on bool) {
			if err := s.setDimWhenIdle(on); err != nil {
				log.Print(err)
			}
		},
		"startAtLogin": func(on bool) {
			if err := s.changeStartAtLogin(on); err != nil {
				log.Print(err)
			}
		},
		"shortcut": func(bool) { s.openSettings() },
		"pause":    func(bool) { s.pause() },
		"quit":     func(bool) { s.quit() },
	}
}

// openSettings wakes a paused companion and opens the list on its menu,
// where the shortcut can be changed.
func (s *shell) openSettings() {
	s.comeBack()
	s.showPanelOn(eventSettingsShown)
}

// addSettingsHeader adds a native section heading where the system has
// one (macOS 14 and later), and a disabled item elsewhere.
func addSettingsHeader(menu *application.Menu) {
	menu.AddSectionHeader(settingsHeader)
	if menu.FindByLabel(settingsHeader) == nil {
		menu.Add(settingsHeader).SetEnabled(false)
	}
}

func (s *shell) bubbleMenu() error {
	items, err := readMenu(embedded)
	if err != nil {
		return err
	}
	return s.fillBubbleMenu(application.NewContextMenu(bubbleMenuName).Menu, items)
}

// fillBubbleMenu adds the shared list to the circle's menu, with a
// separator before the actions, and shows the values in effect.
func (s *shell) fillBubbleMenu(menu *application.Menu, items []menuItem) error {
	actions := s.menuActions()
	native := map[string]*application.MenuItem{}
	addSettingsHeader(menu)
	for i, item := range items {
		act := actions[item.ID]
		if act == nil {
			return fmt.Errorf("the menu item %q has no handler", item.ID)
		}
		if item.Kind == "action" && i > 0 && items[i-1].Kind != "action" {
			menu.AddSeparator()
		}
		var added *application.MenuItem
		switch item.Kind {
		case "switch":
			added = menu.AddCheckbox(item.Label, false)
		case "shortcut", "action":
			added = menu.Add(item.Label)
		default:
			return fmt.Errorf("the menu item %q has the unknown kind %q", item.ID, item.Kind)
		}
		if item.Hint != "" {
			added.SetTooltip(item.Hint)
		}
		// Wails flips a checkbox before the click; every change refreshes
		// the menu after, so a refused one shows the old value again.
		added.OnClick(func(clicked *application.Context) { act(clicked.IsChecked()) })
		native[item.ID] = added
	}
	s.nativeMu.Lock()
	s.native = native
	s.nativeMu.Unlock()
	s.refreshNative()
	return nil
}

// refreshNative shows the values in effect in the right-click menu. The
// system shows them the next time the menu opens.
func (s *shell) refreshNative() {
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	if s.native == nil {
		return
	}
	looks := s.appearance()
	prefs := s.preferences()
	s.native["animate"].SetChecked(looks.Animate)
	s.native["dimWhenIdle"].SetChecked(looks.DimWhenIdle)
	s.native["startAtLogin"].SetChecked(prefs.StartAtLogin)
	s.native["shortcut"].SetLabel(shortcutLabel(prefs.Shortcut, runtime.GOOS == "darwin"))
	s.native["pause"].SetEnabled(!looks.Paused)
}

// macKeycaps is how each part of a shortcut reads on a Mac; model.js
// keycapsOf shows the same in the list.
var macKeycaps = map[string]string{"Ctrl": "⌃", "Alt": "⌥", "Shift": "⇧", "Cmd": "⌘", "Super": "⌘"}

// shortcutLabel names the shortcut item with its keys: "⌃⌥Space" on a
// Mac, "Ctrl+Alt+Space" elsewhere.
func shortcutLabel(shortcut string, isMac bool) string {
	keys := shortcut
	if isMac {
		parts := strings.Split(shortcut, "+")
		for i, part := range parts {
			if symbol, ok := macKeycaps[part]; ok {
				parts[i] = symbol
			}
		}
		keys = strings.Join(parts, "")
	}
	return "Open from anywhere: " + keys + "…"
}

// setAppearanceSwitch answers a POST that turns one look on or off, with
// the appearance in effect.
func (s *shell) setAppearanceSwitch(save func(on bool) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, ok := readBody[struct {
			Enabled *bool `json:"enabled"`
		}](w, r, "{enabled}")
		if !ok {
			return
		}
		if body.Enabled == nil {
			writeJSON(w, http.StatusBadRequest, apiError{Error: "send {enabled}"})
			return
		}
		if err := save(*body.Enabled); err != nil {
			writeJSON(w, http.StatusInternalServerError, apiError{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, s.appearance())
	}
}
