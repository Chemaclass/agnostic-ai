package conversations

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

const ghosttyApp = "/Applications/Ghostty.app/Contents/MacOS/ghostty"

// ghosttyTab is a Ghostty tab on tty whose login is loginPID and whose
// shell runs command as pid.
func ghosttyTab(tty string, loginPID, pid int, command string) []ProcessInfo {
	return []ProcessInfo{
		{PID: loginPID, PPID: 761, Stat: "Ss", TTY: tty, Started: "Tue Nov 10 21:49:14 2026", Command: "/usr/bin/login"},
		{PID: loginPID + 1, PPID: loginPID, Stat: "S", TTY: tty, Started: "Tue Nov 10 21:49:14 2026", Command: "-zsh"},
		{PID: pid, PPID: loginPID + 1, Stat: "S+", TTY: tty, Started: "Tue Nov 10 21:49:17 2026", Command: command},
	}
}

func ghosttyTable(tabs ...[]ProcessInfo) []ProcessInfo {
	table := []ProcessInfo{{PID: 761, PPID: 1, Stat: "S", Started: "Sat Nov  7 15:23:53 2026", Command: ghosttyApp}}
	for _, tab := range tabs {
		table = append(table, tab...)
	}
	return table
}

func TestParseProcessTable_KeepsTheStartTimeAndACommandWithSpaces(t *testing.T) {
	table := parseProcessTable("  761     1 S      ??       Sat Nov  7 15:23:53 2026     /Applications/Ghostty.app/Contents/MacOS/ghostty\n" +
		"56521 56407 S+     ttys000  Tue Nov 10 20:15:20 2026     /Applications/Cursor.app/Contents/Frameworks/Cursor Helper (Plugin).app/Contents/MacOS/Cursor Helper (Plugin)\n" +
		"not a process line\n")

	if len(table) != 2 {
		t.Fatalf("table = %+v", table)
	}
	if app := table[0]; app.PID != 761 || app.PPID != 1 || app.TTY != "" || app.Started != "Sat Nov  7 15:23:53 2026" || app.Command != ghosttyApp {
		t.Errorf("app row = %+v", app)
	}
	if helper := table[1]; helper.TTY != "/dev/ttys000" || helper.Stat != "S+" || !strings.HasSuffix(helper.Command, "Cursor Helper (Plugin)") {
		t.Errorf("helper row = %+v", helper)
	}
}

func TestHostOf_FindsTheAppAndTheTTY(t *testing.T) {
	cursor := "/Applications/Cursor.app/Contents/Frameworks/Cursor Helper.app/Contents/MacOS/Cursor Helper"
	table := ghosttyTable(ghosttyTab("/dev/ttys004", 45469, 45864, "claude"))
	table = append(table,
		ProcessInfo{PID: 900, PPID: 1, Stat: "S", Command: cursor},
		ProcessInfo{PID: 901, PPID: 900, Stat: "Ss", TTY: "/dev/ttys020", Started: "Tue Nov 10 10:00:00 2026", Command: "/bin/zsh"},
		ProcessInfo{PID: 902, PPID: 901, Stat: "S+", TTY: "/dev/ttys020", Command: "claude"},
		ProcessInfo{PID: 6923, PPID: 1, Stat: "S", Command: "/Users/you/.codex/packages/app-server-daemon/bin/codex"},
		ProcessInfo{PID: 50, PPID: 51, Stat: "S", TTY: "/dev/ttys030", Command: "claude"},
		ProcessInfo{PID: 51, PPID: 50, Stat: "S", TTY: "/dev/ttys030", Command: "zsh"},
	)

	if host, ok := hostOf(table, 45864); !ok || host != (terminalHost{App: "Ghostty", TTY: "/dev/ttys004"}) {
		t.Errorf("Ghostty tab: %+v, %v", host, ok)
	}
	if host, ok := hostOf(table, 902); !ok || host.App != "Cursor" || host.TTY != "/dev/ttys020" {
		t.Errorf("an editor's terminal: %+v, %v", host, ok)
	}
	iterm := []ProcessInfo{
		{PID: 700, PPID: 1, Stat: "S", Command: "/Users/you/Library/Application Support/iTerm2/iTermServer-3.5.4"},
		{PID: 701, PPID: 700, Stat: "Ss", TTY: "/dev/ttys040", Started: "Tue Nov 10 09:00:00 2026", Command: "/usr/bin/login"},
		{PID: 702, PPID: 701, Stat: "S", TTY: "/dev/ttys040", Command: "-zsh"},
		{PID: 703, PPID: 702, Stat: "S+", TTY: "/dev/ttys040", Command: "claude"},
	}
	if host, ok := hostOf(append(table, iterm...), 703); !ok || host != (terminalHost{App: "iTerm", TTY: "/dev/ttys040"}) {
		t.Errorf("an iTerm2 shell under its session server: %+v, %v", host, ok)
	}
	if host, ok := hostOf(table, 6923); ok {
		t.Errorf("a daemon without a terminal has no host, got %+v", host)
	}
	if host, ok := hostOf(table, 50); ok || host.TTY != "/dev/ttys030" {
		t.Errorf("a loop of parents stops with the tty and no app, got %+v, %v", host, ok)
	}
}

func pick(c Conversation, threads []threadStart, table []ProcessInfo, processes Processes) (int, []string) {
	return newCodexClients(table, threads, processes).pick(c)
}

// threadsInRepo is threads created in /repo at each time.
func threadsInRepo(created ...string) []threadStart {
	threads := make([]threadStart, len(created))
	for i, at := range created {
		threads[i] = threadStart{created: at, folder: "/repo"}
	}
	return threads
}

