package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// builtTools are tools on a machine that has what the build needs. The
// build writes the program where the Go command would.
func builtTools(t *testing.T, lines ...string) companionTools {
	t.Helper()
	return companionTools{
		succeeds: func(context.Context, string, ...string) bool { return true },
		build: func(_ context.Context, _ string, env []string, line func(string), args ...string) error {
			for _, text := range lines {
				line(text)
			}
			for _, entry := range env {
				if into, ok := strings.CutPrefix(entry, "GOBIN="); ok {
					return os.WriteFile(filepath.Join(into, "companion"), []byte("built from "+args[len(args)-1]), 0o700)
				}
			}
			return errors.New("no GOBIN in the environment")
		},
		start:    func(string, []string) error { return nil },
		builtFor: func(context.Context, string) string { return "1.2.3" },
		quit:     func(context.Context, string) (bool, error) { return false, nil },
		loginOff: func(context.Context, string) error { return nil },
		pause:    func(time.Duration) {},
	}
}

// installedCompanion writes a companion into dir as an earlier install left it.
func installedCompanion(t *testing.T, dir string) string {
	t.Helper()
	installed := filepath.Join(dir, "companion", "agnostic-ai-companion")
	if err := os.MkdirAll(filepath.Dir(installed), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installed, []byte("the one that works"), 0o700); err != nil {
		t.Fatal(err)
	}
	return installed
}

// runsUntilAsked is a companion that runs and ends at the first request to quit.
func runsUntilAsked(asked *int) func(context.Context, string) (bool, error) {
	return func(context.Context, string) (bool, error) {
		*asked++
		return *asked == 1, nil
	}
}

func TestInstallCompanion_BuildsTheCommitThisBinaryCameFromAndSaysHowFarItIs(t *testing.T) {
	dir := t.TempDir()
	tools := builtTools(t, "go: downloading github.com/wailsapp/wails/v3 v3.1.0", "go: downloading golang.org/x/sys v0.49.0", "internal/abi", "runtime", "github.com/chemaclass/agnostic-ai/companion")
	var out bytes.Buffer

	err := installCompanion(context.Background(), &out, false, tools, dir, companionBuild{goos: "darwin", version: "1.2.3", revision: "abc123"})

	if err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(dir, "companion", "agnostic-ai-companion")
	if raw, _ := os.ReadFile(installed); string(raw) != "built from github.com/chemaclass/agnostic-ai/companion@abc123" {
		t.Errorf("installed program = %q", raw)
	}
	want := "Building the companion for agnostic-ai 1.2.3 on this machine.\n" +
		"✓ Go found\n" +
		"✓ the Xcode Command Line Tools found\n" +
		"→ Fetching and compiling. The first build takes a few minutes.\n" +
		"  2 modules fetched, 3 packages compiled\n" +
		"✓ Installed " + installed + "\n" +
		"Start it with: agnostic-ai companion\n"
	if out.String() != want {
		t.Errorf("output:\n%s\nwant:\n%s", out.String(), want)
	}
	if command, _ := os.ReadFile(filepath.Join(dir, companionCommandFile)); strings.TrimSpace(string(command)) == "" {
		t.Error("the path of this agnostic-ai was not written down for the companion")
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".companion-build-*")); len(left) != 0 {
		t.Errorf("the build folder was left behind: %v", left)
	}
}

func TestInstallCompanion_NamesWhatTheMachineLacks(t *testing.T) {
	for name, tc := range map[string]struct {
		goos    string
		missing string
		want    string
	}{
		"no Go":              {"darwin", "go", "the companion needs Go to build. Install Go from https://go.dev/dl/, then run this again"},
		"no Xcode tools":     {"darwin", "xcode-select", "the companion needs the Xcode Command Line Tools to build. Run: xcode-select --install, then run this again"},
		"no Linux libraries": {"linux", "pkg-config", "the companion needs GTK 4, WebKitGTK 6.0 and X11 for developers to build. Install them (Debian and Ubuntu: sudo apt install pkg-config libgtk-4-dev libwebkitgtk-6.0-dev libx11-dev), then run this again"},
	} {
		t.Run(name, func(t *testing.T) {
			tools := builtTools(t)
			tools.succeeds = func(_ context.Context, program string, _ ...string) bool { return program != tc.missing }
			tools.build = func(context.Context, string, []string, func(string), ...string) error {
				t.Error("the build started on a machine that lacks something")
				return nil
			}

			err := installCompanion(context.Background(), &bytes.Buffer{}, false, tools, t.TempDir(), companionBuild{goos: tc.goos, version: "1.2.3", revision: "abc123"})

			if err == nil || err.Error() != tc.want {
				t.Errorf("err = %v", err)
			}
		})
	}
}

