package conversations

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ProcessInfo is one row of the process table.
type ProcessInfo struct {
	PID, PPID int
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

// parseProcessTable reads `ps -axo pid=,ppid=,stat=,tty=,lstart=,comm=`.
// lstart is five fields, and comm, last, may hold spaces.
func parseProcessTable(out string) []ProcessInfo {
	var table []ProcessInfo
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		pid, pidErr := strconv.Atoi(fields[0])
		ppid, ppidErr := strconv.Atoi(fields[1])
		if pidErr != nil || ppidErr != nil {
			continue
		}
		rest := line
		for range 4 {
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
		tty := fields[3]
		if tty == "??" || tty == "?" || tty == "-" {
			tty = ""
		} else {
			tty = "/dev/" + strings.TrimPrefix(tty, "/dev/")
		}
		table = append(table, ProcessInfo{PID: pid, PPID: ppid, Stat: fields[2], TTY: tty, Started: started, Command: strings.TrimSpace(rest[len(lstartLayout):])})
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
}

// terminalApps maps a fragment of an ancestor's command path to the app
// that shows the terminal.
var terminalApps = []struct{ fragment, app string }{
	{"/Terminal.app/", "Terminal"},
	{"/iTerm.app/", "iTerm"},
	// iTerm2 keeps its shells under a session server outside the app.
	{"/iTerm2/iTermServer-", "iTerm"},
	{"/Ghostty.app/", "Ghostty"},
	{"/Warp.app/", "Warp"},
	{"/Cursor.app/", "Cursor"},
	{"/Visual Studio Code.app/", "VS Code"},
	{"/Zed.app/", "Zed"},
}

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
		child = parent
	}
	return host, false
}

// codexClientLead is how long before its thread a Codex client may
// start: the terminal client creates its first thread as it starts.
const codexClientLead = 15 * time.Second

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
	args      map[int]string
	clients   []ProcessInfo
	cwds      map[int]string
}

