package conversations

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	// Leader is the tty's first process under the app, its login or
	// shell. A tab opened later on the same tty has another.
	Leader ProcessRef `json:"leader"`
}

// terminalApps maps a fragment of an ancestor's command path to the app
// that shows the terminal.
var terminalApps = []struct{ fragment, app string }{
	{"/Terminal.app/", "Terminal"},
	{"/iTerm.app/", "iTerm"},
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
				host.Leader = ProcessRef{PID: child.PID, ProcStart: child.Started}
				return host, true
			}
		}
		child = parent
	}
	return host, false
}

// shells are the programs a tab at its prompt runs in the foreground.
var shells = map[string]bool{"zsh": true, "bash": true, "fish": true, "sh": true, "dash": true, "ksh": true, "tcsh": true, "nu": true}

// tabIdle reports whether tab still shows the login the session ran
// under and sits at a shell prompt: only a shell in the foreground.
func tabIdle(table []ProcessInfo, tab terminalHost) bool {
	leader, foreground := false, false
	for _, process := range table {
		if process.TTY != tab.TTY {
			continue
		}
		if process.PID == tab.Leader.PID && process.Started == tab.Leader.ProcStart {
			leader = true
		}
		if strings.Contains(process.Stat, "+") {
			if !shells[strings.TrimPrefix(filepath.Base(process.Command), "-")] {
				return false
			}
			foreground = true
		}
	}
	return leader && foreground
}

// codexClientLead is how long before its thread a Codex client may
// start: the terminal client creates its first thread as it starts.
const codexClientLead = 15 * time.Second

// pickCodexClient finds the terminal client that shows a thread the
// Codex app-server daemon holds: the client started with the thread's
// id, else the only one whose first thread after it started, within
// codexClientLead, is this one, else the only client in the thread's
// folder that started no thread. threads holds when each Codex thread
// was created. Codex does not record which client shows a thread, and
// its daemon keeps a thread after the client quits, so with none of
// these pid is 0 and ttys lists the clients that could still be it.
func pickCodexClient(c Conversation, threads []string, table []ProcessInfo, processes Processes) (pid int, ttys []string) {
	var candidates []ProcessInfo
	for _, process := range table {
		if process.TTY != "" && filepath.Base(process.Command) == "codex" {
			candidates = append(candidates, process)
		}
	}
	if len(candidates) == 0 {
		return 0, nil
	}
	pids := make([]int, len(candidates))
	for i, candidate := range candidates {
		pids[i] = candidate.PID
	}
	args := processes.Args(pids)
	var clients []ProcessInfo
	for _, candidate := range candidates {
		if strings.Contains(args[candidate.PID], "app-server") {
			continue
		}
		if strings.Contains(args[candidate.PID], c.ID) {
			return candidate.PID, nil
		}
		clients = append(clients, candidate)
	}
	if only, ok := onlyOne(clients, func(client ProcessInfo) bool { return firstThreadAfter(client, threads) == c.StartedAt }); ok && c.StartedAt != "" {
		return only.PID, nil
	}
	// A client whose first thread is another one shows that thread.
	var free []ProcessInfo
	for _, client := range clients {
		if firstThreadAfter(client, threads) == "" {
			free = append(free, client)
		}
	}
	cwds := processes.Cwds(pids)
	inFolder := func(client ProcessInfo) bool { return cwds[client.PID] == c.Folder }
	if only, ok := onlyOne(free, inFolder); ok {
		return only.PID, nil
	}
	for _, client := range free {
		if inFolder(client) || len(cwds) == 0 {
			ttys = append(ttys, client.TTY)
		}
	}
	sort.Strings(ttys)
	return 0, ttys
}

// firstThreadAfter is the creation time of the first thread created as
// client started or within codexClientLead after, or "" for none.
func firstThreadAfter(client ProcessInfo, threads []string) string {
	started, err := time.Parse(lstartLayout, client.Started)
	if err != nil {
		return ""
	}
	first, firstAt := "", time.Time{}
	for _, thread := range threads {
		created, err := time.Parse(time.RFC3339, thread)
		if err != nil || created.Before(started) || created.Sub(started) > codexClientLead {
			continue
		}
		if first == "" || created.Before(firstAt) {
			first, firstAt = thread, created
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
	for r := range results {
		for i := range results[r].conversations {
			if c := &results[r].conversations[i]; c.Open.Kind == RouteFocus && c.process != nil {
				live = append(live, c)
			}
		}
	}
	var threads []string
	for _, result := range results {
		for _, c := range result.conversations {
			if c.Tool == ToolCodex && c.StartedAt != "" {
				threads = append(threads, c.StartedAt)
			}
		}
	}
	tabs := s.rememberedTabs()
	if len(live) > 0 {
		if table, err := s.Processes.Table(); err == nil {
			changed := false
			for _, c := range live {
				if place(c, threads, table, s.Processes) && c.placed.host.App != "" && tabs.remember(markKey(c.Tool, c.ID), c.placed.host, s.Now()) {
					changed = true
				}
			}
			if changed {
				s.saveTabs(tabs)
			}
		}
	}
	for r := range results {
		for i := range results[r].conversations {
			c := &results[r].conversations[i]
			if c.Open.Kind != RouteTerminal || c.App != AppCLI {
				continue
			}
			if tab, ok := tabs.Tabs[markKey(c.Tool, c.ID)]; ok && tab.Host.App != "" {
				host := tab.Host
				c.lastTab = &host
				c.Open.Label = "Open in " + host.App
			}
		}
	}
}

// place sets where c shows; false when its terminal was not found.
func place(c *Conversation, threads []string, table []ProcessInfo, processes Processes) bool {
	process := *c.process
	host, _ := hostOf(table, process.PID)
	if host.TTY == "" && c.Tool == ToolCodex {
		pid, ttys := pickCodexClient(*c, threads, table, processes)
		if pid == 0 {
			if len(ttys) > 1 {
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
	if host.App != "" {
		c.Open.Label = "Switch to " + host.App
	}
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

func (f *tabsFile) remember(key string, host terminalHost, now time.Time) bool {
	if f.Tabs[key].Host == host {
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

// tabsPath sits beside the marks file and outside Stamp: it changes
// when a scan runs, not when a conversation does.
func (s *Scanner) tabsPath() string {
	if s.MarksPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(s.MarksPath), "conversation-tabs.json")
}

func (s *Scanner) rememberedTabs() tabsFile {
	file := tabsFile{Version: 1, Tabs: map[string]rememberedTab{}}
	path := s.tabsPath()
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

// saveTabs writes the file; a tab that cannot be remembered only makes
// an ended session resume in a new tab.
func (s *Scanner) saveTabs(file tabsFile) {
	path := s.tabsPath()
	if path == "" {
		return
	}
	_ = writeJSONFile(path, file)
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
