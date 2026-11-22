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
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

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
	want := "Check for updates automatically|Check for updates|Install command line tool|---|Hide Agnostic AI companion|Quit Agnostic AI companion"
	if got != want {
		t.Errorf("menu ends with %q; want %q", got, want)
	}
	for _, label := range []string{"Install command line tool", "Check for updates"} {
		if item := menu.FindByLabel(label); item == nil || !item.Enabled() {
			t.Errorf("menu has no enabled %q item", label)
		}
	}
}

// fakeTerminalCommand is the agnostic-ai command a terminal-installed
// companion runs: a script that answers update-status with status and
// fails upgrade with failure. It returns the file that holds each call's arguments.
func fakeTerminalCommand(t *testing.T, status, failure string) (calls string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake command is a shell script")
	}
	dir := t.TempDir()
	command, calls := filepath.Join(dir, binaryName), filepath.Join(dir, "calls")
	script := fmt.Sprintf("#!/bin/sh\necho \"$@\" >> %q\nif [ \"$1\" = companion ]; then echo %q; exit 0; fi\necho %q >&2\nexit 1\n", calls, status, failure)
	if err := os.WriteFile(command, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	previous := terminalCommand
	t.Cleanup(func() { terminalCommand = previous })
	terminalCommand = func() (string, error) { return command, nil }
	return calls
}

func withVersion(t *testing.T, running string) {
	t.Helper()
	previous := version
	t.Cleanup(func() { version = previous })
	version = running
}

func TestAppMenu_TerminalInstallChecksForUpdatesWithoutTheInstallItem(t *testing.T) {
	fakeTerminalCommand(t, `{"installed":"0.96.0","latest":"0.96.0","method":"homebrew","oneClick":true,"confirmsRelease":true}`, "")
	s, _, _, menu := menuShell(t)

	if s.appCommands == nil || menu.FindByLabel("Check for updates") == nil {
		t.Fatalf("menu = %q; want Check for updates", menuLabels(menu))
	}
	if menu.FindByLabel("Install command line tool") != nil {
		t.Error("a terminal install offers to install the command it already has")
	}
}

func TestAppMenu_NoCommandMeansNoUpdateEntry(t *testing.T) {
	s, _, _, menu := menuShell(t)
	if s.appCommands != nil || menu.FindByLabel("Check for updates") != nil {
		t.Fatalf("a companion without any agnostic-ai command offers updates")
	}
	if got := s.updateStatus(); got.State != updateUnavailable {
		t.Errorf("update status = %+v; want unavailable", got)
	}
}

func TestTerminalUpdate_OffersTheLatestReleaseTheCommandReports(t *testing.T) {
	for _, tc := range []struct {
		name     string
		oneClick bool
		command  string
	}{
		{"macOS runs agnostic-ai upgrade", true, ""},
		{"elsewhere shows the command", false, upgradeCommand},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withVersion(t, "0.96.0")
			previous := terminalOneClick
			t.Cleanup(func() { terminalOneClick = previous })
			terminalOneClick = tc.oneClick
			calls := fakeTerminalCommand(t, `{"installed":"0.96.0","latest":"0.97.0","method":"homebrew","oneClick":true,"confirmsRelease":true}`, "")
			s, _, _, _ := menuShell(t)

			s.appCommands.checkUpdates(t.Context())

			if got := s.updateStatus(); got.State != updateAvailable || got.Version != "0.97.0" || got.Command != tc.command {
				t.Errorf("status = %+v; want 0.97.0 with command %q", got, tc.command)
			}
			if raw, _ := os.ReadFile(calls); strings.TrimSpace(string(raw)) != "companion update-status --json" {
				t.Errorf("calls = %q", raw)
			}
		})
	}
}

