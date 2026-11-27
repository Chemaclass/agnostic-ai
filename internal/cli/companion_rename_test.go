package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func oldManagedCompanion(t *testing.T, dir, goos string) string {
	t.Helper()
	folder := filepath.Join(dir, "companion")
	program := filepath.Join(folder, "agnostic-ai-companion")
	switch goos {
	case "darwin":
		program = filepath.Join(folder, "agnostic-ai companion.app", "Contents", "MacOS", "agnostic-ai-companion")
	case "windows":
		program += ".exe"
	}
	writeCompanionFixture(t, program, "old companion")
	return program
}

func writeCompanionFixture(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestCompanionInstalled_FindsTheEarlierName(t *testing.T) {
	for _, goos := range []string{"darwin", "linux", "windows"} {
		t.Run(goos, func(t *testing.T) {
			dir := t.TempDir()
			old := oldManagedCompanion(t, dir, goos)

			got, found, err := companionInstalled(dir, goos)

			if err != nil || !found || got != old {
				t.Errorf("earlier install = %q, %t, %v, want %q", got, found, err, old)
			}
		})
	}
}

func TestInstallCompanion_LeavesOneAppAfterReplacingEarlierNames(t *testing.T) {
	dir := t.TempDir()
	old := oldManagedCompanion(t, dir, "darwin")
	oldBundle := filepath.Join(dir, "companion", "agnostic-ai companion.app")
	oldBare := filepath.Join(dir, "companion", "agnostic-ai-companion")
	settings := filepath.Join(dir, "companion.json")
	writeCompanionFixture(t, oldBare, "old bare companion")
	writeCompanionFixture(t, settings, "{\"smallCompanion\":true}\n")
	installedCompanion(t, dir, "darwin")

	if err := installCompanion(context.Background(), &bytes.Buffer{}, false, builtTools(t), dir, macBuild(), nil); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{oldBundle, oldBare, old} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("earlier installation remains at %s: %v", path, err)
		}
	}
	newProgram := filepath.Join(dir, "companion", "Agnostic Companion.app", "Contents", "MacOS", "agnostic-companion")
	if data, err := os.ReadFile(newProgram); err != nil || string(data) != "built from github.com/chemaclass/agnostic-ai/companion@abc123" {
		t.Errorf("new installed program = %q, %v", data, err)
	}
	if data, err := os.ReadFile(settings); err != nil || string(data) != "{\"smallCompanion\":true}\n" {
		t.Errorf("settings = %q, %v", data, err)
	}
}

func TestInstallCompanion_RestoresTheEarlierMacLoginItemWhenTheNewAppFails(t *testing.T) {
	dir := t.TempDir()
	old := oldManagedCompanion(t, dir, "darwin")
	oldBundle := filepath.Join(dir, "companion", "agnostic-ai companion.app")
	probe := filepath.Join(oldBundle, "Contents", "MacOS", "agnostic-companion")
	current := companionFile(dir, "darwin")
	tools := builtTools(t)
	var asked int
	tools.quit = runsUntilAsked(&asked)
	var steps []string
	tools.loginStatus = func(_ context.Context, program string) (bool, error) {
		steps = append(steps, "status "+program)
		if _, err := os.Stat(program); err != nil {
			t.Errorf("probe %s: %v", program, err)
		}
		return program == probe, nil
	}
	tools.loginOff = func(_ context.Context, program string) error {
		steps = append(steps, "off "+program)
		return nil
	}
	tools.loginOn = func(_ context.Context, program string) error {
		steps = append(steps, "on "+program)
		return nil
	}
	refused := errors.New("open refused")
	tools.start = func(program string, _ []string) error {
		steps = append(steps, "start "+program)
		if program == current {
			return refused
		}
		return nil
	}
	if err := installCompanion(context.Background(), &bytes.Buffer{}, false, tools, dir, macBuild(), nil); !errors.Is(err, refused) {
		t.Errorf("install error = %v, want %v", err, refused)
	}
	want := []string{"status " + probe, "off " + probe, "status " + current, "on " + current, "start " + current, "off " + current, "on " + old, "start " + old}
	if !slices.Equal(steps, want) {
		t.Errorf("steps = %q, want %q", steps, want)
	}
	if data, err := os.ReadFile(old); err != nil || string(data) != "old companion" {
		t.Errorf("restored program = %q, %v", data, err)
	}
	if _, err := os.Lstat(current); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the failed installation remains: %v", err)
	}
}