func TestPickCodexClient_ByIDThenStartNeverByFolder(t *testing.T) {
	thread := Conversation{Tool: ToolCodex, ID: "01a1c738-eda0-7be0-b151-023a3e0ab7ee", Folder: "/repo", StartedAt: "2026-11-10T21:49:19Z"}
	first := ghosttyTab("/dev/ttys002", 16953, 17199, "codex")
	second := ghosttyTab("/dev/ttys005", 46075, 46288, "codex")
	second[2].Started = "Tue Nov 10 22:33:41 2026"
	table := ghosttyTable(first, second)
	both := processesWithTable(table)

	if pid, ttys := pick(thread, threadsInRepo(thread.StartedAt), table, both); pid != 17199 || ttys != nil {
		t.Errorf("the client that started just before the thread: %d, %v", pid, ttys)
	}
	later := thread
	later.StartedAt = "2026-11-10T23:00:00Z"
	inRepo := both
	inRepo.cwds = map[int]string{17199: "/repo", 46288: "/repo"}
	if pid, ttys := pick(later, threadsInRepo(later.StartedAt), table, inRepo); pid != 0 || strings.Join(ttys, ",") != "/dev/ttys002,/dev/ttys005" {
		t.Errorf("two clients in the folder and neither started with the thread: %d, %v", pid, ttys)
	}
	resumed := both
	resumed.args = map[int]string{46288: "codex resume " + thread.ID}
	if pid, _ := pick(later, threadsInRepo(later.StartedAt), table, resumed); pid != 46288 {
		t.Errorf("a client started with the thread's id holds it: %d", pid)
	}
	elsewhere := both
	elsewhere.cwds = map[int]string{17199: "/repo", 46288: "/other"}
	if pid, ttys := pick(later, threadsInRepo(later.StartedAt), table, elsewhere); pid != 0 || strings.Join(ttys, ",") != "/dev/ttys002" {
		t.Errorf("the only client in the folder may show an empty tab, so it is named, never picked: %d, %v", pid, ttys)
	}
}

// countingProcesses counts how often the costly lookups run.
type countingProcesses struct {
	fakeProcesses
	argsCalls, cwdsCalls int
}

func (c *countingProcesses) Args(pids []int) map[int]string {
	c.argsCalls++
	return c.fakeProcesses.Args(pids)
}

func (c *countingProcesses) Cwds(pids []int) map[int]string {
	c.cwdsCalls++
	return c.fakeProcesses.Cwds(pids)
}

func TestCodexClients_ReadArgsAndFoldersOnce(t *testing.T) {
	first := ghosttyTab("/dev/ttys002", 79980, 80312, "codex")
	first[2].Started = "Tue Nov 10 22:55:27 2026"
	second := ghosttyTab("/dev/ttys004", 81131, 81453, "codex")
	second[2].Started = "Tue Nov 10 22:55:32 2026"
	table := ghosttyTable(first, second)
	created := []string{"2026-11-10T22:55:27Z", "2026-11-10T22:55:33Z", "2026-11-10T23:30:00Z"}
	processes := &countingProcesses{fakeProcesses: processesWithTable(table)}
	clients := newCodexClients(table, threadsInRepo(created...), processes)

	for _, created := range created {
		clients.pick(Conversation{Tool: ToolCodex, ID: "01a1c775-911c-7893-bd08-5f77cc835aa3", Folder: "/repo", StartedAt: created})
	}

	if processes.argsCalls != 1 || processes.cwdsCalls != 1 {
		t.Errorf("args read %d times, folders %d times; want 1 and 1", processes.argsCalls, processes.cwdsCalls)
	}
}

func TestPickCodexClient_EachClientHoldsTheFirstThreadAfterItStarted(t *testing.T) {
	first := ghosttyTab("/dev/ttys002", 79980, 80312, "codex")
	first[2].Started = "Tue Nov 10 22:55:27 2026"
	second := ghosttyTab("/dev/ttys004", 81131, 81453, "codex")
	second[2].Started = "Tue Nov 10 22:55:32 2026"
	table := ghosttyTable(first, second)
	created := []string{"2026-11-10T22:55:27Z", "2026-11-10T22:55:33Z", "2026-11-10T22:58:00Z"}
	for at, want := range map[string]int{created[0]: 80312, created[1]: 81453, created[2]: 0} {
		thread := Conversation{Tool: ToolCodex, ID: "01a1c775-911c-7893-bd08-5f77cc835aa3", Folder: "/repo", StartedAt: at}
		if pid, _ := pick(thread, threadsInRepo(created...), table, processesWithTable(table)); pid != want {
			t.Errorf("thread created %s: client %d, want %d", at, pid, want)
		}
	}
}

func TestPickCodexClient_AClientShowingAnotherThreadIsNotAGuess(t *testing.T) {
	client := ghosttyTab("/dev/ttys002", 79980, 80312, "codex")
	client[2].Started = "Tue Nov 10 22:55:27 2026"
	table := ghosttyTable(client)
	processes := processesWithTable(table)
	processes.cwds = map[int]string{80312: "/repo"}
	threads := threadsInRepo("2026-11-10T22:55:27Z", "2026-11-10T22:55:33Z")
	quit := Conversation{Tool: ToolCodex, ID: "01a1c775-911c-7893-bd08-5f77cc835aa3", Folder: "/repo", StartedAt: threads[1].created}

	if pid, ttys := pick(quit, threads, table, processes); pid != 0 || ttys != nil {
		t.Errorf("the client in the folder shows its own first thread: %d, %v", pid, ttys)
	}
}

