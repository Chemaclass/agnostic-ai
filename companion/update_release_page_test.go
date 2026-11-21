package main

import (
	"errors"
	"net/http"
	"testing"
)

func TestReleasePage_OpensTheKnownReleaseOrTheLatest(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status updateStatus
		want   string
	}{
		{"known release", updateStatus{State: updateAvailable, Version: "0.97.0", Command: upgradeCommand}, "https://github.com/Chemaclass/agnostic-ai/releases/tag/v0.97.0"},
		{"failed release", updateStatus{State: updateFailed, Version: "0.97.0"}, "https://github.com/Chemaclass/agnostic-ai/releases/tag/v0.97.0"},
		{"nothing known", updateStatus{State: updateIdle}, "https://github.com/Chemaclass/agnostic-ai/releases/latest"},
		{"not a stable version", updateStatus{State: updateAvailable, Version: "0.97.0/../x"}, "https://github.com/Chemaclass/agnostic-ai/releases/latest"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, _ := updateShell(t, "", nil)
			var opened []string
			s.openPage = func(url string) error { opened = append(opened, url); return nil }
			s.appCommands.current = tc.status

			if rec := post(s, "/api/update/release-page"); rec.Code != http.StatusNoContent {
				t.Fatalf("POST = %d %s", rec.Code, rec.Body)
			}
			if len(opened) != 1 || opened[0] != tc.want {
				t.Errorf("opened %q; want %q", opened, tc.want)
			}
		})
	}
}

func TestReleasePage_SaysWhenTheBrowserDidNotOpen(t *testing.T) {
	s, _, _ := updateShell(t, "", nil)
	s.openPage = func(string) error { return errors.New("no browser") }

	if rec := post(s, "/api/update/release-page"); rec.Code != http.StatusInternalServerError {
		t.Errorf("POST = %d; want 500", rec.Code)
	}
}

func TestReleasePage_NotFoundWithoutAnUpdateEntry(t *testing.T) {
	s, _, _, _ := menuShell(t)
	s.openPage = func(string) error { t.Error("opened a page"); return nil }

	if rec := post(s, "/api/update/release-page"); rec.Code != http.StatusNotFound {
		t.Errorf("POST = %d; want 404", rec.Code)
	}
}
