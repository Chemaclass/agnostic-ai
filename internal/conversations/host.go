package conversations

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/atomicfile"
)

// ProcessInfo is one row of the process table.
type ProcessInfo struct {
	PID, PPID, PGID int
	// Stat is the ps state; a "+" marks the tty's foreground group.
	Stat string
	// TTY is the terminal device, such as /dev/ttys004; empty for none.
	TTY string
	// Started is ps lstart in UTC and the C locale.
	Started string
	// Command is the executable, a full path for an app's binary.
	Command string
}

// lstartLayout reads ps lstart in the C locale.
const lstartLayout = "Mon Jan _2 15:04:05 2006"

// parseProcessTable reads `ps -axo pid=,ppid=,pgid=,stat=,tty=,lstart=,comm=`.
// lstart is five fields, and comm, last, may hold spaces.
func parseProcessTable(out string) []ProcessInfo {
	var table []ProcessInfo
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 11 {
			continue
		}
		pid, pidErr := strconv.Atoi(fields[0])
		ppid, ppidErr := strconv.Atoi(fields[1])
		pgid, pgidErr := strconv.Atoi(fields[2])
		if pidErr != nil || ppidErr != nil || pgidErr != nil {
			continue
		}
		rest := line
		for range 5 {
			rest = strings.TrimLeft(rest, " ")
			rest = rest[strings.IndexByte(rest, ' '):]
		}
		rest = strings.TrimLeft(rest, " ")
		if len(rest) < len(lstartLayout) {
			continue
		}
		started := rest[:len(lstartLayout)]
		if _, err := time.Parse(lstartLayout, started); err != nil {
			continue
		}
		tty := fields[4]
		if tty == "??" || tty == "?" || tty == "-" {
			tty = ""
		} else {
			tty = "/dev/" + strings.TrimPrefix(tty, "/dev/")
		}
		table = append(table, ProcessInfo{PID: pid, PPID: ppid, PGID: pgid, Stat: fields[3], TTY: tty, Started: started, Command: strings.TrimSpace(rest[len(lstartLayout):])})
	}
	return table
}

// terminalHost is where a session's terminal shows.
type terminalHost struct {
	// App is the app that shows the terminal, such as "Ghostty"; empty
	// when no known app is an ancestor.
	App string `json:"app"`
	// TTY is the terminal device of the session's process.
	TTY string `json:"tty"`
	// tmuxServer is the pid of the tmux server when App is "tmux".
	tmuxServer int
}

// terminalApps maps a fragment of an ancestor's command path to the app
// that shows the terminal.
var terminalApps = []struct{ fragment, app string }{
	{"/Terminal.app/", "Terminal"},
	{"/iTerm.app/", "iTerm"},
	// iTerm2 keeps its shells under a session server outside the app.
	{"/iTerm2/iTermServer-", "iTerm"},
	{"/Ghostty.app/", "Ghostty"},
	{"/WezTerm.app/", "WezTerm"},
	{"/kitty.app/", "kitty"},
	{"/Warp.app/", "Warp"},
	{"/Cursor.app/", "Cursor"},
	{"/Visual Studio Code.app/", "VS Code"},
	{"/Zed.app/", "Zed"},
}

// terminalPrograms names the terminals outside a macOS app bundle, as
// on Linux, by the program that holds the shell.
var terminalPrograms = map[string]string{
	"wezterm-gui":           "WezTerm",
	"kitty":                 "kitty",
	"konsole":               "Konsole",
	"gnome-terminal-server": "GNOME Terminal",
	"ptyxis-agent":          "Ptyxis",
	"alacritty":             "Alacritty",
	"foot":                  "foot",
	"xterm":                 "xterm",
	"tilix":                 "Tilix",
	"xfce4-terminal":        "Xfce Terminal",
}

// programOf is the name of the program ps names, or "" for one inside a
// macOS app bundle, whose path may hold spaces. Linux keeps the first 15
// characters of a name, so a longer one comes back whole; tmux renames
// its server "tmux: server".
func programOf(command string) string {
	if strings.Contains(command, ".app/") {
		return ""
	}
	if filepath.Base(command) == "tmux" {
		return "tmux"
	}
	first, _, _ := strings.Cut(command, " ")
	program := strings.TrimSuffix(filepath.Base(first), ":")
	if len(program) == linuxNameLength {
		for name := range terminalPrograms {
			if strings.HasPrefix(name, program) {
				return name
			}
		}
	}
	return program
}

const linuxNameLength = 15

const maxAncestors = 12