func TestTerminalUpdate_ShowsTheCommandWhenItCannotRunFromTheApp(t *testing.T) {
	withVersion(t, "0.96.0")
	fakeTerminalCommand(t, `{"installed":"0.96.0","latest":"0.97.0","method":"unknown","oneClick":false}`, "")
	s, _, _, _ := menuShell(t)

	s.appCommands.checkUpdates(t.Context())

	if got := s.updateStatus(); got.State != updateAvailable || got.Command != upgradeCommand {
		t.Errorf("status = %+v; want the command to copy", got)
	}
	if rec := post(s, "/api/update/start"); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), upgradeCommand) {
		t.Errorf("start = %d %s; want a refusal naming the command", rec.Code, rec.Body)
	}
}

func TestTerminalUpdate_SaysWhenUpgradeLeftTheCompanionRunning(t *testing.T) {
	withVersion(t, "0.96.0")
	fakeTerminalCommand(t, `{"installed":"0.96.0","latest":"0.97.0","method":"homebrew","oneClick":true,"confirmsRelease":true}`, "")
	s, _, _, _ := menuShell(t)
	s.appCommands.checkUpdates(t.Context())
	script := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(s.appCommands.command, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	s.appCommands.current = updateStatus{State: updateAvailable, Version: "0.97.0"}

	s.appCommands.update("0.97.0")

	if got := s.updateStatus(); got.State != updateFailed || got.Reason != "the companion did not restart. Quit it and open it again" || got.Step != "" {
		t.Errorf("status = %+v", got)
	}
}

func TestTerminalUpdate_RunsUpgradeAndNamesThePackageManagerDelay(t *testing.T) {
	previous := terminalOneClick
	t.Cleanup(func() { terminalOneClick = previous })
	terminalOneClick = true
	withVersion(t, "0.96.0")
	calls := fakeTerminalCommand(t, `{"installed":"0.96.0","latest":"0.97.0","method":"homebrew","oneClick":true,"confirmsRelease":true}`,
		"Error: the CLI is still 0.96.0: Homebrew does not serve 0.97.0 yet. Run agnostic-ai upgrade again later")
	s, _, _, _ := menuShell(t)
	s.appCommands.checkUpdates(t.Context())
	done := make(chan updateStatus, 8)
	s.tellUpdate = func(status updateStatus) { done <- status }

	if rec := post(s, "/api/update/start"); rec.Code != http.StatusNoContent {
		t.Fatalf("start = %d %s", rec.Code, rec.Body)
	}

	var got updateStatus
	for got.State != updateFailed {
		got = <-done
	}
	if got.Reason != "the package manager does not serve the new version yet. Try again later" || got.Command != upgradeCommand {
		t.Errorf("failed status = %+v", got)
	}
	if raw, _ := os.ReadFile(calls); !strings.HasSuffix(strings.TrimSpace(string(raw)), "\nupgrade --companion-confirmed-release 0.97.0") {
		t.Errorf("calls = %q; want upgrade last", raw)
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

// updateFeed is what the pages were told about updates.
type updateFeed struct {
	mu   sync.Mutex
	told []updateStatus
}

func (f *updateFeed) all() []updateStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.told)
}

func (f *updateFeed) last() updateStatus {
	all := f.all()
	if len(all) == 0 {
		return updateStatus{}
	}
	return all[len(all)-1]
}

// updateShell is a disk image companion whose release check answers next, err.
func updateShell(t *testing.T, next string, err error) (*shell, *updateFeed, *application.Menu) {
	t.Helper()
	fakeBundledCommand(t)
	s, _, _, menu := menuShell(t)
	feed := &updateFeed{}
	s.tellUpdate = func(status updateStatus) {
		feed.mu.Lock()
		defer feed.mu.Unlock()
		feed.told = append(feed.told, status)
	}
	s.appCommands.latest = func(context.Context, bool) (release, error) { return release{Version: next}, err }
	s.appCommands.ask = func() { t.Error("asked to update") }
	return s, feed, menu
}

func post(s *shell, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	s.routes(&inbox{}, fstest.MapFS{}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
	return rec
}

func TestCheckUpdates_AutomaticCheckShowsTheReleaseWithoutAsking(t *testing.T) {
	s, feed, menu := updateShell(t, "0.97.0", nil)

	s.appCommands.checkUpdates(t.Context())

	want := []updateStatus{{State: updateChecking, Current: version}, {State: updateAvailable, Version: "0.97.0", Current: version}}
	if fmt.Sprint(feed.all()) != fmt.Sprint(want) {
		t.Errorf("the pages were told %+v; want %+v", feed.all(), want)
	}
	if item := menu.FindByLabel("Update to v0.97.0"); item == nil || !item.Enabled() {
		t.Errorf("menu = %q; want an enabled Update to v0.97.0", menuLabels(menu))
	}
	rec := httptest.NewRecorder()
	s.routes(&inbox{}, fstest.MapFS{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/update", nil))
	var got updateStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got != want[1] {
		t.Errorf("GET /api/update = %d %s", rec.Code, rec.Body)
	}
}

func TestCheckUpdates_DisablesTheEntryWhileChecking(t *testing.T) {
	s, _, menu := updateShell(t, "", nil)
	s.appCommands.latest = func(context.Context, bool) (release, error) {
		if item := menu.FindByLabel("Checking for updates…"); item == nil || item.Enabled() {
			t.Errorf("while checking, menu = %q; want a disabled Checking for updates…", menuLabels(menu))
		}
		return release{}, nil
	}

	s.appCommands.checkUpdates(t.Context())

	if got := s.updateStatus(); got.State != updateIdle || got.Result != "" {
		t.Errorf("after finding nothing on its own, status = %+v; want idle and quiet", got)
	}
}

func TestCheckUpdates_FailedAutomaticCheckKeepsTheLastAnswerSilently(t *testing.T) {
	s, feed, _ := updateShell(t, "0.97.0", nil)
	s.appCommands.checkUpdates(t.Context())
	s.appCommands.latest = func(context.Context, bool) (release, error) { return release{}, errors.New("offline") }

	s.appCommands.checkUpdates(t.Context())

	if got := s.updateStatus(); got != (updateStatus{State: updateAvailable, Version: "0.97.0", Current: version}) {
		t.Errorf("after a failed check, status = %+v; want the last answer", got)
	}
	all := feed.all()
	if checking := all[len(all)-2]; checking.State != updateChecking || checking.Version != "0.97.0" {
		t.Errorf("while checking again, the pages were told %+v; want the known version kept", checking)
	}
}

func TestCheckNow_SaysWhatTheCheckFound(t *testing.T) {
	for _, tc := range []struct {
		name   string
		next   string
		err    error
		result string
	}{
		{"latest", "", nil, updateLatest},
		{"offline", "", errors.New("offline"), updateUnreachable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, feed, _ := updateShell(t, tc.next, tc.err)

			if rec := post(s, "/api/update"); rec.Code != http.StatusNoContent {
				t.Fatalf("POST /api/update = %d %s", rec.Code, rec.Body)
			}

			waitFor(t, "the check", func() bool { return feed.last().Result != "" })
			if got := feed.last(); got.State != updateIdle || got.Result != tc.result {
				t.Errorf("after the check, the pages were told %+v; want idle with %s", got, tc.result)
			}
			if kept := s.updateStatus(); kept.Result != "" {
				t.Errorf("the result is kept for later reads: %+v", kept)
			}
		})
	}
}

func TestCheckNow_AsksBeforeUpdatingToTheReleaseItFound(t *testing.T) {
	s, _, _ := updateShell(t, "0.97.0", nil)
	asked := make(chan updateStatus, 1)
	s.appCommands.ask = func() { asked <- s.updateStatus() }

	post(s, "/api/update")

	if got := <-asked; got.State != updateAvailable || got.Version != "0.97.0" {
		t.Errorf("asked with status %+v", got)
	}
}

func TestStartUpdate_RefusesWithoutAKnownReleaseOrWhileBusy(t *testing.T) {
	s, _, _ := updateShell(t, "0.97.0", nil)
	if rec := post(s, "/api/update/start"); rec.Code != http.StatusConflict {
		t.Errorf("start with no known release = %d", rec.Code)
	}
	s.appCommands.checkUpdates(t.Context())
	s.appCommands.busy.Lock()
	defer s.appCommands.busy.Unlock()
	if rec := post(s, "/api/update/start"); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "busy") {
		t.Errorf("start while busy = %d %s", rec.Code, rec.Body)
	}
	if rec := post(s, "/api/update"); rec.Code != http.StatusConflict {
		t.Errorf("check while busy = %d", rec.Code)
	}
}

func TestStartUpdate_FollowsTheStepsAndNamesTheFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake updater is a shell script")
	}
	s, feed, _ := updateShell(t, "0.97.0", nil)
	s.appCommands.checkUpdates(t.Context())
	script := "#!/bin/sh\necho 'Downloading agnostic-ai 0.97.0.'\nsleep 0.4\necho 'Checking the downloaded app.'\nsleep 0.4\necho 'codesign: invalid signature' >&2\nexit 1\n"
	if err := os.WriteFile(s.appCommands.command, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	if rec := post(s, "/api/update/start"); rec.Code != http.StatusNoContent {
		t.Fatalf("POST /api/update/start = %d %s", rec.Code, rec.Body)
	}

	waitFor(t, "the update to fail", func() bool { return feed.last().State == updateFailed })
	var steps []string
	for _, status := range feed.all() {
		if status.State == updateUpdating {
			steps = append(steps, status.Step)
		}
	}
	if strings.Join(steps, ",") != "downloading,verifying" {
		t.Errorf("steps = %q", steps)
	}
	got := feed.last()
	logPath := filepath.Join(filepath.Dir(s.settingsPath), "companion-update.log")
	if got.Version != "0.97.0" || got.Reason != "the downloaded app failed its signature check" || got.Log != logPath {
		t.Errorf("failed status = %+v", got)
	}
	if label := updateLabel("Check for updates", got); label != "Update to v0.97.0" {
		t.Errorf("after a failure the entry reads %q", label)
	}
}

func TestUpdateFailureReason_NamesTheCheckInPlainWords(t *testing.T) {
	for _, tc := range []struct{ log, step, want string }{
		{"prepare /A.app: agnostic-ai-companion_darwin_arm64.dmg: checksum mismatch", updateDownloading, "the download did not match its checksum"},
		{"x.dmg: build provenance did not verify: gh said no", updateDownloading, "the download's build provenance did not verify"},
		{"codesign: exit status 1", updateVerifying, "the downloaded app failed its signature check"},
		{`/A.app: companion version "0.96.0", expected 0.97.0`, updateVerifying, "the download is not the version it should be"},
		{`checksums.txt: Get "https://github.com/x/checksums.txt": dial tcp: lookup github.com: no such host`, updateDownloading, "the release could not be downloaded. Check your connection"},
		{"x.dmg: download: 404 Not Found", updateDownloading, "the release could not be downloaded. Check your connection"},
		{"/Volumes/agnostic-ai/agnostic-ai companion.app cannot be updated where it is (read-only file system). Move the app to Applications, open it from there, then update", updateDownloading, "the app is not in a folder that can be updated. Move it to Applications, open it from there, then update"},
		{"/A/.companion-update-1/agnostic-ai companion.app is not signed by this project: codesign: exit status 3", updateVerifying, "the download is not signed by this project"},
		{"/A.app: the app that runs is not signed with a project certificate. Download the new disk image by hand once", updateVerifying, "the app that runs is not signed by this project. Download the new disk image by hand once"},
		{"checksums.txt: invalid checksum for x.dmg", updateDownloading, "the release's checksum list could not be read"},
		{"x.dmg: v0.1.0 predates build provenance, which starts at v0.50.0", updateDownloading, "the update command failed"},
		{"replace /A.app: permission denied", updateRestarting, "the update command failed"},
		{"Restarting the companion.\nCould not replace the companion app.\nStarted the earlier companion.\nError: replace /A.app: permission denied", updateRestarting, "the new app could not replace the earlier one, which was kept"},
		{"Restarting the companion.\nSwapped the companion app.\nCould not start the updated companion.\nRestored the earlier companion.\nStarted the earlier companion.\n", updateRestarting, "the updated companion could not start, so the earlier one was restored"},
		{"Restarting the companion.\nSwapped the companion app.\nCould not start the updated companion.\n", updateRestarting, "the updated companion could not restart"},
		{"Restarting the companion.\nCould not replace the companion app.\nCould not start the earlier companion.\n", updateRestarting, "the earlier companion could not restart. Open it again"},
		{"Restarting the companion.\nSwapped the companion app.\nCould not start the updated companion.\nRestored the earlier companion.\nCould not start the earlier companion.\n", updateRestarting, "the earlier companion could not restart. Open it again"},
		{"Restarting the companion.\nCould not replace the companion app.\nCould not restore the earlier companion. rename failed\n", updateRestarting, "the earlier companion could not be restored. See the log for its saved location"},
		{"→ brew update && brew upgrade --cask Chemaclass/tap/agnostic-ai\nsh: brew: command not found\nError: upgrade: run brew: exit status 127", updateDownloading, "the package manager could not run from the companion"},
	} {
		if got := updateFailureReason(tc.log, tc.step); got != tc.want {
			t.Errorf("updateFailureReason(%q) = %q; want %q", tc.log, got, tc.want)
		}
	}
}

func TestUpdateStep_IsTheLastStepTheLogNames(t *testing.T) {
	for _, tc := range []struct{ log, want string }{
		{"", updateDownloading},
		{"Downloading agnostic-ai 0.97.0.\n", updateDownloading},
		{"Downloading agnostic-ai 0.97.0.\nBuild provenance verified.\nChecking the downloaded app.\n", updateVerifying},
		{"Downloading agnostic-ai 0.97.0.\nChecking the downloaded app.\nRestarting the companion.\n", updateRestarting},
	} {
		if got := updateStep(tc.log); got != tc.want {
			t.Errorf("updateStep(%q) = %q; want %q", tc.log, got, tc.want)
		}
	}
}

// bundledScript makes the disk image app's bundled command a script that
// answers update-status with status, and returns the file of its calls.
func bundledScript(t *testing.T, status string) (*shell, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake command is a shell script")
	}
	command := fakeBundledCommand(t)
	calls := filepath.Join(t.TempDir(), "calls")
	script := fmt.Sprintf("#!/bin/sh\necho \"$@\" >> %q\necho %q\n", calls, status)
	if err := os.WriteFile(command, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	s, _, _, _ := menuShell(t)
	s.appCommands.ask = func() {}
	return s, calls
}

func TestDiskImageUpdate_OffersOnlyAReleaseWithADiskImage(t *testing.T) {
	withVersion(t, "0.96.0")
	for _, tc := range []struct {
		image bool
		want  string
	}{{false, ""}, {true, "0.97.0"}} {
		s, _ := bundledScript(t, fmt.Sprintf(`{"installed":"0.96.0","latest":"0.97.0","method":"binary","oneClick":true,"image":%t}`, tc.image))

		s.appCommands.checkUpdates(t.Context())

		if got := s.updateStatus(); got.Version != tc.want || got.Command != "" {
			t.Errorf("with image %v, status = %+v; want version %q", tc.image, got, tc.want)
		}
	}
}

func TestCLIRelease_OnlyTheDiskImageAppFailsOnAnUnreadableImageList(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake command is a shell script")
	}
	command := filepath.Join(t.TempDir(), "agnostic-ai")
	status := `{"installed":"0.96.0","latest":"0.97.0","method":"homebrew","oneClick":true,"confirmsRelease":true,"image":false,"imageError":"checksums.txt: 502"}`
	if err := os.WriteFile(command, []byte(fmt.Sprintf("#!/bin/sh\necho %q\n", status)), 0o700); err != nil {
		t.Fatal(err)
	}

	if _, err := cliRelease(t.Context(), command, "0.96.0", true, true); err == nil || !strings.Contains(err.Error(), "502") {
		t.Errorf("disk image app: err = %v; want the image list error", err)
	}
	if next, err := cliRelease(t.Context(), command, "0.96.0", true, false); err != nil || next.Version != "0.97.0" {
		t.Errorf("terminal install = %+v, %v; want 0.97.0", next, err)
	}
}

