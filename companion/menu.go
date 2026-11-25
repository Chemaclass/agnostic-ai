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

type menuSection struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type menuDefinition struct {
	Sections     []menuSection `json:"sections"`
	UpdatesLabel string        `json:"updatesLabel"`
	Items        []menuItem    `json:"items"`
}

type menuItem struct {
	ID      string `json:"id"`
	Section string `json:"section"`
	Label   string `json:"label"`
	// Kind is switch, shortcut or action.
	Kind string `json:"kind"`
	Hint string `json:"hint,omitempty"`
	// Page items show in the list's menu only: the server holds their value.
	Page bool `json:"page,omitempty"`
}

// readMenu reads the shared menu list from the companion's pages.
func readMenu(files fs.FS) (menuDefinition, error) {
	data, err := fs.ReadFile(files, "frontend/menu.json")
	if err != nil {
		return menuDefinition{}, fmt.Errorf("read the menu list: %w", err)
	}
	var items menuDefinition
	if err := json.Unmarshal(data, &items); err != nil {
		return menuDefinition{}, fmt.Errorf("read the menu list: %w", err)
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
		"smallCompanion": func(on bool) {
			if err := s.setSmallCompanion(on); err != nil {
				log.Print(err)
			}
		},
		"startAtLogin": func(on bool) {
			if err := s.changeStartAtLogin(on); err != nil {
				log.Print(err)
			}
		},
		checkForUpdatesID: func(on bool) {
			if err := s.changeCheckForUpdates(on); err != nil {
				log.Print(err)
			}
		},
		"shortcut":     func(bool) { s.openSettings() },
		"sendShortcut": func(bool) { s.comeBackOpenedOn(eventSendSettingsShown) },
		updateID:       func(bool) { s.runUpdate() },
		"hide":         func(bool) { s.hide() },
		"quit":         func(bool) { s.askQuit() },
	}
}

// openSettings brings a faded companion back and opens the list on its menu,
// where the shortcuts can be changed.
func (s *shell) openSettings() {
	s.comeBackOpenedOn(eventSettingsShown)
}

// askQuit opens the list on the question whether to quit: the menus
// never end the companion without it.
func (s *shell) askQuit() {
	s.comeBackOpenedOn(eventQuitAsked)
}

// macOS before 14 and other systems need a disabled item for the heading.
func addMenuSectionHeader(menu *application.Menu, label string) {
	menu.AddSectionHeader(label)
	if menu.FindByLabel(label) == nil {
		menu.Add(label).SetEnabled(false)
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
var shownIDs = []string{"animate", "dimWhenIdle", "smallCompanion", "startAtLogin", "shortcut", "sendShortcut", "hide"}

func (s *shell) fillBubbleMenu(menu *application.Menu, definition menuDefinition) error {
	actions := s.menuActions()
	native := map[string]*application.MenuItem{}
	labels := map[string]string{}
	s.appCommands = s.ownAppCommands()
	for _, section := range definition.Sections {
		var updates *application.Menu
		started := false
		for _, item := range definition.Items {
			if item.Page || item.Section != section.ID {
				continue
			}
			act := actions[item.ID]
			if act == nil {
				return fmt.Errorf("the menu item %q has no handler", item.ID)
			}
			isUpdate := item.ID == updateID || item.ID == checkForUpdatesID
			if isUpdate && s.appCommands == nil {
				continue
			}
			if !started {
				addMenuSectionHeader(menu, section.Label)
				started = true
			}
			target := menu
			if isUpdate {
				if updates == nil {
					updates = menu.AddSubmenu(definition.UpdatesLabel)
				}
				target = updates
			}
			var added *application.MenuItem
			switch item.Kind {
			case "switch":
				added = target.AddCheckbox(item.Label, false)
			case "shortcut", "action":
				added = target.Add(item.Label)
			default:
				return fmt.Errorf("the menu item %q has the unknown kind %q", item.ID, item.Kind)
			}
			if item.Hint != "" {
				added.SetTooltip(item.Hint)
			}
			// Wails flips the checkbox first; refresh restores a refused change.
			added.OnClick(func(clicked *application.Context) { act(clicked.IsChecked()) })
			native[item.ID] = added
			labels[item.ID] = item.Label
			if item.ID == updateID && s.appCommands.app != "" {
				controls := s.appCommands
				menu.Add("Install command line tool").OnClick(func(*application.Context) { go controls.installCommand() })
			}
		}
	}
	for _, id := range shownIDs {
		if native[id] == nil {
			return fmt.Errorf("the menu list has no %q item", id)
		}
	}
	s.nativeMu.Lock()
	s.native = native
	s.labels = labels
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
	s.native["smallCompanion"].SetChecked(looks.SmallCompanion)
	s.native["startAtLogin"].SetChecked(prefs.StartAtLogin)
	isMac := runtime.GOOS == "darwin"
	s.native["shortcut"].SetLabel(shortcutLabel(s.labels["shortcut"], prefs.Shortcut, isMac))
	s.native["sendShortcut"].SetLabel(shortcutLabel(s.labels["sendShortcut"], prefs.SendShortcut, isMac))
	s.native["hide"].SetLabel(hideLabel(s.labels["hide"], prefs))
	if item := s.native[checkForUpdatesID]; item != nil {
		item.SetChecked(prefs.CheckForUpdates)
	}
	if item := s.native[updateID]; item != nil {
		status := s.updateStatus()
		item.SetLabel(updateLabel(s.labels[updateID], status))
		item.SetEnabled(!status.busy())
	}
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

// shortcutLabel names a shortcut item with its keys.
func shortcutLabel(base, shortcut string, isMac bool) string {
	return base + ": " + keysOf(shortcut, isMac) + "…"
}

// hideLabel names the command that brings the companion back while the
// shortcut does not work; a working shortcut already shows on the item
// above. model.js hideLabelOf says the same in the list.
func hideLabel(base string, prefs preferences) string {
	if prefs.ShortcutProblem == "" {
		return base
	}
	return base + " (agnostic-ai companion brings it back)"
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
