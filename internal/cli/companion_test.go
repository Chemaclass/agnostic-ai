package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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
		builtFor: func(context.Context, string) (string, error) { return "1.2.3", nil },
		quit:     func(context.Context, string) (bool, error) { return false, nil },
		loginOff: func(context.Context, string) error { return nil },
		loginOn:  func(context.Context, string) error { return nil },
		runValueOff: func() (bool, error) {
			return false, nil
		},
		noSessionBus: func() bool { return false },
		sign:         func(context.Context, string) error { return nil },
		rename:       os.Rename,
		pause:        func(time.Duration) {},
	}
}

// installedCompanion writes a companion into dir as an earlier install
// on goos left it.
func installedCompanion(t *testing.T, dir, goos string) string {
	t.Helper()
	installed := companionFile(dir, goos)
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
	tools.sign = func(context.Context, string) error {
		t.Error("a program was signed outside macOS")
		return nil
	}
	var out bytes.Buffer

	err := installCompanion(context.Background(), &out, false, tools, dir, companionBuild{goos: "linux", version: "1.2.3", revision: "abc123"}, nil)

	if err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(dir, "companion", "agnostic-ai-companion")
	if raw, _ := os.ReadFile(installed); string(raw) != "built from github.com/chemaclass/agnostic-ai/companion@abc123" {
		t.Errorf("installed program = %q", raw)
	}
	want := "Building the companion for agnostic-ai 1.2.3 on this machine.\n" +
		"✓ Go found\n" +
		"✓ a C compiler found\n" +
		"✓ GTK 4, WebKitGTK 6.0 and X11 for developers found\n" +
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

			err := installCompanion(context.Background(), &bytes.Buffer{}, false, tools, t.TempDir(), companionBuild{goos: tc.goos, version: "1.2.3", revision: "abc123"}, nil)

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

	err := installCompanion(context.Background(), &bytes.Buffer{}, false, tools, dir, companionBuild{goos: "windows", version: "1.2.3", revision: "abc123"}, nil)

	want := "the build failed (exit status 1):\n# github.com/chemaclass/agnostic-ai/companion\n./main.go:12:2: undefined: nothing"
	if err == nil || err.Error() != want {
		t.Errorf("err = %v", err)
	}
	if raw, _ := os.ReadFile(installed); string(raw) != "the one that works" {
		t.Errorf("the installed companion changed to %q", raw)
	}
}