// hostOf walks up from pid to the app that shows its terminal. ok is
// false when pid has no terminal or no known app is an ancestor; the
// host still carries the tty when there is one.
func hostOf(table []ProcessInfo, pid int) (host terminalHost, ok bool) {
	byPID := make(map[int]ProcessInfo, len(table))
	for _, process := range table {
		byPID[process.PID] = process
	}
	process, found := byPID[pid]
	if !found || process.TTY == "" {
		return terminalHost{}, false
	}
	host.TTY = process.TTY
	child := process
	for range maxAncestors {
		parent, found := byPID[child.PPID]
		if !found || child.PPID <= 1 {
			return host, false
		}
		for _, known := range terminalApps {
			if strings.Contains(parent.Command, known.fragment) {
				host.App = known.app
				return host, true
			}
		}
		if app, ok := terminalPrograms[programOf(parent.Command)]; ok {
			host.App = app
			return host, true
		}
		// The tmux server leaves the terminal that started it, so its
		// panes have no app above them; a tmux client shows them.
		if programOf(parent.Command) == "tmux" && parent.TTY == "" {
			host.App, host.tmuxServer = "tmux", parent.PID
			return host, true
		}
		child = parent
	}
	return host, false
}

// codexClients finds the terminal client that shows a thread the Codex
// app-server daemon holds. Codex does not record which client shows a
// thread, and its daemon keeps a thread after the client quits, so the
// pick reads clues: the command lines once, and the clients' folders
// once and only when no command line names the thread.
type codexClients struct {
	table     []ProcessInfo
	threads   []threadStart
	processes Processes
	loaded    bool
	pids      []int
	clients   []ProcessInfo
	cwds      map[int]string
	runs      map[int]codexRun
	first     map[int]codexFirstThread
}

// threadStart is when and where a Codex CLI thread was created.
type threadStart struct{ id, created, folder string }

type codexFirstThread struct {
	threadStart
	tied bool
}

// newCodexClients reads table and the Codex CLI threads.
func newCodexClients(table []ProcessInfo, threads []threadStart, processes Processes) *codexClients {
	return &codexClients{table: table, threads: threads, processes: processes}
}

func (k *codexClients) load() {
	if k.loaded {
		return
	}
	k.loaded = true
	var candidates []ProcessInfo
	for _, process := range k.table {
		if process.TTY != "" && filepath.Base(process.Command) == "codex" {
			candidates = append(candidates, process)
			k.pids = append(k.pids, process.PID)
		}
	}
	if len(candidates) == 0 {
		return
	}
	args := k.processes.Args(k.pids)
	k.runs = make(map[int]codexRun, len(candidates))
	for _, candidate := range candidates {
		command := args[candidate.PID]
		if !strings.Contains(command, "app-server") {
			k.clients = append(k.clients, candidate)
			k.runs[candidate.PID] = readCodexRun(command)
		}
	}
}

func (k *codexClients) folders() map[int]string {
	if k.cwds == nil {
		k.cwds = k.processes.Cwds(k.pids)
		if k.cwds == nil {
			k.cwds = map[int]string{}
		}
	}
	return k.cwds
}

func (k *codexClients) folder(pid int) string {
	if k.runs[pid].elsewhere {
		return ""
	}
	return k.folders()[pid]
}

// pick identifies a client by an explicit resume ID. Timing and folder
// only suggest candidates because the daemon can keep an exited client's thread.
func (k *codexClients) pick(c Conversation) (int, []ProcessInfo) {
	k.load()
	for _, client := range k.clients {
		run := k.runs[client.PID]
		if run.thread != "" && run.thread == c.ID {
			return client.PID, nil
		}
	}

	created, err := time.Parse(time.RFC3339, c.StartedAt)
	if err != nil {
		return 0, nil
	}
	first := k.firstThreads()
	var candidates []ProcessInfo
	for _, client := range k.clients {
		run := k.runs[client.PID]
		started, err := time.Parse(lstartLayout, client.Started)
		if err != nil {
			continue
		}
		thread := first[client.PID]
		folder := k.folder(client.PID)
		switch {
		case c.Folder == "" || (folder != "" && folder != c.Folder):
			continue
		case run.resumes && run.thread == "" && !created.After(started):
			candidates = append(candidates, client)
		case !run.resumes && !created.Before(started) &&
			(thread.id == "" || thread.id == c.ID || (thread.tied && thread.created == c.StartedAt)):
			candidates = append(candidates, client)
		}
	}
	if len(candidates) == 1 && k.folder(candidates[0].PID) == "" {
		return 0, nil
	}
	return 0, candidates
}

