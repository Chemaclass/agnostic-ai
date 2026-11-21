package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

func findBinary() (string, error) {
	if named := os.Getenv("AGNOSTIC_AI_BIN"); named != "" {
		if _, err := os.Stat(named); err != nil {
			return "", fmt.Errorf("AGNOSTIC_AI_BIN: %w", err)
		}
		return named, nil
	}
	if _, command, ok := ownBundledCommand(); ok {
		return command, nil
	}
	return separateBinary()
}

// separateBinary finds an agnostic-ai command installed apart from the
// disk image app, such as with Homebrew or npm.
func separateBinary() (string, error) {
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

// The dashboard outlives the companion. Only its startup is bounded by ctx.
func openDashboard(ctx context.Context, view *exec.Cmd, open func(string) error) error {
	withoutConsoleWindow(view)
	output, err := view.StdoutPipe()
	if err != nil {
		return fmt.Errorf("read the dashboard address: %w", err)
	}
	problems := &tail{limit: maxProblemBytes}
	view.Stderr = problems
	if err := view.Start(); err != nil {
		return fmt.Errorf("start the dashboard: %w", err)
	}
	address := make(chan string, 1)
	ended := make(chan error, 1)
	go func() {
		lines := bufio.NewScanner(io.LimitReader(output, 4096))
		if lines.Scan() {
			address <- lines.Text()
			_, _ = io.Copy(io.Discard, output)
		}
		_ = output.Close()
		ended <- view.Wait()
		close(address)
	}()
	select {
	case raw, reported := <-address:
		if !reported {
			if exit := <-ended; exit != nil {
				return fmt.Errorf("start the dashboard: %w. %s", exit, problems.String())
			}
			return errors.New("the command exited without a dashboard address. Try opening it again")
		}
		// Older CLI-managed launches open the browser themselves and print its address.
		for _, prefix := range []string{servedLinePrefix, "Using agnostic-ai at "} {
			if legacy, found := strings.CutPrefix(raw, prefix); found {
				raw = legacy
				open = func(string) error { return nil }
				break
			}
		}
		err = openReadyDashboard(ctx, raw, open)
	case <-ctx.Done():
		err = ctx.Err()
	}
	if err != nil {
		_ = view.Process.Kill()
		return fmt.Errorf("open the dashboard: %w. Try opening it again", err)
	}
	return nil
}

func openReadyDashboard(ctx context.Context, raw string, open func(string) error) error {
	address, err := url.Parse(raw)
	if err != nil || address.Scheme != "http" || address.Hostname() != "127.0.0.1" || address.Port() == "" || address.User != nil || address.Path != "/" || address.RawQuery != "" || address.Fragment != "" {
		return fmt.Errorf("unexpected dashboard address %q", raw)
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	health, err := readServerHealth(ctx, client, strings.TrimSuffix(raw, "/"))
	if err != nil {
		return err
	}
	if !health.OK || !health.Global || !health.UI {
		return fmt.Errorf("%s did not serve the global dashboard", raw)
	}
	return open(raw)
}

func (s *shell) dashboard(w http.ResponseWriter, r *http.Request) {
	binary, err := findBinary()
	if err == nil {
		ctx, cancel := context.WithTimeout(r.Context(), serverStartWait)
		defer cancel()
		view := exec.Command(binary, "view", "--global")
		view.Env = append(os.Environ(), "AGNOSTIC_AI_COMPANION_VIEW=1")
		err = openDashboard(ctx, view, s.app.Browser.OpenURL)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{Error: err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