// threadStart is when and where a Codex CLI thread was created.
type threadStart struct{ created, folder string }

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
	k.args = k.processes.Args(k.pids)
	for _, candidate := range candidates {
		if !strings.Contains(k.args[candidate.PID], "app-server") {
			k.clients = append(k.clients, candidate)
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

// pick returns the client that shows c: the one started with its id,
// else the only one whose first thread in its folder after it started
// is c. A client started to resume a thread shows that thread, so only
// the id can pick it. With neither pid is 0 and ttys lists the clients
// in c's folder that started no thread of their own, which could still
// be it. Being in the folder is never enough to pick one: a client
// opened later in the folder shows an empty tab, not a thread the
// daemon kept.
func (k *codexClients) pick(c Conversation) (pid int, ttys []string) {
	k.load()
	var fresh []ProcessInfo
	for _, client := range k.clients {
		if strings.Contains(k.args[client.PID], c.ID) {
			return client.PID, nil
		}
		if !resumesThread(k.args[client.PID]) {
			fresh = append(fresh, client)
		}
	}
	if len(fresh) == 0 {
		return 0, nil
	}
	cwds := k.folders()
	first := func(client ProcessInfo) string { return firstThreadAfter(client, cwds[client.PID], k.threads) }
	if only, ok := onlyOne(fresh, func(client ProcessInfo) bool { return first(client) == c.StartedAt }); ok && c.StartedAt != "" {
		return only.PID, nil
	}
	for _, client := range fresh {
		if first(client) == "" && cwds[client.PID] == c.Folder {
			ttys = append(ttys, client.TTY)
		}
	}
	sort.Strings(ttys)
	return 0, ttys
}

var threadIDPattern = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// resumesThread reports whether a client's command line binds it to a
// thread that already exists, by its id or with `codex resume`.
func resumesThread(args string) bool {
	return threadIDPattern.MatchString(args) || slices.Contains(strings.Fields(args), "resume")
}

// firstThreadAfter is the creation time of the first thread created in
// folder as client started or within codexClientLead after, or "" for
// none. An unknown folder counts every thread.
func firstThreadAfter(client ProcessInfo, folder string, threads []threadStart) string {
	started, err := time.Parse(lstartLayout, client.Started)
	if err != nil {
		return ""
	}
	first, firstAt := "", time.Time{}
	for _, thread := range threads {
		if folder != "" && thread.folder != folder {
			continue
		}
		created, err := time.Parse(time.RFC3339, thread.created)
		if err != nil || created.Before(started) || created.Sub(started) > codexClientLead {
			continue
		}
		if first == "" || created.Before(firstAt) {
			first, firstAt = thread.created, created
		}
	}
	return first
}

func onlyOne(processes []ProcessInfo, matches func(ProcessInfo) bool) (ProcessInfo, bool) {
	var found []ProcessInfo
	for _, process := range processes {
		if matches(process) {
			found = append(found, process)
		}
	}
	if len(found) != 1 {
		return ProcessInfo{}, false
	}
	return found[0], true
}

// placement is where a live session shows: the process in its terminal
// and that terminal's host, or the ttys that could each be it.
type placement struct {
	process liveProcess
	host    terminalHost
	ttys    []string
}

// placeLive finds the terminal of each session a process runs, with one
// look at the process table, and remembers it so that an ended session
// resumes in the tab it ran in. A Codex thread the app-server daemon
// holds shows in a terminal client, which is looked for.
func (s *Scanner) placeLive(results []sourceResult) {
	var live []*Conversation
	var threads []threadStart
	for r := range results {
		for i := range results[r].conversations {
			c := &results[r].conversations[i]
			if c.Open.Kind == RouteFocus && c.process != nil {
				live = append(live, c)
			}
			if c.Tool == ToolCodex && c.App == AppCLI && c.StartedAt != "" {
				threads = append(threads, threadStart{created: c.StartedAt, folder: c.Folder})
			}
		}
	}
	tabs := s.rememberedTabs()
	if len(live) > 0 {
		if table, err := s.Processes.Table(); err == nil {
			seen := map[string]terminalHost{}
			clients := newCodexClients(table, threads, s.Processes)
			for _, c := range live {
				if place(c, clients, table) {
					seen[markKey(c.Tool, c.ID)] = c.placed.host
				}
			}
			if changed := tabs.rememberAll(seen, s.Now()); changed {
				tabs = s.saveTabs(seen)
			}
		}
	}
	for r := range results {
		for i := range results[r].conversations {
			c := &results[r].conversations[i]
			if c.Open.Kind != RouteTerminal || c.App != AppCLI {
				continue
			}
			if tab, ok := tabs.Tabs[markKey(c.Tool, c.ID)]; ok && isTabApp(tab.Host.App) {
				host := tab.Host
				c.lastTab = &host
				c.Open.Label = "Open in " + host.App
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
func place(c *Conversation, clients *codexClients, table []ProcessInfo) (ok bool) {
	process := *c.process
	host, _ := hostOf(table, process.PID)
	if host.TTY == "" && c.Tool == ToolCodex {
		pid, ttys := clients.pick(*c)
		if pid == 0 {
			if len(ttys) > 0 {
				c.placed = &placement{ttys: ttys}
			}
			return false
		}
		process = liveProcess{PID: pid}
		host, _ = hostOf(table, pid)
	}
	if host.TTY == "" {
		return false
	}
	c.placed = &placement{process: process, host: host}
	if !isTabApp(host.App) {
		return false
	}
	c.Open.Label = "Switch to " + host.App
	return true
}

// tabsFile keeps the terminal tab each session last ran in.
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
	temp, err := os.CreateTemp(dir, filepath.Base(path)+"-*")
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	defer func() { _ = os.Remove(temp.Name()) }()
	if _, err := temp.Write(append(data, '\n')); err != nil {
		_ = temp.Close()
		return fmt.Errorf("%s: %w", temp.Name(), err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("%s: %w", temp.Name(), err)
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
