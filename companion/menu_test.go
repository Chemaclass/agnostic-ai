package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// fakeAutostart is the system's login setting.
type fakeAutostart struct {
	enabled bool
	refuse  error
}

func (f *fakeAutostart) Enable() error {
	if f.refuse != nil {
		return f.refuse
	}
	f.enabled = true
	return nil
}

func (f *fakeAutostart) Disable() error {
	if f.refuse != nil {
		return f.refuse
	}
	f.enabled = false
	return nil
}

func (f *fakeAutostart) IsEnabled() (bool, error) { return f.enabled, nil }

// menuShell is a shell whose right-click menu is built from the shared list.
func menuShell(t *testing.T) (*shell, *[]appearance, *[]preferences, *application.Menu) {
	t.Helper()
	s := shellWith(t, &fakeShortcuts{})
	looks, prefs := []appearance{}, []preferences{}
	s.tell = func(a appearance) { looks = append(looks, a) }
	s.tellPreferences = func(p preferences) { prefs = append(prefs, p) }
	items, err := readMenu(embedded)
	if err != nil {
		t.Fatal(err)
	}
	menu := application.NewMenu()
	if err := s.fillBubbleMenu(menu, items); err != nil {
		t.Fatal(err)
	}
	return s, &looks, &prefs, menu
}

func TestMenuList_EveryItemHasAHandlerAndAKnownKind(t *testing.T) {
	items, err := readMenu(embedded)
	if err != nil {
		t.Fatal(err)
	}
	if len(items.Items) != 13 {
		t.Fatalf("menu.json has %d items; want 13", len(items.Items))
	}
	for _, id := range []string{"approvalsMode", "approvalsCompanion", "approveAgentSends"} {
		if i := slices.IndexFunc(items.Items, func(item menuItem) bool { return item.ID == id }); i < 0 || !items.Items[i].Page {
			t.Errorf("%q is missing or not a page item, which the right-click menu skips", id)
		}
	}
	actions := shellWith(t, &fakeShortcuts{}).menuActions()
	for _, item := range items.Items {
		if item.Page {
			continue
		}
		if actions[item.ID] == nil {
			t.Errorf("no handler for %q", item.ID)
		}
		if item.Kind != "switch" && item.Kind != "shortcut" && item.Kind != "action" {
			t.Errorf("%q has the unknown kind %q", item.ID, item.Kind)
		}
	}
}

func TestBubbleMenu_RefusesAnItemWithNoHandler(t *testing.T) {
	s := shellWith(t, &fakeShortcuts{})

	err := s.fillBubbleMenu(application.NewMenu(), menuDefinition{Sections: []menuSection{{ID: "app", Label: "App"}}, Items: []menuItem{{ID: "dance", Section: "app", Label: "Dance", Kind: "action"}}})

	if err == nil || !strings.Contains(err.Error(), `"dance"`) {
		t.Errorf("err = %v; want one naming the item", err)
	}
}

func TestBubbleMenu_RefusesAnUnknownKind(t *testing.T) {
	s := shellWith(t, &fakeShortcuts{})

	err := s.fillBubbleMenu(application.NewMenu(), menuDefinition{Sections: []menuSection{{ID: "app", Label: "App"}}, Items: []menuItem{{ID: "quit", Section: "app", Label: "Quit", Kind: "slider"}}})

	if err == nil || !strings.Contains(err.Error(), `"slider"`) {
		t.Errorf("err = %v; want one naming the kind", err)
	}
}

func TestBubbleMenu_RefusesAListWithoutAnItemItShows(t *testing.T) {
	s := shellWith(t, &fakeShortcuts{})
	items, err := readMenu(embedded)
	if err != nil {
		t.Fatal(err)
	}
	var withoutHide []menuItem
	for _, item := range items.Items {
		if item.ID != "hide" {
			withoutHide = append(withoutHide, item)
		}
	}

	items.Items = withoutHide
	err = s.fillBubbleMenu(application.NewMenu(), items)

	if err == nil || !strings.Contains(err.Error(), `"hide"`) {
		t.Errorf("err = %v; want one naming the missing item", err)
	}
}

