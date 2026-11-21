package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

var companionExecutable = os.Executable

func bundledCommand(executable string) (app, command string, ok bool) {
	executable, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return "", "", false
	}
	contents := filepath.Dir(filepath.Dir(executable))
	app = filepath.Dir(contents)
	if filepath.Base(filepath.Dir(executable)) != "MacOS" || filepath.Base(contents) != "Contents" || filepath.Ext(app) != ".app" {
		return "", "", false
	}
	command = filepath.Join(contents, "Resources", binaryName)
	info, err := os.Stat(command)
	return app, command, err == nil && info.Mode().IsRegular()
}

func ownBundledCommand() (app, command string, ok bool) {
	executable, err := companionExecutable()
	if err != nil {
		return "", "", false
	}
	return bundledCommand(executable)
}

func stableReleaseVersion(named string) ([3]uint64, bool) {
	var version [3]uint64
	parts := strings.Split(strings.TrimPrefix(named, "v"), ".")
	if len(parts) != len(version) {
		return version, false
	}
	for i, part := range parts {
		if part == "" || len(part) > 1 && part[0] == '0' {
			return version, false
		}
		for _, char := range part {
			if char < '0' || char > '9' {
				return version, false
			}
		}
		number, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return version, false
		}
		version[i] = number
	}
	return version, true
}

func newerRelease(next, current string) bool {
	newer, ok := stableReleaseVersion(next)
	installed, known := stableReleaseVersion(current)
	return ok && known && slices.Compare(newer[:], installed[:]) > 0
}

// The states of the update entry both menus show.
const (
	updateUnavailable = "unavailable"
	updateIdle        = "idle"
	updateChecking    = "checking"
	updateAvailable   = "available"
	updateUpdating    = "updating"
	updateFailed      = "failed"
)

// updateID is the update entry in menu.json.
const updateID = "update"

// checkForUpdatesID is the switch for the check the companion makes on its own.
const checkForUpdatesID = "checkForUpdates"

// The steps of an update, from the lines companion update-app prints.
const (
	updateDownloading = "downloading"
	updateVerifying   = "verifying"
	updateRestarting  = "restarting"
)

var updateStepLines = []struct{ prefix, step string }{
	{"Downloading agnostic-ai ", updateDownloading},
	{"Checking the downloaded app.", updateVerifying},
	{"Restarting the companion.", updateRestarting},
}

// What a check someone asked for found, when it found no release to offer.
const (
	updateLatest      = "latest"
	updateUnreachable = "unreachable"
)