// A second launch ends the first client's window. If an older client had
// not started a thread by then, either client might create the next one.
func (k *codexClients) firstThreads() map[int]codexFirstThread {
	if k.first != nil {
		return k.first
	}
	k.first = make(map[int]codexFirstThread, len(k.clients))
	starts := make(map[int]time.Time, len(k.clients))
	ordered := make([]ProcessInfo, 0, len(k.clients))
	for _, client := range k.clients {
		if started, err := time.Parse(lstartLayout, client.Started); err == nil {
			starts[client.PID] = started
			ordered = append(ordered, client)
		}
	}
	sort.Slice(ordered, func(i, j int) bool { return starts[ordered[i].PID].Before(starts[ordered[j].PID]) })
	for i, client := range ordered {
		folder := k.folder(client.PID)
		if folder == "" || k.runs[client.PID].resumes {
			continue
		}
		overlaps := func(other ProcessInfo) bool {
			otherFolder := k.folder(other.PID)
			return otherFolder == "" || otherFolder == folder
		}
		var olderWaiting bool
		for _, other := range ordered[:i] {
			first := k.first[other.PID]
			if overlaps(other) && !k.runs[other.PID].resumes && (first.id == "" || first.tied) {
				olderWaiting = true
				break
			}
		}
		if olderWaiting {
			continue
		}
		var end time.Time
		for _, other := range ordered[i+1:] {
			if overlaps(other) {
				end = starts[other.PID]
				break
			}
		}
		k.first[client.PID] = firstThreadAfter(starts[client.PID], end, folder, k.threads)
	}
	return k.first
}

// codexRun is what a Codex client's command line says about the thread
// it shows.
type codexRun struct {
	// resumes is true for `codex resume` and `codex exec resume`.
	resumes bool
	// thread is the id it resumes, "" for the last one or a picker.
	thread string
	// elsewhere is true when -C or --cd moves it out of its process's
	// folder.
	elsewhere bool
}

// codexValueFlags are the Codex options that take the next field as
// their value.
var codexValueFlags = map[string]bool{
	"-c": true, "--config": true, "-C": true, "--cd": true, "-m": true, "--model": true,
	"-p": true, "--profile": true, "-s": true, "--sandbox": true, "-a": true, "--ask-for-approval": true,
	"-i": true, "--image": true, "--add-dir": true, "--enable": true, "--disable": true, "--local-provider": true,
}

var threadIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// readCodexRun reads a client's command line, which ps prints without
// quotes: a prompt such as "resume the migration" reads as several
// words, so resume with more than one word after it and no id is a
// prompt.
func readCodexRun(args string) codexRun {
	fields := strings.Fields(args)
	if len(fields) == 0 {
		return codexRun{}
	}
	var run codexRun
	var words []string
	last := false
	for i := 1; i < len(fields); i++ {
		field := fields[i]
		if !strings.HasPrefix(field, "-") {
			words = append(words, field)
			continue
		}
		name, _, inline := strings.Cut(field, "=")
		if name == "-C" || name == "--cd" {
			run.elsewhere = true
		}
		if name == "--last" || name == "--all" {
			last = true
		}
		if codexValueFlags[name] && !inline {
			i++
		}
	}
	if len(words) > 0 && words[0] == "exec" {
		words = words[1:]
	}
	if len(words) == 0 || words[0] != "resume" {
		return run
	}
	rest := words[1:]
	switch {
	case len(rest) > 0 && threadIDPattern.MatchString(rest[0]):
		run.resumes, run.thread = true, rest[0]
	case last || len(rest) <= 1:
		run.resumes = true
	}
	return run
}

func firstThreadAfter(started, end time.Time, folder string, threads []threadStart) codexFirstThread {
	var first codexFirstThread
	var firstAt time.Time
	for _, thread := range threads {
		if thread.folder != folder {
			continue
		}
		created, err := time.Parse(time.RFC3339, thread.created)
		if err != nil || created.Before(started) || (!end.IsZero() && !created.Before(end)) {
			continue
		}
		if first.id == "" || created.Before(firstAt) {
			first, firstAt = codexFirstThread{threadStart: thread}, created
		} else if created.Equal(firstAt) && thread.id != first.id {
			first.tied = true
		}
	}
	return first
}

// placement records the terminal and process of a live session. When ttys
// lists candidates, host and process describe the only candidate, if any.
type placement struct {
	process liveProcess
	host    terminalHost
	ttys    []string
	// claudeBackground is true for a Claude Code session started with
	// claude --bg, whose terminal belongs to Claude's daemon, not an app.
	claudeBackground bool
}

