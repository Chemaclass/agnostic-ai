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
	// RunInNewTerminal opens a Terminal window that runs command.
	RunInNewTerminal(ctx context.Context, command string) error
	// FocusTab selects the tab of app that shows tty and brings its
	// window to the front; ok is false when no tab shows that tty.
	FocusTab(ctx context.Context, app, tty string) (ok bool, err error)
	// RunInNewTab opens a tab of app that runs command.
	RunInNewTab(ctx context.Context, app, command string) error
	// TitleTab names the tab of app that shows tty; ok is false when no
	// tab shows that tty.
	TitleTab(ctx context.Context, app, tty, title string) (ok bool, err error)
}

// errNotHere means this OS has no supported way to run the route.
var errNotHere = errors.New("not supported on this system")

// errControlCharacters means a command holds a byte a terminal acts on
// as it is typed: single quotes stop the shell, not the terminal, so a
// carriage return or escape in a folder name would run what follows.
var errControlCharacters = errors.New("holds control characters")

// hasControl reports whether text holds a byte below 0x20 or 0x7f.
func hasControl(text string) bool {
	return strings.IndexFunc(text, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0
}

// errAutomationDenied means macOS refused to let the server control a
// terminal app; the person allows it once per app in System Settings.
var errAutomationDenied = errors.New("macOS blocked controlling the app")

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
		if held, ok := heldNow(c, processes); ok {
			return focusTerminal(ctx, held, processes, launcher)
		}
		return resumeTerminal(ctx, c, launcher)
	default:
		return OpenReply{Message: fmt.Sprintf("No way to open a %s conversation here.", toolLabels[c.Tool])}
	}
}