// updateStatus is what the update entry shows; Version names the newer release.
type updateStatus struct {
	State   string `json:"state"`
	Version string `json:"version,omitempty"`
	// Current is the version that runs.
	Current string `json:"current,omitempty"`
	Step    string `json:"step,omitempty"`
	Result  string `json:"result,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Log     string `json:"log,omitempty"`
	// Command is what to run in a terminal when the companion cannot update itself.
	Command string `json:"command,omitempty"`
	// Terminal says the companion was installed from the terminal and updates through agnostic-ai upgrade.
	Terminal bool `json:"terminal,omitempty"`
	// OlderCLI is a separate agnostic-ai command older than the disk image app.
	OlderCLI *olderCLI `json:"olderCLI,omitempty"`
}

// olderCLI is an agnostic-ai command installed apart from the disk image
// app, such as with Homebrew, that is older than the app. Command upgrades
// it the way it was installed; the companion never runs it.
type olderCLI struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	Command string `json:"command"`
}

// upgradeCommand updates a terminal install, the CLI and the companion together.
const upgradeCommand = "agnostic-ai upgrade"

// release is a newer release to offer; Command is set when the companion
// cannot run the update itself.
type release struct {
	Version string
	Command string
}

// terminalCommand finds the agnostic-ai command of a companion installed
// from the terminal.
var terminalCommand = findBinary

// separateCommand finds an agnostic-ai command installed apart from the disk image app.
var separateCommand = separateBinary

// terminalOneClick says agnostic-ai upgrade can replace and restart the
// companion here; elsewhere the panel shows the command to copy.
var terminalOneClick = runtime.GOOS == "darwin"

// updateStep is the last step the update log names.
func updateStep(log string) string {
	step := updateDownloading
	for line := range strings.Lines(log) {
		for _, known := range updateStepLines {
			if strings.HasPrefix(line, known.prefix) {
				step = known.step
			}
		}
	}
	return step
}

// updateFailureReason says in plain words why an update stopped at step.
func updateFailureReason(log, step string) string {
	switch {
	case strings.Contains(log, "does not serve"):
		return "the package manager does not serve the new version yet. Try again later"
	case strings.Contains(log, "executable file not found") || strings.Contains(log, "command not found"):
		return "the package manager could not run from the companion"
	case strings.Contains(log, "checksum mismatch"):
		return "the download did not match its checksum"
	case strings.Contains(log, "build provenance did not verify"):
		return "the download's build provenance did not verify"
	case strings.Contains(log, "the app that runs is not signed with a project certificate"):
		return "the app that runs is not signed by this project. Download the new disk image by hand once"
	case strings.Contains(log, "is not signed by this project"):
		return "the download is not signed by this project"
	case strings.Contains(log, "codesign"):
		return "the downloaded app failed its signature check"
	case strings.Contains(log, "companion version") || strings.Contains(log, ", got "):
		return "the download is not the version it should be"
	case strings.Contains(log, "Move the app to Applications"):
		return "the app is not in a folder that can be updated. Move it to Applications, open it from there, then update"
	case strings.Contains(log, "invalid checksum for"):
		return "the release's checksum list could not be read"
	case step == updateDownloading && (strings.Contains(log, `Get "https://`) || strings.Contains(log, ": download")):
		return "the release could not be downloaded. Check your connection"
	}
	return "the update command failed"
}

// updateLabel names the update entry in its state; model.js updateLabelOf says the same in the list.
func updateLabel(base string, status updateStatus) string {
	switch status.State {
	case updateChecking:
		return "Checking for updates…"
	case updateAvailable, updateFailed:
		return "Update to v" + status.Version
	case updateUpdating:
		return "Updating to v" + status.Version + "…"
	}
	return base
}

func (status updateStatus) busy() bool {
	return status.State == updateChecking || status.State == updateUpdating
}

// appCommands updates the companion. app is the disk image app holding
// command; a companion installed from the terminal has no app and runs
// agnostic-ai upgrade with its command instead.
type appCommands struct {
	shell        *shell
	app, command string
	busy         sync.Mutex
	latest       func(ctx context.Context, refresh bool) (release, error)
	inform       func(title, message string)
	// ask opens the list on the question whether to update.
	ask func()

	mu       sync.Mutex
	current  updateStatus
	olderCLI *olderCLI
}

func (s *shell) ownAppCommands() *appCommands {
	controls := &appCommands{shell: s, current: updateStatus{State: updateIdle}}
	if app, command, ok := ownBundledCommand(); ok {
		controls.app, controls.command = app, command
		controls.latest = func(ctx context.Context, refresh bool) (release, error) {
			return cliRelease(ctx, command, version, refresh, true)
		}
		controls.current = failedUpdateAfterRestart(controls.logPath())
	} else if command, err := terminalCommand(); err == nil {
		controls.command = command
		controls.latest = func(ctx context.Context, refresh bool) (release, error) {
			return cliRelease(ctx, command, version, refresh, false)
		}
	} else {
		return nil
	}
	controls.inform = func(title, message string) {
		dialog := s.app.Dialog.Info().SetTitle(title).SetMessage(message)
		dialog.AddButton("OK").SetAsDefault()
		dialog.Show()
	}
	controls.ask = func() { s.comeBackOpenedOn(eventUpdateAsked) }
	return controls
}

