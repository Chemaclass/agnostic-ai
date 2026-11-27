package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Where the window library writes the companion's start at login, named
// after the companion. On macOS an app registers with the system
// instead, and the launch agent is what a bare program left; inside the
// app, before macOS 13, it is named after the app.
const (
	companionLaunchAgent        = "wails.autostart." + companionName + ".plist"
	legacyCompanionLaunchAgent  = "wails.autostart." + legacyCompanionName + ".plist"
	companionDesktopEntry       = companionName + ".desktop"
	legacyCompanionDesktopEntry = legacyCompanionName + ".desktop"
	companionRunValue           = `Companion values under HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
)

// companionLoginEntries are the files that start the companion at
// login, given the home and configuration folders; none on a system
// that keeps it elsewhere than in a file. The companion removes the
// entry that names its own path. This reaches one left by a program
// that is gone, moved into the app, or was built somewhere else.
func companionLoginEntries(goos, home, config string) []string {
	switch goos {
	case "darwin":
		agents := filepath.Join(home, "Library", "LaunchAgents")
		return []string{filepath.Join(agents, companionLaunchAgent), filepath.Join(agents, companionBundleID+".plist"), filepath.Join(agents, legacyCompanionLaunchAgent)}
	case "linux":
		return []string{filepath.Join(config, "autostart", companionDesktopEntry), filepath.Join(config, "autostart", legacyCompanionDesktopEntry)}
	default:
		return nil
	}
}

func systemCompanionLoginEntries(goos string) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("locate the companion's start at login: %w", err)
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("locate the companion's start at login: %w", err)
	}
	return companionLoginEntries(goos, home, config), nil
}

// endCompanion asks the running companion to quit and waits until it is
// gone. It reports whether one ran.
func endCompanion(ctx context.Context, tools companionTools, installed string) (bool, error) {
	// Asked from there, the companion either fails or starts a bus of
	// its own and finds no companion on it.
	if tools.noSessionBus() {
		return false, errNoSessionBus
	}
	ran := false
	for try := 0; ; try++ {
		running, err := tools.quit(ctx, installed)
		if err != nil {
			return ran, fmt.Errorf("ask the companion to stop: %w", err)
		}
		if !running {
			return ran, nil
		}
		ran = true
		if try == companionStopTries {
			return true, fmt.Errorf("the companion was asked to stop and still runs. End the %s process yourself", filepath.Base(installed))
		}
		tools.pause(companionStopPause)
	}
}

// companionInstalled reports whether dir holds a built companion, and
// where. On macOS that is the app, or the bare program an install
// before the app left.
func companionInstalled(dir, goos string) (string, bool, error) {
	for _, program := range managedCompanionPrograms(dir, goos) {
		if found, err := companionExists(program); err != nil || found {
			return program, found, err
		}
	}
	return companionFile(dir, goos), false, nil
}

func managedCompanionPrograms(dir, goos string) []string {
	current := companionFile(dir, goos)
	oldBare := legacyBareCompanionFile(dir, goos)
	if goos != "darwin" {
		return []string{current, oldBare}
	}
	oldBundle := filepath.Join(legacyCompanionBundle(dir), "Contents", "MacOS")
	return []string{current,
		filepath.Join(filepath.Dir(current), legacyCompanionName),
		filepath.Join(oldBundle, legacyCompanionName),
		filepath.Join(oldBundle, companionName),
		bareCompanionFile(dir), oldBare,
	}
}

func companionExists(path string) (bool, error) {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("the companion: %w", err)
	}
	return true, nil
}

func stopCompanionCommand(ctx context.Context, out io.Writer, tools companionTools, dir, goos string) error {
	installed, found, err := companionInstalled(dir, goos)
	if err != nil {
		return err
	}
	if !found {
		_, _ = fmt.Fprintln(out, "The companion is not installed.")
		return nil
	}
	ran, err := endCompanion(ctx, tools, installed)
	if errors.Is(err, errNoSessionBus) {
		return fmt.Errorf("%w. Run this command in your desktop session, or end the %s process", err, filepath.Base(installed))
	}
	if err != nil {
		return err
	}
	if !ran {
		_, _ = fmt.Fprintln(out, "No companion is running.")
		return nil
	}
	_, _ = fmt.Fprintln(out, "The companion stopped. Start it again with: agnostic-ai companion")
	return nil
}
