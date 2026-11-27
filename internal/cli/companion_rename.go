package cli

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// #4060 tracks removal of the earlier install and login layout.
func installedLegacyCompanions(dir, goos string) ([]string, error) {
	paths := []string{legacyBareCompanionFile(dir, goos)}
	if goos == "darwin" {
		paths = append(paths, legacyCompanionBundle(dir))
	}
	return existingCompanionPaths(paths)
}

func existingCompanionPaths(paths []string) ([]string, error) {
	var existing []string
	for _, path := range paths {
		if _, err := os.Lstat(path); err == nil {
			existing = append(existing, path)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("inspect %s: %w", path, err)
		}
	}
	return existing, nil
}

func migrateRenamedCompanion(ctx context.Context, out io.Writer, tools companionTools, dir string, build companionBuild, installed, built, target, staging string, legacy []string, login companionRenameLogin) (bool, error) {
	oldBundle := legacyCompanionBundle(dir)
	oldBundleProgram := ""
	if build.goos == "darwin" {
		oldBundleProgram = firstCompanionProgramIn(oldBundle)
	}
	if build.source != "" && (login.enabled || oldBundleProgram != "") {
		program := built
		if build.goos == "darwin" {
			program = filepath.Join(built, "Contents", "MacOS", companionName)
		}
		if _, err := tools.loginStatus(ctx, program); err != nil {
			return false, fmt.Errorf("use a current companion source checkout to move start at login, or use the agnostic-ai CLI from the same checkout as %s: %w", build.source, err)
		}
	}
	paths := slices.Concat(legacy, []string{target})
	if build.goos == "darwin" {
		paths = append(paths, bareCompanionFile(dir))
	}
	paths, err := existingCompanionPaths(paths)
	if err != nil {
		return false, err
	}
	swap := companionRenameSwap{built: built, backup: filepath.Join(staging, companionAsideName), tools: tools}
	if err := os.Mkdir(swap.backup, 0o700); err != nil {
		return false, fmt.Errorf("stage the earlier companion: %w", err)
	}
	var ran bool
	canAskEarlier := false
	if installed != "" {
		ran, err = endCompanion(ctx, tools, installed)
		canAskEarlier = err == nil
		switch {
		case err != nil && ran:
			return false, err
		case errors.Is(err, errNoSessionBus) && login.enabled:
			return false, fmt.Errorf("%w. Run this install in your desktop session to move start at login", err)
		case err != nil:
			_, _ = fmt.Fprintf(out, "! Could not tell whether a companion runs (%v). If one does, end it and start it again.\n", err)
		case ran:
			_, _ = fmt.Fprintln(out, "✓ Stopped the running companion")
		}
	}
	newProgram := companionFile(dir, build.goos)
	var removedLoginFrom string
	var addedLogin bool
	restore := func(cause error) (bool, error) {
		var loginErr error
		if addedLogin {
			loginErr = tools.loginOff(ctx, newProgram)
		}
		kept, filesErr := swap.restore()
		if removedLoginFrom != "" && filesErr == nil {
			loginErr = errors.Join(loginErr, tools.loginOn(ctx, removedLoginFrom))
		}
		loginErr = errors.Join(loginErr, login.restore())
		if filesErr == nil && len(swap.moved) > 0 {
			_, _ = fmt.Fprintln(out, "Restored the earlier companion.")
		}
		_, _ = fmt.Fprintf(out, "Could not install the updated companion: %v\n", cause)
		var startErr error
		if ran && filesErr == nil {
			startErr = startInstalledCompanion(tools, installed)
			if startErr == nil {
				_, _ = fmt.Fprintln(out, "Started the earlier companion.")
			}
		}
		return kept, errors.Join(cause, filesErr, loginErr, startErr)
	}
	for i := range login.entries {
		entry := &login.entries[i]
		if canAskEarlier && entry.program != "" {
			// Let Wails unload a LaunchAgent while its original executable is still there.
			if err := entry.checkUnchanged(); err != nil {
				return restore(err)
			}
			entry.changed = true
			if err := tools.loginOff(ctx, entry.program); err != nil {
				return restore(fmt.Errorf("turn off the earlier companion's launch agent: %w", err))
			}
		}
	}
	for _, path := range paths {
		if err := swap.moveAside(path); err != nil {
			return restore(err)
		}
	}
	if build.goos == "darwin" && oldBundleProgram != "" {
		// Query the signed app's login registration at the original bundle path.
		if err := swap.place(oldBundle); err != nil {
			return restore(err)
		}
		probe := filepath.Join(oldBundle, "Contents", "MacOS", companionName)
		on, err := tools.loginStatus(ctx, probe)
		if err != nil {
			return restore(err)
		}
		if on {
			removedLoginFrom = oldBundleProgram
			if err := tools.loginOff(ctx, probe); err != nil {
				return restore(fmt.Errorf("turn off start at login at %s: %w", oldBundle, err))
			}
		}
	}
	if err := swap.place(target); err != nil {
		return restore(err)
	}
	if err := login.remove(); err != nil {
		return restore(err)
	}
	if login.enabled || removedLoginFrom != "" {
		on, err := tools.loginStatus(ctx, newProgram)
		if err != nil {
			return restore(err)
		}
		if !on {
			addedLogin = true
			if err := tools.loginOn(ctx, newProgram); err != nil {
				return restore(fmt.Errorf("move start at login to %s: %w", newProgram, err))
			}
		}
	}
	if ran {
		start := func() error { return startInstalledCompanion(tools, newProgram) }
		if build.goos == "darwin" {
			err = launchUpdatedCompanion(ctx, out, tools, newProgram, start)
		} else {
			err = start()
		}
		if err != nil {
			return restore(err)
		}
	}
	_, _ = fmt.Fprintf(out, "✓ Installed %s\n", target)
	if login.enabled || removedLoginFrom != "" {
		_, _ = fmt.Fprintln(out, "✓ Start at login stays on")
	}
	if ran {
		_, _ = fmt.Fprintln(out, "✓ Started it again")
	} else {
		_, _ = fmt.Fprintln(out, "Start it with: agnostic-ai companion")
	}
	return false, recordCompanionInstall(dir, build.revision)
}

