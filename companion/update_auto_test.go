package main

import (
	"context"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

const checkForUpdatesLabel = "Check for updates automatically"

func TestCheckForUpdates_OnByDefaultAndInBothMenusWhenTheCompanionCanUpdate(t *testing.T) {
	fakeTerminalCommand(t, `{"installed":"0.96.0","latest":"0.96.0","method":"homebrew","oneClick":true}`, "")
	s, _, _, menu := menuShell(t)

	if !s.preferences().CheckForUpdates {
		t.Error("checking for updates on its own is off by default")
	}
	item := menu.FindByLabel(checkForUpdatesLabel)
	if item == nil || !item.Checked() {
		t.Fatalf("menu = %q; want %q checked", menuLabels(menu), checkForUpdatesLabel)
	}
}

func TestCheckForUpdates_NoSwitchWithoutAnUpdateEntry(t *testing.T) {
	_, _, _, menu := menuShell(t)
	if menu.FindByLabel(checkForUpdatesLabel) != nil {
		t.Errorf("menu = %q; a companion that cannot update shows the switch", menuLabels(menu))
	}
}

func TestCheckForUpdates_TurnedOffSavesAndStopsOnlyTheAutomaticCheck(t *testing.T) {
	withVersion(t, "0.96.0")
	s, calls := bundledScript(t, `{"installed":"0.96.0","latest":"0.96.0","method":"binary","oneClick":true,"image":false}`)
	told := []preferences{}
	s.tellPreferences = func(p preferences) { told = append(told, p) }

	if err := s.changeCheckForUpdates(false); err != nil {
		t.Fatal(err)
	}
	saved, err := readSettings(s.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if saved.checksForUpdates() || len(told) != 1 || told[0].CheckForUpdates {
		t.Errorf("saved %+v, told %+v; want it off in the file and both menus", saved, told)
	}

	s.appCommands.checkUpdates(t.Context())
	if raw, _ := os.ReadFile(calls); len(raw) != 0 {
		t.Errorf("the automatic check ran while off: %q", raw)
	}
	s.appCommands.busy.Lock()
	s.appCommands.check(t.Context(), true)
	s.appCommands.busy.Unlock()
	if raw, _ := os.ReadFile(calls); len(raw) == 0 {
		t.Error("a manual check did not ask the command")
	}
}

func TestOlderCLI_NotesASeparateCommandOlderThanTheApp(t *testing.T) {
	withVersion(t, "0.97.0")
	for _, tc := range []struct {
		name, other string
		older       bool
	}{
		{"older", "0.96.0", true},
		{"same", "0.97.0", false},
		{"newer", "0.98.0", false},
		{"not a release", "dev", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := bundledScript(t, `{"installed":"0.97.0","latest":"0.97.0","method":"binary","oneClick":true,"image":false}`)
			other := fakeSeparateCommand(t, filepath.Join(t.TempDir(), "my bin", binaryName), tc.other)

			s.appCommands.checkUpdates(t.Context())

			got := s.updateStatus().OlderCLI
			if !tc.older {
				if got != nil {
					t.Errorf("older CLI = %+v; want none", got)
				}
				return
			}
			want := olderCLI{Path: other, Version: "0.96.0", Command: fmt.Sprintf("'%s' upgrade", other)}
			if got == nil || *got != want {
				t.Errorf("older CLI = %+v; want %+v", got, want)
			}
		})
	}
}

func TestOlderCLI_NotedEvenWhenTheAutomaticCheckIsOff(t *testing.T) {
	withVersion(t, "0.97.0")
	s, _ := bundledScript(t, `{"installed":"0.97.0","latest":"0.97.0","method":"binary","oneClick":true,"image":false}`)
	fakeSeparateCommand(t, filepath.Join(t.TempDir(), binaryName), "0.96.0")
	off := false
	s.settings.CheckForUpdates = &off

	s.appCommands.checkUpdates(t.Context())

	if s.updateStatus().OlderCLI == nil {
		t.Error("no note about the older command while the automatic check is off")
	}
}

func TestOlderCLI_IgnoresAnAppsBundledCommandOnThePath(t *testing.T) {
	withVersion(t, "0.97.0")
	s, _ := bundledScript(t, `{"installed":"0.97.0","latest":"0.97.0","method":"binary","oneClick":true,"image":false}`)
	link := filepath.Join(t.TempDir(), binaryName)
	if err := os.WriteFile(s.appCommands.command, []byte("#!/bin/sh\necho 'agnostic-ai version 0.96.0'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(s.appCommands.command, link); err != nil {
		t.Fatal(err)
	}
	useSeparateCommand(t, link)

	s.appCommands.noteOlderCLI(context.Background())

	if got := s.appCommands.olderCLI; got != nil {
		t.Errorf("older CLI = %+v; want none for the app's own command", got)
	}

	another := filepath.Join(t.TempDir(), "Downloads", "agnostic-ai companion.app", "Contents", "Resources", binaryName)
	fakeSeparateCommand(t, another, "0.96.0")
	s.appCommands.noteOlderCLI(context.Background())
	if got := s.appCommands.olderCLI; got != nil {
		t.Errorf("older CLI = %+v; want none for another copy of the app", got)
	}
}

func fakeSeparateCommand(t *testing.T, path, reports string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(fmt.Sprintf("#!/bin/sh\necho 'agnostic-ai version %s'\n", reports)), 0o700); err != nil {
		t.Fatal(err)
	}
	useSeparateCommand(t, path)
	return path
}

func useSeparateCommand(t *testing.T, path string) {
	t.Helper()
	previous := separateCommands
	t.Cleanup(func() { separateCommands = previous })
	separateCommands = func() iter.Seq[string] { return slices.Values([]string{path}) }
}

func TestOlderCLI_FindsStandaloneCommandBehindTheAppLink(t *testing.T) {
	withVersion(t, "0.97.0")
	dir := t.TempDir()
	s, _ := bundledScript(t, `{"installed":"0.97.0","latest":"0.97.0","image":false}`)
	link := filepath.Join(dir, binaryName)
	if err := os.Symlink(s.appCommands.command, link); err != nil {
		t.Fatal(err)
	}
	fallback := fakeSeparateCommand(t, filepath.Join(t.TempDir(), "shell's bin", binaryName), "0.96.0")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+filepath.Dir(fallback))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	config, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(config, "agnostic-ai", commandPathFile)
	if err := os.MkdirAll(filepath.Dir(record), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(record, []byte(link), 0o600); err != nil {
		t.Fatal(err)
	}
	separateCommands = standaloneCandidates
	previousFolders := usualSystemFolders
	t.Cleanup(func() { usualSystemFolders = previousFolders })
	usualSystemFolders = nil
	if found, err := separateBinary(); err != nil || found != link {
		t.Errorf("normal command = %q, %v; want the recorded link", found, err)
	}

	s.appCommands.noteOlderCLI(t.Context())

	if got := s.appCommands.olderCLI; got == nil || got.Path != fallback || got.Command != shellQuote(fallback)+" upgrade" {
		t.Errorf("older CLI = %+v; want the quoted standalone path %q", got, fallback)
	}
}
