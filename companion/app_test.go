package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestFindBinary_UsesBundledCommandBeforeOtherLocations(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("APPDATA", home)
	t.Setenv("PATH", "")
	t.Setenv("AGNOSTIC_AI_BIN", "")
	previousFolders, previousSystem := usualFolders, usualSystemFolders
	t.Cleanup(func() {
		usualFolders, usualSystemFolders = previousFolders, previousSystem
	})
	usualFolders, usualSystemFolders = nil, nil
	command := fakeBundledCommand(t)
	found, err := findBinary()
	if err != nil || found != command {
		t.Errorf("findBinary() = %q, %v; want bundled %q", found, err, command)
	}
	external := filepath.Join(home, binaryName)
	if runtime.GOOS == "windows" {
		external += ".exe"
	}
	if err := os.WriteFile(external, []byte("external"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", home)
	found, err = findBinary()
	if err != nil || found != command {
		t.Errorf("with an older command on PATH got %q, %v; want bundled %q", found, err, command)
	}
	t.Setenv("AGNOSTIC_AI_BIN", external)
	found, err = findBinary()
	if err != nil || found != external {
		t.Errorf("findBinary() = %q, %v; want external %q", found, err, external)
	}
}

func TestLatestCompanionRelease_OffersOnlyNewStableVersions(t *testing.T) {
	cases := []struct {
		current, tag      string
		draft, prerelease bool
		want              string
	}{
		{"0.95.0", "v0.96.0", false, false, "0.96.0"},
		{"0.95.0", "v0.95.0", false, false, ""},
		{"0.96.0", "v0.95.0", false, false, ""},
		{"0.95.0", "v0.97.0-rc.1", false, true, ""},
		{"0.95.0", "v0.97.0", true, false, ""},
		{"dev", "v0.97.0", false, false, ""},
		{"0.9.0", "v0.10.0", false, false, "0.10.0"},
	}
	for _, tc := range cases {
		t.Run(tc.current+"/"+tc.tag, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Accept") != "application/vnd.github+json" {
					t.Error("missing GitHub accept header")
				}
				_, _ = fmt.Fprintf(w, `{"tag_name":%q,"draft":%t,"prerelease":%t,"assets":[{"name":%q,"state":"uploaded","size":123}]}`, tc.tag, tc.draft, tc.prerelease, updateImageName())
			}))
			defer srv.Close()
			got, err := latestCompanionRelease(context.Background(), srv.Client(), srv.URL, tc.current, updateImageName())
			if err != nil || got != tc.want {
				t.Errorf("latest release = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestLatestCompanionRelease_RequiresImageForThisMac(t *testing.T) {
	const arm = "agnostic-ai-companion_darwin_arm64.dmg"
	for _, asset := range []releaseAsset{
		{},
		{Name: "agnostic-ai-companion_darwin_amd64.dmg", State: "uploaded", Size: 100},
		{Name: "agnostic-ai-companion_darwin_arm64.tar.gz", State: "uploaded", Size: 100},
		{Name: arm, State: "starter", Size: 100},
		{Name: arm, State: "uploaded", Size: 0},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v0.96.0", "assets": []releaseAsset{asset}})
		}))
		got, err := latestCompanionRelease(t.Context(), srv.Client(), srv.URL, "0.95.0", arm)
		srv.Close()
		if err != nil || got != "" {
			t.Errorf("with asset %+v got %q, %v; want no update", asset, got, err)
		}
	}
}

func TestLatestCompanionRelease_ReportsFailedChecks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "rate limited", http.StatusForbidden) }))
	defer srv.Close()
	_, err := latestCompanionRelease(context.Background(), srv.Client(), srv.URL, "0.95.0", "agnostic-ai-companion_darwin_arm64.dmg")
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("failed release check = %v", err)
	}
}