func TestInstallCompanion_KeepsTheInstalledCompanionWhenTheBuildFails(t *testing.T) {
	dir := t.TempDir()
	installed := filepath.Join(dir, "companion", "agnostic-ai-companion")
	if err := os.MkdirAll(filepath.Dir(installed), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installed, []byte("the one that works"), 0o700); err != nil {
		t.Fatal(err)
	}
	tools := builtTools(t)
	tools.build = func(_ context.Context, _ string, _ []string, line func(string), _ ...string) error {
		line("# github.com/chemaclass/agnostic-ai/companion")
		line("./main.go:12:2: undefined: nothing")
		return errors.New("exit status 1")
	}

	err := installCompanion(context.Background(), &bytes.Buffer{}, false, tools, dir, companionBuild{goos: "windows", version: "1.2.3", revision: "abc123"})

	want := "the build failed (exit status 1):\n# github.com/chemaclass/agnostic-ai/companion\n./main.go:12:2: undefined: nothing"
	if err == nil || err.Error() != want {
		t.Errorf("err = %v", err)
	}
	if raw, _ := os.ReadFile(installed); string(raw) != "the one that works" {
		t.Errorf("the installed companion changed to %q", raw)
	}
}

func TestInstallCompanion_AsksForASourceWhenTheBuildRecordsNoCommit(t *testing.T) {
	err := installCompanion(context.Background(), &bytes.Buffer{}, false, builtTools(t), t.TempDir(), companionBuild{goos: "darwin", version: "1.2.3"})

	want := "this agnostic-ai does not record the commit it was built from, so it cannot fetch the companion's source; pass --source with the companion folder of an agnostic-ai checkout"
	if err == nil || err.Error() != want {
		t.Errorf("err = %v", err)
	}
}

func TestCompanionBuild_BuildsFromACheckoutInThatFolder(t *testing.T) {
	build := companionBuild{goos: "windows", version: "1.2.3", source: "/code/agnostic-ai/companion", into: "/staging/companion"}

	args, dir := build.goArgs()

	want := "build -v -tags production private_mac_apis -trimpath -ldflags -s -w -X main.version=1.2.3 -H windowsgui -o " + filepath.Join("/staging/companion", "agnostic-ai-companion.exe") + " ."
	if strings.Join(args, " ") != want || dir != "/code/agnostic-ai/companion" {
		t.Errorf("args = %q in %q", strings.Join(args, " "), dir)
	}
	if env := build.goEnv(nil); len(env) != 0 {
		t.Errorf("Windows needs no C and a checkout build no GOBIN, got %v", env)
	}
}

func TestRunCompanion_SaysHowToInstallAMissingCompanion(t *testing.T) {
	tools := builtTools(t)
	tools.start = func(string, []string) error {
		t.Error("something was started")
		return nil
	}

	err := runCompanion(context.Background(), &bytes.Buffer{}, tools, t.TempDir(), "darwin", "1.2.3")

	if err == nil || err.Error() != "the companion is not installed yet. Build it for this machine with: agnostic-ai companion install" {
		t.Errorf("err = %v", err)
	}
}