func TestInstallCompanion_AsksForASourceWhenTheBuildRecordsNoCommit(t *testing.T) {
	err := installCompanion(context.Background(), &bytes.Buffer{}, false, builtTools(t), t.TempDir(), companionBuild{goos: "darwin", version: "1.2.3"}, nil)

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

	err := runCompanion(context.Background(), &bytes.Buffer{}, tools, t.TempDir(), "darwin", "1.2.3", "")

	if err == nil || err.Error() != "the companion is not installed yet. Install it with: agnostic-ai companion install" {
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

	err := runCompanion(context.Background(), &out, tools, dir, "linux", "1.3.0", "")

	if err != nil || started != installed || !pointsBack {
		t.Fatalf("err %v, started %q, told where agnostic-ai is: %t", err, started, pointsBack)
	}
	want := "The companion is on your screen. Its ⋯ menu hides it or quits it, and its shortcut brings a hidden one back. `agnostic-ai companion stop` ends it.\n" +
		"! It was built for agnostic-ai 1.2.3 and this is 1.3.0. Install it again with: agnostic-ai companion install\n"
	if out.String() != want {
		t.Errorf("output:\n%s", out.String())
	}
}

func TestInstallCompanion_StopsARunningCompanionAndStartsTheNewOne(t *testing.T) {
	dir := t.TempDir()
	installed := installedCompanion(t, dir, "linux")
	tools := builtTools(t)
	var asked int
	tools.quit = runsUntilAsked(&asked)
	var started string
	tools.start = func(path string, _ []string) error {
		started = path
		return nil
	}
	var out bytes.Buffer

	err := installCompanion(context.Background(), &out, false, tools, dir, companionBuild{goos: "linux", version: "1.2.3", revision: "abc123"}, nil)

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
	installed := installedCompanion(t, dir, "linux")
	tools := builtTools(t)
	tools.quit = func(context.Context, string) (bool, error) { return false, errors.New("no answer") }
	tools.start = func(string, []string) error {
		t.Error("something was started")
		return nil
	}
	var out bytes.Buffer

	err := installCompanion(context.Background(), &out, false, tools, dir, companionBuild{goos: "linux", version: "1.2.3", revision: "abc123"}, nil)

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
	installed := installedCompanion(t, dir, "linux")
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

	err := installCompanion(context.Background(), &bytes.Buffer{}, false, tools, dir, companionBuild{goos: "linux", version: "1.2.3", revision: "abc123"}, nil)

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
	installedCompanion(t, dir, "linux")
	tools := builtTools(t)
	tools.quit = func(context.Context, string) (bool, error) { return false, errors.New("signal: killed") }

	err := stopCompanionCommand(context.Background(), &bytes.Buffer{}, tools, dir, "linux")

	if err == nil || err.Error() != "ask the companion to stop: signal: killed" {
		t.Errorf("err = %v", err)
	}
}

func TestStopCompanion_EndsTheRunningCompanion(t *testing.T) {
	dir := t.TempDir()
	installedCompanion(t, dir, "linux")
	tools := builtTools(t)
	var asked int
	tools.quit = runsUntilAsked(&asked)
	var out bytes.Buffer

	err := stopCompanionCommand(context.Background(), &out, tools, dir, "linux")

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
				installedCompanion(t, dir, "linux")
			} else {
				tools.quit = func(context.Context, string) (bool, error) {
					t.Error("a companion that is not installed was asked to quit")
					return false, nil
				}
			}
			var out bytes.Buffer

			err := stopCompanionCommand(context.Background(), &out, tools, dir, "linux")

			if err != nil || out.String() != tc.want {
				t.Errorf("err %v, output:\n%s", err, out.String())
			}
		})
	}
}

func TestStopCompanion_FailsWhenTheCompanionNeverEnds(t *testing.T) {
	dir := t.TempDir()
	installedCompanion(t, dir, "linux")
	tools := builtTools(t)
	tools.quit = func(context.Context, string) (bool, error) { return true, nil }

	err := stopCompanionCommand(context.Background(), &bytes.Buffer{}, tools, dir, "linux")

	want := "the companion was asked to stop and still runs. End the agnostic-ai-companion process yourself"
	if err == nil || err.Error() != want {
		t.Errorf("err = %v", err)
	}
}