func launchAgentPlist(program string) string {
	return fmt.Sprintf(`<plist><dict><key>ProgramArguments</key><array><string>%s</string></array></dict></plist>`, program)
}

func TestInstallCompanion_ReplacesAnEarlierLaunchAgentBeforeRegisteringTheNewApp(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS LaunchAgent plist handling")
	}
	dir := t.TempDir()
	old := oldManagedCompanion(t, dir, "darwin")
	realOld := resolvedPath(old)
	agent := filepath.Join(t.TempDir(), "dev.agnostic-ai.companion.plist")
	writeCompanionFixture(t, agent, launchAgentPlist(old))
	tools := builtTools(t)
	var unregistered string
	tools.loginOff = func(_ context.Context, program string) error {
		unregistered = program
		return os.Remove(agent)
	}
	tools.loginOn = func(_ context.Context, program string) error {
		if _, err := os.Lstat(agent); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("agent was still there before registering the new app: %v", err)
		}
		return os.WriteFile(agent, []byte(launchAgentPlist(program)), 0o600)
	}
	if err := installCompanion(context.Background(), &bytes.Buffer{}, false, tools, dir, macBuild(), []string{agent}); err != nil {
		t.Fatal(err)
	}
	if unregistered != realOld {
		t.Errorf("unregistered %q, want the earlier app %q", unregistered, old)
	}
	if data, err := os.ReadFile(agent); err != nil || string(data) != launchAgentPlist(companionFile(dir, "darwin")) {
		t.Errorf("the new app's launch agent = %q, %v", data, err)
	}
}

func TestInstallCompanion_PreservesAnUnmanagedLegacyLaunchAgent(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS LaunchAgent plist handling")
	}
	dir := t.TempDir()
	external := filepath.Join(t.TempDir(), "agnostic-ai-companion")
	writeCompanionFixture(t, external, "external companion")
	agent := filepath.Join(t.TempDir(), "wails.autostart.agnostic-ai-companion.plist")
	plist := launchAgentPlist(external)
	writeCompanionFixture(t, agent, plist)
	tools := builtTools(t)
	tools.loginOff = func(context.Context, string) error {
		t.Error("the unrelated login item was unregistered")
		return nil
	}
	tools.loginOn = func(context.Context, string) error {
		t.Error("the new companion was registered at login")
		return nil
	}

	if err := installCompanion(context.Background(), &bytes.Buffer{}, false, tools, dir, macBuild(), []string{agent}); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(agent); err != nil || string(data) != plist {
		t.Errorf("unmanaged launch agent = %q, %v", data, err)
	}
}

func TestInstallCompanion_DoesNotTurnOnAnEarlierDisabledDesktopEntry(t *testing.T) {
	dir := t.TempDir()
	old := oldManagedCompanion(t, dir, "linux")
	autostart := filepath.Join(t.TempDir(), "autostart")
	oldEntry := filepath.Join(autostart, "agnostic-ai-companion.desktop")
	writeCompanionFixture(t, oldEntry, "[Desktop Entry]\nExec="+old+"\nHidden = true\n")
	tools := builtTools(t)
	tools.loginOn = func(context.Context, string) error {
		t.Error("an earlier disabled entry was enabled")
		return nil
	}
	build := companionBuild{goos: "linux", version: "1.2.3", revision: "abc123"}
	if err := installCompanion(context.Background(), &bytes.Buffer{}, false, tools, dir, build, []string{oldEntry}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(oldEntry); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("earlier disabled entry remains: %v", err)
	}
}