func TestPickCodexClient_AClientResumingAnotherThreadIsNeverTimed(t *testing.T) {
	client := ghosttyTab("/dev/ttys002", 79980, 80312, "codex")
	client[2].Started = "Tue Nov 10 22:55:27 2026"
	table := ghosttyTable(client)
	processes := processesWithTable(table)
	processes.cwds = map[int]string{80312: "/repo"}
	created := Conversation{Tool: ToolCodex, ID: "01a1c775-911c-7893-bd08-5f77cc835aa3", Folder: "/repo", StartedAt: "2026-11-10T22:55:33Z"}
	threads := []threadStart{{created: created.StartedAt, folder: "/repo"}}

	for _, args := range []string{"codex resume 01a1c738-eda0-7be0-b151-023a3e0ab7ee", "codex resume --last", "codex resume", "codex -m gpt-5 resume nightly"} {
		processes.args = map[int]string{80312: args}
		if pid, ttys := pick(created, threads, table, processes); pid != 0 || ttys != nil {
			t.Errorf("%q shows the thread it resumed, not one created after it started: %d, %v", args, pid, ttys)
		}
	}
}

func TestPickCodexClient_ANewThreadIsTimedWhateverItsArgumentsSay(t *testing.T) {
	client := ghosttyTab("/dev/ttys002", 79980, 80312, "/opt/homebrew/bin/codex")
	client[2].Started = "Tue Nov 10 22:55:27 2026"
	table := ghosttyTable(client)
	processes := processesWithTable(table)
	processes.cwds = map[int]string{80312: "/repo"}
	created := Conversation{Tool: ToolCodex, ID: "01a1c775-911c-7893-bd08-5f77cc835aa3", Folder: "/repo", StartedAt: "2026-11-10T22:55:33Z"}
	threads := []threadStart{{created: created.StartedAt, folder: "/repo"}}

	for _, args := range []string{
		"/opt/homebrew/bin/codex resume the migration where we left it",
		"/opt/homebrew/bin/codex -m gpt-5 please resume",
		"/opt/homebrew/bin/codex -c model=o3 fix the 01a1c738-eda0-7be0-b151-023a3e0ab7ee bug",
		"/opt/homebrew/bin/codex fork 01a1c738-eda0-7be0-b151-023a3e0ab7ee",
	} {
		processes.args = map[int]string{80312: args}
		if pid, _ := pick(created, threads, table, processes); pid != 80312 {
			t.Errorf("%q starts a new thread, so it is timed: %d", args, pid)
		}
	}
	forked := Conversation{Tool: ToolCodex, ID: "01a1c738-eda0-7be0-b151-023a3e0ab7ee", Folder: "/repo", StartedAt: "2026-11-10T20:00:00Z"}
	processes.args = map[int]string{80312: "/opt/homebrew/bin/codex fork " + forked.ID}
	if pid, _ := pick(forked, threads, table, processes); pid != 0 {
		t.Errorf("a fork shows its new thread, not the one it forked: %d", pid)
	}
}

func TestPickCodexClient_AClientResumingWithoutAnIDMayShowAnOlderThread(t *testing.T) {
	client := ghosttyTab("/dev/ttys002", 79980, 80312, "codex")
	client[2].Started = "Tue Nov 10 22:55:27 2026"
	table := ghosttyTable(client)
	processes := processesWithTable(table)
	processes.cwds = map[int]string{80312: "/repo"}
	older := Conversation{Tool: ToolCodex, ID: "01a1c738-eda0-7be0-b151-023a3e0ab7ee", Folder: "/repo", StartedAt: "2026-11-10T20:00:00Z"}
	threads := []threadStart{{created: older.StartedAt, folder: "/repo"}}

	for _, args := range []string{"codex resume --last", "codex resume", "codex resume nightly"} {
		processes.args = map[int]string{80312: args}
		if pid, ttys := pick(older, threads, table, processes); pid != 0 || strings.Join(ttys, ",") != "/dev/ttys002" {
			t.Errorf("%q may show the older thread in its folder, so it is named: %d, %v", args, pid, ttys)
		}
	}
	processes.args = map[int]string{80312: "codex resume 01a1c775-911c-7893-bd08-5f77cc835aa3"}
	if pid, ttys := pick(older, threads, table, processes); pid != 0 || ttys != nil {
		t.Errorf("a client resuming another id is not named: %d, %v", pid, ttys)
	}
}

func TestPickCodexClient_AClientStartedInAnotherFolderHasNoKnownFolder(t *testing.T) {
	client := ghosttyTab("/dev/ttys002", 79980, 80312, "codex")
	client[2].Started = "Tue Nov 10 22:55:27 2026"
	table := ghosttyTable(client)
	processes := processesWithTable(table)
	processes.cwds = map[int]string{80312: "/home"}
	created := Conversation{Tool: ToolCodex, ID: "01a1c775-911c-7893-bd08-5f77cc835aa3", Folder: "/repo", StartedAt: "2026-11-10T22:55:33Z"}
	threads := []threadStart{{created: created.StartedAt, folder: "/repo"}}

	for _, args := range []string{"codex -C /repo", "codex --cd /repo", "codex --cd=/repo"} {
		processes.args = map[int]string{80312: args}
		if pid, _ := pick(created, threads, table, processes); pid != 80312 {
			t.Errorf("%q works in another folder than its process: %d", args, pid)
		}
	}
}