func TestBubbleMenu_FollowsTheSharedList(t *testing.T) {
	s, _, _, menu := menuShell(t)
	s.settings.Shortcut = "Ctrl+Alt+Space"
	s.refreshNative()

	var got []string
	for i := 0; menu.ItemAt(i) != nil; i++ {
		item := menu.ItemAt(i)
		switch {
		case item.IsSeparator():
			got = append(got, "---")
		case item.IsCheckbox():
			got = append(got, "[x] "+item.Label())
		default:
			got = append(got, item.Label())
		}
	}
	want := []string{"Appearance", "[x] Animate companion", "[x] Dim when idle", "[x] Small companion", "Shortcuts",
		shortcutLabel("Open from anywhere", "Ctrl+Alt+Space", runtime.GOOS == "darwin"),
		shortcutLabel("Send copied text", "Ctrl+Alt+V", runtime.GOOS == "darwin"), "App", "[x] Start at login", "Hide Agnostic AI companion", "Quit Agnostic AI companion"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("menu = %q\nwant %q", got, want)
	}
	if !s.native["quit"].Enabled() {
		t.Error("Quit cannot be chosen")
	}
}

func TestBubbleMenu_UsesDeclaredSectionOrderAndSkipsPageOnlySections(t *testing.T) {
	s := shellWith(t, &fakeShortcuts{})
	definition, err := readMenu(embedded)
	if err != nil {
		t.Fatal(err)
	}
	slices.Reverse(definition.Sections)
	menu := application.NewMenu()
	if err := s.fillBubbleMenu(menu, definition); err != nil {
		t.Fatal(err)
	}
	var headings []string
	for i := 0; menu.ItemAt(i) != nil; i++ {
		item := menu.ItemAt(i)
		if !item.Enabled() {
			headings = append(headings, item.Label())
		}
	}
	if want := []string{"App", "Shortcuts", "Appearance"}; !slices.Equal(headings, want) {
		t.Errorf("headings = %q; want %q", headings, want)
	}
	for _, id := range []string{"approvalsMode", "approvalsCompanion", "approveAgentSends"} {
		if s.native[id] != nil {
			t.Errorf("native menu contains page-only %q", id)
		}
	}
}

func TestShortcutLabel_ShowsTheKeysAsEachSystemWritesThem(t *testing.T) {
	cases := []struct {
		base, shortcut string
		mac            bool
		want           string
	}{
		{"Open from anywhere", "Ctrl+Alt+Space", true, "Open from anywhere: ⌃⌥Space…"},
		{"Open from anywhere", "Cmd+Shift+K", true, "Open from anywhere: ⌘⇧K…"},
		{"Open from anywhere", "Ctrl+Alt+Space", false, "Open from anywhere: Ctrl+Alt+Space…"},
		{"Open from anywhere", "Super+Shift+K", false, "Open from anywhere: Super+Shift+K…"},
		{"Send copied text", "Ctrl+Alt+V", true, "Send copied text: ⌃⌥V…"},
	}
	for _, tc := range cases {
		if got := shortcutLabel(tc.base, tc.shortcut, tc.mac); got != tc.want {
			t.Errorf("shortcutLabel(%q, mac %v) = %q; want %q", tc.shortcut, tc.mac, got, tc.want)
		}
	}
}

func TestAppearanceRoutes_SaveTellThePagesAndCheckTheNativeItem(t *testing.T) {
	for _, tc := range []struct {
		path, id string
		read     func(appearance) bool
	}{
		{"/api/appearance/animate", "animate", func(a appearance) bool { return a.Animate }},
		{"/api/appearance/dim-when-idle", "dimWhenIdle", func(a appearance) bool { return a.DimWhenIdle }},
		{"/api/appearance/small-companion", "smallCompanion", func(a appearance) bool { return a.SmallCompanion }},
	} {
		t.Run(tc.id, func(t *testing.T) {
			s, looks, _, _ := menuShell(t)
			before := tc.read(s.appearance())
			routes := s.routes(&inbox{}, fstest.MapFS{})

			rec := httptest.NewRecorder()
			body := `{"enabled":true}`
			if before {
				body = `{"enabled":false}`
			}
			routes.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(body)))

			if rec.Code != http.StatusOK {
				t.Fatalf("POST %s = %d %s", tc.path, rec.Code, rec.Body)
			}
			if tc.read(s.appearance()) == before {
				t.Error("the choice did not change")
			}
			if len(*looks) == 0 || tc.read((*looks)[len(*looks)-1]) == before {
				t.Errorf("the pages were told %+v", *looks)
			}
			if s.native[tc.id].Checked() == before {
				t.Error("the right-click menu still shows the old choice")
			}
		})
	}
}