func TestInstallCompanion_PreservesAnEarlierDesktopEntryChangedDuringInstall(t *testing.T) {
	dir := t.TempDir()
	old := oldManagedCompanion(t, dir, "linux")
	entry := filepath.Join(t.TempDir(), "agnostic-ai-companion.desktop")
	writeCompanionFixture(t, entry, "[Desktop Entry]\nExec="+old+"\n")
	const updated = "[Desktop Entry]\nExec=/new\n"
	tools := builtTools(t)
	tools.rename = func(from, to string) error {
		if from == old {
			if err := os.WriteFile(entry, []byte(updated), 0o600); err != nil {
				return err
			}
		}
		return os.Rename(from, to)
	}
	tools.loginOn = func(context.Context, string) error {
		t.Error("the changed entry was used to set up start at login")
		return nil
	}
	build := companionBuild{goos: "linux", version: "1.2.3", revision: "abc123"}
	err := installCompanion(context.Background(), &bytes.Buffer{}, false, tools, dir, build, []string{entry})
	if err == nil || !strings.Contains(err.Error(), "changed during install") {
		t.Errorf("install error = %v, want a changed login entry error", err)
	}
	if data, err := os.ReadFile(entry); err != nil || string(data) != updated {
		t.Errorf("the changed login entry = %q, %v", data, err)
	}
	if data, err := os.ReadFile(old); err != nil || string(data) != "old companion" {
		t.Errorf("restored program = %q, %v", data, err)
	}
}

func TestInstallCompanion_PreservesADesktopEntryRecreatedDuringRollback(t *testing.T) {
	dir := t.TempDir()
	old := oldManagedCompanion(t, dir, "linux")
	entry := filepath.Join(t.TempDir(), "agnostic-ai-companion.desktop")
	writeCompanionFixture(t, entry, "[Desktop Entry]\nExec="+old+"\n")
	const replacement = "[Desktop Entry]\nExec=/recreated\nHidden=true\n"
	refused := errors.New("the new login entry was refused")
	tools := builtTools(t)
	tools.loginOn = func(context.Context, string) error {
		if _, err := os.Lstat(entry); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the old entry was not removed: %v", err)
		}
		writeCompanionFixture(t, entry, replacement)
		return refused
	}
	build := companionBuild{goos: "linux", version: "1.2.3", revision: "abc123"}
	err := installCompanion(context.Background(), &bytes.Buffer{}, false, tools, dir, build, []string{entry})
	if !errors.Is(err, refused) || !strings.Contains(err.Error(), "was recreated during install") {
		t.Errorf("install error = %v, want the refusal and the rollback conflict", err)
	}
	if data, err := os.ReadFile(entry); err != nil || string(data) != replacement {
		t.Errorf("the recreated login entry = %q, %v", data, err)
	}
	if data, err := os.ReadFile(old); err != nil || string(data) != "old companion" {
		t.Errorf("restored program = %q, %v", data, err)
	}
}

func TestInstallCompanion_MovesAStaleLoginEntryFromAnEarlierInstall(t *testing.T) {
	for _, goos := range []string{"darwin", "linux", "windows"} {
		t.Run(goos, func(t *testing.T) {
			dir := t.TempDir()
			tools := builtTools(t)
			var paths []string
			var earlierEntry bool
			switch goos {
			case "darwin":
				if runtime.GOOS != "darwin" {
					t.Skip("macOS LaunchAgent plist handling")
				}
				agent := filepath.Join(t.TempDir(), "wails.autostart.agnostic-ai-companion.plist")
				program := filepath.Join(dir, "companion", "agnostic-ai-companion")
				writeCompanionFixture(t, agent, launchAgentPlist(program))
				paths = []string{agent}
			case "linux":
				entry := filepath.Join(t.TempDir(), "agnostic-ai-companion.desktop")
				writeCompanionFixture(t, entry, "[Desktop Entry]\nExec="+legacyBareCompanionFile(dir, goos)+"\n")
				paths = []string{entry}
			case "windows":
				earlierEntry = true
				tools.legacyRunValue = func() (companionLegacyRunValue, error) {
					return companionLegacyRunValue{program: legacyBareCompanionFile(dir, goos), enabled: true, remove: func() error { earlierEntry = false; return nil }}, nil
				}
			}
			var enabled string
			tools.loginOn = func(_ context.Context, program string) error {
				enabled = program
				return nil
			}
			build := companionBuild{goos: goos, version: "1.2.3", revision: "abc123"}
			if err := installCompanion(context.Background(), &bytes.Buffer{}, false, tools, dir, build, paths); err != nil {
				t.Fatal(err)
			}
			if earlierEntry || enabled != companionFile(dir, goos) {
				t.Errorf("old entry remains = %t, enabled %q", earlierEntry, enabled)
			}
			for _, path := range paths {
				if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("the earlier entry at %s remains: %v", path, err)
				}
			}
		})
	}
}