func TestPickCodexClient_AThreadInAnotherFolderIsNotTheClientsFirst(t *testing.T) {
	client := ghosttyTab("/dev/ttys002", 79980, 80312, "codex")
	client[2].Started = "Tue Nov 10 22:55:27 2026"
	table := ghosttyTable(client)
	processes := processesWithTable(table)
	processes.cwds = map[int]string{80312: "/other"}
	created := Conversation{Tool: ToolCodex, ID: "01a1c775-911c-7893-bd08-5f77cc835aa3", Folder: "/repo", StartedAt: "2026-11-10T22:55:33Z"}
	threads := []threadStart{{created: created.StartedAt, folder: "/repo"}}

	if pid, ttys := pick(created, threads, table, processes); pid != 0 || ttys != nil {
		t.Errorf("a client in another folder did not create the thread: %d, %v", pid, ttys)
	}
}

func processesWithTable(table []ProcessInfo) fakeProcesses {
	alive := map[int]bool{}
	for _, process := range table {
		alive[process.PID] = true
	}
	return fakeProcesses{alive: alive, table: table}
}

func TestScan_CodexThreadHeldByTheDaemonSwitchesToItsTerminalClient(t *testing.T) {
	thread := "01a1c738-eda0-7be0-b151-023a3e0ab7ee"
	scanner, stores := testScanner(t, fakeStores(t, map[string]string{
		"state_5.sqlite|FROM threads": `[{"id":"` + thread + `","cwd":"/repo","created_at_ms":1762811359000,"updated_at_ms":1762811587000,"originator":"codex-tui","source":"vscode","name":"Review"}]`,
	}), nil)
	table := ghosttyTable(ghosttyTab("/dev/ttys002", 16953, 17199, "codex"))
	table[3].Started = "Mon Nov 10 21:49:17 2025"
	table = append(table, ProcessInfo{PID: 6923, PPID: 1, Stat: "S", Started: "Sun Nov  8 10:58:14 2025", Command: "/Users/you/.codex/packages/app-server-daemon/bin/codex"})
	processes := processesWithTable(table)
	processes.held = map[string]int{thread + ".lock": 6923}
	scanner.Processes = processes
	scanner.Now = func() time.Time { return time.UnixMilli(1762811600000) }
	write(t, filepath.Join(stores.CodexDir, "state_5.sqlite"), "")
	write(t, filepath.Join(stores.CodexDir, "thread-writer-locks", thread+".lock"), "")

	c, found := scanner.Find(context.Background(), ToolCodex, thread, ScanOptions{})
	if !found || c.Open.Kind != RouteFocus || c.Open.Label != "Switch to Ghostty" {
		t.Fatalf("the open thread switches to Ghostty: %s", asJSON(t, c))
	}
	launcher := &fakeLauncher{focusOK: true}
	reply := Open(context.Background(), c, scanner.Processes, launcher)

	if strings.Join(launcher.calls, "; ") != "focus Ghostty /dev/ttys002" || reply != (OpenReply{Opened: true, Message: "Switched to the Ghostty tab that runs it."}) {
		t.Errorf("calls %v, reply %+v", launcher.calls, reply)
	}
}

func TestScan_TwoCodexClientsInOneFolderEachSwitchToTheirOwnTab(t *testing.T) {
	first, second, quit := "01a1c775-7c2b-7011-a261-3a61edc96e71", "01a1c775-911c-7893-bd08-5f77cc835aa3", "01a1c775-aaaa-7893-bd08-5f77cc835aa3"
	scanner, stores := testScanner(t, fakeStores(t, map[string]string{
		"state_5.sqlite|FROM threads": `[
			{"id":"` + first + `","cwd":"/repo","created_at_ms":1762815327400,"updated_at_ms":1762815400000,"originator":"codex-tui","source":"vscode","name":"First"},
			{"id":"` + second + `","cwd":"/repo","created_at_ms":1762815333100,"updated_at_ms":1762815400000,"originator":"codex-tui","source":"vscode","name":"Second"},
			{"id":"` + quit + `","cwd":"/repo","created_at_ms":1762815000000,"updated_at_ms":1762815400000,"originator":"codex-tui","source":"vscode","name":"Quit"}
		]`,
	}), nil)
	a := ghosttyTab("/dev/ttys002", 79973, 80312, "codex")
	a[2].Started = "Mon Nov 10 22:55:27 2025"
	b := ghosttyTab("/dev/ttys004", 81125, 81453, "codex")
	b[2].Started = "Mon Nov 10 22:55:32 2025"
	table := append(ghosttyTable(a, b), ProcessInfo{PID: 6923, PPID: 1, Stat: "S", Started: "Sun Nov  8 10:58:14 2025", Command: "/Users/you/.codex/packages/app-server-daemon/bin/codex"})
	processes := processesWithTable(table)
	processes.held = map[string]int{first + ".lock": 6923, second + ".lock": 6923, quit + ".lock": 6923}
	processes.cwds = map[int]string{80312: "/repo", 81453: "/repo"}
	scanner.Processes = processes
	scanner.Now = func() time.Time { return time.UnixMilli(1762815500000) }
	write(t, filepath.Join(stores.CodexDir, "state_5.sqlite"), "")
	for _, id := range []string{first, second, quit} {
		write(t, filepath.Join(stores.CodexDir, "thread-writer-locks", id+".lock"), "")
	}

	for id, want := range map[string]string{first: "focus Ghostty /dev/ttys002", second: "focus Ghostty /dev/ttys004", quit: ""} {
		c, _ := scanner.Find(context.Background(), ToolCodex, id, ScanOptions{})
		launcher := &fakeLauncher{focusOK: true}
		reply := Open(context.Background(), c, scanner.Processes, launcher)
		if strings.Join(launcher.calls, "; ") != want || reply.Opened != (want != "") {
			t.Errorf("%s: calls %v, reply %+v", id, launcher.calls, reply)
		}
		if want == "" && (reply.Command != "" || !strings.Contains(reply.Message, "Codex keeps this thread open in the background")) {
			t.Errorf("a thread no client shows must not offer a second resume: %+v", reply)
		}
	}
}