func TestAppearanceRoutes_RefuseABodyWithoutEnabled(t *testing.T) {
	s, _, _, _ := menuShell(t)
	routes := s.routes(&inbox{}, fstest.MapFS{})

	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/appearance/animate", strings.NewReader(`{}`)))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("= %d %s", rec.Code, rec.Body)
	}
}

func TestNativeStartAtLogin_ChangesTheSystemAndTellsTheList(t *testing.T) {
	s, _, prefs, _ := menuShell(t)

	s.menuActions()["startAtLogin"](true)

	if !s.login.(*fakeAutostart).enabled {
		t.Error("the system setting did not change")
	}
	if len(*prefs) == 0 || !(*prefs)[len(*prefs)-1].StartAtLogin {
		t.Errorf("the list was told %+v; want start at login on", *prefs)
	}
	if !s.native["startAtLogin"].Checked() {
		t.Error("the right-click item is not checked")
	}
}

func TestNativeStartAtLogin_ShowsTheSystemValueWhenRefused(t *testing.T) {
	s, _, prefs, _ := menuShell(t)
	s.login.(*fakeAutostart).refuse = errors.New("not allowed")
	// Wails flips a checkbox before it calls back.
	s.native["startAtLogin"].SetChecked(true)

	s.menuActions()["startAtLogin"](true)

	if s.native["startAtLogin"].Checked() {
		t.Error("the right-click item shows a change the system refused")
	}
	if len(*prefs) == 0 || (*prefs)[len(*prefs)-1].StartAtLogin {
		t.Errorf("the list was told %+v", *prefs)
	}
}

func TestLoginRoute_ChecksTheNativeItemAndTellsTheList(t *testing.T) {
	s, _, prefs, _ := menuShell(t)
	routes := s.routes(&inbox{}, fstest.MapFS{})

	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/preferences/start-at-login", strings.NewReader(`{"enabled":true}`)))

	if rec.Code != http.StatusOK || !s.native["startAtLogin"].Checked() {
		t.Errorf("= %d, native checked %v", rec.Code, s.native["startAtLogin"].Checked())
	}
	if len(*prefs) == 0 || !(*prefs)[len(*prefs)-1].StartAtLogin {
		t.Errorf("the list was told %+v", *prefs)
	}
}

func TestShortcutRoute_RelabelsTheNativeItem(t *testing.T) {
	s, _, prefs, _ := menuShell(t)
	routes := s.routes(&inbox{}, fstest.MapFS{})

	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/preferences/shortcut", strings.NewReader(`{"shortcut":"Cmd+Shift+K"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("= %d %s", rec.Code, rec.Body)
	}
	if got, want := s.native["shortcut"].Label(), shortcutLabel("Open from anywhere", "Cmd+Shift+K", runtime.GOOS == "darwin"); got != want {
		t.Errorf("label = %q; want %q", got, want)
	}
	if len(*prefs) == 0 || (*prefs)[len(*prefs)-1].Shortcut != "Cmd+Shift+K" {
		t.Errorf("the list was told %+v", *prefs)
	}
}

func TestSendShortcutRoute_SavesRelabelsAndTellsTheList(t *testing.T) {
	s, _, prefs, _ := menuShell(t)
	routes := s.routes(&inbox{}, fstest.MapFS{})

	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/preferences/send-shortcut", strings.NewReader(`{"shortcut":"Cmd+Shift+B"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("= %d %s", rec.Code, rec.Body)
	}
	if got, want := s.native["sendShortcut"].Label(), shortcutLabel("Send copied text", "Cmd+Shift+B", runtime.GOOS == "darwin"); got != want {
		t.Errorf("label = %q; want %q", got, want)
	}
	if len(*prefs) == 0 || (*prefs)[len(*prefs)-1].SendShortcut != "Cmd+Shift+B" || (*prefs)[len(*prefs)-1].Shortcut != defaultShortcut {
		t.Errorf("the list was told %+v", *prefs)
	}
}