func TestCompanionLoginStatus_RunsTheVersionRequestOnAnEarlierBuild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stand-in is a shell script")
	}
	program := filepath.Join(t.TempDir(), "agnostic-companion")
	writeCompanionFixture(t, program, `#!/bin/sh
if [ "$1" = "--version" ]; then
  printf '0.99.0\n'
else
  printf 'a plain start would open the app\n' >&2
  exit 1
fi
`)
	on, err := companionLoginStatus(context.Background(), program)
	if on || err == nil || !strings.Contains(err.Error(), "did not report whether start at login is on") {
		t.Errorf("status = %t, %v, want a missing status response", on, err)
	}
}

func TestInstallCompanion_DoesNotReplaceARunValueDisabledByWindows(t *testing.T) {
	dir := t.TempDir()
	old := oldManagedCompanion(t, dir, "windows")
	tools := builtTools(t)
	removed := false
	tools.legacyRunValue = func() (companionLegacyRunValue, error) {
		return companionLegacyRunValue{program: old, remove: func() error { removed = true; return nil }}, nil
	}
	tools.loginOn = func(context.Context, string) error {
		t.Error("the new name was set to start at login despite Windows' approval state")
		return nil
	}
	build := companionBuild{goos: "windows", version: "1.2.3", revision: "abc123"}
	if err := installCompanion(context.Background(), &bytes.Buffer{}, false, tools, dir, build, nil); err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Error("the earlier Run value remains")
	}
}

func TestInstallCompanion_ReplacesAnEarlierLinuxBuildOutsideTheDesktopSession(t *testing.T) {
	dir := t.TempDir()
	old := oldManagedCompanion(t, dir, "linux")
	tools := builtTools(t)
	tools.noSessionBus = func() bool { return true }
	tools.quit = func(context.Context, string) (bool, error) {
		t.Error("an earlier program was asked to quit without a session bus")
		return false, nil
	}
	build := companionBuild{goos: "linux", version: "1.2.3", revision: "abc123"}
	if err := installCompanion(context.Background(), &bytes.Buffer{}, false, tools, dir, build, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(old); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the earlier installation remains: %v", err)
	}
}

func TestUninstallCompanion_RemovesLoginItemsFromBothMacAppNames(t *testing.T) {
	dir := t.TempDir()
	old := oldManagedCompanion(t, dir, "darwin")
	current := installedCompanion(t, dir, "darwin")
	tools := builtTools(t)
	var turnedOff []string
	tools.loginOff = func(_ context.Context, program string) error {
		turnedOff = append(turnedOff, program)
		return nil
	}
	if err := uninstallCompanion(context.Background(), &bytes.Buffer{}, tools, dir, "darwin", nil); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(turnedOff, []string{current, old}) {
		t.Errorf("turned off %q, want both installed paths", turnedOff)
	}
}

func localSourceCompanionTools(t *testing.T) companionTools {
	t.Helper()
	tools := builtTools(t)
	tools.build = func(_ context.Context, _ string, _ []string, _ func(string), args ...string) error {
		for i, arg := range args[:len(args)-1] {
			if arg == "-o" {
				return os.WriteFile(args[i+1], []byte("a companion built from a source checkout"), 0o700)
			}
		}
		return errors.New("the build did not pass an output path")
	}
	return tools
}

