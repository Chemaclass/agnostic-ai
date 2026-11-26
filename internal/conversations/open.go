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
	"slices"
	"strings"
	"time"
	"unicode/utf8"
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
	// ReadTabNames lists every tab of app with the name the person gave
	// it and the ttys of its panes, in one look.
	ReadTabNames(ctx context.Context, app string) ([]TerminalTab, error)
	// TypeInTab types text into the tab of app that shows tty, without
	// pressing Enter, and brings it to the front; ok is false when no
	// tab shows that tty. An app that pastes from the clipboard puts
	// text there first only when setClipboard is true; otherwise it
	// pastes only when the clipboard holds exactly text.
	TypeInTab(ctx context.Context, app, tty, text string, setClipboard bool) (ok bool, err error)
	// ShowTmuxPane selects the pane on tty of the tmux server with pid
	// server in its window, and returns the pid of the tmux client that
	// shows it, or 0 when none does. It never moves a client to another
	// session.
	ShowTmuxPane(ctx context.Context, server int, tty string) (client int, err error)
}

// errNotHere means this OS has no supported way to run the route.
var errNotHere = errors.New("not supported on this system")

// runRefused is what a NoRun launcher returns instead of running or
// typing a command; the reply gives reason and the command to run by hand.
type runRefused struct{ reason string }

func (e *runRefused) Error() string { return e.reason }

// noRunLauncher opens links and folders and focuses tabs as its
// launcher does, and refuses to run or type a command in a terminal.
type noRunLauncher struct {
	Launcher
	reason string
}

// NoRun wraps launcher for a caller that may not start a command: an
// open still focuses a live chat, and a resume replies with reason and
// the command to run by hand.
func NoRun(launcher Launcher, reason string) Launcher {
	return noRunLauncher{Launcher: launcher, reason: reason}
}

func (l noRunLauncher) RunInNewTerminal(context.Context, string) error {
	return &runRefused{l.reason}
}

func (l noRunLauncher) RunInNewTab(context.Context, string, string) error {
	return &runRefused{l.reason}
}

func (l noRunLauncher) TypeInTab(context.Context, string, string, string, bool) (bool, error) {
	return false, &runRefused{l.reason}
}

// errControlCharacters means a command holds a character a terminal
// acts on as it is typed: single quotes stop the shell, not the terminal, so a
// carriage return or escape in a folder name would run what follows.
var errControlCharacters = errors.New("holds control characters")

// hasControl reports whether text holds a control character, C0, DEL
// or C1 (some terminals read U+009B as CSI), or is not valid UTF-8,
// which a terminal may decode as one.
func hasControl(text string) bool {
	if !utf8.ValidString(text) {
		return true
	}
	return strings.IndexFunc(text, func(r rune) bool { return r < 0x20 || (r >= 0x7f && r <= 0x9f) }) >= 0
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
		return linuxLauncher{getenv: os.Getenv, start: startApart}
	case "windows":
		return windowsLauncher{run: run, start: startInNewConsole}
	default:
		return unsupportedLauncher{}
	}
}

// Open reopens a conversation along its route. The conversation comes
// from the server's own scan, never from the request.
func Open(ctx context.Context, c Conversation, processes Processes, launcher Launcher) OpenReply {
	reply := openRoute(ctx, c, processes, launcher)
	reply.Command = commandHere(shellSystem, reply.Command)
	return reply
}

func openRoute(ctx context.Context, c Conversation, processes Processes, launcher Launcher) OpenReply {
	if c.App == AppCloud || c.Tool == ToolChatGPT {
		return openCloud(ctx, c, launcher)
	}
	switch c.Open.Kind {
	case RouteLink:
		return openLink(ctx, c, launcher)
	case RouteFocus:
		return focusTerminal(ctx, c, processes, launcher)
	case RouteTerminal:
		held, ok := heldNow(c, processes)
		if ok {
			return focusTerminal(ctx, held, processes, launcher)
		}
		if livenessUnknown(held) {
			return OpenReply{Message: livenessUnknownMessage, Command: c.Open.Command}
		}
		return resumeTerminal(ctx, c, launcher)
	default:
		return OpenReply{Message: fmt.Sprintf("No way to open a %s conversation here.", toolLabels[c.Tool])}
	}
}