func TestDiskImageUpdate_AManualCheckAsksTheCLIToRefresh(t *testing.T) {
	withVersion(t, "0.96.0")
	s, calls := bundledScript(t, `{"installed":"0.96.0","latest":"0.96.0","method":"binary","oneClick":true,"image":false}`)

	s.appCommands.checkUpdates(t.Context())
	if rec := post(s, "/api/update"); rec.Code != http.StatusNoContent {
		t.Fatalf("POST /api/update = %d", rec.Code)
	}

	waitFor(t, "the manual check", func() bool {
		if !s.appCommands.busy.TryLock() {
			return false
		}
		s.appCommands.busy.Unlock()
		raw, _ := os.ReadFile(calls)
		return strings.Count(string(raw), "\n") == 2
	})
	raw, _ := os.ReadFile(calls)
	if string(raw) != "companion update-status --json\ncompanion update-status --json --refresh\n" {
		t.Errorf("calls = %q", raw)
	}
}

func TestFailedUpdateAfterRestart_ShowsARollbackOnceOnTheNextStart(t *testing.T) {
	withVersion(t, "0.96.0")
	for _, tc := range []struct {
		name, log string
		want      updateStatus
	}{
		{"rollback", "Downloading agnostic-ai 0.97.0.\nChecking the downloaded app.\nRestarting the companion.\nError: replace /A.app: permission denied\n",
			updateStatus{State: updateFailed, Version: "0.97.0", Reason: "the update command failed"}},
		{"finished", "Downloading agnostic-ai 0.97.0.\nRestarting the companion.\nReplaced the companion app.\n", updateStatus{State: updateIdle}},
		{"failed before the restart", "Downloading agnostic-ai 0.97.0.\nError: checksum mismatch\n", updateStatus{State: updateIdle}},
		{"no log", "", updateStatus{State: updateIdle}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "companion-update.log")
			if tc.log != "" {
				if err := os.WriteFile(path, []byte(tc.log), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.want.State == updateFailed {
				tc.want.Log = path
			}

			if got := failedUpdateAfterRestart(path, false); got != tc.want {
				t.Errorf("first start = %+v; want %+v", got, tc.want)
			}
			if got := failedUpdateAfterRestart(path, false); got.State != updateIdle {
				t.Errorf("second start = %+v; want idle", got)
			}
		})
	}
}

