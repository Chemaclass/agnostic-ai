package conversations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Launcher runs what reopens a conversation.
type Launcher interface {
	// OpenLink hands a URL to the app registered for its scheme.
	OpenLink(ctx context.Context, link string) error
	// OpenFolder opens folder in an editor app, which brings the window
	// that already holds it to the front, and returns once it had a
	// moment to get there.
	OpenFolder(ctx context.Context, app, folder string) error
	// ActivateApp brings an app to the front.
	ActivateApp(ctx context.Context, app string) error
	// RunInNewTerminal opens a terminal window that runs command.
	RunInNewTerminal(ctx context.Context, command string) error
	// FocusTerminalTab brings the Terminal tab on tty to the front; ok
	// is false when no tab shows that tty.
	FocusTerminalTab(ctx context.Context, tty string) (ok bool, err error)
}

// errNotHere means this OS has no supported way to run the route.
var errNotHere = errors.New("not supported on this system")

// errAutomationDenied means macOS refused to let the server control
// Terminal; the person allows it once in System Settings.
var errAutomationDenied = errors.New("macOS blocked controlling Terminal")

// linkSchemes are the only schemes the server opens.
var linkSchemes = map[string]bool{"cursor": true, "vscode": true, "vscode-insiders": true, "vscodium": true, "claude": true, "codex": true}

const launchTimeout = 15 * time.Second

// An editor link goes to whichever window is in front, so it waits for
// the folder's window: a moment to bring an open one forward, longer to
// open a new one.
const (
	editorFocusDelay = 1200 * time.Millisecond
	editorOpenDelay  = 5 * time.Second
)

// editorUserDirs maps an editor app to its folder under Application
// Support, whose globalStorage/storage.json records the open windows.
var editorUserDirs = map[string]string{
	"Cursor":                        "Cursor",
	"Visual Studio Code":            "Code",
	"Visual Studio Code - Insiders": "Code - Insiders",
	"VSCodium":                      "VSCodium",
}

// openWindowFolders lists the folders of the windows the editor state at
// storagePath records, as folderKey values. The editor saves it as windows
// change, so a window opened moments ago may not be listed yet.
func openWindowFolders(storagePath string) map[string]bool {
	folders := map[string]bool{}
	data, err := os.ReadFile(storagePath)
	if err != nil {
		return folders
	}
	var state struct {
		WindowsState struct {
			OpenedWindows []struct {
				Folder string `json:"folder"`
			} `json:"openedWindows"`
			LastActiveWindow struct {
				Folder string `json:"folder"`
			} `json:"lastActiveWindow"`
		} `json:"windowsState"`
	}
	if json.Unmarshal(data, &state) != nil {
		return folders
	}
	for _, window := range append(state.WindowsState.OpenedWindows, state.WindowsState.LastActiveWindow) {
		// The editor percent-encodes every character but letters, digits,
		// "-._~" and "/", so @scope is written %40scope.
		if u, err := url.Parse(window.Folder); err == nil && u.Scheme == "file" {
			folders[windowPath(u.Path)] = true
		}
	}
	return folders
}

// folderKey is the form a folder takes in openWindowFolders.
func folderKey(folder string) string {
	return windowPath(uriPath(filepath.ToSlash(folder)))
}

// windowPath drops a trailing slash and writes a Windows drive in lower
// case, as the editor does: /c:/Users.
func windowPath(path string) string {
	path = strings.TrimSuffix(path, "/")
	if len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = "/" + strings.ToLower(path[1:2]) + path[2:]
	}
	return path
}

// enclosingWindowFolder returns the nearest folder, folder itself or
// one above it, that the editor state at storagePath lists a window on.
// Opening that folder brings its window forward; opening a subfolder of
// it would open a new window. A window on home or the root holds no
// project, so the search stops below them.
func enclosingWindowFolder(storagePath, folder, home string) (string, bool) {
	windows := openWindowFolders(storagePath)
	dir := filepath.Clean(folder)
	if windows[folderKey(dir)] {
		return dir, true
	}
	for {
		parent := filepath.Dir(dir)
		if parent == dir || parent == filepath.Dir(parent) || parent == filepath.Clean(home) {
			return "", false
		}
		if windows[folderKey(parent)] {
			return parent, true
		}
		dir = parent
	}
}

// SystemLauncher returns the launcher for the running OS.
func SystemLauncher() Launcher {
	switch runtime.GOOS {
	case "darwin":
		return macLauncher{}
	case "linux":
		return linuxLauncher{}
	default:
		return unsupportedLauncher{}
	}
}

// Open reopens a conversation along its route. The conversation comes
// from the server's own scan, never from the request.
func Open(ctx context.Context, c Conversation, processes Processes, launcher Launcher) OpenReply {
	if c.App == AppCloud {
		return openCloud(ctx, c, launcher)
	}
	switch c.Open.Kind {
	case RouteLink:
		return openLink(ctx, c, launcher)
	case RouteFocus:
		return focusTerminal(ctx, c, processes, launcher)
	case RouteTerminal:
		return newTerminal(ctx, c, launcher)
	default:
		return OpenReply{Message: fmt.Sprintf("No way to open a %s conversation here.", toolLabels[c.Tool])}
	}
}

