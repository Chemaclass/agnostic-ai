package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestSettings_KeptShellsRoundTripThroughTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "companion.json")
	wake := time.Date(2026, 11, 22, 10, 30, 0, 0, time.UTC)
	written := settings{Shortcut: "Cmd+Shift+K", KeptShells: []keptShell{{Tool: "claude", ID: "a", PID: 531, ProcStart: "Sun Nov 22 08:00:00 2026", WakeAt: wake, Keeps: 2}}}

	if err := writeSettings(path, written); err != nil {
		t.Fatal(err)
	}
	read, err := readSettings(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(read.KeptShells) != 1 || read.KeptShells[0] != written.KeptShells[0] {
		t.Errorf("kept shells = %+v; want %+v", read.KeptShells, written.KeptShells)
	}
}

func TestSettings_AFileWithoutKeptShellsStillLoads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "companion.json")
	if err := os.WriteFile(path, []byte(`{"shortcut":"Cmd+Shift+K"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	read, err := readSettings(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(read.KeptShells) != 0 || read.Shortcut != "Cmd+Shift+K" {
		t.Errorf("read %+v", read)
	}
}

const shellOnlyList = `{"needsYou":1,"conversations":[{"tool":"claude","id":"a","state":"blocked","live":true,"waiting":{"for":"shell"},"stuckShells":[{"pid":531,"procStart":"start-531"}]}]}`

func TestWaitingIn_SkipsARowWhoseShellsAreAllKept(t *testing.T) {
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	kept := []keptShell{{Tool: "claude", ID: "a", PID: 531, ProcStart: "start-531", WakeAt: now.Add(30 * time.Minute), Keeps: 1}}

	got, ok := waitingIn([]byte(shellOnlyList), kept, now)

	if !ok || len(got.ids) != 0 {
		t.Errorf("waitingIn = %+v, %v; want the kept row skipped", got, ok)
	}
}

func TestWaitingIn_CountsARowWhoseKeepIsDue(t *testing.T) {
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	kept := []keptShell{{Tool: "claude", ID: "a", PID: 531, ProcStart: "start-531", WakeAt: now, Keeps: 1}}

	got, ok := waitingIn([]byte(shellOnlyList), kept, now)

	if !ok || got.ids["claude:a"] != "claude" {
		t.Errorf("waitingIn = %+v, %v; want the due row counted", got, ok)
	}
}

func TestWaitingIn_CountsARowWhenTheKeptShellStartedAtAnotherTime(t *testing.T) {
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	kept := []keptShell{{Tool: "claude", ID: "a", PID: 531, ProcStart: "another start", WakeAt: now.Add(time.Hour), Keeps: 1}}

	got, _ := waitingIn([]byte(shellOnlyList), kept, now)

	if got.ids["claude:a"] != "claude" {
		t.Errorf("waitingIn = %+v; a reused PID matched an old record", got)
	}
}

func keptShellsOf(t *testing.T, rec *httptest.ResponseRecorder) []keptShell {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("= %d %s", rec.Code, rec.Body)
	}
	var body struct {
		Shells []keptShell `json:"shells"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Shells
}

func TestKeptShellRoutes_KeepReadAndAskAgain(t *testing.T) {
	s := shellWith(t, &fakeShortcuts{})
	var told [][]keptShell
	s.tellKeptShells = func(kept []keptShell) { told = append(told, kept) }
	routes := s.routes(&inbox{}, fstest.MapFS{})
	wake := time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second)
	serve := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rec
	}

	kept := keptShellsOf(t, serve(http.MethodPost, "/api/kept-shells/keep", `{"tool":"claude","id":"a","pid":531,"procStart":"start-531","wakeAt":"`+wake.Format(time.RFC3339)+`","keeps":1}`))
	if len(kept) != 1 || kept[0].PID != 531 || !kept[0].WakeAt.Equal(wake) || kept[0].Keeps != 1 {
		t.Fatalf("after keep = %+v", kept)
	}
	later := wake.Add(30 * time.Minute)
	kept = keptShellsOf(t, serve(http.MethodPost, "/api/kept-shells/keep", `{"tool":"claude","id":"a","pid":531,"procStart":"start-531","wakeAt":"`+later.Format(time.RFC3339)+`","keeps":2}`))
	if len(kept) != 1 || kept[0].Keeps != 2 || !kept[0].WakeAt.Equal(later) {
		t.Fatalf("a second keep of the same shell = %+v; want it replaced", kept)
	}
	saved, err := readSettings(s.settingsPath)
	if err != nil || len(saved.KeptShells) != 1 {
		t.Fatalf("saved = %+v, %v", saved.KeptShells, err)
	}
	if read := keptShellsOf(t, serve(http.MethodGet, "/api/kept-shells", "")); len(read) != 1 {
		t.Errorf("read = %+v", read)
	}

	kept = keptShellsOf(t, serve(http.MethodPost, "/api/kept-shells/forget", `{"shells":[{"pid":531,"procStart":"start-531"}]}`))
	if len(kept) != 0 {
		t.Errorf("after ask again = %+v", kept)
	}
	if len(told) != 3 || len(told[2]) != 0 {
		t.Errorf("the pages were told %+v", told)
	}
}

func TestKeptShellRoutes_RefuseAKeepWithoutAShell(t *testing.T) {
	s := shellWith(t, &fakeShortcuts{})
	routes := s.routes(&inbox{}, fstest.MapFS{})
	wake := time.Now().Add(30 * time.Minute).Format(time.RFC3339)
	for _, body := range []string{
		`{}`,
		`{"tool":"claude","id":"a","procStart":"s","wakeAt":"` + wake + `","keeps":1}`,
		`{"tool":"claude","id":"a","pid":531,"wakeAt":"` + wake + `","keeps":1}`,
		`{"tool":"claude","id":"a","pid":531,"procStart":"s","keeps":1}`,
		`{"tool":"claude","id":"a","pid":531,"procStart":"s","wakeAt":"` + wake + `"}`,
	} {
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/kept-shells/keep", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d %s", body, rec.Code, rec.Body)
		}
	}
}