// heldNow checks a cursor-agent chat's store again before it is resumed:
// the chat may have started after the scan, or be older than the scan
// checks, and a resume would start a second copy of it.
func heldNow(c Conversation, processes Processes) (Conversation, bool) {
	if c.Tool != ToolCursor || c.store == "" {
		return c, false
	}
	pid := processes.Holders([]string{c.store})[c.store]
	if pid <= 0 {
		return c, false
	}
	// Another program with the store open, such as a database viewer,
	// must not stop an ended chat from resuming.
	command, _, _ := strings.Cut(processes.Args([]int{pid})[pid], " ")
	if !agentCommands[filepath.Base(command)] {
		return c, false
	}
	c.Live = true
	c.process = &liveProcess{PID: pid}
	c.Open.Kind = RouteFocus
	if table, err := processes.Table(); err == nil {
		place(&c, nil, table)
	}
	return c, true
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

// cloudOpening is what opening a tool's cloud conversation says.
type cloudOpening struct {
	// opened is the reply when the link opened.
	opened string
	// failed starts the reply when it did not; the link follows.
	failed string
	// home is where the person finds the conversation by hand.
	home string
}

var cloudOpenings = map[Tool]cloudOpening{
	ToolClaude: {opened: "Opened on claude.ai.", failed: "Could not open a browser. This session runs in the cloud: open ", home: claudeCloudLink},
	ToolCodex:  {opened: "Opened in the Codex app.", failed: "Could not open the Codex app. This thread runs in the cloud: open ", home: "the Codex app"},
}

// openCloud opens a cloud conversation where its tool shows it: a
// Claude Code session's own page in the browser, a Codex thread in the
// Codex app.
func openCloud(ctx context.Context, c Conversation, launcher Launcher) OpenReply {
	opening := cloudOpenings[c.Tool]
	if err := checkedLink(c.Open.Link); err != nil {
		return OpenReply{Message: "This conversation runs in the cloud and its link is not one this version opens. Find it in " + opening.home + " by its title."}
	}
	if err := launcher.OpenLink(ctx, c.Open.Link); err != nil {
		return OpenReply{Message: opening.failed + c.Open.Link}
	}
	return OpenReply{Opened: true, Message: opening.opened}
}

// tabApps are the terminal apps whose tabs the server selects by tty
// and opens new tabs in.
var tabApps = map[string]bool{"Terminal": true, "Ghostty": true}

// editorTerminals are the editors whose window on a folder holds the
// terminal, by the name hostOf gives them, with their macOS name.
var editorTerminals = map[string]string{
	"Cursor":  "Cursor",
	"VS Code": "Visual Studio Code",
	"Zed":     "Zed",
}

func allowControl(verb, app, then string) string {
	return fmt.Sprintf("macOS blocked %s %s. Allow agnostic-ai companion, or the terminal that runs agnostic-ai serve, to control %s in System Settings › Privacy & Security › Automation, %s.", verb, app, app, then)
}

// resumeTerminal resumes an ended session in a new tab of the app it
// last ran in. Its old tab is never typed into: a shell waiting in a
// builtin such as read looks the same as one at its prompt.
func resumeTerminal(ctx context.Context, c Conversation, launcher Launcher) OpenReply {
	app := ""
	if c.lastTab != nil {
		app = c.lastTab.App
	}
	opened := openInTab(ctx, launcher, app, c.Open.Command)
	byHand := func(message string) OpenReply { return OpenReply{Message: message, Command: c.Open.Command} }
	switch {
	case opened.err == nil && opened.window:
		return OpenReply{Opened: true, Message: "Resumed in a new Terminal window."}
	case opened.err == nil:
		return OpenReply{Opened: true, Message: fmt.Sprintf("Resumed in a new %s tab.", opened.app)}
	case errors.Is(opened.err, errControlCharacters):
		return OpenReply{Message: "Its folder name holds control characters, which a terminal would act on as they are typed, so it is not resumed from here."}
	case errors.Is(opened.err, errAutomationDenied):
		return byHand(allowControl("opening", opened.app, "or run this command yourself"))
	case opened.window && errors.Is(opened.err, errNotHere):
		return byHand("This system cannot open a terminal from the page. Run this command in a terminal.")
	case opened.window:
		return byHand("Could not open a Terminal window. Run this command in a terminal.")
	case errors.Is(opened.err, context.DeadlineExceeded):
		return byHand(waitingForPrompt(opened.app))
	default:
		return byHand(fmt.Sprintf("Could not open a %s tab. Run this command in a terminal.", opened.app))
	}
}

func waitingForPrompt(app string) string {
	return fmt.Sprintf("macOS is waiting for you to allow control of %s. Answer its prompt, then click again.", app)
}

// tabOpening is where openInTab ran a command, or why it could not.
type tabOpening struct {
	// app got the command: the app asked for, or Terminal for a window.
	app string
	// window is true when it went to a new Terminal window.
	window bool
	err    error
}

// openInTab runs command in a new tab of app when app takes one, else
// in a new Terminal window. Terminal before macOS 26 has no do script
// in new tab, so a failed Terminal tab falls back to a window; a refused
// or unanswered permission does not, since the window needs it too.
func openInTab(ctx context.Context, launcher Launcher, app, command string) tabOpening {
	if hasControl(command) {
		return tabOpening{app: app, err: errControlCharacters}
	}
	if tabApps[app] {
		err := launcher.RunInNewTab(ctx, app, command)
		if err == nil || app != "Terminal" || errors.Is(err, errAutomationDenied) || errors.Is(err, context.DeadlineExceeded) {
			return tabOpening{app: app, err: err}
		}
	}
	return tabOpening{app: "Terminal", window: true, err: launcher.RunInNewTerminal(ctx, command)}
}

// focusTerminal switches to the exact tab that runs a live session. It
// never starts a second copy, and never offers the resume command for
// one: that would compete with the running one.
// Only a selected tab counts as opened; an app brought forward with
// another tab in front says where the session is instead. A session
// that ended since the scan resumes as an ended one does.
func focusTerminal(ctx context.Context, c Conversation, processes Processes, launcher Launcher) OpenReply {
	if c.process == nil || !processes.Alive(c.process.PID, c.process.ProcStart) {
		return resumeTerminal(ctx, c, launcher)
	}
	switchThere := func(message string) OpenReply {
		return OpenReply{Message: message}
	}
	placed := c.placed
	// A thread the Codex daemon holds is live even with no client in
	// front of it, so the reply never offers to resume it again.
	if c.Tool == ToolCodex && (placed == nil || len(placed.ttys) > 0) {
		switch {
		case placed == nil:
			return OpenReply{Message: fmt.Sprintf("Codex keeps this thread open in the background (process %d), but no Codex terminal shows it.", c.process.PID)}
		case len(placed.ttys) == 1:
			return OpenReply{Message: "Codex keeps this thread open in the background, and the terminal that shows it could not be identified. It may be the Codex session on " + shortTTY(placed.ttys[0]) + "."}
		default:
			return OpenReply{Message: fmt.Sprintf("Codex keeps this thread open in the background and does not say which of its sessions shows it (%s). Switch to it there.", strings.Join(shortTTYs(placed.ttys), ", "))}
		}
	}
	if placed == nil {
		return switchThere(fmt.Sprintf("It is open (process %d), but the terminal that runs it could not be found. Switch to it there.", c.process.PID))
	}
	app, pid, tty := placed.host.App, placed.process.PID, shortTTY(placed.host.TTY)
	if tabApps[app] {
		ok, err := launcher.FocusTab(ctx, app, placed.host.TTY)
		switch {
		case errors.Is(err, errAutomationDenied):
			return switchThere(allowControl("switching to", app, "then click again"))
		case errors.Is(err, context.DeadlineExceeded):
			return switchThere(waitingForPrompt(app))
		case err != nil || !ok:
			return switchThere(fmt.Sprintf("It runs in %s on %s (process %d), but no tab shows that terminal. Switch to it there.", app, tty, pid))
		default:
			return OpenReply{Opened: true, Message: fmt.Sprintf("Switched to the %s tab that runs it.", app)}
		}
	}
	if editor, ok := editorTerminals[app]; ok && c.Folder != "" {
		if err := launcher.OpenFolder(ctx, editor, c.Folder); err != nil {
			return switchThere(fmt.Sprintf("It runs in %s's terminal on %s (process %d). Switch to it there.", app, tty, pid))
		}
		return switchThere(fmt.Sprintf("Brought its %s window to the front, but the terminal inside it cannot be selected from outside. It runs on %s (process %d).", app, tty, pid))
	}
	if app == "" {
		return switchThere(fmt.Sprintf("It runs on %s (process %d) in an app this version cannot switch to. Switch to it there.", tty, pid))
	}
	if err := launcher.ActivateApp(ctx, app); err != nil {
		return switchThere(fmt.Sprintf("It runs in %s on %s (process %d). Switch to it there.", app, tty, pid))
	}
	return switchThere(fmt.Sprintf("Brought %s to the front, but its tab cannot be selected from outside. It runs on %s (process %d).", app, tty, pid))
}

// TitleTab gives the tab that runs a live session the name the person
// chose. It does nothing for a session that ended or runs where no tab
// can be named.
func TitleTab(ctx context.Context, c Conversation, processes Processes, launcher Launcher, title string) error {
	if c.process == nil || c.placed == nil || !tabApps[c.placed.host.App] || !processes.Alive(c.process.PID, c.process.ProcStart) {
		return nil
	}
	ok, err := launcher.TitleTab(ctx, c.placed.host.App, c.placed.host.TTY, title)
	if err == nil && !ok {
		err = fmt.Errorf("no %s tab shows %s", c.placed.host.App, shortTTY(c.placed.host.TTY))
	}
	return err
}

func shortTTY(tty string) string { return strings.TrimPrefix(tty, "/dev/") }

func shortTTYs(ttys []string) []string {
	short := make([]string, len(ttys))
	for i, tty := range ttys {
		short[i] = shortTTY(tty)
	}
	return short
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
		// An unanswered Automation prompt holds osascript until the timeout.
		if ctx.Err() != nil {
			return "", fmt.Errorf("%s: %w", name, ctx.Err())
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
	`tell application "Terminal"`,
	"do script (item 1 of argv)",
	"activate",
	"end tell",
}

// script wraps lines in a handler that reads its values from argv.
func script(lines ...string) []string {
	args := []string{"-e", "on run argv"}
	for _, line := range lines {
		args = append(args, "-e", line)
	}
	return append(args, "-e", "end run")
}

// tabScripts hold, per app, the lines that find the tab whose tty is
// item 1 of argv as t, select it, and close the blocks they opened.
var tabScripts = map[string]struct{ open, match, close []string }{
	"Terminal": {
		open:  []string{`tell application "Terminal"`, "repeat with w in windows", "repeat with t in tabs of w", "if tty of t is (item 1 of argv) then"},
		match: []string{"set selected of t to true", "set index of w to 1"},
		close: []string{"end if", "end repeat", "end repeat", "end tell"},
	},
	"Ghostty": {
		open:  []string{`tell application "Ghostty"`, "repeat with t in terminals", "if tty of t is (item 1 of argv) then"},
		match: []string{"focus t"},
		close: []string{"end if", "end repeat", "end tell"},
	},
}

// titleTabLines name the tab whose terminal's tty is item 1 of argv
// after item 2. Ghostty names a tab, not the terminal inside it.
var titleTabLines = map[string][]string{
	"Terminal": {
		`tell application "Terminal"`,
		"repeat with w in windows",
		"repeat with t in tabs of w",
		"if tty of t is (item 1 of argv) then",
		"set custom title of t to (item 2 of argv)",
		`return "ok"`,
		"end if",
		"end repeat",
		"end repeat",
		"end tell",
		`return "missing"`,
	},
	"Ghostty": {
		`tell application "Ghostty"`,
		"repeat with w in windows",
		"repeat with k in tabs of w",
		"repeat with t in terminals of k",
		"if tty of t is (item 1 of argv) then",
		"set name override of k to (item 2 of argv)",
		`return "ok"`,
		"end if",
		"end repeat",
		"end repeat",
		"end repeat",
		"end tell",
		`return "missing"`,
	},
}

func titleTabArgs(app, tty, title string) []string {
	return append(script(titleTabLines[app]...), tty, title)
}

func focusTabArgs(app, tty string) []string {
	tabs := tabScripts[app]
	lines := append(append([]string{}, tabs.open...), tabs.match...)
	lines = append(lines, "activate", `return "ok"`)
	lines = append(lines, tabs.close...)
	lines = append(lines, `return "missing"`)
	return append(script(lines...), tty)
}

// newTabLines open a tab that runs item 1 of argv, in the front window
// or, with none open, in a new one.
var newTabLines = map[string][]string{
	"Terminal": {
		`tell application "Terminal"`,
		"do script in new tab (item 1 of argv)",
		"activate",
		"end tell",
	},
	"Ghostty": {
		`tell application "Ghostty"`,
		"if (count of windows) is 0 then",
		"set k to selected tab of (new window)",
		"else",
		"set k to new tab in front window",
		"end if",
		"set t to focused terminal of k",
		"input text (item 1 of argv) to t",
		`send key "enter" to t`,
		"focus t",
		"activate",
		"end tell",
	},
}

func runInNewTabArgs(app, command string) []string {
	return append(script(newTabLines[app]...), command)
}

func newTerminalArgs(command string) []string {
	return append(script(newTerminalScript...), command)
}

func (macLauncher) RunInNewTerminal(ctx context.Context, command string) error {
	_, err := run(ctx, "osascript", newTerminalArgs(command)...)
	return err
}

func (macLauncher) FocusTab(ctx context.Context, app, tty string) (bool, error) {
	if _, ok := tabScripts[app]; !ok {
		return false, errNotHere
	}
	out, err := run(ctx, "osascript", focusTabArgs(app, tty)...)
	if err != nil {
		return false, err
	}
	return out == "ok", nil
}

func (macLauncher) TitleTab(ctx context.Context, app, tty, title string) (bool, error) {
	if _, ok := titleTabLines[app]; !ok {
		return false, errNotHere
	}
	out, err := run(ctx, "osascript", titleTabArgs(app, tty, title)...)
	if err != nil {
		return false, err
	}
	return out == "ok", nil
}

func (macLauncher) RunInNewTab(ctx context.Context, app, command string) error {
	if _, ok := newTabLines[app]; !ok {
		return errNotHere
	}
	_, err := run(ctx, "osascript", runInNewTabArgs(app, command)...)
	return err
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

func (linuxLauncher) FocusTab(context.Context, string, string) (bool, error) {
	return false, errNotHere
}

func (linuxLauncher) RunInNewTab(context.Context, string, string) error { return errNotHere }

func (linuxLauncher) TitleTab(context.Context, string, string, string) (bool, error) {
	return false, errNotHere
}

type unsupportedLauncher struct{}

func (unsupportedLauncher) OpenLink(context.Context, string) error { return errNotHere }

func (unsupportedLauncher) OpenFolder(context.Context, string, string) error { return errNotHere }

func (unsupportedLauncher) ActivateApp(context.Context, string) error { return errNotHere }

func (unsupportedLauncher) RunInNewTerminal(context.Context, string) error { return errNotHere }

func (unsupportedLauncher) FocusTab(context.Context, string, string) (bool, error) {
	return false, errNotHere
}

func (unsupportedLauncher) RunInNewTab(context.Context, string, string) error { return errNotHere }

func (unsupportedLauncher) TitleTab(context.Context, string, string, string) (bool, error) {
	return false, errNotHere
}