// livenessUnknownMessage says why a terminal session whose process could
// not be checked is not resumed: a second client would compete with a
// running one.
const livenessUnknownMessage = "Could not check whether this session still runs, so it is not resumed from here. If it has ended, run this command in a terminal."

// livenessUnknown: the scan could not tell whether a process still
// holds this terminal session.
func livenessUnknown(c Conversation) bool {
	return c.liveUnknown && !c.Live && c.Open.Command != ""
}

// subagentFinished is the reply for a subagent its chat no longer
// lists, once the chat opened: it finished, or it was never there.
const subagentFinished = "That subagent has finished. Its chat is open."

// paneTarget is the helper whose pane an open of subagent brings
// forward: the subagent itself, or the nearest helper it runs inside,
// when one has a terminal pane of its own.
func paneTarget(helpers []helper, subagent string) (helper, bool) {
	picked, listed := subagentIn(helpers, subagent)
	if subagent == "" || !listed {
		return helper{}, false
	}
	for _, h := range append([]helper{picked}, ancestorsOf(helpers, picked)...) {
		if h.Reach == reachPane && h.member != "" {
			return h, true
		}
	}
	return helper{}, false
}

// subagentReply adds which subagent was asked for to the reply of what
// opened: its own pane, the pane of a helper it runs inside, or the chat
// it runs in. Only its own pane needs no note. Otherwise the note says
// how to reach it from there: the agent rows below the prompt, the row
// of each helper it runs inside, or the command that shows its pane.
// Only a terminal tab shows the rows; the tool takes no command that
// shows a subagent, and nothing is typed into the tab.
func subagentReply(reply OpenReply, c Conversation, picked helper, pane, attach string) OpenReply {
	reply.Helper = &picked
	if pane == picked.ID {
		reply.Reached = reachedSubagent
		return reply
	}
	reply.Reached = reachedParent
	// The helpers it runs inside, nearest first, up to the one whose
	// pane came to the front.
	var inside []helper
	for _, h := range ancestorsOf(c.Helpers, picked) {
		if h.ID == pane {
			break
		}
		inside = append(inside, h)
	}
	name := picked.shownName()
	rows := pane != "" || (c.App == AppCLI && c.Open.Kind == RouteFocus && (c.placed == nil || !c.placed.claudeBackground))
	switch {
	case attach != "":
		owner := picked
		if target, ok := paneTarget(c.Helpers, picked.ID); ok {
			owner = target
		}
		reply.Message += fmt.Sprintf(" The pane of '%s' is in a tmux session no terminal shows. Run this command to see it.", owner.shownName())
		reply.Command = attach
	case !rows && name != "":
		reply.Message += fmt.Sprintf(" Its subagent is '%s'.", name)
	case !rows:
	case len(inside) > 0 || (pane == "" && picked.Reach == reachNested && picked.ParentName != ""):
		reply.Message += nestedNote(picked, inside)
	case name != "":
		reply.Message += fmt.Sprintf(" Press the down arrow to pick '%s' below the prompt, then Enter.", name)
	default:
		reply.Message += " Press the down arrow to pick its subagent below the prompt, then Enter."
	}
	return reply
}

// nestedNote says which helpers to open, outermost first, to reach a
// subagent that runs inside others; inside lists them nearest first.
func nestedNote(picked helper, inside []helper) string {
	if len(inside) == 0 {
		return fmt.Sprintf(" It runs inside '%s'. Open that row first.", picked.ParentName)
	}
	parent := inside[0].shownName()
	if len(inside) == 1 {
		return fmt.Sprintf(" It runs inside '%s'. Open that row first.", parent)
	}
	path := make([]string, 0, len(inside))
	for _, h := range slices.Backward(inside) {
		path = append(path, "'"+h.shownName()+"'")
	}
	return fmt.Sprintf(" It runs inside '%s'. Open %s.", parent, strings.Join(path, ", then "))
}