func TestRunCompanion_StartsItAndSaysWhenItWasBuiltForAnotherRelease(t *testing.T) {
	dir := t.TempDir()
	installed := filepath.Join(dir, "companion", "agnostic-ai-companion")
	if err := os.MkdirAll(filepath.Dir(installed), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installed, nil, 0o700); err != nil {
		t.Fatal(err)
	}
	tools := builtTools(t)
	var started string
	var pointsBack bool
	tools.start = func(path string, env []string) error {
		started = path
		for _, entry := range env {
			if strings.HasPrefix(entry, "AGNOSTIC_AI_BIN=") {
				pointsBack = true
			}
		}
		return nil
	}
	var out bytes.Buffer

	err := runCompanion(context.Background(), &out, tools, dir, "darwin", "1.3.0")

	if err != nil || started != installed || !pointsBack {
		t.Fatalf("err %v, started %q, told where agnostic-ai is: %t", err, started, pointsBack)
	}
	want := "The companion is on your screen. Its ⋯ menu hides it, its shortcut brings it back, and `agnostic-ai companion stop` ends it.\n" +
		"! It was built for agnostic-ai \"1.2.3\" and this is 1.3.0. Build it again with: agnostic-ai companion install\n"
	if out.String() != want {
		t.Errorf("output:\n%s", out.String())
	}
}

func TestInstallCompanion_StopsARunningCompanionAndStartsTheNewOne(t *testing.T) {
	dir := t.TempDir()
	installed := installedCompanion(t, dir)
	tools := builtTools(t)
	var asked int
	tools.quit = runsUntilAsked(&asked)
	var started string
	tools.start = func(path string, _ []string) error {
		started = path
		return nil
	}
	var out bytes.Buffer

	err := installCompanion(context.Background(), &out, false, tools, dir, companionBuild{goos: "darwin", version: "1.2.3", revision: "abc123"})

	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(installed); string(raw) != "built from github.com/chemaclass/agnostic-ai/companion@abc123" {
		t.Errorf("installed program = %q", raw)
	}
	if started != installed {
		t.Errorf("started %q, want the new companion", started)
	}
	want := "✓ Stopped the running companion\n" +
		"✓ Installed " + installed + "\n" +
		"✓ Started it again\n"
	if !strings.HasSuffix(out.String(), want) {
		t.Errorf("output:\n%s\nwant it to end with:\n%s", out.String(), want)
	}
}

func TestInstallCompanion_InstallsWhenTheRunningCompanionCannotBeAsked(t *testing.T) {
	dir := t.TempDir()
	installed := installedCompanion(t, dir)
	tools := builtTools(t)
	tools.quit = func(context.Context, string) (bool, error) { return false, errors.New("no answer") }
	tools.start = func(string, []string) error {
		t.Error("something was started")
		return nil
	}
	var out bytes.Buffer

	err := installCompanion(context.Background(), &out, false, tools, dir, companionBuild{goos: "darwin", version: "1.2.3", revision: "abc123"})

	if err != nil {
		t.Fatal(err)
	}
	want := "! Could not tell whether a companion runs (ask the companion to stop: no answer). If one does, end it and start it again.\n" +
		"✓ Installed " + installed + "\n" +
		"Start it with: agnostic-ai companion\n"
	if !strings.HasSuffix(out.String(), want) {
		t.Errorf("output:\n%s\nwant it to end with:\n%s", out.String(), want)
	}
}

func TestInstallCompanion_StartsTheOldCompanionAgainWhenItCannotBeReplaced(t *testing.T) {
	if os.Geteuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("needs a folder this user cannot write to")
	}
	dir := t.TempDir()
	installed := installedCompanion(t, dir)
	folder := filepath.Dir(installed)
	if err := os.Chmod(folder, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(folder, 0o700) })
	tools := builtTools(t)
	var asked int
	tools.quit = runsUntilAsked(&asked)
	var pauses int
	tools.pause = func(time.Duration) { pauses++ }
	var started string
	tools.start = func(path string, _ []string) error {
		started = path
		return nil
	}

	err := installCompanion(context.Background(), &bytes.Buffer{}, false, tools, dir, companionBuild{goos: "darwin", version: "1.2.3", revision: "abc123"})

	if err == nil || !strings.HasPrefix(err.Error(), "install the companion: ") {
		t.Errorf("err = %v", err)
	}
	if started != installed {
		t.Errorf("started %q: the companion that ran was left stopped", started)
	}
	if raw, _ := os.ReadFile(installed); string(raw) != "the one that works" {
		t.Errorf("the installed companion changed to %q", raw)
	}
	// One pause while the companion ended, then one for each new try at the move.
	if pauses != 1+companionStopTries {
		t.Errorf("paused %d times, want the move tried again %d times", pauses, companionStopTries)
	}
}

