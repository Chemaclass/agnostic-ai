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

// updateStatus is what the update entry shows; Version names the newer release.
type updateStatus struct {
	State   string `json:"state"`
	Version string `json:"version,omitempty"`
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

type appCommands struct {
	shell        *shell
	app, command string
	busy         sync.Mutex
	latest       func(context.Context) (string, error)

	mu      sync.Mutex
	current updateStatus
}

func (s *shell) ownAppCommands() *appCommands {
	app, command, ok := ownBundledCommand()
	if !ok {
		return nil
	}
	return &appCommands{shell: s, app: app, command: command, current: updateStatus{State: updateIdle}, latest: func(ctx context.Context) (string, error) {
		return latestCompanionRelease(ctx, &http.Client{Timeout: 15 * time.Second}, "https://api.github.com/repos/Chemaclass/agnostic-ai/releases/latest", version, updateImageName())
	}}
}

// updateStatus is the update entry's state; a companion without a bundled command has none yet.
func (s *shell) updateStatus() updateStatus {
	if s.appCommands == nil {
		return updateStatus{State: updateUnavailable}
	}
	return s.appCommands.status()
}

// runUpdate checks for updates, or offers the known one.
func (s *shell) runUpdate() {
	if s.appCommands != nil {
		go s.appCommands.clicked()
	}
}

func (s *shell) getUpdate(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.updateStatus())
}

func (c *appCommands) status() updateStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.current
}

func (c *appCommands) setStatus(status updateStatus) {
	c.mu.Lock()
	c.current = status
	c.mu.Unlock()
	c.shell.refreshNative()
	if c.shell.tellUpdate != nil {
		c.shell.tellUpdate(status)
	}
}

func (c *appCommands) clicked() {
	status := c.status()
	switch status.State {
	case updateIdle:
		c.checkUpdates(context.Background(), true)
	case updateAvailable, updateFailed:
		if c.busy.TryLock() {
			defer c.busy.Unlock()
			c.offer(context.Background(), status.Version)
		}
	}
}

func (c *appCommands) inform(title, message string) {
	dialog := c.shell.app.Dialog.Info().SetTitle(title).SetMessage(message)
	dialog.AddButton("OK").SetAsDefault()
	dialog.Show()
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

func (c *appCommands) checkUpdates(ctx context.Context, manual bool) {
	if !c.busy.TryLock() {
		return
	}
	defer c.busy.Unlock()
	known := c.status()
	c.setStatus(updateStatus{State: updateChecking})
	next, err := c.latest(ctx)
	if ctx.Err() != nil {
		c.setStatus(known)
		return
	}
	if err != nil {
		c.setStatus(known)
		if manual {
			c.inform("Check for updates", "Could not check for updates. Check your connection and try again.")
		} else {
			log.Print(err)
		}
		return
	}
	if next == "" {
		c.setStatus(updateStatus{State: updateIdle})
		if manual {
			c.inform("Check for updates", fmt.Sprintf("You have the latest version (v%s).", version))
		}
		return
	}
	c.setStatus(updateStatus{State: updateAvailable, Version: next})
	if manual {
		c.offer(ctx, next)
	}
}

// offer asks before updating to next; the caller holds busy.
func (c *appCommands) offer(ctx context.Context, next string) {
	dialog := c.shell.app.Dialog.Question().SetTitle("Update available").SetMessage(fmt.Sprintf("Agnostic AI %s is available. Update the app and its bundled command line tool, then restart?", next))
	approved := make(chan bool, 1)
	dialog.AddButton("Later").SetAsCancel().SetAsDefault().OnClick(func() { approved <- false })
	dialog.AddButton("Update and restart").OnClick(func() { approved <- true })
	dialog.Show()
	select {
	case update := <-approved:
		if update {
			c.update(next)
		}
	case <-ctx.Done():
	}
}

func (c *appCommands) update(next string) {
	c.setStatus(updateStatus{State: updateUpdating, Version: next})
	failed := func(message string) {
		c.setStatus(updateStatus{State: updateFailed, Version: next})
		c.inform("Update failed", message)
	}
	path := filepath.Join(filepath.Dir(c.shell.settingsPath), "companion-update.log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		failed(fmt.Sprintf("%s: %v", path, err))
		return
	}
	defer func() { _ = file.Close() }()
	// The updater must survive the companion ending while it replaces the app.
	command := exec.Command(c.command, "companion", "update-app", "--app", c.app, "--version", next)
	command.Stdout, command.Stderr = file, file
	if err := command.Start(); err != nil {
		failed(err.Error())
		return
	}
	if err := command.Wait(); err != nil {
		out, readErr := os.ReadFile(path)
		message := fmt.Sprintf("The update failed: %v. Choose Update to v%s to retry. Details: %s", err, next, path)
		if readErr == nil {
			message += "\n" + strings.TrimSpace(string(out))
		}
		failed(message)
	}
}

func (c *appCommands) watchUpdates(ctx context.Context) {
	if c == nil {
		return
	}
	c.checkUpdates(ctx, false)
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.checkUpdates(ctx, false)
		}
	}
}