func TestInstallCompanion_RejectsAnEarlierSourceCheckoutBeforeStoppingTheMacApp(t *testing.T) {
	dir, source := t.TempDir(), t.TempDir()
	old := oldManagedCompanion(t, dir, "darwin")
	writeCompanionFixture(t, filepath.Join(source, "go.mod"), "module github.com/chemaclass/agnostic-ai/companion\n")
	tools := localSourceCompanionTools(t)
	tools.quit = func(context.Context, string) (bool, error) {
		t.Error("the earlier app was asked to stop")
		return false, nil
	}
	noStatus := errors.New("missing login status response")
	tools.loginStatus = func(_ context.Context, program string) (bool, error) {
		if !strings.Contains(program, ".companion-build-") {
			t.Errorf("asked %q before staging the new app", program)
		}
		return false, noStatus
	}
	build := companionBuild{goos: "darwin", version: "1.2.3", source: source}
	err := installCompanion(context.Background(), &bytes.Buffer{}, false, tools, dir, build, nil)
	if !errors.Is(err, noStatus) || !strings.Contains(err.Error(), "use a current companion source checkout") {
		t.Errorf("install error = %v, want the source compatibility error", err)
	}
	if data, err := os.ReadFile(old); err != nil || string(data) != "old companion" {
		t.Errorf("installed program = %q, %v", data, err)
	}
}

func TestInstallCompanion_UsesAnEarlierSourceCheckoutWithoutLoginMigration(t *testing.T) {
	dir, source := t.TempDir(), t.TempDir()
	old := oldManagedCompanion(t, dir, "linux")
	writeCompanionFixture(t, filepath.Join(source, "go.mod"), "module github.com/chemaclass/agnostic-ai/companion\n")
	tools := localSourceCompanionTools(t)
	tools.loginStatus = func(context.Context, string) (bool, error) {
		t.Error("source checkout was asked about login unnecessarily")
		return false, nil
	}
	build := companionBuild{goos: "linux", version: "1.2.3", source: source}
	if err := installCompanion(context.Background(), &bytes.Buffer{}, false, tools, dir, build, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(old); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the earlier program remains: %v", err)
	}
}

func TestInstallCompanion_PreservesAnUnmanagedLegacyLoginEntry(t *testing.T) {
	external := filepath.Join(t.TempDir(), "agnostic-ai-companion")
	for _, goos := range []string{"linux", "windows"} {
		t.Run(goos, func(t *testing.T) {
			dir := t.TempDir()
			tools := builtTools(t)
			var paths []string
			const entryBody = "[Desktop Entry]\nExec="
			entry := filepath.Join(t.TempDir(), "agnostic-ai-companion.desktop")
			switch goos {
			case "linux":
				writeCompanionFixture(t, entry, entryBody+external+" --hidden\n")
				paths = []string{entry}
			case "windows":
				tools.legacyRunValue = func() (companionLegacyRunValue, error) {
					return companionLegacyRunValue{program: external + ".exe", enabled: true, remove: func() error {
						t.Error("the unrelated Run value was removed")
						return nil
					}}, nil
				}
			}
			tools.loginOn = func(context.Context, string) error {
				t.Error("the new companion was registered at login")
				return nil
			}
			build := companionBuild{goos: goos, version: "1.2.3", revision: "abc123"}
			if err := installCompanion(context.Background(), &bytes.Buffer{}, false, tools, dir, build, paths); err != nil {
				t.Fatal(err)
			}
			if goos == "linux" {
				if data, err := os.ReadFile(entry); err != nil || string(data) != entryBody+external+" --hidden\n" {
					t.Errorf("unmanaged desktop entry = %q, %v", data, err)
				}
			}
		})
	}
}

func TestInstallCompanion_IgnoresAnUnreadableApprovalForAnUnmanagedRunValue(t *testing.T) {
	dir := t.TempDir()
	tools := builtTools(t)
	denied := errors.New("approval key access denied")
	tools.legacyRunValue = func() (companionLegacyRunValue, error) {
		return companionLegacyRunValue{program: filepath.Join(t.TempDir(), "other.exe"), approvalErr: denied, remove: func() error {
			t.Error("the unrelated Run value was removed")
			return nil
		}}, nil
	}
	build := companionBuild{goos: "windows", version: "1.2.3", revision: "abc123"}
	if err := installCompanion(context.Background(), &bytes.Buffer{}, false, tools, dir, build, nil); err != nil {
		t.Errorf("install failed on another build's approval: %v", err)
	}
	old := oldManagedCompanion(t, dir, "windows")
	tools.legacyRunValue = func() (companionLegacyRunValue, error) {
		return companionLegacyRunValue{program: old, approvalErr: denied, remove: func() error { return nil }}, nil
	}
	if err := installCompanion(context.Background(), &bytes.Buffer{}, false, tools, dir, build, nil); !errors.Is(err, denied) {
		t.Errorf("install error = %v, want the approval error", err)
	}
}