func TestStopCompanion_NamesWhatFailedWhenTheCompanionCannotBeAsked(t *testing.T) {
	dir := t.TempDir()
	installedCompanion(t, dir)
	tools := builtTools(t)
	tools.quit = func(context.Context, string) (bool, error) { return false, errors.New("signal: killed") }

	err := stopCompanionCommand(context.Background(), &bytes.Buffer{}, tools, dir, "darwin")

	if err == nil || err.Error() != "ask the companion to stop: signal: killed" {
		t.Errorf("err = %v", err)
	}
}

func TestStopCompanion_EndsTheRunningCompanion(t *testing.T) {
	dir := t.TempDir()
	installedCompanion(t, dir)
	tools := builtTools(t)
	var asked int
	tools.quit = runsUntilAsked(&asked)
	var out bytes.Buffer

	err := stopCompanionCommand(context.Background(), &out, tools, dir, "darwin")

	if err != nil {
		t.Fatal(err)
	}
	if asked != 2 {
		t.Errorf("asked %d times, want once to stop it and once to see that it ended", asked)
	}
	if out.String() != "The companion stopped. Start it again with: agnostic-ai companion\n" {
		t.Errorf("output:\n%s", out.String())
	}
}

func TestStopCompanion_SucceedsWhenNoneRuns(t *testing.T) {
	for name, tc := range map[string]struct {
		install bool
		want    string
	}{
		"installed":     {true, "No companion is running.\n"},
		"not installed": {false, "The companion is not installed.\n"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			tools := builtTools(t)
			if tc.install {
				installedCompanion(t, dir)
			} else {
				tools.quit = func(context.Context, string) (bool, error) {
					t.Error("a companion that is not installed was asked to quit")
					return false, nil
				}
			}
			var out bytes.Buffer

			err := stopCompanionCommand(context.Background(), &out, tools, dir, "darwin")

			if err != nil || out.String() != tc.want {
				t.Errorf("err %v, output:\n%s", err, out.String())
			}
		})
	}
}

func TestStopCompanion_FailsWhenTheCompanionNeverEnds(t *testing.T) {
	dir := t.TempDir()
	installedCompanion(t, dir)
	tools := builtTools(t)
	tools.quit = func(context.Context, string) (bool, error) { return true, nil }

	err := stopCompanionCommand(context.Background(), &bytes.Buffer{}, tools, dir, "darwin")

	want := "the companion was asked to stop and still runs. End the agnostic-ai-companion process yourself"
	if err == nil || err.Error() != want {
		t.Errorf("err = %v", err)
	}
}