func TestUninstallCompanion_RemovesItAndKeepsTheMarks(t *testing.T) {
	dir := t.TempDir()
	installed := installedCompanion(t, dir, "linux")
	for _, name := range []string{companionCommandFile, companionSettingsFile, "conversation-marks.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// A login entry the companion did not remove: it names a build at another path.
	entry := filepath.Join(t.TempDir(), "agnostic-ai-companion.desktop")
	if err := os.WriteFile(entry, []byte("[Desktop Entry]"), 0o600); err != nil {
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

	err := uninstallCompanion(context.Background(), &out, tools, dir, "linux", []string{entry})

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

	err := uninstallCompanion(context.Background(), &out, tools, dir, "linux", []string{entry})

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

func TestUninstallCompanion_RemovesACompanionThatCannotRun(t *testing.T) {
	dir := t.TempDir()
	installedCompanion(t, dir, "linux")
	for _, name := range []string{companionCommandFile, companionRevisionFile, companionSettingsFile} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	tools := builtTools(t)
	tools.quit = func(context.Context, string) (bool, error) { return false, errors.New("exec format error") }
	tools.loginOff = func(context.Context, string) error { return errors.New("exec format error") }
	tools.builtFor = func(context.Context, string) (string, error) { return "", errCompanionCannotRun }
	var out bytes.Buffer

	err := uninstallCompanion(context.Background(), &out, tools, dir, "linux", nil)

	if err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"companion", companionCommandFile, companionRevisionFile, companionSettingsFile} {
		if _, err := os.Stat(filepath.Join(dir, gone)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s is still there (%v)", gone, err)
		}
	}
	want := "! Could not stop the companion (ask the companion to stop: exec format error). If it still runs, end the agnostic-ai-companion process.\n" +
		"! Could not turn off start at login (exec format error). Remove agnostic-ai companion from the items that open at login.\n" +
		"✓ Removed " + filepath.Join(dir, "companion") + "\n" +
		"Your read and archive marks stay: agnostic-ai serve uses them too.\n"
	if out.String() != want {
		t.Errorf("output:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestUninstallCompanion_KeepsACompanionThatRunsButDoesNotAnswer(t *testing.T) {
	for name, change := range map[string]func(*companionTools){
		"quit times out": func(tools *companionTools) {
			tools.quit = func(context.Context, string) (bool, error) { return false, context.DeadlineExceeded }
		},
		"still runs": func(tools *companionTools) {
			tools.quit = func(context.Context, string) (bool, error) { return true, nil }
		},
		"quit and version time out": func(tools *companionTools) {
			tools.quit = func(context.Context, string) (bool, error) { return false, context.DeadlineExceeded }
			tools.builtFor = func(context.Context, string) (string, error) { return "", context.DeadlineExceeded }
		},
		"login off fails": func(tools *companionTools) {
			tools.loginOff = func(context.Context, string) error { return errors.New("the system refused") }
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			installed := installedCompanion(t, dir, "linux")
			tools := builtTools(t)
			change(&tools)

			err := uninstallCompanion(context.Background(), io.Discard, tools, dir, "linux", nil)

			if err == nil {
				t.Error("uninstall succeeded")
			}
			if _, statErr := os.Stat(installed); statErr != nil {
				t.Errorf("removed a companion that can still run: %v", statErr)
			}
		})
	}
}

func TestRunCompanion_AfterUninstallSaysHowToInstall(t *testing.T) {
	dir := t.TempDir()
	installedCompanion(t, dir, "linux")
	tools := builtTools(t)
	if err := uninstallCompanion(context.Background(), &bytes.Buffer{}, tools, dir, "linux", nil); err != nil {
		t.Fatal(err)
	}

	err := runCompanion(context.Background(), &bytes.Buffer{}, tools, dir, "linux", "1.2.3", "")

	if err == nil || err.Error() != "the companion is not installed yet. Install it with: agnostic-ai companion install" {
		t.Errorf("err = %v", err)
	}
}

func TestUninstallCompanion_DeletesTheWindowsLoginValueOfAGoneProgram(t *testing.T) {
	tools := builtTools(t)
	tools.runValueOff = func() (bool, error) { return true, nil }
	var out bytes.Buffer

	err := uninstallCompanion(context.Background(), &out, tools, t.TempDir(), "windows", nil)

	want := "✓ Start at login is off\n" +
		"The companion is not installed.\n"
	if err != nil || out.String() != want {
		t.Errorf("err %v, output:\n%s", err, out.String())
	}
}

func TestUninstallCompanion_DeletesTheWindowsLoginValueOfAProgramThatCannotRun(t *testing.T) {
	dir := t.TempDir()
	installedCompanion(t, dir, "windows")
	tools := builtTools(t)
	tools.quit = func(context.Context, string) (bool, error) { return false, errors.New("bad exe format") }
	tools.loginOff = func(context.Context, string) error { return errors.New("bad exe format") }
	tools.builtFor = func(context.Context, string) (string, error) { return "", errCompanionCannotRun }
	tools.runValueOff = func() (bool, error) { return true, nil }
	var out bytes.Buffer

	err := uninstallCompanion(context.Background(), &out, tools, dir, "windows", nil)

	want := "! Could not stop the companion (ask the companion to stop: bad exe format). If it still runs, end the agnostic-ai-companion process.\n" +
		"✓ Start at login is off\n" +
		"✓ Removed " + companionFolder(dir) + "\n" +
		"Your read and archive marks stay: agnostic-ai serve uses them too.\n"
	if err != nil || out.String() != want {
		t.Errorf("err %v, output:\n%s\nwant:\n%s", err, out.String(), want)
	}
}

func TestUninstallCompanion_FailsWhenTheWindowsLoginValueStays(t *testing.T) {
	tools := builtTools(t)
	tools.runValueOff = func() (bool, error) { return false, errors.New("access is denied") }

	err := uninstallCompanion(context.Background(), &bytes.Buffer{}, tools, t.TempDir(), "windows", nil)

	want := `turn off start at login: delete the agnostic-ai-companion value under HKCU\Software\Microsoft\Windows\CurrentVersion\Run: access is denied`
	if err == nil || err.Error() != want {
		t.Errorf("err = %v", err)
	}
}

func TestUninstallCompanion_RemovesTheWebviewDataAnOlderWindowsCompanionLeft(t *testing.T) {
	config := t.TempDir()
	dir := filepath.Join(config, "agnostic-ai")
	installedCompanion(t, dir, "windows")
	old := filepath.Join(config, "agnostic-ai-companion.exe", "EBWebView")
	if err := os.MkdirAll(old, 0o700); err != nil {
		t.Fatal(err)
	}

	err := uninstallCompanion(context.Background(), &bytes.Buffer{}, builtTools(t), dir, "windows", nil)

	if err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{filepath.Join(config, "agnostic-ai-companion.exe"), companionFolder(dir)} {
		if _, err := os.Stat(gone); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s is still there (%v)", gone, err)
		}
	}
}

func TestUninstallCompanion_RemovesTheFilesWithoutASessionBus(t *testing.T) {
	dir := t.TempDir()
	installedCompanion(t, dir, "linux")
	entry := filepath.Join(t.TempDir(), "agnostic-ai-companion.desktop")
	if err := os.WriteFile(entry, []byte("[Desktop Entry]"), 0o600); err != nil {
		t.Fatal(err)
	}
	tools := builtTools(t)
	tools.quit = func(context.Context, string) (bool, error) {
		return false, errors.New("exit status 1: failed to initialize single instance manager")
	}
	tools.noSessionBus = func() bool { return true }
	tools.loginOff = func(context.Context, string) error {
		t.Error("a companion with no session bus was asked to turn off start at login")
		return nil
	}
	var out bytes.Buffer

	err := uninstallCompanion(context.Background(), &out, tools, dir, "linux", []string{entry})

	if err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{companionFolder(dir), entry} {
		if _, err := os.Stat(gone); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s is still there (%v)", gone, err)
		}
	}
	want := "! this shell has no D-Bus session bus, so it cannot reach a companion. If a companion runs in another session, end the agnostic-ai-companion process.\n" +
		"✓ Start at login is off\n" +
		"✓ Removed " + companionFolder(dir) + "\n" +
		"Your read and archive marks stay: agnostic-ai serve uses them too.\n"
	if out.String() != want {
		t.Errorf("output:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestStopCompanion_SaysWhenThereIsNoSessionBus(t *testing.T) {
	dir := t.TempDir()
	installedCompanion(t, dir, "linux")
	tools := builtTools(t)
	tools.quit = func(context.Context, string) (bool, error) { return false, errors.New("exit status 1") }
	tools.noSessionBus = func() bool { return true }

	err := stopCompanionCommand(context.Background(), &bytes.Buffer{}, tools, dir, "linux")

	want := "this shell has no D-Bus session bus, so it cannot reach a companion. Run this command in your desktop session, or end the agnostic-ai-companion process"
	if err == nil || err.Error() != want {
		t.Errorf("err = %v", err)
	}
}

func TestSessionBusMissing_LooksWhereTheDBusLibraryLooks(t *testing.T) {
	for name, tc := range map[string]struct {
		env     map[string]string
		present string
		missing bool
	}{
		"an address in the environment": {env: map[string]string{"DBUS_SESSION_BUS_ADDRESS": "unix:path=/run/user/1000/bus"}},
		"the bus in the runtime folder": {present: "/run/user/1000/bus"},
		"an address file":               {present: "/run/user/1000/dbus-session"},
		"the bus in XDG_RUNTIME_DIR":    {env: map[string]string{"XDG_RUNTIME_DIR": "/tmp/xdg"}, present: "/tmp/xdg/bus"},
		"none of them":                  {env: map[string]string{"XDG_RUNTIME_DIR": "/tmp/xdg"}, missing: true},
	} {
		t.Run(name, func(t *testing.T) {
			exists := func(path string) bool { return path == filepath.FromSlash(tc.present) }

			got := sessionBusMissing(func(key string) string { return tc.env[key] }, "1000", exists)

			if got != tc.missing {
				t.Errorf("missing = %v, want %v", got, tc.missing)
			}
		})
	}
}

func TestCompanionLoginEntries_AreWhereEachSystemKeepsThem(t *testing.T) {
	agents := filepath.Join("/home/ada", "Library", "LaunchAgents")
	for goos, want := range map[string][]string{
		"darwin":  {filepath.Join(agents, "wails.autostart.agnostic-ai-companion.plist"), filepath.Join(agents, "dev.agnostic-ai.companion.plist")},
		"linux":   {filepath.Join("/home/ada/.config", "autostart", "agnostic-ai-companion.desktop")},
		"windows": nil,
	} {
		if got := companionLoginEntries(goos, "/home/ada", "/home/ada/.config"); !slices.Equal(got, want) {
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
	installed := installedCompanion(t, dir, "linux")
	tools := builtTools(t)
	tools.quit = func(context.Context, string) (bool, error) { return true, nil }
	tools.start = func(string, []string) error {
		t.Error("a start would open the list of the companion that still runs")
		return nil
	}

	err := installCompanion(context.Background(), &bytes.Buffer{}, false, tools, dir, companionBuild{goos: "linux", version: "1.2.3", revision: "abc123"}, nil)

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
	installed := installedCompanion(t, dir, "linux")
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

	err := installCompanion(context.Background(), &out, false, tools, dir, companionBuild{goos: "linux", version: "1.2.3", revision: "abc123"}, nil)

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
	installedCompanion(t, dir, "linux")
	left := filepath.Join(dir, ".companion-build-123", "companion")
	if err := os.MkdirAll(left, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(left, "agnostic-ai-companion"), []byte("half built"), 0o700); err != nil {
		t.Fatal(err)
	}

	err := uninstallCompanion(context.Background(), &bytes.Buffer{}, builtTools(t), dir, "linux", nil)

	if err != nil {
		t.Fatal(err)
	}
	if found, _ := filepath.Glob(filepath.Join(dir, ".companion-build-*")); len(found) != 0 {
		t.Errorf("build folders left behind: %v", found)
	}
}

func TestUninstallCompanion_TreatsTheConfigPathLiterally(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "config[ab]")
	installedCompanion(t, dir, "linux")
	build := filepath.Join(dir, ".companion-build-own")
	sibling := filepath.Join(root, "configa", ".companion-build-other")
	kept := filepath.Join(dir, "other-folder")
	for _, path := range []string{build, sibling, kept} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	if err := uninstallCompanion(context.Background(), &bytes.Buffer{}, builtTools(t), dir, "linux", nil); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(build); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("build folder remains: %v", err)
	}
	for _, path := range []string{sibling, kept} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("unrelated folder was removed: %v", err)
		}
	}
}

func TestUninstallCompanion_SucceedsWithoutAConfigDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	if err := uninstallCompanion(context.Background(), &bytes.Buffer{}, builtTools(t), dir, "linux", nil); err != nil {
		t.Errorf("uninstall without a config directory: %v", err)
	}
}

