package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

func updateImageName() string {
	return "agnostic-ai-companion_darwin_" + nativeUpdateArch() + ".dmg"
}

type releaseAsset struct {
	Name  string `json:"name"`
	State string `json:"state"`
	Size  int64  `json:"size"`
}

func latestCompanionRelease(ctx context.Context, client *http.Client, url, current, image string) (string, error) {
	installed, ok := stableReleaseVersion(current)
	if !ok {
		return "", nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("check for companion updates: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "agnostic-ai-companion/"+current)
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("check for companion updates: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("check for companion updates: GitHub returned %s", response.Status)
	}
	var release struct {
		Tag        string         `json:"tag_name"`
		Draft      bool           `json:"draft"`
		Prerelease bool           `json:"prerelease"`
		Assets     []releaseAsset `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&release); err != nil {
		return "", fmt.Errorf("read latest companion release: %w", err)
	}
	latest, ok := stableReleaseVersion(release.Tag)
	if !ok || release.Draft || release.Prerelease || slices.Compare(latest[:], installed[:]) <= 0 {
		return "", nil
	}
	for _, asset := range release.Assets {
		if asset.Name == image && asset.State == "uploaded" && asset.Size > 0 {
			return strings.TrimPrefix(release.Tag, "v"), nil
		}
	}
	return "", nil
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
	case strings.Contains(log, "install method unknown"):
		return "this install cannot be updated from the companion"
	case strings.Contains(log, "executable file not found") || strings.Contains(log, "command not found"):
		return "the package manager could not run from the companion"
	case strings.Contains(log, "checksum mismatch"):
		return "the download did not match its checksum"
	case strings.Contains(log, "build provenance did not verify"):
		return "the download's build provenance did not verify"
	case strings.Contains(log, "codesign"):
		return "the downloaded app failed its signature check"
	case strings.Contains(log, "companion version") || strings.Contains(log, ", got "):
		return "the download is not the version it should be"
	case strings.Contains(log, ": stage update: "):
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
	latest       func(context.Context) (release, error)
	inform       func(title, message string)
	// ask opens the list on the question whether to update.
	ask func()

	mu      sync.Mutex
	current updateStatus
}

func (s *shell) ownAppCommands() *appCommands {
	controls := &appCommands{shell: s, current: updateStatus{State: updateIdle}}
	if app, command, ok := ownBundledCommand(); ok {
		controls.app, controls.command = app, command
		controls.latest = func(ctx context.Context) (release, error) {
			next, err := latestCompanionRelease(ctx, &http.Client{Timeout: 15 * time.Second}, "https://api.github.com/repos/Chemaclass/agnostic-ai/releases/latest", version, updateImageName())
			return release{Version: next}, err
		}
	} else if command, err := terminalCommand(); err == nil {
		controls.command = command
		controls.latest = func(ctx context.Context) (release, error) { return terminalRelease(ctx, command, version) }
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

// terminalRelease asks command for the latest stable release. The update
// runs in one click on macOS, where agnostic-ai upgrade also replaces and
// restarts the companion.
func terminalRelease(ctx context.Context, command, current string) (release, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	ask := exec.CommandContext(ctx, command, "companion", "update-status", "--json")
	withoutConsoleWindow(ask)
	out, err := ask.Output()
	if err != nil {
		return release{}, fmt.Errorf("check for companion updates: %w", err)
	}
	var status struct {
		Latest   string `json:"latest"`
		OneClick bool   `json:"oneClick"`
	}
	if err := json.Unmarshal(out, &status); err != nil {
		return release{}, fmt.Errorf("check for companion updates: %w", err)
	}
	installed, ok := stableReleaseVersion(current)
	if !ok {
		return release{}, nil
	}
	latest, ok := stableReleaseVersion(status.Latest)
	if !ok || slices.Compare(latest[:], installed[:]) <= 0 {
		return release{}, nil
	}
	next := release{Version: strings.TrimPrefix(status.Latest, "v")}
	if !status.OneClick || runtime.GOOS != "darwin" {
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
	kept := status
	kept.Result = ""
	c.mu.Lock()
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
	c.check(ctx, false)
}

// check looks for a newer release; the caller holds busy. Only a check
// someone asked for says what it found, and asks before updating.
func (c *appCommands) check(ctx context.Context, asked bool) {
	known := c.status()
	c.setStatus(updateStatus{State: updateChecking, Version: known.Version})
	next, err := c.latest(ctx)
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
func (c *appCommands) update(next string) {
	path := filepath.Join(filepath.Dir(c.shell.settingsPath), "companion-update.log")
	step := updateDownloading
	c.setStatus(updateStatus{State: updateUpdating, Version: next, Step: step})
	failed := func(reason string) {
		status := updateStatus{State: updateFailed, Version: next, Reason: reason, Log: path}
		if c.app == "" {
			status.Command = upgradeCommand
		}
		c.setStatus(status)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
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
	withoutConsoleWindow(command)
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
			}
			return
		case <-ticker.C:
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