// claudePTYHost is the parent Claude Code's daemon gives each claude
// --bg session: it renames itself, so macOS ps comm shows these words.
const claudePTYHost = "claude bg-pty-host"

// linuxCommLimit is how many characters of a process name Linux keeps,
// so ps comm there shows claudePTYHost as "claude bg-pty-h". See
// proc_pid_comm(5).
const linuxCommLimit = 15

// claudeBackgroundSession reports whether pid's parent is the terminal
// Claude Code's daemon runs a background session in. A parent whose
// name is cut to Linux's limit is confirmed by its full command line,
// which starts "claude bg-pty-host".
func claudeBackgroundSession(table []ProcessInfo, pid int, args func(pids []int) map[int]string) bool {
	var parentPID int
	for _, process := range table {
		if process.PID == pid {
			parentPID = process.PPID
		}
	}
	for _, process := range table {
		if process.PID != parentPID {
			continue
		}
		switch process.Command {
		case claudePTYHost:
			return true
		case claudePTYHost[:linuxCommLimit]:
			return isClaudePTYHostArgs(args([]int{process.PID})[process.PID])
		}
	}
	return false
}

// isClaudePTYHostArgs reads the parent's command line, such as
// "claude bg-pty-host --bg-pty-host /tmp/cc-daemon-501/x.pty.sock ...".
func isClaudePTYHostArgs(args string) bool {
	fields := strings.Fields(args)
	return len(fields) >= 2 && filepath.Base(fields[0]) == "claude" && fields[1] == "bg-pty-host"
}

// placeLive finds the terminal of each session a process runs, with one
// look at the process table, and remembers its app so that an ended
// session resumes in a new tab there, as does claude attach for a
// Claude Code background session. A Codex thread the app-server daemon
// holds shows in a terminal client, which is looked for.
func (s *Scanner) placeLive(results []sourceResult, readTable func() ([]ProcessInfo, error)) {
	var live, liveHelpers []*Conversation
	var threads []threadStart
	for r := range results {
		for i := range results[r].conversations {
			c := &results[r].conversations[i]
			if c.Open.Kind == RouteFocus && c.process != nil {
				live = append(live, c)
			}
			if c.Tool == ToolCodex && c.App == AppCLI && c.StartedAt != "" {
				threads = append(threads, threadStart{id: c.ID, created: c.StartedAt, folder: c.Folder})
			}
		}
		for i := range results[r].unlisted {
			c := &results[r].unlisted[i]
			if c.Open.Kind == RouteFocus && c.process != nil {
				liveHelpers = append(liveHelpers, c)
			}
		}
	}
	tabs := s.rememberedTabs()
	if len(live) > 0 || len(liveHelpers) > 0 {
		if table, err := readTable(); err == nil {
			seen := map[string]terminalHost{}
			clients := newCodexClients(table, threads, s.Processes)
			for _, c := range live {
				if place(c, clients, table, s.Processes.Args) {
					seen[markKey(c.Tool, c.ID)] = c.placed.host
				}
			}
			// A helper's thread shows only in the terminal of the process
			// that holds it. The Codex clients are not asked: a run that a
			// Claude Code command started has no client of its own.
			for _, c := range liveHelpers {
				placeAt(c, *c.process, table, s.Processes.Args)
			}
			if changed := tabs.rememberAll(seen, s.Now()); changed {
				tabs = s.saveTabs(seen)
			}
		}
	}
	for r := range results {
		for i := range results[r].conversations {
			c := &results[r].conversations[i]
			endedInTerminal := c.Open.Kind == RouteTerminal && c.App == AppCLI
			claudeBackground := c.placed != nil && c.placed.claudeBackground
			if !endedInTerminal && !claudeBackground {
				continue
			}
			tab, ok := tabs.Tabs[markKey(c.Tool, c.ID)]
			if !ok || !isTabApp(tab.Host.App) {
				continue
			}
			host := tab.Host
			c.lastTab = &host
			// A background session's click may switch to a tab of
			// another app that already shows it, so its label stays.
			if endedInTerminal {
				c.Open.Label = "Resume in " + host.App
			}
		}
	}
}

func isTabApp(app string) bool {
	_, ok := tabApps[app]
	return ok
}

