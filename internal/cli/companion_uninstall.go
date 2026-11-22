package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// uninstallCompanion ends the companion, takes it out of what starts at
// login, and deletes the program and its settings. The login entry goes
// first, while the program that knows how to remove it is still there;
// loginEntries are the files that hold it, removed too when they are left.
func uninstallCompanion(ctx context.Context, out io.Writer, tools companionTools, dir, goos string, loginEntries []string) error {
	unlock, err := lockCompanionFolder(companionFolder(dir))
	if err != nil {
		return err
	}
	defer unlock()
	installed, found, err := companionInstalled(dir, goos)
	if err != nil {
		return err
	}
	loginOff := false
	if found {
		if loginOff, err = stopAndDisableLogin(ctx, out, tools, installed, goos); err != nil {
			return err
		}
	}
	left, err := removeLoginEntries(loginEntries)
	if err != nil {
		return fmt.Errorf("turn off start at login: %w", err)
	}
	if goos == "windows" {
		removed, err := tools.runValueOff()
		if err != nil {
			return fmt.Errorf("turn off start at login: delete the %s: %w", companionRunValue, err)
		}
		left = left || removed
	}
	if loginOff || left {
		_, _ = fmt.Fprintln(out, "✓ Start at login is off")
	}
	if err := removeCompanionFiles(tools, dir, goos, found); err != nil {
		return err
	}
	if !found {
		_, _ = fmt.Fprintln(out, "The companion is not installed.")
		return nil
	}
	_, _ = fmt.Fprintf(out, "✓ Removed %s\n", companionFolder(dir))
	_, _ = fmt.Fprintln(out, "Your read and archive marks stay: agnostic-ai serve uses them too.")
	return nil
}

// stopAndDisableLogin ends the installed companion and has it turn off
// its start at login. A companion that cannot run, such as one built for
// another processor, must not keep its own files, so its failures are
// only warnings. One that runs keeps them until it answers.
func stopAndDisableLogin(ctx context.Context, out io.Writer, tools companionTools, installed, goos string) (loginOff bool, err error) {
	cannotRun := func() bool {
		_, err := tools.builtFor(ctx, installed)
		return errors.Is(err, errCompanionCannotRun)
	}
	ran, err := endCompanion(ctx, tools, installed)
	// Without a session bus no companion runs in this session, and the
	// program cannot turn off start at login: the login entries the caller
	// removes do.
	noBus := errors.Is(err, errNoSessionBus)
	brokenProgram := err != nil && !ran && !noBus && cannotRun()
	switch {
	case noBus:
		_, _ = fmt.Fprintf(out, "! %v. If a companion runs in another session, end the %s process.\n", err, companionName)
	case err != nil && !brokenProgram:
		return false, err
	case err != nil:
		_, _ = fmt.Fprintf(out, "! Could not stop the companion (%v). If it still runs, end the %s process.\n", err, companionName)
	case ran:
		_, _ = fmt.Fprintln(out, "✓ Stopped the companion")
	}
	if !noBus {
		err = tools.loginOff(ctx, installed)
	}
	switch {
	case noBus:
	case err == nil:
		loginOff = true
	case brokenProgram || cannotRun():
		// On Windows the caller deletes the registry value instead.
		if goos == "windows" {
			break
		}
		_, _ = fmt.Fprintf(out, "! Could not turn off start at login (%v). Remove %s from the items that open at login.\n", err, companionAppName)
	case ran:
		return false, fmt.Errorf("turn off start at login: %w. The companion was stopped; start it again with: agnostic-ai companion", err)
	default:
		return false, fmt.Errorf("turn off start at login: %w", err)
	}
	return loginOff, nil
}

// removeCompanionFiles deletes the companion's folder, its record files
// and any build folders. A companion that was found may still hold its
// files a moment after it ended.
func removeCompanionFiles(tools companionTools, dir, goos string, found bool) error {
	folder := companionFolder(dir)
	remove := func() error {
		if goos == "windows" {
			if err := os.RemoveAll(oldWebviewFolder(dir)); err != nil {
				return err
			}
		}
		return os.RemoveAll(folder)
	}
	var err error
	if found {
		err = whileHeld(tools, remove)
	} else {
		err = remove()
	}
	if err != nil {
		return fmt.Errorf("remove the companion: %w", err)
	}
	for _, name := range []string{companionCommandFile, companionRevisionFile, companionSettingsFile} {
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove the companion: %w", err)
		}
	}
	if err := removeBuildFolders(dir, time.Time{}); err != nil {
		return fmt.Errorf("remove the companion: %w", err)
	}
	return nil
}

// removeBuildFolders deletes the build folders an interrupted install
// left. A zero before removes them all. Otherwise it keeps those changed
// since before, which an install may still use, and those that hold an
// app kept aside, the only copy of a companion an install could not put back.
func removeBuildFolders(dir string, before time.Time) error {
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), companionBuildPrefix) {
			continue
		}
		folder := filepath.Join(dir, entry.Name())
		if !before.IsZero() {
			info, err := entry.Info()
			if err != nil || info.ModTime().After(before) {
				continue
			}
			if _, err := os.Stat(filepath.Join(folder, companionAsideName)); err == nil {
				continue
			}
		}
		if err := os.RemoveAll(folder); err != nil {
			return err
		}
	}
	return nil
}
