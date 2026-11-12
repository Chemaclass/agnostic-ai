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
		"hide":     func(bool) { s.hide() },
		"quit":     func(bool) { s.askQuit() },
	}
}

// openSettings brings a faded companion back and opens the list on its menu,
// where the shortcut can be changed.
func (s *shell) openSettings() {
	s.comeBackOpenedOn(eventSettingsShown)
}

// askQuit opens the list on the question whether to quit: the menus
// never end the companion without it.
func (s *shell) askQuit() {
	s.comeBackOpenedOn(eventQuitAsked)
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

// shownIDs are the items refreshNative keeps current; the list must hold each.
var shownIDs = []string{"animate", "dimWhenIdle", "startAtLogin", "shortcut", "hide"}

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
	for _, id := range shownIDs {
		if native[id] == nil {
			return fmt.Errorf("the menu list has no %q item", id)
		}
	}
	s.nativeMu.Lock()
	s.native = native
	s.hideBase = native["hide"].Label()
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
	s.native["hide"].SetLabel(hideLabel(s.hideBase, prefs, runtime.GOOS == "darwin"))
}

// macKeycaps is how each part of a shortcut reads on a Mac; model.js
// keycapsOf shows the same in the list.
var macKeycaps = map[string]string{"Ctrl": "⌃", "Alt": "⌥", "Shift": "⇧", "Cmd": "⌘", "Super": "⌘"}

// keysOf writes a shortcut as each system shows it: "⌃⌥Space" on a
// Mac, "Ctrl+Alt+Space" elsewhere.
func keysOf(shortcut string, isMac bool) string {
	if !isMac {
		return shortcut
	}
	parts := strings.Split(shortcut, "+")
	for i, part := range parts {
		if symbol, ok := macKeycaps[part]; ok {
			parts[i] = symbol
		}
	}
	return strings.Join(parts, "")
}

// shortcutLabel names the shortcut item with its keys.
func shortcutLabel(shortcut string, isMac bool) string {
	return "Open from anywhere: " + keysOf(shortcut, isMac) + "…"
}

// hideLabel adds to the Hide item what brings the companion back: the
// shortcut while it works, otherwise the command. model.js hideLabelOf
// says the same in the list.
func hideLabel(base string, prefs preferences, isMac bool) string {
	way := "agnostic-ai companion"
	if prefs.ShortcutProblem == "" {
		way = keysOf(prefs.Shortcut, isMac)
	}
	return base + " (" + way + " brings it back)"
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