// openLink hands the conversation to its app. An editor extension
// opens a link in the frontmost window of the editor, so the folder the
// conversation ran in opens first; when it cannot, the link still goes,
// and may land in another window.
func openLink(ctx context.Context, c Conversation, launcher Launcher) OpenReply {
	app := strings.TrimPrefix(c.Open.Label, "Open in ")
	folderShown := true
	if c.editor != nil && c.Folder != "" {
		folderShown = launcher.OpenFolder(ctx, c.editor.app, c.Folder) == nil
	}
	if err := launcher.OpenLink(ctx, c.Open.Link); err != nil {
		return OpenReply{Message: fmt.Sprintf("Could not hand the conversation to %s. Open this link instead: %s", app, c.Open.Link)}
	}
	if !folderShown {
		return OpenReply{Opened: true, Message: fmt.Sprintf("Opened in %s, but its folder %s could not be opened, so it may show in another window.", app, c.Folder)}
	}
	return OpenReply{Opened: true, Message: fmt.Sprintf("Opened in %s.", app)}
}

// openCloud opens a cloud session's own page in the browser.
func openCloud(ctx context.Context, c Conversation, launcher Launcher) OpenReply {
	if err := checkedLink(c.Open.Link); err != nil {
		return OpenReply{Message: "This session runs in the cloud and its link is not one this version opens. Find it at " + claudeCloudLink + " by its title."}
	}
	if err := launcher.OpenLink(ctx, c.Open.Link); err != nil {
		return OpenReply{Message: fmt.Sprintf("Could not open a browser. This session runs in the cloud: open %s", c.Open.Link)}
	}
	return OpenReply{Opened: true, Message: "Opened on claude.ai."}
}

func newTerminal(ctx context.Context, c Conversation, launcher Launcher) OpenReply {
	err := launcher.RunInNewTerminal(ctx, c.Open.Command)
	switch {
	case err == nil:
		return OpenReply{Opened: true, Message: "Resumed in a new Terminal window."}
	case errors.Is(err, errAutomationDenied):
		return OpenReply{Message: "macOS blocked opening Terminal. Allow agnostic-ai to control Terminal in System Settings › Privacy & Security › Automation, or run this command yourself.", Command: c.Open.Command}
	case errors.Is(err, errNotHere):
		return OpenReply{Message: "This system cannot open a terminal from the page. Run this command in a terminal.", Command: c.Open.Command}
	default:
		return OpenReply{Message: "Could not open a Terminal window. Run this command in a terminal.", Command: c.Open.Command}
	}
}

// focusTerminal switches to the Terminal tab that runs the session. A
// session that ended since the scan resumes in a new window instead;
// one running in another app's terminal is left there, since a second
// copy of a live session would compete with it.
func focusTerminal(ctx context.Context, c Conversation, processes Processes, launcher Launcher) OpenReply {
	if c.process == nil || !processes.Alive(c.process.PID, c.process.ProcStart) {
		return newTerminal(ctx, c, launcher)
	}
	app, err := processes.TerminalApp(c.process.PID)
	if err != nil || app != "Terminal" {
		return bringHostForward(ctx, c, app, launcher)
	}
	tty, err := processes.TTY(c.process.PID)
	if err != nil {
		return OpenReply{Message: fmt.Sprintf("It is open in Terminal (process %d), but its tab could not be found. Switch to it there.", c.process.PID), Command: c.Open.Command}
	}
	ok, err := launcher.FocusTerminalTab(ctx, tty)
	switch {
	case errors.Is(err, errAutomationDenied):
		return OpenReply{Message: "macOS blocked switching to Terminal. Allow agnostic-ai to control Terminal in System Settings › Privacy & Security › Automation.", Command: c.Open.Command}
	case err != nil || !ok:
		return OpenReply{Message: fmt.Sprintf("It is open in Terminal (process %d), but its tab could not be found. Switch to it there.", c.process.PID), Command: c.Open.Command}
	default:
		return OpenReply{Opened: true, Message: "Switched to the Terminal tab that runs it."}
	}
}

// terminalHosts are the apps besides Terminal that can hold a live
// session, by the name TerminalApp gives them, with their macOS
// application name. An editor opens folders, which brings the window
// that holds the session's folder forward; a tab inside it cannot be
// chosen from outside.
var terminalHosts = map[string]struct {
	app     string
	folders bool
}{
	"Cursor":  {"Cursor", true},
	"VS Code": {"Visual Studio Code", true},
	"Zed":     {"Zed", true},
	"iTerm":   {"iTerm", false},
	"Ghostty": {"Ghostty", false},
	"Warp":    {"Warp", false},
}