func TestFailedUpdateAfterRestart_StaysThroughTheAutomaticCheckAtStart(t *testing.T) {
	withVersion(t, "0.96.0")
	s, _ := bundledScript(t, `{"installed":"0.96.0","latest":"0.97.0","method":"binary","oneClick":true,"image":true}`)
	log := "Downloading agnostic-ai 0.97.0.\nRestarting the companion.\nError: replace /A.app: permission denied\n"
	if err := os.WriteFile(s.appCommands.logPath(), []byte(log), 0o600); err != nil {
		t.Fatal(err)
	}
	s.appCommands.current = failedUpdateAfterRestart(s.appCommands.logPath(), s.appCommands.app == "")

	s.appCommands.checkUpdates(t.Context())

	if got := s.updateStatus(); got.State != updateFailed || got.Reason != "the update command failed" {
		t.Errorf("after the start check, status = %+v; want the failure kept", got)
	}
}

func TestFailedUpdateAfterRestart_IgnoresAnUpdateNotNewerThanThisVersion(t *testing.T) {
	for _, running := range []string{"0.97.0", "0.98.0", "0.100.0"} {
		t.Run(running, func(t *testing.T) {
			withVersion(t, running)
			path := filepath.Join(t.TempDir(), "companion-update.log")
			if err := os.WriteFile(path, []byte("Downloading agnostic-ai 0.97.0.\nRestarting the companion.\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if got := failedUpdateAfterRestart(path, false); got.State != updateIdle {
				t.Errorf("status = %+v; want idle", got)
			}
		})
	}
}

func TestTerminalUpdate_DoesNotOfferAReleaseOlderThanTheInstalledCLI(t *testing.T) {
	withVersion(t, "0.96.0")
	fakeTerminalCommand(t, `{"installed":"0.98.0","latest":"0.97.0","method":"binary","oneClick":true}`, "")
	s, _, _, _ := menuShell(t)
	s.appCommands.check(t.Context(), false)
	if got := s.updateStatus(); got.State != updateIdle {
		t.Errorf("status = %+v; want no downgrade offered", got)
	}
}

func TestFailedUpdateAfterRestart_UsesExplicitOutcomes(t *testing.T) {
	withVersion(t, "0.97.0")
	for _, tc := range []struct {
		name, log, reason, command string
	}{
		{"failed DMG restore", "Downloading agnostic-ai 0.97.0.\nRestarting the companion.\nSwapped the companion app.\nCould not start the updated companion.\nCould not restore the earlier companion.\n", "the earlier companion could not be restored. See the log for its saved location", ""},
		{"terminal launch failed", "Updating the terminal companion to 0.97.0.\nTerminal update failed: CLI upgrade succeeded, but companion update failed: start the companion: open failed\n", "the CLI was upgraded, but the companion could not restart", upgradeCommand},
		{"terminal launch restored", "Updating the terminal companion to 0.97.0.\nCould not start the updated companion.\nRestored the earlier companion.\nCould not install the updated companion: start the companion: open failed\nStarted the earlier companion.\n", "the updated companion could not start, so the earlier one was restored", upgradeCommand},
		{"terminal upgrade finished", "Updating the terminal companion to 0.97.0.\nTerminal update completed.\n", "", ""},
		{"terminal upgrade in progress", "Updating the terminal companion to 0.97.0.\n", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "companion-update.log")
			if err := os.WriteFile(path, []byte(tc.log), 0o600); err != nil {
				t.Fatal(err)
			}
			terminal := strings.HasPrefix(tc.log, terminalTarget)
			if wrong := failedUpdateAfterRestart(path, !terminal); wrong.State != updateIdle {
				t.Errorf("other install type got status %+v; want idle", wrong)
			}
			got := failedUpdateAfterRestart(path, terminal)
			if tc.reason == "" {
				if got.State != updateIdle {
					t.Errorf("status = %+v; want idle", got)
				}
			} else if got.State != updateFailed || got.Reason != tc.reason || got.Log != path || got.Command != tc.command {
				t.Errorf("status = %+v; want %q, log path and command %q", got, tc.reason, tc.command)
			}
		})
	}
}