// cliRelease asks command for the latest stable release; the CLI keeps
// its answer for a day unless refresh is set. The disk image app offers a
// release only when it has a disk image for this Mac. A terminal install
// updates in one click on macOS, where agnostic-ai upgrade also replaces
// and restarts the companion.
func cliRelease(ctx context.Context, command, current string, refresh, app bool) (release, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	args := []string{"companion", "update-status", "--json"}
	if refresh {
		args = append(args, "--refresh")
	}
	ask := exec.CommandContext(ctx, command, args...)
	withoutConsoleWindow(ask)
	out, err := ask.Output()
	if err != nil {
		return release{}, fmt.Errorf("check for companion updates: %w", err)
	}
	var status struct {
		Latest     string `json:"latest"`
		OneClick   bool   `json:"oneClick"`
		Image      bool   `json:"image"`
		ImageError string `json:"imageError"`
	}
	if err := json.Unmarshal(out, &status); err != nil {
		return release{}, fmt.Errorf("check for companion updates: %w", err)
	}
	if !newerRelease(status.Latest, current) {
		return release{}, nil
	}
	next := release{Version: strings.TrimPrefix(status.Latest, "v")}
	if app {
		if status.ImageError != "" {
			return release{}, fmt.Errorf("check for companion updates: %s", status.ImageError)
		}
		if !status.Image {
			return release{}, nil
		}
		return next, nil
	}
	if !status.OneClick || !terminalOneClick {
		next.Command = upgradeCommand
	}
	return next, nil
}

// updateStatus is the update entry's state; a companion that finds no agnostic-ai command has none.
func (s *shell) updateStatus() updateStatus {
	if s.appCommands == nil {
		return updateStatus{State: updateUnavailable}
	}
	return s.appCommands.status()
}

// runUpdate answers the update entry in the right-click menu: the list
// opens on the question when a release is known, and shows the check otherwise.
func (s *shell) runUpdate() {
	if s.appCommands == nil {
		return
	}
	switch s.appCommands.status().State {
	case updateAvailable, updateFailed:
		s.appCommands.ask()
	case updateIdle:
		if s.appCommands.begin(func() { s.appCommands.check(context.Background(), true) }) {
			s.comeBackOpenedOn(eventPanelShown)
		}
	}
}

// releasesURL is the project's release page; only stable releases are offered.
const releasesURL = "https://github.com/Chemaclass/agnostic-ai/releases"

// openReleasePage answers the list's Release page: the known release, or the
// latest one, where the companion cannot update itself.
func (s *shell) openReleasePage(w http.ResponseWriter, _ *http.Request) {
	c := s.appCommands
	if c == nil {
		writeJSON(w, http.StatusNotFound, apiError{Error: "this companion has no update entry; open " + releasesURL + "/latest"})
		return
	}
	page := releasesURL + "/latest"
	if named := c.status().Version; named != "" {
		if _, ok := stableReleaseVersion(named); ok {
			page = releasesURL + "/tag/v" + named
		}
	}
	if err := s.openPage(page); err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{Error: fmt.Sprintf("open %s: %v", page, err)})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *shell) getUpdate(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.updateStatus())
}