func TestScan_OnlyTerminalAppsAreRememberedAndRenamed(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, nil)
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	writeClaudeLive(t, stores, idSafari, 902)
	scanner.Processes = processesWithTable([]ProcessInfo{
		{PID: 900, PPID: 1, Stat: "S", Command: "/Applications/Cursor.app/Contents/Frameworks/Cursor Helper.app/Contents/MacOS/Cursor Helper"},
		{PID: 901, PPID: 900, Stat: "Ss", TTY: "/dev/ttys020", Command: "/bin/zsh"},
		{PID: 902, PPID: 901, Stat: "S+", TTY: "/dev/ttys020", Command: "claude"},
	})

	live, _ := scanner.Find(context.Background(), ToolClaude, idSafari, ScanOptions{})
	scanner.Processes = fakeProcesses{}
	ended, _ := scanner.Find(context.Background(), ToolClaude, idSafari, ScanOptions{})

	if live.Open.Label != "Open in Terminal" || ended.Open.Label != "Open in Terminal" || ended.lastTab != nil {
		t.Errorf("a session in Cursor's terminal: live %q, ended %q, last tab %+v", live.Open.Label, ended.Open.Label, ended.lastTab)
	}
}

func TestReadTabs_LoadsAFileWrittenWithTheTabsLogin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conversation-tabs.json")
	write(t, path, `{"version":1,"tabs":{"claude:x":{"host":{"app":"Ghostty","tty":"/dev/ttys004","leader":{"pid":45469,"procStart":"Tue Nov 10 21:49:14 2026"}},"at":"2026-11-10T12:00:00Z"}}}`)

	tab, ok := readTabs(path).Tabs["claude:x"]
	if !ok || tab.Host != (terminalHost{App: "Ghostty", TTY: "/dev/ttys004"}) || tab.At != "2026-11-10T12:00:00Z" {
		t.Errorf("tab = %+v, %v", tab, ok)
	}
}

func TestRemember_RefreshesADayOldTimeSoPruningCountsFromLastSeen(t *testing.T) {
	host := terminalHost{App: "Ghostty", TTY: "/dev/ttys004"}
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	file := tabsFile{Tabs: map[string]rememberedTab{"claude:x": {Host: host, At: stamp(now.Add(-2 * time.Hour))}}}
	if file.remember("claude:x", host, now) {
		t.Error("an unchanged tab seen two hours ago needs no write")
	}
	file.Tabs["claude:x"] = rememberedTab{Host: host, At: stamp(now.Add(-25 * time.Hour))}
	if !file.remember("claude:x", host, now) || file.Tabs["claude:x"].At != stamp(now) {
		t.Errorf("a day-old time is refreshed: %+v", file.Tabs["claude:x"])
	}
}

func TestScan_LiveEntrypointBeatsTheTranscript(t *testing.T) {
	for name, tc := range map[string]struct {
		live  string
		calls string
	}{
		"plugin-born session resumed in Ghostty": {live: "cli", calls: "focus Ghostty /dev/ttys004"},
		"plugin session still in Cursor":         {live: "claude-vscode", calls: "folder Cursor /tmp/app; link cursor://anthropic.claude-code/open?session=" + idSafari},
	} {
		t.Run(name, func(t *testing.T) {
			scanner, stores := testScanner(t, noSQLite, nil)
			write(t, filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", idSafari+".jsonl"), lines(
				obj{"type": "user", "message": obj{"role": "user", "content": "Fix the build"}, "timestamp": "2026-11-07T10:00:00.000Z", "entrypoint": "claude-vscode", "cwd": "/tmp/app"},
			))
			write(t, filepath.Join(stores.ClaudeDir, "sessions", "45864.json"), `{"pid":45864,"sessionId":"`+idSafari+`","status":"idle","entrypoint":"`+tc.live+`"}`)
			scanner.Processes = processesWithTable(ghosttyTable(ghosttyTab("/dev/ttys004", 45469, 45864, "claude")))

			c, _ := scanner.Find(context.Background(), ToolClaude, idSafari, ScanOptions{Editor: "cursor"})
			launcher := &fakeLauncher{focusOK: true}
			Open(context.Background(), c, scanner.Processes, launcher)

			if actual := strings.Join(launcher.calls, "; "); actual != tc.calls {
				t.Errorf("calls %q, want %q", actual, tc.calls)
			}
		})
	}
}

func TestScan_RemembersTheTabAnEndedSessionRanIn(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, nil)
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	writeClaudeLive(t, stores, idSafari, 45864)
	tab := ghosttyTab("/dev/ttys004", 45469, 45864, "claude")
	scanner.Processes = processesWithTable(ghosttyTable(tab))

	live, _ := scanner.Find(context.Background(), ToolClaude, idSafari, ScanOptions{})
	if live.Open.Kind != RouteFocus {
		t.Fatalf("live row: %s", asJSON(t, live.Open))
	}

	prompt := []ProcessInfo{tab[0], {PID: 45470, PPID: 45469, Stat: "Ss+", TTY: "/dev/ttys004", Command: "-zsh"}}
	scanner.Processes = processesWithTable(ghosttyTable(prompt))
	ended, _ := scanner.Find(context.Background(), ToolClaude, idSafari, ScanOptions{})
	launcher := &fakeLauncher{focusOK: true}
	reply := Open(context.Background(), ended, scanner.Processes, launcher)

	want := "new tab Ghostty cd '/tmp/app' && claude --resume " + idSafari
	if strings.Join(launcher.calls, "; ") != want || reply != (OpenReply{Opened: true, Message: "Resumed in a new Ghostty tab."}) {
		t.Errorf("calls %v, reply %+v", launcher.calls, reply)
	}
	if ended.Open.Label != "Open in Ghostty" {
		t.Errorf("label = %q", ended.Open.Label)
	}
}