func TestUninstallCompanion_RemovesItAndKeepsTheMarks(t *testing.T) {
	dir := t.TempDir()
	installed := installedCompanion(t, dir)
	for _, name := range []string{companionCommandFile, companionSettingsFile, "conversation-marks.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// A login entry the companion did not remove: it names a build at another path.
	entry := filepath.Join(t.TempDir(), "wails.autostart.agnostic-ai-companion.plist")
	if err := os.WriteFile(entry, []byte("<plist/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	tools := builtTools(t)
	var asked int
	tools.quit = runsUntilAsked(&asked)
	var steps []string
	tools.loginOff = func(_ context.Context, path string) error {
		if asked != 2 {
			t.Error("start at login was turned off while the companion still ran")
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("the companion was deleted before start at login was turned off: %v", err)
		}
		steps = append(steps, "login off "+path)
		return nil
	}
	var out bytes.Buffer

	err := uninstallCompanion(context.Background(), &out, tools, dir, "darwin", entry)

	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 || steps[0] != "login off "+installed {
		t.Errorf("steps = %v", steps)
	}
	for _, gone := range []string{"companion", companionCommandFile, companionSettingsFile} {
		if _, err := os.Stat(filepath.Join(dir, gone)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s is still there (%v)", gone, err)
		}
	}
	if _, err := os.Stat(entry); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the login entry is still there (%v)", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "conversation-marks.json")); err != nil {
		t.Errorf("the marks agnostic-ai serve shares were removed: %v", err)
	}
	want := "✓ Stopped the companion\n" +
		"✓ Start at login is off\n" +
		"✓ Removed " + filepath.Join(dir, "companion") + "\n" +
		"Your read and archive marks stay: agnostic-ai serve uses them too.\n"
	if out.String() != want {
		t.Errorf("output:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestUninstallCompanion_SaysWhenNothingIsInstalled(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, companionSettingsFile), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The case a deleted program leaves: a login entry that points at nothing.
	entry := filepath.Join(t.TempDir(), "agnostic-ai-companion.desktop")
	if err := os.WriteFile(entry, []byte("[Desktop Entry]"), 0o600); err != nil {
		t.Fatal(err)
	}
	tools := builtTools(t)
	tools.loginOff = func(context.Context, string) error {
		t.Error("a companion that is not installed was run")
		return nil
	}
	var out bytes.Buffer

	err := uninstallCompanion(context.Background(), &out, tools, dir, "darwin", entry)

	if err != nil || out.String() != "✓ Start at login is off\nThe companion is not installed.\n" {
		t.Errorf("err %v, output:\n%s", err, out.String())
	}
	if _, err := os.Stat(entry); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the login entry that points at nothing is still there (%v)", err)
	}
	if _, err := os.Stat(filepath.Join(dir, companionSettingsFile)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the settings an earlier companion left are still there (%v)", err)
	}
}

func TestUninstallCompanion_KeepsEverythingWhenStartAtLoginStaysOn(t *testing.T) {
	dir := t.TempDir()
	installed := installedCompanion(t, dir)
	kept := []string{installed, filepath.Join(dir, companionCommandFile), filepath.Join(dir, companionSettingsFile)}
	for _, path := range kept[1:] {
		if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	tools := builtTools(t)
	tools.loginOff = func(context.Context, string) error { return errors.New("the system refused") }

	err := uninstallCompanion(context.Background(), &bytes.Buffer{}, tools, dir, "darwin", "")

	if err == nil || err.Error() != "turn off start at login: the system refused" {
		t.Errorf("err = %v", err)
	}
	for _, path := range kept {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Errorf("removed although the login item stays: %v", statErr)
		}
	}
}

func TestRunCompanion_AfterUninstallSaysHowToInstall(t *testing.T) {
	dir := t.TempDir()
	installedCompanion(t, dir)
	tools := builtTools(t)
	if err := uninstallCompanion(context.Background(), &bytes.Buffer{}, tools, dir, "darwin", ""); err != nil {
		t.Fatal(err)
	}

	err := runCompanion(context.Background(), &bytes.Buffer{}, tools, dir, "darwin", "1.2.3")

	if err == nil || err.Error() != "the companion is not installed yet. Build it for this machine with: agnostic-ai companion install" {
		t.Errorf("err = %v", err)
	}
}

func TestUninstallCompanion_NamesTheWindowsLoginEntryItCannotRemove(t *testing.T) {
	var out bytes.Buffer

	err := uninstallCompanion(context.Background(), &out, builtTools(t), t.TempDir(), "windows", "")

	want := "! If the companion started at login, delete the agnostic-ai-companion value under HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Run.\n" +
		"The companion is not installed.\n"
	if err != nil || out.String() != want {
		t.Errorf("err %v, output:\n%s", err, out.String())
	}
}

func TestCompanionLoginEntry_IsWhereEachSystemKeepsIt(t *testing.T) {
	for goos, want := range map[string]string{
		"darwin":  filepath.Join("/home/ada", "Library", "LaunchAgents", "wails.autostart.agnostic-ai-companion.plist"),
		"linux":   filepath.Join("/home/ada/.config", "autostart", "agnostic-ai-companion.desktop"),
		"windows": "",
	} {
		if got := companionLoginEntry(goos, "/home/ada", "/home/ada/.config"); got != want {
			t.Errorf("%s: %q, want %q", goos, got, want)
		}
	}
}

// The companion answers a request to quit with how it ends; this pins
// the CLI's reading of it against a program that ends each way.
func TestQuitCompanion_ReadsHowTheCompanionEnded(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in companion is a shell script")
	}
	for name, tc := range map[string]struct {
		script  string
		running bool
		err     string
	}{
		"a companion ran and was told": {"exit 0", true, ""},
		"none ran":                     {"exit 3", false, ""},
		"it failed":                    {"echo 'companion.json: bad settings' >&2; exit 1", false, "exit status 1: companion.json: bad settings"},
		"it got another argument":      {`[ "$1" = "--quit" ] || exit 9; exit 3`, false, ""},
	} {
		t.Run(name, func(t *testing.T) {
			stub := filepath.Join(t.TempDir(), "agnostic-ai-companion")
			if err := os.WriteFile(stub, []byte("#!/bin/sh\n"+tc.script+"\n"), 0o700); err != nil {
				t.Fatal(err)
			}

			running, err := quitCompanion(context.Background(), stub)

			said := ""
			if err != nil {
				said = err.Error()
			}
			if running != tc.running || said != tc.err {
				t.Errorf("running %t, err %q; want %t, %q", running, said, tc.running, tc.err)
			}
		})
	}
}

func TestInstallCompanion_LeavesACompanionThatWillNotEndAlone(t *testing.T) {
	dir := t.TempDir()
	installed := installedCompanion(t, dir)
	tools := builtTools(t)
	tools.quit = func(context.Context, string) (bool, error) { return true, nil }
	tools.start = func(string, []string) error {
		t.Error("a start would open the list of the companion that still runs")
		return nil
	}

	err := installCompanion(context.Background(), &bytes.Buffer{}, false, tools, dir, companionBuild{goos: "darwin", version: "1.2.3", revision: "abc123"})

	want := "the companion was asked to stop and still runs. End the agnostic-ai-companion process yourself"
	if err == nil || err.Error() != want {
		t.Errorf("err = %v", err)
	}
	if raw, _ := os.ReadFile(installed); string(raw) != "the one that works" {
		t.Errorf("the installed companion changed to %q", raw)
	}
}

func TestInstallCompanion_SaysItInstalledAndStartsTheNewOneWhenTheNoteFails(t *testing.T) {
	dir := t.TempDir()
	installed := installedCompanion(t, dir)
	// A folder where the note goes: the note cannot be written.
	if err := os.Mkdir(filepath.Join(dir, companionCommandFile), 0o700); err != nil {
		t.Fatal(err)
	}
	tools := builtTools(t)
	var asked int
	tools.quit = runsUntilAsked(&asked)
	var started string
	tools.start = func(path string, _ []string) error {
		started = path
		return nil
	}
	var out bytes.Buffer

	err := installCompanion(context.Background(), &out, false, tools, dir, companionBuild{goos: "darwin", version: "1.2.3", revision: "abc123"})

	if err == nil || !strings.HasPrefix(err.Error(), "record the agnostic-ai binary: ") {
		t.Errorf("err = %v", err)
	}
	if started != installed {
		t.Errorf("started %q: the companion that ran was left stopped", started)
	}
	if !strings.HasSuffix(out.String(), "✓ Installed "+installed+"\n") {
		t.Errorf("output does not end with the install, though the new program is in place:\n%s", out.String())
	}
}

func TestUninstallCompanion_RemovesWhatAnInterruptedInstallLeft(t *testing.T) {
	dir := t.TempDir()
	installedCompanion(t, dir)
	left := filepath.Join(dir, ".companion-build-123", "companion")
	if err := os.MkdirAll(left, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(left, "agnostic-ai-companion"), []byte("half built"), 0o700); err != nil {
		t.Fatal(err)
	}

	err := uninstallCompanion(context.Background(), &bytes.Buffer{}, builtTools(t), dir, "darwin", "")

	if err != nil {
		t.Fatal(err)
	}
	if found, _ := filepath.Glob(filepath.Join(dir, ".companion-build-*")); len(found) != 0 {
		t.Errorf("build folders left behind: %v", found)
	}
}
