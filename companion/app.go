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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
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

func latestCompanionRelease(ctx context.Context, client *http.Client, url, current string) (string, error) {
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
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("check for companion updates: GitHub returned %s", response.Status)
	}
	var release struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&release); err != nil {
		return "", fmt.Errorf("read latest companion release: %w", err)
	}
	latest, ok := stableReleaseVersion(release.Tag)
	if !ok || release.Draft || release.Prerelease {
		return "", nil
	}
	for i := range installed {
		if latest[i] > installed[i] {
			return strings.TrimPrefix(release.Tag, "v"), nil
		}
		if latest[i] < installed[i] {
			return "", nil
		}
	}
	return "", nil
}

type appCommands struct {
	shell        *shell
	app, command string
	busy         sync.Mutex
}

func (s *shell) addAppMenu(menu *application.Menu) *appCommands {
	app, command, ok := ownBundledCommand()
	if !ok {
		return nil
	}
	controls := &appCommands{shell: s, app: app, command: command}
	menu.AddSeparator()
	menu.Add("Install command line tool").OnClick(func(*application.Context) { go controls.installCommand() })
	menu.Add("Check for updates").OnClick(func(*application.Context) { go controls.checkUpdates(context.Background(), true) })
	return controls
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
	next, err := latestCompanionRelease(ctx, &http.Client{Timeout: 15 * time.Second}, "https://api.github.com/repos/Chemaclass/agnostic-ai/releases/latest", version)
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		if manual {
			c.inform("Check for updates", err.Error())
		} else {
			log.Print(err)
		}
		return
	}
	if next == "" {
		if manual {
			c.inform("Check for updates", "No newer stable release is available for this build.")
		}
		return
	}
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
	path := filepath.Join(filepath.Dir(c.shell.settingsPath), "companion-update.log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		c.inform("Update failed", fmt.Sprintf("%s: %v", path, err))
		return
	}
	defer file.Close()
	// The updater must survive the companion ending while it replaces the app.
	command := exec.Command(c.command, "companion", "update-app", "--app", c.app, "--version", next)
	command.Stdout, command.Stderr = file, file
	if err := command.Start(); err != nil {
		c.inform("Update failed", err.Error())
		return
	}
	if err := command.Wait(); err != nil {
		out, readErr := os.ReadFile(path)
		message := fmt.Sprintf("The update failed: %v. Details: %s", err, path)
		if readErr == nil {
			message += "\n" + strings.TrimSpace(string(out))
		}
		c.inform("Update failed", message)
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