func fakeBundledCommand(t *testing.T) string {
	t.Helper()
	app := filepath.Join(t.TempDir(), "Applications", "agnostic-ai companion.app")
	command := filepath.Join(app, "Contents", "Resources", binaryName)
	if err := os.MkdirAll(filepath.Dir(command), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(command, []byte("command"), 0o700); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(app, "Contents", "MacOS", "agnostic-ai-companion")
	if err := os.MkdirAll(filepath.Dir(executable), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("companion"), 0o700); err != nil {
		t.Fatal(err)
	}
	previousExecutable := companionExecutable
	t.Cleanup(func() { companionExecutable = previousExecutable })
	companionExecutable = func() (string, error) { return filepath.Join(app, "Contents", "MacOS", "agnostic-ai-companion"), nil }
	command, err := filepath.EvalSymlinks(command)
	if err != nil {
		t.Fatal(err)
	}
	return command
}

func menuLabels(menu *application.Menu) []string {
	var got []string
	for i := 0; menu.ItemAt(i) != nil; i++ {
		item := menu.ItemAt(i)
		if item.IsSeparator() {
			got = append(got, "---")
			continue
		}
		got = append(got, item.Label())
	}
	return got
}

func TestAppMenu_OffersUpdatesAndInstallAboveHideAndQuit(t *testing.T) {
	command := fakeBundledCommand(t)
	s, _, _, menu := menuShell(t)
	if s.appCommands == nil || s.appCommands.command != command {
		t.Fatalf("app controls = %+v; want bundled command %s", s.appCommands, command)
	}
	got := strings.Join(menuLabels(menu)[7:], "|")
	want := "Check for updates|Install command line tool|---|Hide Agnostic AI companion|Quit Agnostic AI companion"
	if got != want {
		t.Errorf("menu ends with %q; want %q", got, want)
	}
	for _, label := range []string{"Install command line tool", "Check for updates"} {
		if item := menu.FindByLabel(label); item == nil || !item.Enabled() {
			t.Errorf("menu has no enabled %q item", label)
		}
	}
}

func TestAppMenu_TerminalInstallHasNoUpdateEntryYet(t *testing.T) {
	s, _, _, menu := menuShell(t)
	if s.appCommands != nil || menu.FindByLabel("Check for updates") != nil {
		t.Fatalf("a companion without a bundled command offers updates")
	}
	if got := s.updateStatus(); got.State != updateUnavailable {
		t.Errorf("update status = %+v; want unavailable", got)
	}
}

func TestUpdateLabel_FollowsTheState(t *testing.T) {
	for _, tc := range []struct {
		status updateStatus
		want   string
	}{
		{updateStatus{State: updateIdle}, "Check for updates"},
		{updateStatus{State: updateChecking}, "Checking for updates…"},
		{updateStatus{State: updateAvailable, Version: "0.97.0"}, "Update to v0.97.0"},
		{updateStatus{State: updateUpdating, Version: "0.97.0"}, "Updating to v0.97.0…"},
		{updateStatus{State: updateFailed, Version: "0.97.0"}, "Update to v0.97.0"},
	} {
		if got := updateLabel("Check for updates", tc.status); got != tc.want {
			t.Errorf("updateLabel(%+v) = %q; want %q", tc.status, got, tc.want)
		}
	}
}

// updateShell is a disk image companion whose release check answers next, err.
func updateShell(t *testing.T, next string, err error) (*shell, *[]updateStatus, *application.Menu) {
	t.Helper()
	fakeBundledCommand(t)
	s, _, _, menu := menuShell(t)
	told := []updateStatus{}
	s.tellUpdate = func(status updateStatus) { told = append(told, status) }
	s.appCommands.latest = func(context.Context) (string, error) { return next, err }
	return s, &told, menu
}

func TestCheckUpdates_AutomaticCheckShowsTheReleaseWithoutADialog(t *testing.T) {
	s, told, menu := updateShell(t, "0.97.0", nil)

	// A dialog would need the app, which this shell does not have.
	s.appCommands.checkUpdates(t.Context(), false)

	want := []updateStatus{{State: updateChecking}, {State: updateAvailable, Version: "0.97.0"}}
	if fmt.Sprint(*told) != fmt.Sprint(want) {
		t.Errorf("the pages were told %+v; want %+v", *told, want)
	}
	if item := menu.FindByLabel("Update to v0.97.0"); item == nil || !item.Enabled() {
		t.Errorf("menu = %q; want an enabled Update to v0.97.0", menuLabels(menu))
	}
	rec := httptest.NewRecorder()
	s.routes(&inbox{}, fstest.MapFS{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/update", nil))
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"state":"available","version":"0.97.0"}` {
		t.Errorf("GET /api/update = %d %s", rec.Code, rec.Body)
	}
}

func TestCheckUpdates_DisablesTheEntryWhileChecking(t *testing.T) {
	s, _, menu := updateShell(t, "", nil)
	s.appCommands.latest = func(context.Context) (string, error) {
		if item := menu.FindByLabel("Checking for updates…"); item == nil || item.Enabled() {
			t.Errorf("while checking, menu = %q; want a disabled Checking for updates…", menuLabels(menu))
		}
		return "", nil
	}

	s.appCommands.checkUpdates(t.Context(), false)

	if got := s.updateStatus(); got.State != updateIdle {
		t.Errorf("after finding nothing, status = %+v; want idle", got)
	}
}

func TestCheckUpdates_FailedAutomaticCheckKeepsTheLastAnswerSilently(t *testing.T) {
	s, told, _ := updateShell(t, "0.97.0", nil)
	s.appCommands.checkUpdates(t.Context(), false)
	s.appCommands.latest = func(context.Context) (string, error) { return "", errors.New("offline") }

	s.appCommands.checkUpdates(t.Context(), false)

	if got := s.updateStatus(); got != (updateStatus{State: updateAvailable, Version: "0.97.0"}) {
		t.Errorf("after a failed check, status = %+v; want the last answer", got)
	}
	if checking := (*told)[len(*told)-2]; checking != (updateStatus{State: updateChecking, Version: "0.97.0"}) {
		t.Errorf("while checking again, the pages were told %+v; want the known version kept", checking)
	}
	if last := (*told)[len(*told)-1]; last.State != updateAvailable {
		t.Errorf("the pages were last told %+v", last)
	}
}

func TestRunUpdate_ManualCheckSaysWhenItCouldNotReachTheRelease(t *testing.T) {
	s, _, _ := updateShell(t, "", errors.New("offline"))
	said := make(chan string, 1)
	s.appCommands.inform = func(_, message string) { said <- message }

	rec := httptest.NewRecorder()
	s.routes(&inbox{}, fstest.MapFS{}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/update", nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("POST /api/update = %d", rec.Code)
	}
	if got := <-said; got != "Could not check for updates. Check your connection and try again." {
		t.Errorf("the manual check said %q", got)
	}
	if got := s.updateStatus(); got.State != updateIdle {
		t.Errorf("after a failed manual check, status = %+v; want idle", got)
	}
}