// heldNow checks a cursor-agent chat's store again before it is resumed:
// the chat may have started after the scan, or be older than the scan
// checks, and a resume would start a second copy of it.
func heldNow(c Conversation, processes Processes) (Conversation, bool) {
	if c.Tool != ToolCursor || c.store == "" {
		return c, false
	}
	// This fresh read alone decides whether the chat may still run.
	held, err := processes.Holders([]string{c.store})
	pid := held[c.store]
	if pid <= 0 {
		c.liveUnknown = err != nil
		return c, false
	}
	// Another program with the store open, such as a database viewer,
	// must not stop an ended chat from resuming; a holder whose command
	// cannot be read may be the chat itself.
	if command := processes.Args([]int{pid})[pid]; !agentCommands[commandName(command)] {
		c.liveUnknown = err != nil || command == ""
		return c, false
	}
	c.Live = true
	c.process = &liveProcess{PID: pid}
	c.Open.Kind = RouteFocus
	if table, err := processes.Table(); err == nil {
		place(&c, nil, table, processes.Args)
	}
	return c, true
}

// openLink hands the conversation to its app. An editor extension
// opens a link in the frontmost window of the editor, so the folder the
// conversation ran in opens first; when it cannot, the link still goes,
// and may land in another window.
func openLink(ctx context.Context, c Conversation, launcher Launcher) OpenReply {
	app := strings.TrimPrefix(c.Open.Label, "Open in ")
	var folderErr error
	if c.editor != nil && c.Folder != "" {
		folderErr = launcher.OpenFolder(ctx, c.editor.app, c.Folder)
	}
	if err := launcher.OpenLink(ctx, c.Open.Link); err != nil {
		return OpenReply{Message: fmt.Sprintf("Could not hand the conversation to %s. Open this link instead: %s", app, c.Open.Link)}
	}
	if errors.Is(folderErr, errNotHere) {
		return OpenReply{Opened: true, Message: fmt.Sprintf("Opened in %s. On this system agnostic-ai cannot open its folder first, so it may show in another window.", app)}
	}
	if folderErr != nil {
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
	ToolClaude:  {opened: "Opened on claude.ai.", failed: "Could not open a browser. This session runs in the cloud: open ", home: claudeCloudLink},
	ToolCodex:   {opened: "Opened in the Codex app.", failed: "Could not open the Codex app. This thread runs in the cloud: open ", home: "the Codex app"},
	ToolChatGPT: {opened: "Opened on chatgpt.com.", failed: "Could not open a browser. Open ", home: "chatgpt.com"},
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
var tabApps = map[string]bool{"Terminal": true, "Ghostty": true, "iTerm": true, "WezTerm": true, "kitty": true, "Konsole": true}

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
	controlled := OpenReply{Message: "Its folder name holds control characters, which a terminal would act on as they are typed, so it is not resumed from here."}
	if c.Open.Command == "" && c.Folder == "" {
		return OpenReply{Message: "Its folder is not known, and it resumes only from the folder it started in, so it is not resumed from here."}
	}
	if c.Open.Command == "" {
		return controlled
	}
	opened := openInTab(ctx, launcher, app, c.Open.Command)
	switch {
	case opened.err == nil && opened.window:
		return OpenReply{Opened: true, Message: "Resumed in a new Terminal window."}
	case opened.err == nil:
		return OpenReply{Opened: true, Message: fmt.Sprintf("Resumed in a new %s tab.", opened.app)}
	case errors.Is(opened.err, errControlCharacters):
		return controlled
	default:
		return notOpenedInTab(opened, c.Open.Command)
	}
}

// notOpenedInTab says why openInTab could not run command, and offers
// it to run by hand.
func notOpenedInTab(opened tabOpening, command string) OpenReply {
	byHand := func(message string) OpenReply { return OpenReply{Message: message, Command: command} }
	refused, isRefused := errors.AsType[*runRefused](opened.err)
	switch {
	case isRefused:
		return byHand(refused.reason + " Run this command in a terminal.")
	case errors.Is(opened.err, errAutomationDenied):
		return byHand(allowControl("opening", opened.app, "or run this command yourself"))
	case opened.window && errors.Is(opened.err, errNotHere):
		return byHand("This system cannot open a terminal from the page. Run this command in a terminal.")
	case opened.window:
		return byHand("Could not open a Terminal window. Run this command in a terminal.")
	case errors.Is(opened.err, context.DeadlineExceeded):
		return byHand(waitingForPrompt(opened.app))
	case errors.Is(opened.err, errKonsoleCommandsOff):
		return byHand("Opened a new Konsole tab in its folder, but " + opened.err.Error() + ", or run this command there.")
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
// in new tab, so a failed Terminal tab falls back to a window, as does a
// tab app whose command cannot reach it; a refused or unanswered
// permission does not, since the window needs it too.
func openInTab(ctx context.Context, launcher Launcher, app, command string) tabOpening {
	if hasControl(command) {
		return tabOpening{app: app, err: errControlCharacters}
	}
	if tabApps[app] {
		err := launcher.RunInNewTab(ctx, app, command)
		fallsBack := app == "Terminal" || errors.Is(err, errNoTabCommand)
		_, refused := errors.AsType[*runRefused](err)
		if err == nil || !fallsBack || errors.Is(err, errAutomationDenied) || errors.Is(err, context.DeadlineExceeded) || refused {
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
	if c.process == nil {
		return resumeTerminal(ctx, c, launcher)
	}
	alive, err := processes.AliveAmong([]ProcessRef{{PID: c.process.PID, ProcStart: c.process.ProcStart}})
	if err != nil {
		return OpenReply{Message: livenessUnknownMessage, Command: c.Open.Command}
	}
	if !alive[c.process.PID] {
		return resumeTerminal(ctx, c, launcher)
	}
	placed := c.placed
	// Windows places no process in a terminal, and no window comes forward.
	// Nor can it tell a thread Codex's background service keeps, which no
	// resume command may compete with.
	if placed == nil && shellSystem == "windows" {
		resume := c.Open.Command
		if c.Tool == ToolCodex {
			resume = ""
		}
		return resumeElsewhere(fmt.Sprintf("It is open in another window (process %d), and agnostic-ai cannot bring a window forward on Windows. Switch to it there.", c.process.PID), resume)
	}
	// A thread the Codex daemon holds is live even with no client in
	// front of it, so the reply never offers to resume it again.
	if c.Tool == ToolCodex && (placed == nil || len(placed.ttys) > 0) {
		switch {
		case placed == nil:
			return OpenReply{Message: fmt.Sprintf("Codex keeps this thread open in the background (process %d), but no Codex terminal shows it.", c.process.PID)}
		case len(placed.ttys) == 1:
			if !processes.Alive(placed.process.PID, placed.process.ProcStart) {
				return OpenReply{Message: "Could not confirm that the only Codex terminal that may show this thread is still open."}
			}
			reply := switchToHost(ctx, c.Folder, *placed, processes, launcher, "")
			reply.Guessed = reply.Opened
			return reply
		default:
			return OpenReply{Message: fmt.Sprintf("Codex keeps this thread open in the background and does not say which of its sessions shows it (%s). Switch to it there.", strings.Join(shortTTYs(placed.ttys), ", "))}
		}
	}
	if placed == nil {
		return resumeElsewhere(fmt.Sprintf("It is open (process %d), but the terminal that runs it could not be found, as in a detached tmux session or after its tab closed. Switch to it there.", c.process.PID), c.Open.Command)
	}
	if c.Tool == ToolCodex && !placed.clientStillRuns(c.process.PID, processes) {
		return OpenReply{Message: "Could not confirm that the Codex terminal still shows this thread."}
	}
	if placed.claudeBackground {
		return openClaudeBackground(ctx, c, processes, launcher)
	}
	resume := c.Open.Command
	// The daemon keeps holding the thread after its client ends, so a resume would compete with it.
	if c.Tool == ToolCodex && placed.process.PID != c.process.PID {
		resume = ""
	}
	return switchToHost(ctx, c.Folder, *placed, processes, launcher, resume)
}

// resumeElsewhere is the reply for a session a process holds where no
// tab can be shown. It offers the resume command, which never runs
// here: a second copy would compete with the first.
func resumeElsewhere(message, resume string) OpenReply {
	if resume == "" {
		return OpenReply{Message: message}
	}
	return OpenReply{Message: message + " To resume it elsewhere, end that process first, then run this command.", Command: resume}
}

// switchToHost brings forward the terminal placed shows: the exact tab
// in a tab app, else the app, or the editor window open on folder.
// resume, when set, is offered when no tab shows the terminal.
func switchToHost(ctx context.Context, folder string, placed placement, processes Processes, launcher Launcher, resume string) OpenReply {
	switchThere := func(message string) OpenReply {
		return OpenReply{Message: message}
	}
	app, pid, tty := placed.host.App, placed.process.PID, shortTTY(placed.host.TTY)
	if app == "tmux" {
		if client, err := launcher.ShowTmuxPane(ctx, placed.host.tmuxServer, placed.host.TTY); err == nil && client != 0 {
			if table, err := processes.Table(); err == nil {
				if outer, _ := hostOf(table, client); outer.TTY != "" && outer.App != "tmux" {
					placed.host = outer
					return switchToHost(ctx, folder, placed, processes, launcher, resume)
				}
			}
		}
		app = ""
	}
	where := "It runs"
	tab := "the " + app + " tab that runs it"
	if len(placed.ttys) == 1 {
		where = "It may run"
		tab = "the " + app + " tab with the only Codex session that may show this thread"
	}
	cannot := "its tab cannot be selected from outside"
	setupNeeded := false
	if tabApps[app] {
		ok, err := launcher.FocusTab(ctx, app, placed.host.TTY)
		var setup tabSetupError
		switch {
		case errors.As(err, &setup):
			cannot, setupNeeded = setup.Error(), true
		case errors.Is(err, errNoTabCommand):
			// Without its command the app still comes forward below.
		case ok && errors.Is(err, errNotInFront):
			return switchThere("Selected " + tab + ". Its window may be behind other windows.")
		case errors.Is(err, errAutomationDenied):
			return switchThere(allowControl("switching to", app, "then click again"))
		case errors.Is(err, context.DeadlineExceeded):
			return switchThere(waitingForPrompt(app))
		case err != nil || !ok:
			return resumeElsewhere(fmt.Sprintf("%s in %s on %s (process %d), but no tab shows that terminal. Switch to it there.", where, app, tty, pid), resume)
		default:
			return OpenReply{Opened: true, Message: "Switched to " + tab + "."}
		}
	}
	if editor, ok := editorTerminals[app]; ok && folder != "" {
		if err := launcher.OpenFolder(ctx, editor, folder); err != nil {
			return switchThere(fmt.Sprintf("%s in %s's terminal on %s (process %d). Switch to it there.", where, app, tty, pid))
		}
		return switchThere(fmt.Sprintf("Brought its %s window to the front, but the terminal inside it cannot be selected from outside. %s on %s (process %d).", app, where, tty, pid))
	}
	if app == "" {
		return resumeElsewhere(fmt.Sprintf("%s on %s (process %d) in an app this version cannot switch to. Switch to it there.", where, tty, pid), resume)
	}
	if err := launcher.ActivateApp(ctx, app); err != nil {
		if setupNeeded {
			return switchThere(fmt.Sprintf("%s in %s on %s (process %d), but %s. Switch to it there.", where, app, tty, pid, cannot))
		}
		return switchThere(fmt.Sprintf("%s in %s on %s (process %d). Switch to it there.", where, app, tty, pid))
	}
	return switchThere(fmt.Sprintf("Brought %s to the front, but %s. %s on %s (process %d).", app, cannot, where, tty, pid))
}

// TitleTab gives the tab that runs a live session the name the person
// chose. It does nothing for a session that ended or runs where no tab
// can be named.
func TitleTab(ctx context.Context, c Conversation, processes Processes, launcher Launcher, title string) error {
	if _, ok := tabOf(c); !ok || !stillRuns(c, processes) {
		return nil
	}
	ok, err := launcher.TitleTab(ctx, c.placed.host.App, c.placed.host.TTY, title)
	if err == nil && !ok {
		err = fmt.Errorf("no %s tab shows %s", c.placed.host.App, shortTTY(c.placed.host.TTY))
	}
	return err
}

// tabOf is the tab of an app in tabApps that shows c's live session
// on one exact tty.
func tabOf(c Conversation) (terminalHost, bool) {
	if c.process == nil || c.placed == nil || len(c.placed.ttys) > 0 || !tabApps[c.placed.host.App] {
		return terminalHost{}, false
	}
	return c.placed.host, true
}

// stillRuns reports whether the session tabOf placed still runs, and
// for Codex whether its terminal client does.
func stillRuns(c Conversation, processes Processes) bool {
	return processes.Alive(c.process.PID, c.process.ProcStart) && (c.Tool != ToolCodex || c.placed.clientStillRuns(c.process.PID, processes))
}

func (p placement) clientStillRuns(holderPID int, processes Processes) bool {
	return p.process.PID == holderPID || processes.Alive(p.process.PID, p.process.ProcStart)
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
	// The web pages the server opens: a cloud session's or a ChatGPT chat's own.
	if cloudSessionLinkPattern.MatchString(link) || chatGPTLinkPattern.MatchString(link) {
		return nil
	}
	parsed, err := url.Parse(link)
	if err != nil || !linkSchemes[parsed.Scheme] {
		return fmt.Errorf("refusing to open %q", link)
	}
	return nil
}

func run(ctx context.Context, name string, args ...string) (string, error) {
	return runInput(ctx, "", name, args...)
}

// runInput runs a command like run, with input on its standard input.
func runInput(ctx context.Context, input, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, launchTimeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// osascript reports a refused Automation permission as error -1743.
		if strings.Contains(stderr.String(), "-1743") {
			return "", errAutomationDenied
		}
		// System Events reports keys it may not press as error 1002.
		if strings.Contains(stderr.String(), "(1002)") {
			return "", errAccessibilityDenied
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
	"iTerm": {
		open:  []string{`tell application "iTerm"`, "repeat with w in windows", "repeat with k in tabs of w", "repeat with t in sessions of k", "if tty of t is (item 1 of argv) then"},
		match: []string{"select t", "select k", "select w"},
		close: []string{"end if", "end repeat", "end repeat", "end repeat", "end tell"},
	},
}

// titleTabLines name the tab whose terminal's tty is item 1 of argv
// after item 2. Ghostty names a tab, not the terminal inside it; iTerm2
// names the session in the pane.
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
	"iTerm": {
		`tell application "iTerm"`,
		"repeat with w in windows",
		"repeat with k in tabs of w",
		"repeat with t in sessions of k",
		"if tty of t is (item 1 of argv) then",
		"set name of t to (item 2 of argv)",
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
	"iTerm": {
		`tell application "iTerm"`,
		"if (count of windows) is 0 then",
		"set w to (create window with default profile)",
		"else",
		"set w to current window",
		"tell w to create tab with default profile",
		"end if",
		"tell current session of w to write text (item 1 of argv)",
		"activate",
		"end tell",
	},
}

// typeInTabLines type item 2 of argv into the terminal whose tty is
// item 1 of argv. Ghostty takes it as a paste, so a program that asks
// for bracketed paste gets every line in one paste. iTerm2 types it, so
// the script wraps it in the bracketed paste markers itself; every tool
// typed into asks for bracketed paste. Terminal has no such
// command (do script presses Enter), so it pastes the clipboard with
// Cmd+V, only once the tab is selected in the front window. Item 3 is
// "set" to put the text on the clipboard first, else the clipboard must
// already hold it. The delay lets go of the keys that asked for the send,
// which would otherwise turn Cmd+V into another shortcut.
var typeInTabLines = map[string][]string{
	"Terminal": {
		`tell application "Terminal"`,
		"repeat with w in windows",
		"repeat with t in tabs of w",
		"if tty of t is (item 1 of argv) then",
		"set selected of t to true",
		"set index of w to 1",
		"activate",
		"delay 0.3",
		"repeat 20 times",
		"if frontmost then exit repeat",
		"delay 0.05",
		"end repeat",
		`if not frontmost then return "behind"`,
		`if tty of selected tab of front window is not (item 1 of argv) then return "behind"`,
		`if (item 3 of argv) is "set" then`,
		"tell current application to set the clipboard to (item 2 of argv)",
		"else",
		"try",
		"tell current application to set copied to (the clipboard as text)",
		"on error",
		`return "changed"`,
		"end try",
		"considering case",
		`if copied is not (item 2 of argv) then return "changed"`,
		"end considering",
		"end if",
		`tell application "System Events" to keystroke "v" using command down`,
		`return "ok"`,
		"end if",
		"end repeat",
		"end repeat",
		"end tell",
		`return "missing"`,
	},
	"Ghostty": {
		`tell application "Ghostty"`,
		"repeat with t in terminals",
		"if tty of t is (item 1 of argv) then",
		"input text (item 2 of argv) to t",
		"focus t",
		"activate",
		`return "ok"`,
		"end if",
		"end repeat",
		"end tell",
		`return "missing"`,
	},
	"iTerm": {
		`tell application "iTerm"`,
		"repeat with w in windows",
		"repeat with k in tabs of w",
		"repeat with t in sessions of k",
		"if tty of t is (item 1 of argv) then",
		`tell t to write text ((character id 27) & "[200~" & (item 2 of argv) & (character id 27) & "[201~") without newline`,
		"select t",
		"select k",
		"select w",
		"activate",
		`return "ok"`,
		"end if",
		"end repeat",
		"end repeat",
		"end repeat",
		"end tell",
		`return "missing"`,
	},
}

func typeInTabArgs(app, tty, text string, setClipboard bool) []string {
	clipboard := "check"
	if setClipboard {
		clipboard = "set"
	}
	return append(script(typeInTabLines[app]...), tty, text, clipboard)
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
	if driver, ok := commandTabs[app]; ok {
		return driver.focus(ctx, tty)
	}
	// Konsole is driven only on Linux; elsewhere it comes forward as an
	// app without its command does.
	if _, ok := tabScripts[app]; !ok {
		return false, errNoTabCommand
	}
	out, err := run(ctx, "osascript", focusTabArgs(app, tty)...)
	if err != nil {
		return false, err
	}
	return out == "ok", nil
}

func (macLauncher) TitleTab(ctx context.Context, app, tty, title string) (bool, error) {
	if driver, ok := commandTabs[app]; ok {
		return driver.title(ctx, tty, title)
	}
	if _, ok := titleTabLines[app]; !ok {
		return false, errNotHere
	}
	out, err := run(ctx, "osascript", titleTabArgs(app, tty, title)...)
	if err != nil {
		return false, err
	}
	return out == "ok", nil
}

func (macLauncher) ReadTabNames(ctx context.Context, app string) ([]TerminalTab, error) {
	args, ok := tabNameReadArgs(app)
	if !ok {
		return nil, errNotHere
	}
	out, err := run(ctx, "osascript", args...)
	if err != nil {
		return nil, err
	}
	return parseTabNames(out), nil
}

func (macLauncher) TypeInTab(ctx context.Context, app, tty, text string, setClipboard bool) (bool, error) {
	if driver, ok := commandTabs[app]; ok {
		return driver.typeIn(ctx, tty, text)
	}
	if _, ok := typeInTabLines[app]; !ok {
		return false, errNotHere
	}
	out, err := run(ctx, "osascript", typeInTabArgs(app, tty, text, setClipboard)...)
	switch {
	case err != nil:
		return false, err
	case out == "behind":
		return false, errNotInFront
	case out == "changed":
		return false, errClipboardChanged
	default:
		return out == "ok", nil
	}
}

func (macLauncher) RunInNewTab(ctx context.Context, app, command string) error {
	if driver, ok := commandTabs[app]; ok {
		return driver.newTab(ctx, command)
	}
	if _, ok := newTabLines[app]; !ok {
		return errNoTabCommand
	}
	_, err := run(ctx, "osascript", runInNewTabArgs(app, command)...)
	return err
}

// linuxTerminal is the terminal Debian and Ubuntu let the person choose.
const linuxTerminal = "x-terminal-emulator"

func (macLauncher) ShowTmuxPane(ctx context.Context, server int, tty string) (int, error) {
	return tmuxServers{run: run, path: tmuxPath, sockets: tmuxSockets}.show(ctx, server, tty)
}

type linuxLauncher struct {
	getenv func(string) string
	// start starts a program and returns without waiting for it.
	start func(name string, args ...string) error
}

// startApart starts a program in its own session: a terminal window
// stays open after the request that opened it ends, and after the
// server's own terminal closes.
func startApart(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func (linuxLauncher) OpenLink(ctx context.Context, link string) error {
	if err := checkedLink(link); err != nil {
		return err
	}
	_, err := run(ctx, "xdg-open", link)
	return err
}

func (linuxLauncher) OpenFolder(context.Context, string, string) error { return errNotHere }

func (linuxLauncher) ActivateApp(context.Context, string) error { return errNotHere }

// RunInNewTerminal runs command in the person's interactive shell, so
// a tool found only through their shell startup files is found, then
// keeps a shell open, as a macOS Terminal window does. A server with no
// display, such as one reached over SSH, would show no window.
func (l linuxLauncher) RunInNewTerminal(_ context.Context, command string) error {
	if l.getenv("DISPLAY") == "" && l.getenv("WAYLAND_DISPLAY") == "" {
		return errNotHere
	}
	shell := l.getenv("SHELL")
	if shell == "" {
		shell = "sh"
	}
	err := l.start(linuxTerminal, "-e", shell, "-ic", command+"; exec "+shellQuote(shell))
	if errors.Is(err, exec.ErrNotFound) {
		return errNotHere
	}
	return err
}

func (linuxLauncher) FocusTab(ctx context.Context, app, tty string) (bool, error) {
	if driver, ok := linuxCommandTabs[app]; ok {
		return driver.focus(ctx, tty)
	}
	return false, errNotHere
}

func (linuxLauncher) RunInNewTab(ctx context.Context, app, command string) error {
	if driver, ok := linuxCommandTabs[app]; ok {
		return driver.newTab(ctx, command)
	}
	return errNotHere
}

func (linuxLauncher) ShowTmuxPane(ctx context.Context, server int, tty string) (int, error) {
	return tmuxServers{run: run, path: tmuxPath, sockets: tmuxSockets}.show(ctx, server, tty)
}

func (linuxLauncher) TitleTab(ctx context.Context, app, tty, title string) (bool, error) {
	if driver, ok := linuxCommandTabs[app]; ok {
		return driver.title(ctx, tty, title)
	}
	return false, errNotHere
}

func (linuxLauncher) ReadTabNames(context.Context, string) ([]TerminalTab, error) {
	return nil, errNotHere
}

func (linuxLauncher) TypeInTab(ctx context.Context, app, tty, text string, _ bool) (bool, error) {
	if driver, ok := linuxCommandTabs[app]; ok {
		return driver.typeIn(ctx, tty, text)
	}
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

func (unsupportedLauncher) ShowTmuxPane(context.Context, int, string) (int, error) {
	return 0, errNotHere
}

func (unsupportedLauncher) TitleTab(context.Context, string, string, string) (bool, error) {
	return false, errNotHere
}

func (unsupportedLauncher) ReadTabNames(context.Context, string) ([]TerminalTab, error) {
	return nil, errNotHere
}

func (unsupportedLauncher) TypeInTab(context.Context, string, string, string, bool) (bool, error) {
	return false, errNotHere
}