func firstCompanionProgramIn(bundle string) string {
	if info, err := os.Lstat(bundle); err != nil || !info.IsDir() {
		return ""
	}
	for _, name := range []string{legacyCompanionName, companionName} {
		program := filepath.Join(bundle, "Contents", "MacOS", name)
		if info, err := os.Stat(program); err == nil && info.Mode().IsRegular() {
			return program
		}
	}
	return ""
}

type companionRenameSwap struct {
	built, placed, backup string
	moved                 []string
	tools                 companionTools
}

func (s *companionRenameSwap) moveAside(path string) error {
	to := filepath.Join(s.backup, filepath.Base(path))
	if err := whileHeld(s.tools, func() error { return s.tools.rename(path, to) }); err != nil {
		return fmt.Errorf("move the earlier companion at %s: %w", path, err)
	}
	s.moved = append(s.moved, path)
	return nil
}

func (s *companionRenameSwap) place(path string) error {
	from := s.built
	if s.placed != "" {
		from = s.placed
	}
	if err := s.tools.rename(from, path); err != nil {
		return fmt.Errorf("install the companion at %s: %w", path, err)
	}
	s.placed = path
	return nil
}

func (s *companionRenameSwap) restore() (kept bool, err error) {
	if s.placed != "" {
		if err = whileHeld(s.tools, func() error { return s.tools.rename(s.placed, s.built) }); err != nil {
			err = fmt.Errorf("move the failed install from %s: %w", s.placed, err)
		}
	}
	for _, path := range slices.Backward(s.moved) {
		from := filepath.Join(s.backup, filepath.Base(path))
		if restoreErr := s.tools.rename(from, path); restoreErr != nil {
			kept = true
			err = errors.Join(err, companionKeptAside{err: restoreErr, aside: from, installed: path})
		}
	}
	return kept, err
}

type companionRenameLogin struct {
	entries    []companionLoginFile
	runValue   companionLegacyRunValue
	enabled    bool
	removedRun bool
}

type companionLoginFile struct {
	path, program string
	content       []byte
	info          fs.FileInfo
	changed       bool
}

type companionLegacyRunValue struct {
	remove, restore func() error
	program         string
	enabled         bool
	unknownApproval bool
}

func readRenamedCompanionLogin(tools companionTools, dir, goos string, paths []string) (companionRenameLogin, error) {
	var login companionRenameLogin
	oldBare := legacyBareCompanionFile(dir, goos)
	if goos == "windows" {
		value, err := tools.legacyRunValue()
		if err != nil || value.remove == nil || !sameCompanionProgram(value.program, oldBare, goos) {
			return login, err
		}
		return companionRenameLogin{runValue: value, enabled: value.enabled}, nil
	}
	oldBundle := legacyCompanionBundle(dir)
	for _, path := range paths {
		name := filepath.Base(path)
		if goos == "linux" && name != legacyCompanionDesktopEntry || goos == "darwin" && name == companionLaunchAgent {
			continue
		}
		info, err := os.Lstat(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return login, fmt.Errorf("%s: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return login, fmt.Errorf("%s: expected a regular login entry", path)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return login, fmt.Errorf("%s: %w", path, err)
		}
		if goos == "linux" && !sameCompanionProgram(desktopEntryProgram(string(content)), oldBare, goos) {
			continue
		}
		entry := companionLoginFile{path: path, content: content, info: info}
		if goos == "darwin" {
			entry.program = earlierLaunchAgentProgram(content, oldBundle, oldBare)
			if entry.program == "" {
				continue
			}
			if info, err := os.Stat(entry.program); err != nil || !info.Mode().IsRegular() {
				entry.program = ""
			}
		}
		login.entries = append(login.entries, entry)
		login.enabled = login.enabled || goos != "linux" || !desktopAutostartDisabled(string(content))
	}
	return login, nil
}

func earlierLaunchAgentProgram(data []byte, oldBundle, oldBare string) string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	var plist struct {
		Arguments []string `xml:"dict>array>string"`
	}
	if err := xml.Unmarshal(data, &plist); err != nil || len(plist.Arguments) == 0 {
		return ""
	}
	program := resolvedPath(plist.Arguments[0])
	if program == resolvedPath(oldBare) {
		return program
	}
	if bundle, ok := companionAppForProgram(program); ok && bundle == resolvedPath(oldBundle) && knownCompanionExecutable(filepath.Base(program)) {
		return program
	}
	return ""
}