func TestFailedUpdateAfterRestart_SaysWhetherTheEarlierAppWasKeptOrRestored(t *testing.T) {
	withVersion(t, "0.96.0")
	for _, tc := range []struct{ name, log, reason string }{
		{"swap failed", "Downloading agnostic-ai 0.97.0.\nRestarting the companion.\nCould not replace the companion app.\nStarted the earlier companion.\n", "the new app could not replace the earlier one, which was kept"},
		{"rolled back", "Downloading agnostic-ai 0.97.0.\nRestarting the companion.\nSwapped the companion app.\nCould not start the updated companion.\nRestored the earlier companion.\nStarted the earlier companion.\n", "the updated companion could not start, so the earlier one was restored"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "companion-update.log")
			if err := os.WriteFile(path, []byte(tc.log), 0o600); err != nil {
				t.Fatal(err)
			}
			if got := failedUpdateAfterRestart(path, false); got.State != updateFailed || got.Version != "0.97.0" || got.Reason != tc.reason || got.Log != path {
				t.Errorf("status = %+v; want %q with the log path", got, tc.reason)
			}
		})
	}
}

func TestTerminalUpdate_OffersTheRepairWhenTheCLIIsCurrentAndTheCompanionIsOlder(t *testing.T) {
	wasOneClick := terminalOneClick
	terminalOneClick = true
	t.Cleanup(func() { terminalOneClick = wasOneClick })
	withVersion(t, "0.96.0")
	fakeTerminalCommand(t, `{"installed":"0.97.0","latest":"0.97.0","method":"homebrew","oneClick":true,"confirmsRelease":true}`, "")
	s, _, _, _ := menuShell(t)
	s.appCommands.check(t.Context(), false)
	if got := s.updateStatus(); got.State != updateAvailable || got.Version != "0.97.0" || got.Command != "" {
		t.Errorf("status = %+v; want a one-click update to 0.97.0", got)
	}
}