func TestInstallCompanion_RemovesBuildFoldersAnInterruptedInstallLeft(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-2 * companionBuildAge)
	folders := map[string]bool{"left": true, "in use": false, "kept aside": false}
	for name := range folders {
		folder := filepath.Join(dir, companionBuildPrefix+name)
		if err := os.MkdirAll(filepath.Join(folder, "companion"), 0o700); err != nil {
			t.Fatal(err)
		}
		if name == "kept aside" {
			if err := os.MkdirAll(filepath.Join(folder, companionAsideName), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		if name != "in use" {
			if err := os.Chtimes(folder, old, old); err != nil {
				t.Fatal(err)
			}
		}
	}

	err := installCompanion(context.Background(), io.Discard, false, builtTools(t), dir, companionBuild{goos: "linux", version: "1.2.3", revision: "abc123"}, nil)

	if err != nil {
		t.Fatal(err)
	}
	for name, removed := range folders {
		_, err := os.Stat(filepath.Join(dir, companionBuildPrefix+name))
		if gone := errors.Is(err, os.ErrNotExist); gone != removed {
			t.Errorf("%s: removed %t, want %t", name, gone, removed)
		}
	}
}

func TestInstallCompanion_InstallsNothingWhenInterrupted(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	tools := builtTools(t)
	build := tools.build
	tools.build = func(ctx context.Context, dir string, env []string, line func(string), args ...string) error {
		cancel()
		return build(ctx, dir, env, line, args...)
	}

	err := installCompanion(ctx, io.Discard, false, tools, dir, companionBuild{goos: "linux", version: "1.2.3", revision: "abc123"}, nil)

	if !errors.Is(err, context.Canceled) || !strings.HasPrefix(err.Error(), "the install was interrupted and changed nothing") {
		t.Errorf("err = %v", err)
	}
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if entry.Name() != "companion" {
			t.Errorf("left %s", entry.Name())
		}
	}
	if _, err := os.Stat(companionFile(dir, "linux")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("installed although interrupted (%v)", err)
	}
}