// bringHostForward shows the app whose terminal runs a live session:
// the editor window on its folder, or the terminal app.
func bringHostForward(ctx context.Context, c Conversation, app string, launcher Launcher) OpenReply {
	host, known := terminalHosts[app]
	if !known {
		return OpenReply{Message: fmt.Sprintf("It is open in another app (process %d). Switch to it there.", c.process.PID), Command: c.Open.Command}
	}
	if host.folders && c.Folder != "" {
		if err := launcher.OpenFolder(ctx, host.app, c.Folder); err != nil {
			return OpenReply{Message: fmt.Sprintf("It is open in %s's terminal (process %d). Switch to it there.", app, c.process.PID), Command: c.Open.Command}
		}
		return OpenReply{Opened: true, Message: fmt.Sprintf("Brought its %s window to the front. It runs in that window's terminal (process %d).", app, c.process.PID)}
	}
	if err := launcher.ActivateApp(ctx, host.app); err != nil {
		return OpenReply{Message: fmt.Sprintf("It is open in %s (process %d). Switch to it there.", app, c.process.PID), Command: c.Open.Command}
	}
	return OpenReply{Opened: true, Message: fmt.Sprintf("Brought %s to the front. It runs in one of its tabs (process %d).", app, c.process.PID)}
}

func checkedLink(link string) error {
	// The one kind of web page the server opens: a cloud session's own.
	if cloudSessionLinkPattern.MatchString(link) {
		return nil
	}
	parsed, err := url.Parse(link)
	if err != nil || !linkSchemes[parsed.Scheme] {
		return fmt.Errorf("refusing to open %q", link)
	}
	return nil
}

func run(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, launchTimeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// osascript reports a refused Automation permission as error -1743.
		if strings.Contains(stderr.String(), "-1743") {
			return "", errAutomationDenied
		}
		return "", fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

type macLauncher struct{}

func (macLauncher) OpenLink(ctx context.Context, link string) error {
	if err := checkedLink(link); err != nil {
		return err
	}
	_, err := run(ctx, "open", link)
	return err
}

func (macLauncher) OpenFolder(ctx context.Context, app, folder string) error {
	delay := editorOpenDelay
	home, _ := os.UserHomeDir()
	if userDir, ok := editorUserDirs[app]; ok {
		if window, open := enclosingWindowFolder(filepath.Join(home, "Library", "Application Support", userDir, "User", "globalStorage", "storage.json"), folder, home); open {
			folder, delay = window, editorFocusDelay
		}
	}
	if _, err := run(ctx, "open", "-a", app, folder); err != nil {
		return err
	}
	select {
	case <-time.After(delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (macLauncher) ActivateApp(ctx context.Context, app string) error {
	_, err := run(ctx, "open", "-a", app)
	return err
}

// The command and tty reach AppleScript as arguments, never as script text.
var newTerminalScript = []string{
	"-e", "on run argv",
	"-e", `tell application "Terminal"`,
	"-e", "do script (item 1 of argv)",
	"-e", "activate",
	"-e", "end tell",
	"-e", "end run",
}

var focusTabScript = []string{
	"-e", "on run argv",
	"-e", `tell application "Terminal"`,
	"-e", "repeat with w in windows",
	"-e", "repeat with t in tabs of w",
	"-e", "if tty of t is (item 1 of argv) then",
	"-e", "set selected of t to true",
	"-e", "set index of w to 1",
	"-e", "activate",
	"-e", `return "ok"`,
	"-e", "end if",
	"-e", "end repeat",
	"-e", "end repeat",
	"-e", "end tell",
	"-e", `return "missing"`,
	"-e", "end run",
}

func newTerminalArgs(command string) []string {
	return append(append([]string{}, newTerminalScript...), command)
}

func focusTabArgs(tty string) []string {
	return append(append([]string{}, focusTabScript...), tty)
}

func (macLauncher) RunInNewTerminal(ctx context.Context, command string) error {
	_, err := run(ctx, "osascript", newTerminalArgs(command)...)
	return err
}

func (macLauncher) FocusTerminalTab(ctx context.Context, tty string) (bool, error) {
	out, err := run(ctx, "osascript", focusTabArgs(tty)...)
	if err != nil {
		return false, err
	}
	return out == "ok", nil
}

type linuxLauncher struct{}

func (linuxLauncher) OpenLink(ctx context.Context, link string) error {
	if err := checkedLink(link); err != nil {
		return err
	}
	_, err := run(ctx, "xdg-open", link)
	return err
}

func (linuxLauncher) OpenFolder(context.Context, string, string) error { return errNotHere }

func (linuxLauncher) ActivateApp(context.Context, string) error { return errNotHere }

func (linuxLauncher) RunInNewTerminal(context.Context, string) error { return errNotHere }

func (linuxLauncher) FocusTerminalTab(context.Context, string) (bool, error) {
	return false, errNotHere
}

type unsupportedLauncher struct{}

func (unsupportedLauncher) OpenLink(context.Context, string) error { return errNotHere }

func (unsupportedLauncher) OpenFolder(context.Context, string, string) error { return errNotHere }

func (unsupportedLauncher) ActivateApp(context.Context, string) error { return errNotHere }

func (unsupportedLauncher) RunInNewTerminal(context.Context, string) error { return errNotHere }

func (unsupportedLauncher) FocusTerminalTab(context.Context, string) (bool, error) {
	return false, errNotHere
}