func TestStartUpdate_RefusesAReleaseThatReplacedTheConfirmedOffer(t *testing.T) {
	s, _, _ := updateShell(t, "0.98.0", nil)
	s.appCommands.checkUpdates(t.Context())
	rec := post(s, "/api/update/start?version=0.97.0")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "the offered release changed") {
		t.Errorf("start = %d %s; want a stale confirmation conflict", rec.Code, rec.Body)
	}
}

func TestTerminalUpdate_LegacyCLIUsesCopyCommand(t *testing.T) {
	wasOneClick := terminalOneClick
	terminalOneClick = true
	t.Cleanup(func() { terminalOneClick = wasOneClick })
	withVersion(t, "0.96.0")
	fakeTerminalCommand(t, `{"installed":"0.96.0","latest":"0.97.0","method":"homebrew","oneClick":true}`, "")
	s, _, _, _ := menuShell(t)
	s.appCommands.check(t.Context(), false)
	if got := s.updateStatus(); got.State != updateAvailable || got.Command != upgradeCommand {
		t.Errorf("status = %+v; want the copy command for an older CLI", got)
	}
}

func TestTerminalUpdate_ClearsTheOfferWhenTheReleaseChanges(t *testing.T) {
	withVersion(t, "0.96.0")
	fakeTerminalCommand(t, `{}`, "Terminal update failed: release changed since confirmation: approved 0.97.0, latest 0.98.0")
	s, _, _, _ := menuShell(t)
	var last updateStatus
	s.tellUpdate = func(status updateStatus) { last = status }
	s.appCommands.update("0.97.0")
	if last.State != updateIdle || last.Version != "" || last.Result != updateReleaseChanged || last.Log == "" {
		t.Errorf("status = %+v; want an expired offer and a log", last)
	}
	if got := failedUpdateAfterRestart(s.appCommands.logPath(), s.appCommands.app == ""); got.State != updateIdle {
		t.Errorf("after restart = %+v; want the handled refusal cleared", got)
	}
}