// checkNow answers the list's Check for updates.
func (s *shell) checkNow(w http.ResponseWriter, _ *http.Request) {
	c := s.appCommands
	if c == nil {
		writeJSON(w, http.StatusNotFound, apiError{Error: "this companion cannot update itself; run agnostic-ai upgrade"})
		return
	}
	if !c.begin(func() { c.check(context.Background(), true) }) {
		writeJSON(w, http.StatusConflict, apiError{Error: "the companion is busy with an update or an install; try again in a moment"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// startUpdate answers the list's Update and restart, and Retry.
func (s *shell) startUpdate(w http.ResponseWriter, _ *http.Request) {
	c := s.appCommands
	if c == nil {
		writeJSON(w, http.StatusNotFound, apiError{Error: "this companion cannot update itself; run agnostic-ai upgrade"})
		return
	}
	status := c.status()
	if status.State != updateAvailable && status.State != updateFailed {
		writeJSON(w, http.StatusConflict, apiError{Error: "no update is known; check for updates first"})
		return
	}
	if status.State == updateAvailable && status.Command != "" {
		writeJSON(w, http.StatusConflict, apiError{Error: "the companion cannot update this install itself; run " + status.Command + " in a terminal"})
		return
	}
	if !c.begin(func() { c.update(status.Version) }) {
		writeJSON(w, http.StatusConflict, apiError{Error: "the companion is busy with an update or an install; try again in a moment"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *appCommands) status() updateStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.current
}

// setStatus keeps status and tells the pages. What a check found is told
// once and not kept, so a later read does not say it again.
func (c *appCommands) setStatus(status updateStatus) {
	status.Current = version
	status.Terminal = c.app == ""
	c.mu.Lock()
	status.OlderCLI = c.olderCLI
	kept := status
	kept.Result = ""
	c.current = kept
	c.mu.Unlock()
	c.shell.refreshNative()
	if c.shell.tellUpdate != nil {
		c.shell.tellUpdate(status)
	}
}

// begin runs work in the background unless a check, an update or an install runs.
func (c *appCommands) begin(work func()) bool {
	if !c.busy.TryLock() {
		return false
	}
	go func() {
		defer c.busy.Unlock()
		work()
	}()
	return true
}

func (c *appCommands) installCommand() {
	if !c.busy.TryLock() {
		return
	}
	defer c.busy.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, c.command, "companion", "install-command", "--app", c.app).CombinedOutput()
	message := strings.TrimSpace(string(out))
	if err != nil {
		message = fmt.Sprintf("Could not install the command line tool: %v\n%s", err, message)
	}
	c.inform("Command line tool", message)
}

func (c *appCommands) checkUpdates(ctx context.Context) {
	if !c.busy.TryLock() {
		return
	}
	defer c.busy.Unlock()
	if c.shell.autoUpdateCheck() {
		c.check(ctx, false)
		return
	}
	c.noteOlderCLI(ctx)
	c.setStatus(c.status())
}

func (s *shell) autoUpdateCheck() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings.checksForUpdates()
}

// noteOlderCLI looks for a separate agnostic-ai command older than the
// disk image app; the caller holds busy.
func (c *appCommands) noteOlderCLI(ctx context.Context) {
	if c.app == "" {
		return
	}
	found := findOlderCLI(ctx)
	c.mu.Lock()
	c.olderCLI = found
	c.mu.Unlock()
}

func findOlderCLI(ctx context.Context) *olderCLI {
	path, err := separateCommand()
	if err != nil {
		return nil
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil
	}
	// Any app's bundled command refuses upgrade and points to its menu.
	if strings.HasSuffix(filepath.Dir(resolved), filepath.Join(".app", "Contents", "Resources")) {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ask := exec.CommandContext(ctx, path, "--version")
	withoutConsoleWindow(ask)
	out, err := ask.Output()
	if err != nil {
		return nil
	}
	named := strings.TrimPrefix(strings.TrimSpace(string(out)), "agnostic-ai version ")
	if !newerRelease(version, named) {
		return nil
	}
	return &olderCLI{Path: path, Version: named, Command: shellQuote(path) + " upgrade"}
}

// shellQuote writes path as one word for a POSIX shell.
func shellQuote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

// check looks for a newer release; the caller holds busy. Only a check
// someone asked for says what it found, and asks before updating.
func (c *appCommands) check(ctx context.Context, asked bool) {
	known := c.status()
	// A failure stays until someone retries or checks again.
	if !asked && known.State == updateFailed {
		c.noteOlderCLI(ctx)
		c.setStatus(known)
		return
	}
	c.setStatus(updateStatus{State: updateChecking, Version: known.Version})
	c.noteOlderCLI(ctx)
	next, err := c.latest(ctx, asked)
	if ctx.Err() != nil {
		c.setStatus(known)
		return
	}
	if err != nil {
		if asked {
			known.Result = updateUnreachable
		} else {
			log.Print(err)
		}
		c.setStatus(known)
		return
	}
	if next.Version == "" {
		found := updateStatus{State: updateIdle}
		if asked {
			found.Result = updateLatest
		}
		c.setStatus(found)
		return
	}
	c.setStatus(updateStatus{State: updateAvailable, Version: next.Version, Command: next.Command})
	if asked {
		c.ask()
	}
}

// update replaces the app with next; the caller holds busy. The panel
// follows the steps the updater writes to its log.
func (c *appCommands) logPath() string {
	return filepath.Join(filepath.Dir(c.shell.settingsPath), "companion-update.log")
}

// The lines update-app writes around the swap: a log that reached the
// restart but not the swap ended in a rollback this companion never saw.
const (
	restartingLine = "Restarting the companion."
	replacedLine   = "Replaced the companion app."
	// shownLine marks a failure already shown, so the next start stays quiet.
	shownLine = "The companion showed this failure."
)

// failedUpdateAfterRestart is the status a disk image app starts with:
// failed when the last update stopped after it ended the companion.
func failedUpdateAfterRestart(path string) updateStatus {
	idle := updateStatus{State: updateIdle}
	raw, err := os.ReadFile(path)
	text := string(raw)
	if err != nil || !strings.Contains(text, restartingLine) || strings.Contains(text, replacedLine) || strings.Contains(text, shownLine) {
		return idle
	}
	var next string
	for line := range strings.Lines(text) {
		if named, ok := strings.CutPrefix(line, "Downloading agnostic-ai "); ok {
			next = strings.TrimSuffix(strings.TrimSpace(named), ".")
		}
	}
	// A bundled CLI from before replacedLine finished the update into this version.
	if !newerRelease(next, version) {
		return idle
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return idle
	}
	_, err = fmt.Fprintln(file, shownLine)
	if closeErr := file.Close(); err != nil || closeErr != nil {
		return idle
	}
	return updateStatus{State: updateFailed, Version: next, Reason: updateFailureReason(text, updateRestarting), Log: path}
}

func (c *appCommands) update(next string) {
	path := c.logPath()
	// agnostic-ai upgrade names no steps the panel follows.
	step := ""
	if c.app != "" {
		step = updateDownloading
	}
	c.setStatus(updateStatus{State: updateUpdating, Version: next, Step: step})
	var file *os.File
	failed := func(reason string) {
		if file != nil {
			_, _ = fmt.Fprintln(file, shownLine)
		}
		status := updateStatus{State: updateFailed, Version: next, Reason: reason, Log: path}
		if c.app == "" {
			status.Command = upgradeCommand
		}
		c.setStatus(status)
	}
	// Append, so the updater and this companion never write over each other.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		file = nil
		log.Print(err)
		failed("the update log could not be written")
		return
	}
	defer func() { _ = file.Close() }()
	// The updater must survive the companion ending while it replaces the
	// app, so it writes to the log file and never to a pipe.
	command := exec.Command(c.command, "companion", "update-app", "--app", c.app, "--version", next)
	if c.app == "" {
		command = exec.Command(c.command, "upgrade")
		command.Env = append(os.Environ(), "PATH="+packageManagerPath(os.Getenv("PATH")))
	}
	detached(command)
	command.Stdout, command.Stderr = file, file
	if err := command.Start(); err != nil {
		log.Print(err)
		failed("the update command could not start")
		return
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			out, _ := os.ReadFile(path)
			if err != nil {
				failed(updateFailureReason(string(out), updateStep(string(out))))
				return
			}
			// The updater restarts the companion; one still running here was left out.
			failed("the companion did not restart. Quit it and open it again")
			return
		case <-ticker.C:
			if step == "" {
				continue
			}
			out, _ := os.ReadFile(path)
			if now := updateStep(string(out)); now != step {
				step = now
				c.setStatus(updateStatus{State: updateUpdating, Version: next, Step: step})
			}
		}
	}
}

// packageManagerPath adds the folders package managers install into to
// path: an app started from the desktop gets a short PATH without them.
func packageManagerPath(path string) string {
	folders := filepath.SplitList(path)
	extra := usualSystemFolders
	if home, err := os.UserHomeDir(); err == nil {
		for _, folder := range usualFolders {
			extra = append(extra, filepath.Join(home, folder))
		}
	}
	for _, folder := range extra {
		if !slices.Contains(folders, folder) {
			folders = append(folders, folder)
		}
	}
	return strings.Join(folders, string(os.PathListSeparator))
}

func (c *appCommands) watchUpdates(ctx context.Context) {
	if c == nil {
		return
	}
	c.checkUpdates(ctx)
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.checkUpdates(ctx)
		}
	}
}