func writeClaudeLive(t *testing.T, stores Stores, id string, pid int) {
	t.Helper()
	write(t, filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", id+".jsonl"), lines(
		obj{"type": "user", "message": obj{"role": "user", "content": "Fix the build"}, "timestamp": "2026-11-07T10:00:00.000Z", "entrypoint": "cli", "cwd": "/tmp/app"},
	))
	write(t, filepath.Join(stores.ClaudeDir, "sessions", strconv.Itoa(pid)+".json"), `{"pid":`+strconv.Itoa(pid)+`,"sessionId":"`+id+`","status":"idle"}`)
}

func TestOpen_FocusesTheExactTabOrSaysWhy(t *testing.T) {
	ghostty := terminalHost{App: "Ghostty", TTY: "/dev/ttys004"}
	at := func(host terminalHost) Conversation {
		return Conversation{Tool: ToolClaude, Folder: "/tmp/app", Open: Route{Kind: RouteFocus, Command: "claude --resume x"}, process: &liveProcess{PID: 42}, placed: &placement{process: liveProcess{PID: 42}, host: host}}
	}
	alive := fakeProcesses{alive: map[int]bool{42: true}}
	for name, tc := range map[string]struct {
		conversation Conversation
		launcher     fakeLauncher
		reply        OpenReply
		calls        string
	}{
		"Ghostty tab": {
			conversation: at(ghostty),
			launcher:     fakeLauncher{focusOK: true},
			reply:        OpenReply{Opened: true, Message: "Switched to the Ghostty tab that runs it."},
			calls:        "focus Ghostty /dev/ttys004",
		},
		"Terminal tab": {
			conversation: at(terminalHost{App: "Terminal", TTY: "/dev/ttys018"}),
			launcher:     fakeLauncher{focusOK: true},
			reply:        OpenReply{Opened: true, Message: "Switched to the Terminal tab that runs it."},
			calls:        "focus Terminal /dev/ttys018",
		},
		"iTerm2, not supported yet": {
			conversation: at(terminalHost{App: "iTerm", TTY: "/dev/ttys040"}),
			launcher:     fakeLauncher{focusOK: true},
			reply:        OpenReply{Message: "Brought iTerm to the front, but its tab cannot be selected from outside. It runs on ttys040 (process 42)."},
			calls:        "activate iTerm",
		},
		"tab not found": {
			conversation: at(ghostty),
			reply:        OpenReply{Message: "It runs in Ghostty on ttys004 (process 42), but no tab shows that terminal. Switch to it there."},
			calls:        "focus Ghostty /dev/ttys004",
		},
		"Automation refused": {
			conversation: at(ghostty),
			launcher:     fakeLauncher{err: errAutomationDenied},
			reply:        OpenReply{Message: "macOS blocked switching to Ghostty. Allow agnostic-ai companion, or the terminal that runs agnostic-ai serve, to control Ghostty in System Settings › Privacy & Security › Automation, then click again."},
			calls:        "focus Ghostty /dev/ttys004",
		},
		"Automation prompt unanswered": {
			conversation: at(ghostty),
			launcher:     fakeLauncher{err: context.DeadlineExceeded},
			reply:        OpenReply{Message: "macOS is waiting for you to allow control of Ghostty. Answer its prompt, then click again."},
			calls:        "focus Ghostty /dev/ttys004",
		},
		"editor's terminal": {
			conversation: at(terminalHost{App: "Cursor", TTY: "/dev/ttys020"}),
			reply:        OpenReply{Message: "Brought its Cursor window to the front, but the terminal inside it cannot be selected from outside. It runs on ttys020 (process 42)."},
			calls:        "folder Cursor /tmp/app",
		},
		"Warp": {
			conversation: at(terminalHost{App: "Warp", TTY: "/dev/ttys021"}),
			reply:        OpenReply{Message: "Brought Warp to the front, but its tab cannot be selected from outside. It runs on ttys021 (process 42)."},
			calls:        "activate Warp",
		},
		"unknown app": {
			conversation: at(terminalHost{TTY: "/dev/ttys022"}),
			reply:        OpenReply{Message: "It runs on ttys022 (process 42) in an app this version cannot switch to. Switch to it there."},
		},
		"no terminal found": {
			conversation: Conversation{Tool: ToolClaude, Open: Route{Kind: RouteFocus, Command: "claude --resume x"}, process: &liveProcess{PID: 42}},
			reply:        OpenReply{Message: "It is open (process 42), but the terminal that runs it could not be found. Switch to it there."},
		},
		"ended since the scan, Automation refused": {
			conversation: Conversation{Tool: ToolClaude, Open: Route{Kind: RouteFocus, Command: "claude --resume x"}, process: &liveProcess{PID: 43}},
			launcher:     fakeLauncher{err: errAutomationDenied},
			reply:        OpenReply{Message: "macOS blocked opening Terminal. Allow agnostic-ai companion, or the terminal that runs agnostic-ai serve, to control Terminal in System Settings › Privacy & Security › Automation, or run this command yourself.", Command: "claude --resume x"},
			calls:        "terminal claude --resume x",
		},
		"two Codex clients could hold it": {
			conversation: Conversation{Tool: ToolCodex, Open: Route{Kind: RouteFocus, Command: "codex resume x"}, process: &liveProcess{PID: 42}, placed: &placement{ttys: []string{"/dev/ttys002", "/dev/ttys007"}}},
			reply:        OpenReply{Message: "Codex keeps this thread open in the background and does not say which of its sessions shows it (ttys002, ttys007). Switch to it there."},
		},
		"one Codex client could hold it": {
			conversation: Conversation{Tool: ToolCodex, Open: Route{Kind: RouteFocus, Command: "codex resume x"}, process: &liveProcess{PID: 42}, placed: &placement{ttys: []string{"/dev/ttys002"}}},
			reply:        OpenReply{Message: "Codex keeps this thread open in the background, and the terminal that shows it could not be identified. It may be the Codex session on ttys002."},
		},
	} {
		t.Run(name, func(t *testing.T) {
			launcher := tc.launcher
			reply := Open(context.Background(), tc.conversation, alive, &launcher)
			if reply != tc.reply {
				t.Errorf("reply = %+v, want %+v", reply, tc.reply)
			}
			if actual := strings.Join(launcher.calls, "; "); actual != tc.calls {
				t.Errorf("calls = %q, want %q", actual, tc.calls)
			}
		})
	}
}