func TestTogglePanel_TheCircleAsksForTheFirstWaitingRow(t *testing.T) {
	s, _ := placedShell(t)
	panel := s.panel.(*fakeWindow)
	toggle := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		s.routes(&inbox{}, fstest.MapFS{}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/panel/toggle", strings.NewReader(body)))
		return rec
	}

	if rec := toggle(`{"jump":true}`); rec.Code != http.StatusNoContent {
		t.Fatalf("toggle = %d %s", rec.Code, rec.Body)
	}
	if !panel.IsVisible() || len(panel.events) != 1 || panel.events[0] != eventPanelShown {
		t.Fatalf("visible %v, events %v; want the list shown once", panel.IsVisible(), panel.events)
	}
	if got, ok := panel.eventData[0][0].(panelOpening); !ok || !got.Jump || got.From == "" {
		t.Errorf("panel-shown data = %#v; want the corner and jump", panel.eventData[0])
	}

	toggle("")
	if panel.IsVisible() {
		t.Fatal("a second click left the list open")
	}
	s.mu.Lock()
	s.hiddenByFocusAt = time.Time{}
	s.mu.Unlock()
	toggle("")
	last := panel.eventData[len(panel.eventData)-1]
	if _, ok := last[0].(string); !ok {
		t.Errorf("panel-shown data = %#v; want the corner alone after a toggle with no jump", last)
	}
	if rec := toggle("{"); rec.Code != http.StatusBadRequest {
		t.Errorf("a broken body = %d; want 400", rec.Code)
	}
}
