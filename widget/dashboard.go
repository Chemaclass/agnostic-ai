package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

// binaryName is the agnostic-ai command, which serves the full web interface.
const binaryName = "agnostic-ai"

// usualFolders are searched after PATH: an app started from the desktop
// gets a short PATH that leaves them out.
var usualFolders = []string{".local/bin", "go/bin"}
var usualSystemFolders = []string{"/opt/homebrew/bin", "/usr/local/bin"}

var errNoBinary = errors.New("the agnostic-ai command was not found; install it, or set AGNOSTIC_AI_BIN to its path")

// findBinary locates the agnostic-ai command: AGNOSTIC_AI_BIN when set,
// then PATH, then the usual install folders.
func findBinary() (string, error) {
	if named := os.Getenv("AGNOSTIC_AI_BIN"); named != "" {
		if _, err := os.Stat(named); err != nil {
			return "", fmt.Errorf("AGNOSTIC_AI_BIN: %w", err)
		}
		return named, nil
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
// configuration, which lists every conversation as the widget does.
// `agnostic-ai view` reuses a server that already runs or starts one,
// and that server keeps running after the widget quits.
func openDashboard() error {
	binary, err := findBinary()
	if err != nil {
		return err
	}
	view := exec.Command(binary, "view", "--global")
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