func TestTitleTab_NamesOnlyTheTabOfALiveSession(t *testing.T) {
	at := func(host terminalHost) Conversation {
		return Conversation{Tool: ToolClaude, Open: Route{Kind: RouteFocus}, process: &liveProcess{PID: 42}, placed: &placement{process: liveProcess{PID: 42}, host: host}}
	}
	alive := fakeProcesses{alive: map[int]bool{42: true}}
	for name, tc := range map[string]struct {
		conversation Conversation
		processes    fakeProcesses
		launcher     fakeLauncher
		calls        string
		fails        bool
	}{
		"Ghostty tab":        {conversation: at(terminalHost{App: "Ghostty", TTY: "/dev/ttys004"}), processes: alive, launcher: fakeLauncher{focusOK: true}, calls: "title Ghostty /dev/ttys004 Fix login"},
		"Terminal tab":       {conversation: at(terminalHost{App: "Terminal", TTY: "/dev/ttys018"}), processes: alive, launcher: fakeLauncher{focusOK: true}, calls: "title Terminal /dev/ttys018 Fix login"},
		"tab gone":           {conversation: at(terminalHost{App: "Ghostty", TTY: "/dev/ttys004"}), processes: alive, calls: "title Ghostty /dev/ttys004 Fix login", fails: true},
		"app without tabs":   {conversation: at(terminalHost{App: "iTerm", TTY: "/dev/ttys040"}), processes: alive},
		"session ended":      {conversation: at(terminalHost{App: "Ghostty", TTY: "/dev/ttys004"})},
		"terminal not found": {conversation: Conversation{Tool: ToolClaude, process: &liveProcess{PID: 42}}, processes: alive},
	} {
		t.Run(name, func(t *testing.T) {
			launcher := tc.launcher
			err := TitleTab(context.Background(), tc.conversation, tc.processes, &launcher, "Fix login")
			if (err != nil) != tc.fails {
				t.Errorf("err = %v, want failure %v", err, tc.fails)
			}
			if actual := strings.Join(launcher.calls, "; "); actual != tc.calls {
				t.Errorf("calls = %q, want %q", actual, tc.calls)
			}
		})
	}
}

func TestOpen_EndedSessionOpensANewTabInTheAppItRanIn(t *testing.T) {
	tab := terminalHost{App: "Ghostty", TTY: "/dev/ttys004"}
	ended := Conversation{Tool: ToolClaude, Open: Route{Kind: RouteTerminal, Command: "claude --resume x"}, lastTab: &tab}
	login := ProcessInfo{PID: 45469, PPID: 761, Stat: "Ss", TTY: "/dev/ttys004", Started: "Tue Nov 10 21:49:14 2026", Command: "/usr/bin/login"}
	// A shell at its prompt and one waiting in a builtin such as read
	// look the same in the process table.
	shell := processesWithTable(ghosttyTable([]ProcessInfo{login, {PID: 45470, PPID: 45469, Stat: "Ss+", TTY: "/dev/ttys004", Command: "-zsh"}}))
	terminalTab := terminalHost{App: "Terminal", TTY: "/dev/ttys009"}
	for name, tc := range map[string]struct {
		conversation Conversation
		processes    fakeProcesses
		launcher     fakeLauncher
		reply        OpenReply
		calls        string
	}{
		"Ghostty tab with only its shell in front": {
			conversation: ended, processes: shell, launcher: fakeLauncher{focusOK: true},
			reply: OpenReply{Opened: true, Message: "Resumed in a new Ghostty tab."},
			calls: "new tab Ghostty claude --resume x",
		},
		"Automation refused": {
			conversation: ended, processes: shell, launcher: fakeLauncher{err: errAutomationDenied},
			reply: OpenReply{Message: "macOS blocked opening Ghostty. Allow agnostic-ai companion, or the terminal that runs agnostic-ai serve, to control Ghostty in System Settings › Privacy & Security › Automation, or run this command yourself.", Command: "claude --resume x"},
			calls: "new tab Ghostty claude --resume x",
		},
		"Automation prompt unanswered": {
			conversation: ended, processes: shell, launcher: fakeLauncher{err: context.DeadlineExceeded},
			reply: OpenReply{Message: "macOS is waiting for you to allow control of Ghostty. Answer its prompt, then click again.", Command: "claude --resume x"},
			calls: "new tab Ghostty claude --resume x",
		},
		"Terminal tab": {
			conversation: Conversation{Tool: ToolClaude, Open: Route{Kind: RouteTerminal, Command: "claude --resume x"}, lastTab: &terminalTab},
			reply:        OpenReply{Opened: true, Message: "Resumed in a new Terminal tab."},
			calls:        "new tab Terminal claude --resume x",
		},
		"Terminal without new tabs falls back to a window": {
			conversation: Conversation{Tool: ToolClaude, Open: Route{Kind: RouteTerminal, Command: "claude --resume x"}, lastTab: &terminalTab},
			launcher:     fakeLauncher{newTabErr: errors.New("no such command")},
			reply:        OpenReply{Opened: true, Message: "Resumed in a new Terminal window."},
			calls:        "new tab Terminal claude --resume x; terminal claude --resume x",
		},
		"no tab remembered": {
			conversation: Conversation{Tool: ToolClaude, Open: Route{Kind: RouteTerminal, Command: "claude --resume x"}},
			reply:        OpenReply{Opened: true, Message: "Resumed in a new Terminal window."},
			calls:        "terminal claude --resume x",
		},
		"ended since the scan": {
			conversation: Conversation{Tool: ToolClaude, Open: Route{Kind: RouteFocus, Command: "claude --resume x"}, process: &liveProcess{PID: 42}, lastTab: &tab},
			processes:    shell, launcher: fakeLauncher{focusOK: true},
			reply: OpenReply{Opened: true, Message: "Resumed in a new Ghostty tab."},
			calls: "new tab Ghostty claude --resume x",
		},
	} {
		t.Run(name, func(t *testing.T) {
			launcher := tc.launcher
			reply := Open(context.Background(), tc.conversation, tc.processes, &launcher)
			if reply != tc.reply {
				t.Errorf("reply = %+v, want %+v", reply, tc.reply)
			}
			if actual := strings.Join(launcher.calls, "; "); actual != tc.calls {
				t.Errorf("calls = %q, want %q", actual, tc.calls)
			}
		})
	}
}