func TestSendShortcutRoute_RefusesTheOpenKeys(t *testing.T) {
	s, _, _, _ := menuShell(t)
	routes := s.routes(&inbox{}, fstest.MapFS{})

	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/preferences/send-shortcut", strings.NewReader(`{"shortcut":"Ctrl+Alt+Space"}`)))

	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "already opens the list") {
		t.Errorf("= %d %s", rec.Code, rec.Body)
	}
}

func TestNativeSendShortcut_OpensTheListOnItsOwnSetting(t *testing.T) {
	s, _ := placedShell(t)

	s.menuActions()["sendShortcut"](false)

	panel := s.panel.(*fakeWindow)
	if !panel.IsVisible() || strings.Join(panel.events, ",") != eventSendSettingsShown {
		t.Errorf("list shown %v, events %v", panel.IsVisible(), panel.events)
	}
}

func TestGetPreferences_ShowsALoginChangeMadeElsewhere(t *testing.T) {
	s, _, _, _ := menuShell(t)
	// Changed in the system's settings while the companion ran.
	s.login.(*fakeAutostart).enabled = true

	rec := httptest.NewRecorder()
	s.getPreferences(rec, httptest.NewRequest(http.MethodGet, "/api/preferences", nil))

	if !s.native["startAtLogin"].Checked() {
		t.Error("the right-click item missed the system's value")
	}
}

func TestHideLabel_NamesTheCommandOnlyWithoutAWorkingShortcut(t *testing.T) {
	cases := []struct {
		name  string
		prefs preferences
		want  string
	}{
		{"working shortcut", preferences{Shortcut: "Ctrl+Alt+Space"}, "Hide Agnostic AI companion"},
		{"no working shortcut", preferences{Shortcut: "Ctrl+Alt+Space", ShortcutProblem: "Ctrl+Alt+Space is not active: taken"}, "Hide Agnostic AI companion (agnostic-ai companion brings it back)"},
	}
	for _, tc := range cases {
		if got := hideLabel("Hide Agnostic AI companion", tc.prefs); got != tc.want {
			t.Errorf("%s: hideLabel = %q; want %q", tc.name, got, tc.want)
		}
	}
}

func TestHideItem_FollowsTheShortcut(t *testing.T) {
	s, _, _, _ := menuShell(t)
	routes := s.routes(&inbox{}, fstest.MapFS{})

	routes.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/preferences/shortcut", strings.NewReader(`{"shortcut":"Cmd+Shift+K"}`)))
	if got, want := s.native["hide"].Label(), "Hide Agnostic AI companion"; got != want {
		t.Errorf("label = %q; want %q", got, want)
	}

	s.mu.Lock()
	s.shortcutProblem = "Cmd+Shift+K is not active: taken"
	s.mu.Unlock()
	s.refreshNative()
	if got := s.native["hide"].Label(); !strings.Contains(got, "agnostic-ai companion brings it back") {
		t.Errorf("label = %q; want the other way back", got)
	}
}

func TestHideItem_NamesTheCommandWhenTheShortcutIsRefusedAtStart(t *testing.T) {
	s, _, _, _ := menuShell(t)
	s.shortcuts = &fakeShortcuts{refused: map[string]bool{defaultShortcut: true}}

	s.registerShortcuts()

	if got := s.native["hide"].Label(); !strings.Contains(got, "agnostic-ai companion brings it back") {
		t.Errorf("label = %q; want the command as the way back", got)
	}
}

func TestMenuList_QuitSaysTheShortcutStops(t *testing.T) {
	items, err := readMenu(embedded)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items.Items {
		if item.ID == "quit" && !strings.Contains(item.Hint, "shortcut stops working") {
			t.Errorf("the Quit hint %q does not say the shortcut stops working", item.Hint)
		}
	}
}