func TestRunCompanion_SaysWhenTheCompanionCannotRun(t *testing.T) {
	dir := t.TempDir()
	installed := installedCompanion(t, dir, "linux")
	tools := builtTools(t)
	tools.builtFor = func(context.Context, string) (string, error) { return "", errCompanionCannotRun }
	tools.start = func(string, []string) error {
		t.Error("started a companion that cannot run")
		return nil
	}
	var out bytes.Buffer

	err := runCompanion(context.Background(), &out, tools, dir, "linux", "1.2.3", "")

	if err == nil || !strings.HasPrefix(err.Error(), "the companion at "+installed+" could not run (") || !strings.HasSuffix(err.Error(), "). Install it again with: agnostic-ai companion install") {
		t.Errorf("err = %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("output:\n%s", out.String())
	}
}

func TestRunCompanion_SaysWhenAnotherCommitInstalledIt(t *testing.T) {
	dir := t.TempDir()
	if err := installCompanion(context.Background(), io.Discard, false, builtTools(t), dir, companionBuild{goos: "linux", version: "1.2.3", revision: "0123456789abcdef"}, nil); err != nil {
		t.Fatal(err)
	}
	for revision, want := range map[string]string{
		"0123456789abcdef": "",
		"":                 "",
		"fedcba9876543210": "! It was installed by agnostic-ai 1.2.3 from commit 0123456789ab, and this one is from commit fedcba987654. Install it again with: agnostic-ai companion install\n",
	} {
		var out bytes.Buffer
		if err := runCompanion(context.Background(), &out, builtTools(t), dir, "linux", "1.2.3", revision); err != nil {
			t.Fatal(err)
		}
		if got := strings.TrimPrefix(out.String(), "The companion is on your screen. Its ⋯ menu hides it or quits it, and its shortcut brings a hidden one back. `agnostic-ai companion stop` ends it.\n"); got != want {
			t.Errorf("revision %q: output:\n%s", revision, out.String())
		}
	}
}

func TestInstallCompanion_FinishesOnceItStartsReplacing(t *testing.T) {
	dir := t.TempDir()
	installed := installedCompanion(t, dir, "linux")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tools := builtTools(t)
	asked := 0
	tools.quit = func(ctx context.Context, _ string) (bool, error) {
		cancel()
		if err := ctx.Err(); err != nil {
			return false, err
		}
		asked++
		return asked == 1, nil
	}
	started := ""
	tools.start = func(path string, _ []string) error {
		started = path
		return nil
	}

	err := installCompanion(ctx, io.Discard, false, tools, dir, companionBuild{goos: "linux", version: "1.2.3", revision: "abc123"}, nil)

	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(installed); string(raw) != "built from github.com/chemaclass/agnostic-ai/companion@abc123" {
		t.Errorf("installed program = %q", raw)
	}
	if started != installed {
		t.Errorf("started %q: the stopped companion was not started again", started)
	}
}

func TestCompanionBuiltFor_TellsAProgramThatCannotRunFromASlowOne(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses shell scripts")
	}
	dir := t.TempDir()
	write := func(name, content string, mode os.FileMode) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
		return path
	}
	for _, tc := range []struct {
		name, path string
		cannotRun  bool
	}{
		{"missing", filepath.Join(dir, "missing"), true},
		{"not executable", write("plain", "#!/bin/sh\necho 1.2.3\n", 0o600), true},
		{"not a program", write("garbage", "\x00\x01\x02\x03", 0o700), true},
		{"fails silently", write("fails", "#!/bin/sh\nexit 1\n", 0o700), true},
		{"slow", write("slow", "#!/bin/sh\nexec sleep 5\n", 0o700), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			defer cancel()

			_, err := companionBuiltFor(ctx, tc.path)

			if err == nil || errors.Is(err, errCompanionCannotRun) != tc.cannotRun {
				t.Errorf("err = %v, cannot run %t", err, tc.cannotRun)
			}
		})
	}
	version, err := companionBuiltFor(context.Background(), write("works", "#!/bin/sh\necho 1.2.3\n", 0o700))
	if err != nil || version != "1.2.3" {
		t.Errorf("version %q, err %v", version, err)
	}
}

