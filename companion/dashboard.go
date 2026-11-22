package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

// separateBinary keeps the normal recorded, PATH and fallback order.
func separateBinary() (string, error) {
	if recorded, ok := recordedBinary(); ok {
		return recorded, nil
	}
	if found, err := exec.LookPath(binaryName); err == nil {
		return found, nil
	}
	for _, folder := range usualBinaryFolders() {
		if found, err := exec.LookPath(filepath.Join(folder, binaryName)); err == nil {
			return found, nil
		}
	}
	return "", errNoBinary
}

func usualBinaryFolders() []string {
	folders := append([]string(nil), usualSystemFolders...)
	if home, err := os.UserHomeDir(); err == nil {
		for _, folder := range usualFolders {
			folders = append(folders, filepath.Join(home, folder))
		}
	}
	return folders
}

// standaloneCandidates visits each location so a link to a bundled CLI
// cannot hide the next command. The dashboard still uses separateBinary.
func standaloneCandidates() iter.Seq[string] {
	return func(yield func(string) bool) {
		if recorded, ok := recordedBinary(); ok && !yield(recorded) {
			return
		}
		folders := append(filepath.SplitList(os.Getenv("PATH")), usualBinaryFolders()...)
		for _, folder := range folders {
			// As in exec.LookPath, refuse to run a command found via a relative PATH entry.
			if !filepath.IsAbs(folder) {
				continue
			}
			if found, err := exec.LookPath(filepath.Join(folder, binaryName)); err == nil && !yield(found) {
				return
			}
		}
	}
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
		// Only an agnostic-ai older than this companion prints its own lines here.
		if strings.HasPrefix(raw, servedLinePrefix) || strings.HasPrefix(raw, "Using agnostic-ai at ") {
			err = mismatched(view.Path)
		} else {
			err = openReadyDashboard(ctx, raw, open)
		}
	case <-ctx.Done():
		err = ctx.Err()
	}
	if errors.Is(err, errMismatchedAgnosticAI) {
		_ = view.Process.Kill()
		return err
	}
	if err != nil {
		_ = view.Process.Kill()
		return fmt.Errorf("open the dashboard: %w. Try opening it again", err)
	}
	return nil
}

// matchesCompanion asks binary whether it knows --require-companion-secret
// before the dashboard runs it: an older agnostic-ai would serve the
// dashboard without the checks this companion relies on. The new one
// refuses at once without a secret, before it reads or listens.
func matchesCompanion(ctx context.Context, binary string) error {
	probe := exec.CommandContext(ctx, binary, "serve", "--require-companion-secret")
	probe.Env = append(os.Environ(), secretEnv+"=")
	withoutConsoleWindow(probe)
	said, _ := probe.CombinedOutput()
	if strings.Contains(string(said), "unknown flag") {
		return mismatched(binary)
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
	file, err := offerUnlockFile(ctx, client, address)
	if err == nil {
		return open(file)
	}
	// An older CLI or a server without sessions, such as on Windows, opens locked.
	log.Printf("open the dashboard locked: %v", err)
	return open(raw)
}

// unlockFileName is how the server names an unlock file: its port and a
// random id.
var unlockFileName = regexp.MustCompile(`^unlock-([0-9]+)-[0-9a-f]{12}\.html$`)

// offerUnlockFile asks the dashboard server for an unlock file, as a
// later agnostic-ai view does, so the page it opens gets the key that
// lets it loosen approvals. It returns the file only when it sits in this
// account's unlock folder and names the server's port: another program
// holding the port must not choose what the companion opens.
func offerUnlockFile(ctx context.Context, client *http.Client, address *url.URL) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, address.JoinPath("api", "session", "offer").String(), http.NoBody)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unlock offer answered %s", resp.Status)
	}
	var reply struct {
		File string `json:"file"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&reply); err != nil {
		return "", fmt.Errorf("read the unlock offer: %w", err)
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	file := filepath.Clean(reply.File)
	name := unlockFileName.FindStringSubmatch(filepath.Base(file))
	if !filepath.IsAbs(file) || name == nil || name[1] != address.Port() {
		return "", fmt.Errorf("the unlock offer named %q, outside the unlock folder", reply.File)
	}
	folder, err := filepath.EvalSymlinks(filepath.Join(config, "agnostic-ai", "unlock"))
	if err != nil {
		return "", fmt.Errorf("find the unlock folder: %w", err)
	}
	if parent, err := filepath.EvalSymlinks(filepath.Dir(file)); err != nil || parent != folder {
		return "", fmt.Errorf("the unlock offer named %q, outside the unlock folder", reply.File)
	}
	info, err := os.Lstat(file)
	if err != nil {
		return "", fmt.Errorf("%s: %w", file, err)
	}
	if err := ownUnlockFile(info); err != nil {
		return "", fmt.Errorf("%s: %w", file, err)
	}
	return file, nil
}

func (s *shell) dashboard(w http.ResponseWriter, r *http.Request) {
	binary, err := findBinary()
	if err == nil {
		ctx, cancel := context.WithTimeout(r.Context(), serverStartWait)
		defer cancel()
		err = matchesCompanion(ctx, binary)
		if err == nil {
			view := exec.Command(binary, "view", "--global")
			view.Env = append(os.Environ(), "AGNOSTIC_AI_COMPANION_VIEW=1")
			err = openDashboard(ctx, view, func(target string) error {
				if filepath.IsAbs(target) {
					return s.app.Browser.OpenFile(target)
				}
				return s.app.Browser.OpenURL(target)
			})
		}
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{Error: err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