func TestInstallCompanion_RestoresTheEarlierRunValueWhenLoginFails(t *testing.T) {
	dir := t.TempDir()
	old := oldManagedCompanion(t, dir, "windows")
	tools := builtTools(t)
	removed, restored := 0, 0
	tools.legacyRunValue = func() (companionLegacyRunValue, error) {
		return companionLegacyRunValue{
			program: old,
			enabled: true,
			remove:  func() error { removed++; return nil },
			restore: func() error { restored++; return nil },
		}, nil
	}
	refused := errors.New("the new Run value was refused")
	tools.loginOn = func(context.Context, string) error { return refused }
	build := companionBuild{goos: "windows", version: "1.2.3", revision: "abc123"}
	err := installCompanion(context.Background(), &bytes.Buffer{}, false, tools, dir, build, nil)
	if !errors.Is(err, refused) {
		t.Errorf("install error = %v, want the refusal", err)
	}
	if removed != 1 || restored != 1 {
		t.Errorf("Run value removed %d times and restored %d times, want once each", removed, restored)
	}
	if data, err := os.ReadFile(old); err != nil || string(data) != "old companion" {
		t.Errorf("restored program = %q, %v", data, err)
	}
}

func TestInstallCompanion_KeepsStartAtLoginOffForAnUnknownWindowsApproval(t *testing.T) {
	dir := t.TempDir()
	old := oldManagedCompanion(t, dir, "windows")
	tools := builtTools(t)
	removed := false
	tools.legacyRunValue = func() (companionLegacyRunValue, error) {
		return companionLegacyRunValue{program: old, unknownApproval: true, remove: func() error { removed = true; return nil }}, nil
	}
	tools.loginOn = func(context.Context, string) error {
		t.Error("start at login was turned on despite an unknown approval")
		return nil
	}
	var out bytes.Buffer
	build := companionBuild{goos: "windows", version: "1.2.3", revision: "abc123"}
	if err := installCompanion(context.Background(), &out, false, tools, dir, build, nil); err != nil {
		t.Fatal(err)
	}
	if !removed || !strings.Contains(out.String(), "start at login stays off") {
		t.Errorf("removed = %t, output = %q", removed, out.String())
	}
}

func TestLoginEntryPrograms(t *testing.T) {
	for content, want := range map[string]string{
		"[Desktop Entry]\nExec=/usr/bin/foo\n":                       "/usr/bin/foo",
		"[Desktop Entry]\nExec=/usr/bin/foo\t--flag\n":               "/usr/bin/foo",
		"[Desktop Entry]\nExec=\"/path with spaces/foo\" --flag\n":   "/path with spaces/foo",
		"[Desktop Entry]\nExec=\"/path/with \\\"quote/foo\"\n":       "/path/with \"quote/foo",
		"[Desktop Action x]\nExec=/other\n[Desktop Entry]\nName=x\n": "",
		"[Desktop Entry]\nExec = /spaced/foo\n":                      "/spaced/foo",
		"login entry with no Exec line":                              "",
	} {
		if got := desktopEntryProgram(content); got != want {
			t.Errorf("desktopEntryProgram(%q) = %q, want %q", content, got, want)
		}
	}
	for command, want := range map[string]string{
		`C:\old\agnostic-ai-companion.exe`:            `C:\old\agnostic-ai-companion.exe`,
		`"C:\Program Files\x\companion.exe" --hidden`: `C:\Program Files\x\companion.exe`,
		`"C:\unterminated`:                            "",
	} {
		if got := windowsCommandProgram(command); got != want {
			t.Errorf("windowsCommandProgram(%q) = %q, want %q", command, got, want)
		}
	}
}
