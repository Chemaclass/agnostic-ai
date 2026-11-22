package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// runsFor is a companion the process list shows for the given number of
// looks after each start, then no more. A negative count shows it always.
func runsFor(looks int) func(context.Context, string) (bool, error) {
	seen := 0
	return func(context.Context, string) (bool, error) {
		seen++
		return looks < 0 || seen <= looks, nil
	}
}

func TestCompanionStaysUp_WaitsUntilTheNewCompanionStaysInTheProcessList(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		running func(context.Context, string) (bool, error)
		want    string
	}{
		{"stays up", runsFor(-1), ""},
		{"quits while it starts", runsFor(2), "quit right after it started"},
		{"never shows up", runsFor(0), "did not start within"},
		{"shows up late and stays", func() func(context.Context, string) (bool, error) {
			looks := 0
			return func(context.Context, string) (bool, error) {
				looks++
				return looks > companionStartTries-1, nil
			}
		}(), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tools := builtTools(t)
			tools.running = tc.running
			pauses := 0
			tools.pause = func(time.Duration) { pauses++ }

			err := companionStaysUp(context.Background(), io.Discard, tools, "/apps/agnostic-ai-companion")

			if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Errorf("err = %v; want %q", err, tc.want)
			}
			if pauses > companionStartTries+companionSettleTries {
				t.Errorf("looked %d times; want at most %d", pauses, companionStartTries+companionSettleTries)
			}
		})
	}
}

func TestCompanionStaysUp_KeepsTheNewCompanionWhenTheProcessListCannotBeRead(t *testing.T) {
	t.Parallel()
	tools := builtTools(t)
	tools.running = func(context.Context, string) (bool, error) { return false, errors.New("ps missing") }
	var out bytes.Buffer

	err := companionStaysUp(context.Background(), &out, tools, "/apps/agnostic-ai-companion")

	if err != nil || !strings.Contains(out.String(), "ps missing") {
		t.Errorf("err = %v, output %q; want a warning and no rollback", err, &out)
	}
}

func TestProgramListed_MatchesTheAppAndTheProgramInsideIt(t *testing.T) {
	t.Parallel()
	program := "/var/x/agnostic-ai companion.app/Contents/MacOS/agnostic-ai-companion"
	for _, tc := range []struct {
		list string
		want bool
	}{
		{"/sbin/launchd\n/private/var/x/agnostic-ai companion.app/Contents/MacOS/agnostic-ai-companion\n", true},
		{"/usr/local/bin/agnostic-ai-companion\n", false},
		{"/Applications/other.app/Contents/MacOS/agnostic-ai-companion\n", false},
		{"/var/x/agnostic-ai companion.app/Contents/MacOS/agnostic-ai-companion-helper\n", false},
	} {
		if got := programListed(tc.list, program); got != tc.want {
			t.Errorf("programListed(%q) = %t; want %t", tc.list, got, tc.want)
		}
	}
	if !programListed("agnostic-ai-companion\n/opt/c/agnostic-ai-companion\n", "/home/me/c/agnostic-ai-companion") {
		t.Error("a program outside an app was not found by its name")
	}
}

func TestInstallCompanion_RestoresTheEarlierAppWhenTheNewOneQuitsWhileItStarts(t *testing.T) {
	for _, tc := range []struct {
		name    string
		running func(context.Context, string) (bool, error)
	}{
		{"quits while it starts", runsFor(2)},
		{"never shows up", runsFor(0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			installed := installedCompanion(t, dir, "darwin")
			tools := builtTools(t)
			var asked int
			tools.quit = runsUntilAsked(&asked)
			var started []string
			tools.start = func(path string, _ []string) error {
				raw, _ := os.ReadFile(path)
				started = append(started, string(raw))
				return nil
			}
			kept := false
			tools.running = func(ctx context.Context, program string) (bool, error) {
				if len(started) == 0 {
					return false, nil
				}
				if aside, _ := filepath.Glob(filepath.Join(dir, companionBuildPrefix+"*", companionAsideName)); len(aside) == 1 {
					kept = true
				}
				return tc.running(ctx, program)
			}
			var out bytes.Buffer

			err := installCompanion(context.Background(), &out, false, tools, dir, macBuild(), nil)

			if err == nil {
				t.Fatal("err = nil; want the start failure")
			}
			if !kept {
				t.Error("the earlier app was not kept while the new one started")
			}
			if raw, _ := os.ReadFile(installed); string(raw) != "the one that works" {
				t.Errorf("installed program = %q; want the earlier companion back", raw)
			}
			want := []string{"built from github.com/chemaclass/agnostic-ai/companion@abc123", "the one that works"}
			if strings.Join(started, "|") != strings.Join(want, "|") {
				t.Errorf("started %q; want the new companion, then the earlier one", started)
			}
			for _, line := range []string{"Could not start the updated companion.", "Restored the earlier companion.", "Started the earlier companion."} {
				if !strings.Contains(out.String(), line) {
					t.Errorf("output %q; want %q", &out, line)
				}
			}
		})
	}
}