func TestOpen_CodexThreadStillHeldNeverResumesAgain(t *testing.T) {
	held := Conversation{Tool: ToolCodex, Open: Route{Kind: RouteFocus, Command: "codex resume x"}, process: &liveProcess{PID: 6923}}
	launcher := &fakeLauncher{}

	reply := Open(context.Background(), held, fakeProcesses{alive: map[int]bool{6923: true}}, launcher)

	if len(launcher.calls) != 0 || reply.Opened || !strings.Contains(reply.Message, "Codex keeps this thread open in the background (process 6923)") || reply.Command != "" {
		t.Errorf("a thread the daemon holds must launch nothing: calls %v, reply %+v", launcher.calls, reply)
	}
}

func TestScan_LiveClaudePluginSessionKeepsItsEditorLink(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, nil)
	write(t, filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", idSafari+".jsonl"), lines(
		obj{"type": "user", "message": obj{"role": "user", "content": "Fix the build"}, "timestamp": "2026-11-07T10:00:00.000Z", "entrypoint": "claude-vscode", "cwd": "/tmp/app"},
	))
	write(t, filepath.Join(stores.ClaudeDir, "sessions", "902.json"), `{"pid":902,"sessionId":"`+idSafari+`","status":"idle"}`)
	scanner.Processes = processesWithTable(ghosttyTable(ghosttyTab("/dev/ttys004", 45469, 902, "claude")))

	c, _ := scanner.Find(context.Background(), ToolClaude, idSafari, ScanOptions{Editor: "cursor"})
	launcher := &fakeLauncher{}
	reply := Open(context.Background(), c, scanner.Processes, launcher)

	if want := "folder Cursor /tmp/app; link cursor://anthropic.claude-code/open?session=" + idSafari; strings.Join(launcher.calls, "; ") != want || !reply.Opened {
		t.Errorf("calls %v, reply %+v", launcher.calls, reply)
	}
}

func TestOsascriptArgs_KeepEveryValueOutOfTheScript(t *testing.T) {
	hostile := "cd '/tmp/a\"b' && echo \"quoted\"\nrm -rf ~"
	cases := map[string][]string{"new terminal": newTerminalArgs(hostile)}
	for _, app := range []string{"Terminal", "Ghostty"} {
		cases["focus "+app] = focusTabArgs(app, hostile)
		cases["new tab "+app] = runInNewTabArgs(app, hostile)
		cases["title "+app] = titleTabArgs(app, "/dev/ttys004", hostile)
	}
	for name, args := range cases {
		if len(args) < 3 || args[0] != "-e" || args[1] != "on run argv" {
			t.Errorf("%s: the script must read argv, actual %q", name, args)
			continue
		}
		if args[len(args)-1] != hostile {
			t.Errorf("%s: the value must be the last argument, actual %q", name, args[len(args)-1])
		}
		for _, arg := range args[:len(args)-1] {
			if strings.Contains(arg, "echo") {
				t.Errorf("%s: the value leaked into the script text: %q", name, arg)
			}
		}
	}
}

func TestSystemProcesses_TableListsThisProcess(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the table is read for macOS terminal apps")
	}
	table, err := SystemProcesses{}.Table()
	if err != nil {
		t.Fatal(err)
	}
	for _, process := range table {
		if process.PID == os.Getpid() {
			if process.PPID != os.Getppid() {
				t.Errorf("ppid = %d, want %d", process.PPID, os.Getppid())
			}
			if _, err := time.Parse(lstartLayout, process.Started); err != nil {
				t.Errorf("start time %q: %v", process.Started, err)
			}
			return
		}
	}
	t.Errorf("this process is missing from %d rows", len(table))
}