// place sets where c shows. ok is true when it shows in a tab of an app
// in tabApps, which is worth remembering.
func place(c *Conversation, clients *codexClients, table []ProcessInfo, args func(pids []int) map[int]string) (ok bool) {
	process := *c.process
	host, _ := hostOf(table, process.PID)
	if host.TTY == "" && c.Tool == ToolCodex {
		pid, candidates := clients.pick(*c)
		if pid == 0 {
			if len(candidates) > 0 {
				c.placed = &placement{ttys: codexClientTTYs(candidates)}
			}
			if len(candidates) == 1 {
				client := candidates[0]
				c.placed.host, _ = hostOf(table, client.PID)
				c.placed.process = liveProcess{PID: client.PID, ProcStart: client.Started}
				if isTabApp(c.placed.host.App) {
					c.Open.Label = "Switch to " + c.placed.host.App
				}
			}
			return false
		}
		process = liveProcess{PID: pid}
	}
	return placeAt(c, process, table, args)
}

func codexClientTTYs(clients []ProcessInfo) []string {
	if len(clients) == 0 {
		return nil
	}
	ttys := make([]string, 0, len(clients))
	for _, client := range clients {
		ttys = append(ttys, client.TTY)
	}
	sort.Strings(ttys)
	return ttys
}

// placeAt sets where c shows: the terminal of process, when it has one.
// ok is true when that is a tab of an app in tabApps.
func placeAt(c *Conversation, process liveProcess, table []ProcessInfo, args func(pids []int) map[int]string) (ok bool) {
	host, _ := hostOf(table, process.PID)
	if host.TTY == "" {
		return false
	}
	c.placed = &placement{process: process, host: host}
	if c.Tool == ToolClaude && host.App == "" && claudeBackgroundSession(table, process.PID, args) {
		c.placed.claudeBackground = true
	}
	if !isTabApp(host.App) {
		return false
	}
	c.Open.Label = "Switch to " + host.App
	return true
}

// tabsFile keeps the terminal app and tty each session last ran in,
// so an ended session resumes in a new tab of that app.
type tabsFile struct {
	Version int                      `json:"version"`
	Tabs    map[string]rememberedTab `json:"tabs"`
}

type rememberedTab struct {
	Host terminalHost `json:"host"`
	At   string       `json:"at"`
}

// tabsKept is how long a remembered tab is kept after its session was
// last seen running.
const tabsKept = 30 * 24 * time.Hour

// tabsSeenRefresh is how old a remembered tab's time may grow while its
// session still runs, so pruning counts from when it was last seen.
const tabsSeenRefresh = 24 * time.Hour

func (f *tabsFile) remember(key string, host terminalHost, now time.Time) bool {
	if kept, ok := f.Tabs[key]; ok && kept.Host == host && now.Sub(parseStamp(kept.At)) < tabsSeenRefresh {
		return false
	}
	f.Tabs[key] = rememberedTab{Host: host, At: stamp(now)}
	for other, tab := range f.Tabs {
		if now.Sub(parseStamp(tab.At)) > tabsKept {
			delete(f.Tabs, other)
		}
	}
	return true
}

func (f *tabsFile) rememberAll(seen map[string]terminalHost, now time.Time) bool {
	changed := false
	for key, host := range seen {
		if f.remember(key, host, now) {
			changed = true
		}
	}
	return changed
}

// tabsPath sits beside the marks file and outside Stamp: it changes
// when a scan runs, not when a conversation does.
func (s *Scanner) tabsPath() string {
	if s.MarksPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(s.MarksPath), "conversation-tabs.json")
}

func (s *Scanner) rememberedTabs() tabsFile {
	return readTabs(s.tabsPath())
}

func readTabs(path string) tabsFile {
	file := tabsFile{Version: 1, Tabs: map[string]rememberedTab{}}
	if path == "" {
		return file
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return file
	}
	var read tabsFile
	if json.Unmarshal(data, &read) != nil || read.Tabs == nil {
		return file
	}
	return read
}

// saveTabs records seen in the file under the lock every server shares,
// so one server's tabs do not discard another's, and returns the file
// as written. A tab that cannot be remembered only makes an ended
// session resume in a new tab.
func (s *Scanner) saveTabs(seen map[string]terminalHost) tabsFile {
	path := s.tabsPath()
	if path == "" {
		return readTabs("")
	}
	release, err := lockMarks(path)
	if err != nil {
		return readTabs(path)
	}
	defer release()
	file := readTabs(path)
	if file.rememberAll(seen, s.Now()) {
		_ = writeJSONFile(path, file)
	}
	return file
}

// writeJSONFile replaces path in one step, so a reader never sees half a file.
func writeJSONFile(path string, value any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("%s: %w", dir, err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return atomicfile.Replace(path, append(data, '\n'), 0o600)
}