func TestInstallCompanion_RestoresTheEarlierAppWhenTheRecordFailsAndTheNewOneDoesNotStart(t *testing.T) {
	dir := t.TempDir()
	installed := installedCompanion(t, dir, "darwin")
	// A folder where the note goes: the note cannot be written.
	if err := os.Mkdir(filepath.Join(dir, companionCommandFile), 0o700); err != nil {
		t.Fatal(err)
	}
	tools := builtTools(t)
	var asked int
	tools.quit = runsUntilAsked(&asked)
	starts := 0
	tools.start = func(string, []string) error {
		starts++
		if starts == 1 {
			return errors.New("open refused")
		}
		return nil
	}
	var out bytes.Buffer

	err := installCompanion(context.Background(), &out, false, tools, dir, macBuild(), nil)

	if err == nil || !strings.Contains(err.Error(), "open refused") || !strings.Contains(err.Error(), "record the agnostic-ai binary") {
		t.Errorf("err = %v; want the start and the record failures", err)
	}
	if raw, _ := os.ReadFile(installed); string(raw) != "the one that works" {
		t.Errorf("installed program = %q; want the earlier companion back", raw)
	}
	if !strings.Contains(out.String(), "Restored the earlier companion.") {
		t.Errorf("output %q", &out)
	}
}

func TestCompanionUpdateApp_RestoresTheEarlierAppWhenTheNewOneQuitsWhileItStarts(t *testing.T) {
	app := testDownloadableApp(t, t.TempDir())
	tools := builtTools(t)
	tools.quit = func(context.Context, string) (bool, error) { return false, nil }
	var launches []string
	looks := 0
	tools.running = func(context.Context, string) (bool, error) {
		if len(launches) == 0 {
			return false, nil
		}
		// The new app shows up once, then quits.
		looks++
		return looks == 1, nil
	}
	var out bytes.Buffer
	deps := companionAppDeps{
		prepare: func(_ context.Context, _ io.Writer, _ string, staging string) (string, error) {
			built := testDownloadableApp(t, staging)
			if err := os.WriteFile(filepath.Join(built, "Contents", "Resources", binaryName), []byte("new cli"), 0o700); err != nil {
				t.Fatal(err)
			}
			return built, nil
		}, sameSigner: acceptSigner, runBuilt: acceptBuilt, start: func(path string) error {
			raw, _ := os.ReadFile(filepath.Join(path, "Contents", "Resources", binaryName))
			launches = append(launches, string(raw))
			return nil
		},
	}

	err := updateCompanionApp(context.Background(), &out, app, "2.0.0", tools, deps)

	if err == nil || !strings.Contains(err.Error(), "quit right after it started") {
		t.Errorf("err = %v; want the early exit", err)
	}
	if strings.Join(launches, ", ") != "new cli, cli" {
		t.Errorf("launches = %q; want the new app, then the restored one", launches)
	}
	raw, _ := os.ReadFile(filepath.Join(app, "Contents", "Resources", binaryName))
	if string(raw) != "cli" {
		t.Errorf("installed CLI = %q; want the earlier app back", raw)
	}
	for _, line := range []string{"Could not start the updated companion.", "Restored the earlier companion.", "Started the earlier companion."} {
		if !strings.Contains(out.String(), line) {
			t.Errorf("output %q; want %q", &out, line)
		}
	}
	if strings.Contains(out.String(), "Replaced the companion app.") {
		t.Errorf("output %q reports success", &out)
	}
}

func TestInstallCompanion_RefusesWhileAnotherInstallHoldsTheFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows takes no folder lock")
	}
	dir := t.TempDir()
	installedCompanion(t, dir, "darwin")
	release, err := lockCompanionFolder(companionFolder(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	tools := builtTools(t)
	tools.build = func(context.Context, string, []string, func(string), ...string) error {
		t.Error("built while another install ran")
		return errors.New("unreachable")
	}

	err = installCompanion(context.Background(), io.Discard, false, tools, dir, macBuild(), nil)

	if err == nil || !strings.Contains(err.Error(), "another install or update of the companion") {
		t.Errorf("err = %v; want the second install refused", err)
	}
}

func TestUninstallCompanion_RefusesWhileAnInstallHoldsTheFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows takes no folder lock")
	}
	dir := t.TempDir()
	installed := installedCompanion(t, dir, "linux")
	release, err := lockCompanionFolder(companionFolder(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	err = uninstallCompanion(context.Background(), io.Discard, builtTools(t), dir, "linux", nil)

	if err == nil || !strings.Contains(err.Error(), "another install or update of the companion") {
		t.Errorf("err = %v; want the uninstall refused", err)
	}
	if _, statErr := os.Stat(installed); statErr != nil {
		t.Errorf("the companion was removed under the install: %v", statErr)
	}
}

func TestInstallCompanion_StartsAnUpdatedCompanionOnceTheEarlierOneLeavesTheList(t *testing.T) {
	dir := t.TempDir()
	installedCompanion(t, dir, "darwin")
	tools := builtTools(t)
	var asked int
	tools.quit = runsUntilAsked(&asked)
	started := false
	tools.start = func(string, []string) error { started = true; return nil }
	lingering := 3
	tools.running = func(context.Context, string) (bool, error) {
		if !started {
			lingering--
			return lingering >= 0, nil
		}
		return true, nil
	}

	if err := installCompanion(context.Background(), io.Discard, false, tools, dir, macBuild(), nil); err != nil {
		t.Fatal(err)
	}
	if lingering >= 0 {
		t.Errorf("started while the earlier companion was still listed (%d looks left)", lingering)
	}
}

func TestInstallCompanion_DoesNotWatchTheStartOffMacOS(t *testing.T) {
	for _, goos := range []string{"linux", "windows"} {
		t.Run(goos, func(t *testing.T) {
			dir := t.TempDir()
			installedCompanion(t, dir, goos)
			tools := builtTools(t)
			var asked int
			tools.quit = runsUntilAsked(&asked)
			tools.running = func(context.Context, string) (bool, error) {
				t.Error("watched the process list off macOS")
				return true, nil
			}

			if err := installCompanion(context.Background(), io.Discard, false, tools, dir, companionBuild{goos: goos, version: "1.2.3", revision: "abc123"}, nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}