func sameCompanionProgram(program, managed, goos string) bool {
	if program == "" {
		return false
	}
	program, managed = resolvedPath(program), resolvedPath(managed)
	if goos == "windows" {
		return strings.EqualFold(program, managed)
	}
	return program == managed
}

// desktopEntryProgram reads the program from the Exec line the way Wails writes it.
func desktopEntryProgram(content string) string {
	inDesktop := false
	for line := range strings.Lines(content) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			inDesktop = line == "[Desktop Entry]"
		}
		value, ok := strings.CutPrefix(line, "Exec=")
		if !inDesktop || !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if quoted, ok := strings.CutPrefix(value, `"`); ok {
			var program strings.Builder
			for i := 0; i < len(quoted); i++ {
				switch {
				case quoted[i] == '"':
					return program.String()
				case quoted[i] == '\\' && i+1 < len(quoted):
					i++
				}
				program.WriteByte(quoted[i])
			}
			return ""
		}
		if fields := strings.Fields(value); len(fields) > 0 {
			return fields[0]
		}
		return ""
	}
	return ""
}

// windowsCommandProgram reads the program from a Run value, quoted when its path has spaces.
func windowsCommandProgram(command string) string {
	command = strings.TrimSpace(command)
	if quoted, ok := strings.CutPrefix(command, `"`); ok {
		program, _, found := strings.Cut(quoted, `"`)
		if !found {
			return ""
		}
		return program
	}
	if fields := strings.Fields(command); len(fields) > 0 {
		return fields[0]
	}
	return ""
}

func desktopAutostartDisabled(content string) bool {
	inDesktop := false
	for line := range strings.Lines(content) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			inDesktop = line == "[Desktop Entry]"
		}
		if !inDesktop {
			continue
		}
		key, value, _ := strings.Cut(line, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if key == "Hidden" && value == "true" || key == "X-GNOME-Autostart-enabled" && value == "false" {
			return true
		}
	}
	return false
}

func (l *companionRenameLogin) remove() error {
	if l.runValue.remove != nil {
		if err := l.runValue.remove(); err != nil {
			return fmt.Errorf("remove the earlier start at login value: %w", err)
		}
		l.removedRun = true
	}
	for i := range l.entries {
		entry := &l.entries[i]
		if err := entry.checkUnchanged(); err != nil {
			if entry.changed && errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return err
		}
		if err := os.Remove(entry.path); err != nil {
			return fmt.Errorf("remove the earlier start at login entry %s: %w", entry.path, err)
		}
		entry.changed = true
	}
	return nil
}

func (l *companionRenameLogin) restore() error {
	var err error
	if l.removedRun {
		if restoreErr := l.runValue.restore(); restoreErr != nil {
			err = fmt.Errorf("restore the earlier start at login value: %w", restoreErr)
		}
	}
	for _, entry := range l.entries {
		if entry.changed {
			if writeErr := entry.restore(); writeErr != nil {
				err = errors.Join(err, fmt.Errorf("restore the start at login entry %s: %w", entry.path, writeErr))
			}
		}
	}
	return err
}

func (e companionLoginFile) checkUnchanged() error {
	checkInfo := func() error {
		info, err := os.Lstat(e.path)
		if err != nil {
			return fmt.Errorf("check the earlier start at login entry: %w", err)
		}
		if !info.Mode().IsRegular() || info.Mode() != e.info.Mode() || !os.SameFile(info, e.info) {
			return fmt.Errorf("start at login entry %s changed during install", e.path)
		}
		return nil
	}
	if err := checkInfo(); err != nil {
		return err
	}
	content, err := os.ReadFile(e.path)
	if err != nil {
		return fmt.Errorf("check the earlier start at login entry: %w", err)
	}
	if !bytes.Equal(content, e.content) {
		return fmt.Errorf("start at login entry %s changed during install", e.path)
	}
	return checkInfo()
}

func (e companionLoginFile) restore() error {
	file, err := os.OpenFile(e.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		if e.checkUnchanged() == nil {
			return nil
		}
		return fmt.Errorf("start at login entry was recreated during install: %w", err)
	}
	if err != nil {
		return err
	}
	_, err = file.Write(e.content)
	if err == nil {
		err = file.Chmod(e.info.Mode().Perm())
	}
	return errors.Join(err, file.Close())
}
