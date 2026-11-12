package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// binaryName is the agnostic-ai command, which serves the full web interface.
const binaryName = "agnostic-ai"

// usualFolders are searched after PATH: an app started from the desktop
// gets a short PATH that leaves them out.
var usualFolders = []string{".local/bin", "go/bin"}
var usualSystemFolders = []string{"/opt/homebrew/bin", "/usr/local/bin"}

var errNoBinary = errors.New("the agnostic-ai command was not found; install it, or set AGNOSTIC_AI_BIN to its path")

// commandPathFile holds the path of the agnostic-ai command that
// installed or last started this companion, beside the companion's
// settings. A companion started at login has no other way to learn it.
const commandPathFile = "companion-command"

// recordedBinary is the path `agnostic-ai companion` wrote down, when
// that file exists and the command is still there.
func recordedBinary() (string, bool) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", false
	}
	raw, err := os.ReadFile(filepath.Join(dir, "agnostic-ai", commandPathFile))
	if err != nil {
		return "", false
	}
	named := strings.TrimSpace(string(raw))
	if _, err := os.Stat(named); named == "" || err != nil {
		return "", false
	}
	return named, true
}

// findBinary locates the agnostic-ai command: AGNOSTIC_AI_BIN when set,
// then the path `agnostic-ai companion` wrote down, then PATH, then the
// usual install folders.
func findBinary() (string, error) {
	if named := os.Getenv("AGNOSTIC_AI_BIN"); named != "" {
		if _, err := os.Stat(named); err != nil {
			return "", fmt.Errorf("AGNOSTIC_AI_BIN: %w", err)
		}
		return named, nil
	}
	if recorded, ok := recordedBinary(); ok {
		return recorded, nil
	}
	if found, err := exec.LookPath(binaryName); err == nil {
		return found, nil
	}
	folders := usualSystemFolders
	if home, err := os.UserHomeDir(); err == nil {
		for _, folder := range usualFolders {
			folders = append(folders, filepath.Join(home, folder))
		}
	}
	for _, folder := range folders {
		if found, err := exec.LookPath(filepath.Join(folder, binaryName)); err == nil {
			return found, nil
		}
	}
	return "", errNoBinary
}

// openDashboard opens the full web interface on the user-level
// configuration, which lists every conversation as the companion does.
// `agnostic-ai view` reuses a server that already serves that page or
// starts one, apart from the companion's own, and that server keeps
// running after the companion pauses or quits.
func openDashboard() error {
	binary, err := findBinary()
	if err != nil {
		return err
	}
	view := exec.Command(binary, "view", "--global")
	withoutConsoleWindow(view)
	if err := view.Start(); err != nil {
		return fmt.Errorf("open the dashboard: %w", err)
	}
	// Collect the child when it ends, so it leaves no zombie behind.
	go func() { _ = view.Wait() }()
	return nil
}

// dashboard answers POST /api/dashboard.
func dashboard(w http.ResponseWriter, _ *http.Request) {
	if err := openDashboard(); err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{Error: err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