func TestRunCompanion_StartsAFirstLaunchTooSlowToSayItsVersion(t *testing.T) {
	dir := t.TempDir()
	installed := installedCompanion(t, dir, "linux")
	tools := builtTools(t)
	tools.builtFor = func(context.Context, string) (string, error) { return "", context.DeadlineExceeded }
	started := ""
	tools.start = func(path string, _ []string) error {
		started = path
		return nil
	}
	var out bytes.Buffer

	err := runCompanion(context.Background(), &out, tools, dir, "linux", "1.3.0", "")

	if err != nil || started != installed {
		t.Fatalf("err %v, started %q", err, started)
	}
	if out.String() != "The companion is on your screen. Its ⋯ menu hides it or quits it, and its shortcut brings a hidden one back. `agnostic-ai companion stop` ends it.\n" {
		t.Errorf("output:\n%s", out.String())
	}
}

func TestUninstallCompanion_SaysHowToStartTheCompanionItStopped(t *testing.T) {
	dir := t.TempDir()
	installedCompanion(t, dir, "linux")
	tools := builtTools(t)
	var asked int
	tools.quit = runsUntilAsked(&asked)
	tools.loginOff = func(context.Context, string) error { return errors.New("the system refused") }

	err := uninstallCompanion(context.Background(), io.Discard, tools, dir, "linux", nil)

	want := "turn off start at login: the system refused. The companion was stopped; start it again with: agnostic-ai companion"
	if err == nil || err.Error() != want {
		t.Errorf("err = %v", err)
	}
}
