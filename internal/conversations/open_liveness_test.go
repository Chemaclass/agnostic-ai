package conversations

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func codexThreadScanner(t *testing.T, id string, processes fakeProcesses) *Scanner {
	t.Helper()
	scanner, stores := testScanner(t, fakeStores(t, map[string]string{
		"state_5.sqlite|FROM threads": `[{"id":"` + id + `","cwd":"/tmp/app","created_at_ms":1762500000000,"updated_at_ms":1762510000000,"originator":"codex-tui","source":"cli","name":"Parser fix"}]`,
	}), processes)
	scanner.Now = func() time.Time { return time.UnixMilli(1762520000000) }
	write(t, filepath.Join(stores.CodexDir, "state_5.sqlite"), "")
	write(t, filepath.Join(stores.CodexDir, "thread-writer-locks", id+".lock"), "")
	return scanner
}

func TestOpen_CodexThreadWhoseLockHolderLookupFailedIsNotResumed(t *testing.T) {
	const id = "55555555-5555-4555-8555-555555555555"
	scanner := codexThreadScanner(t, id, fakeProcesses{readFails: true})
	view := scanner.Scan(context.Background(), ScanOptions{})
	thread := byID(t, view, id)
	launcher := &fakeLauncher{focusOK: true}

	reply := Open(context.Background(), thread, scanner.Processes, launcher)

	if len(launcher.calls) != 0 || reply.Opened {
		t.Errorf("a thread whose liveness is unknown must launch nothing: calls %v, reply %+v", launcher.calls, reply)
	}
	if !strings.Contains(reply.Message, "Could not check whether this session still runs") || !strings.Contains(reply.Command, "resume") {
		t.Errorf("want the reason and the command to run by hand, got %+v", reply)
	}
}

func TestOpen_CodexThreadConfirmedClosedResumes(t *testing.T) {
	const id = "55555555-5555-4555-8555-555555555555"
	scanner := codexThreadScanner(t, id, fakeProcesses{})
	view := scanner.Scan(context.Background(), ScanOptions{})
	launcher := &fakeLauncher{focusOK: true}

	reply := Open(context.Background(), byID(t, view, id), scanner.Processes, launcher)

	if len(launcher.calls) != 1 || !strings.Contains(launcher.calls[0], "resume") || !reply.Opened {
		t.Errorf("a thread no process holds resumes: calls %v, reply %+v", launcher.calls, reply)
	}
}

func TestOpen_CodexWithoutALockFolderStillResumes(t *testing.T) {
	const id = "55555555-5555-4555-8555-555555555555"
	scanner, stores := testScanner(t, fakeStores(t, map[string]string{
		"state_5.sqlite|FROM threads": `[{"id":"` + id + `","cwd":"/tmp/app","created_at_ms":1762500000000,"updated_at_ms":1762510000000,"originator":"codex-tui","source":"cli","name":"Parser fix"}]`,
	}), fakeProcesses{readFails: true})
	scanner.Now = func() time.Time { return time.UnixMilli(1762520000000) }
	write(t, filepath.Join(stores.CodexDir, "state_5.sqlite"), "")
	view := scanner.Scan(context.Background(), ScanOptions{})
	launcher := &fakeLauncher{focusOK: true}

	reply := Open(context.Background(), byID(t, view, id), scanner.Processes, launcher)

	if len(launcher.calls) != 1 || !reply.Opened {
		t.Errorf("a Codex that keeps no lock folder leaves nothing to check, so it resumes: calls %v, reply %+v", launcher.calls, reply)
	}
}

func TestOpen_ClaudeSessionWhoseProcessReadFailedIsNotResumed(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{readFails: true})
	writeMarkedSession(t, stores, id, "2026-11-07T11:00:00.000Z")
	write(t, filepath.Join(stores.ClaudeDir, "sessions", "77.json"), `{"pid":77,"sessionId":"`+id+`","status":"idle"}`)
	view := scanner.Scan(context.Background(), ScanOptions{})
	launcher := &fakeLauncher{focusOK: true}

	reply := Open(context.Background(), byID(t, view, id), scanner.Processes, launcher)

	if len(launcher.calls) != 0 || reply.Opened || !strings.Contains(reply.Message, "Could not check") || !strings.Contains(reply.Command, "--resume") {
		t.Errorf("a session whose process could not be read must launch nothing: calls %v, reply %+v", launcher.calls, reply)
	}
}

func openClaudeSession(t *testing.T, processes fakeProcesses, sessions func(Stores)) (OpenReply, []string) {
	t.Helper()
	const id = "11111111-1111-4111-8111-111111111111"
	scanner, stores := testScanner(t, noSQLite, processes)
	writeMarkedSession(t, stores, id, "2026-11-07T11:00:00.000Z")
	if sessions != nil {
		sessions(stores)
	}
	view := scanner.Scan(context.Background(), ScanOptions{})
	launcher := &fakeLauncher{focusOK: true}
	reply := Open(context.Background(), byID(t, view, id), scanner.Processes, launcher)
	return reply, launcher.calls
}

func TestOpen_ClaudeSessionLivenessFromItsSessionsFolder(t *testing.T) {
	for name, tc := range map[string]struct {
		processes fakeProcesses
		sessions  func(Stores)
		resumes   bool
	}{
		"a session file of a running process cannot be read": {
			processes: fakeProcesses{alive: map[int]bool{77: true}},
			sessions: func(stores Stores) {
				if err := os.MkdirAll(filepath.Join(stores.ClaudeDir, "sessions", "77.json"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
		"the sessions folder cannot be listed": {
			sessions: func(stores Stores) { write(t, filepath.Join(stores.ClaudeDir, "sessions"), "not a folder") },
		},
		"no sessions folder and a good process read": {resumes: true},
	} {
		t.Run(name, func(t *testing.T) {
			reply, calls := openClaudeSession(t, tc.processes, tc.sessions)
			if tc.resumes {
				if len(calls) != 1 || !reply.Opened {
					t.Errorf("want a resume: calls %v, reply %+v", calls, reply)
				}
				return
			}
			if len(calls) != 0 || reply.Opened || !strings.Contains(reply.Message, "Could not check") || !strings.Contains(reply.Command, "--resume") {
				t.Errorf("want no launch and the command by hand: calls %v, reply %+v", calls, reply)
			}
		})
	}
}

func TestOpen_CursorAgentChatTheScanCouldNotSeeFollowsAFreshRead(t *testing.T) {
	for name, tc := range map[string]struct {
		processes fakeProcesses
		scanSaw   bool
		resumes   bool
	}{
		"a clean read finds no holder":              {processes: fakeProcesses{}, resumes: true},
		"the read fails":                            {processes: fakeProcesses{readFails: true}},
		"the read fails after a scan that saw it":   {processes: fakeProcesses{readFails: true}, scanSaw: true},
		"a holder whose command cannot be read":     {processes: fakeProcesses{held: map[string]int{"store.db": 31}}, scanSaw: true},
		"another program holds the store":           {processes: fakeProcesses{held: map[string]int{"store.db": 31}, args: map[int]string{31: "/usr/bin/sqlite3 store.db"}}, scanSaw: true, resumes: true},
		"a clean read after a scan that saw it too": {processes: fakeProcesses{}, scanSaw: true, resumes: true},
	} {
		t.Run(name, func(t *testing.T) {
			chat := Conversation{Tool: ToolCursor, App: AppCLI, store: filepath.Join(t.TempDir(), "store.db"), liveUnknown: !tc.scanSaw, Open: Route{Kind: RouteTerminal, Command: "cursor-agent --resume x"}}
			launcher := &fakeLauncher{focusOK: true}

			reply := Open(context.Background(), chat, tc.processes, launcher)

			if resumed := len(launcher.calls) == 1 && reply.Opened; resumed != tc.resumes {
				t.Errorf("resumed = %v, want %v: calls %v, reply %+v", resumed, tc.resumes, launcher.calls, reply)
			}
		})
	}
}

func TestOpen_LiveSessionWhoseProcessCheckFailsIsNotResumed(t *testing.T) {
	session := Conversation{Tool: ToolClaude, App: AppCLI, Open: Route{Kind: RouteFocus, Command: "claude --resume x"}, process: &liveProcess{PID: 42, ProcStart: "Sat Nov  7 11:00:00 2026"}}
	launcher := &fakeLauncher{focusOK: true}

	reply := Open(context.Background(), session, fakeProcesses{readFails: true}, launcher)

	if len(launcher.calls) != 0 || reply.Opened || !strings.Contains(reply.Message, "Could not check") || !strings.Contains(reply.Command, "--resume") {
		t.Errorf("a failed process check must launch nothing: calls %v, reply %+v", launcher.calls, reply)
	}
}

func TestSend_CursorAgentChatWhoseHolderCheckFailedSaysSo(t *testing.T) {
	chat := Conversation{Tool: ToolCursor, App: AppCLI, store: filepath.Join(t.TempDir(), "store.db"), Open: Route{Kind: RouteTerminal, Command: "cursor-agent --resume x"}}
	launcher := &fakeLauncher{focusOK: true}

	reply := Send(context.Background(), chat, "hi", fakeProcesses{readFails: true}, launcher)

	if reply.Sent || !strings.Contains(reply.Message, "Could not check whether it still runs") || len(launcher.calls) != 0 {
		t.Errorf("reply %+v, calls %v", reply, launcher.calls)
	}
}

type processesWithStarts struct {
	codexArgvProcesses
	starts map[int]string
}

func (p processesWithStarts) Alive(pid int, started string) bool {
	start, alive := p.starts[pid]
	return alive && (started == "" || started == start)
}

func (p processesWithStarts) AliveAmong(processes []ProcessRef) (map[int]bool, error) {
	alive := make(map[int]bool)
	for _, process := range processes {
		if p.Alive(process.PID, process.ProcStart) {
			alive[process.PID] = true
		}
	}
	return alive, nil
}

func TestReopen_UsesTheCodexTabOnlyWhileItsClientRuns(t *testing.T) {
	const (
		id        = "01a1c738-eda0-7be0-b151-023a3e0ab7ee"
		clientPID = 17199
		daemonPID = 6923
	)
	terminal := ghosttyTab("/dev/ttys002", 16953, clientPID, "codex")
	table := append(ghosttyTable(terminal), ProcessInfo{PID: daemonPID, Command: "codex"})
	processes := processesWithStarts{
		codexArgvProcesses: codexArgvProcesses{
			fakeProcesses: fakeProcesses{table: table},
			argv:          map[int][]string{clientPID: {"codex", "resume", id}},
		},
		starts: map[int]string{daemonPID: "", clientPID: terminal[2].Started},
	}
	c := Conversation{
		Tool: ToolCodex, ID: id, Folder: "/repo",
		Open:    Route{Kind: RouteFocus, Command: "codex resume " + id},
		process: &liveProcess{PID: daemonPID},
	}
	if !place(&c, newCodexClients(table, nil, processes), table, processes.Args) {
		t.Fatal("the resumed client has a Ghostty tab")
	}
	if c.placed.process.ProcStart != terminal[2].Started {
		t.Errorf("placed client start = %q, want %q", c.placed.process.ProcStart, terminal[2].Started)
	}

	for _, tc := range []struct {
		name        string
		clientStart string
		switches    bool
	}{
		{name: "same client", clientStart: terminal[2].Started, switches: true},
		{name: "client exited"},
		{name: "client PID reused", clientStart: "Tue Nov 10 22:00:00 2026"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			processes := processes
			processes.starts = map[int]string{daemonPID: ""}
			if tc.clientStart != "" {
				processes.starts[clientPID] = tc.clientStart
			}

			scanner, _ := testScanner(t, noSQLite, processes)
			scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
			now := time.Date(2026, 11, 10, 22, 5, 0, 0, time.UTC)
			scanner.Now = func() time.Time { return now }
			if err := scanner.Mark(ToolCodex, id, MarkCompleted); err != nil {
				t.Fatal(err)
			}
			key := markKey(ToolCodex, id)
			before := storedMarks(t, scanner.MarksPath).Marks[key]
			opts := ScanOptions{}
			scanner.keepRead(scanner.beginRead(), opts, []sourceResult{found(ToolCodex, []Conversation{c})})
			now = now.Add(time.Second)

			launcher := &fakeLauncher{focusOK: true}
			scanner.lock()
			reply, found := scanner.Reopen(context.Background(), ToolCodex, id, opts, launcher)
			scanner.unlock()

			after := storedMarks(t, scanner.MarksPath).Marks[key]
			if !found || reply.Opened != tc.switches || reply.Guessed || reply.Command != "" {
				t.Errorf("found %t, reply %+v, want opened %t and no guess or resume", found, reply, tc.switches)
			}
			if tc.switches {
				if got := strings.Join(launcher.calls, "; "); got != "focus Ghostty /dev/ttys002" {
					t.Errorf("open calls %q, want focus on the live client's tab", got)
				}
				if after.CompletedAt != "" || after.SeenAt == before.SeenAt {
					t.Errorf("confirmed open left stale marks: before %+v, after %+v", before, after)
				}
			} else {
				if len(launcher.calls) != 0 || !strings.Contains(reply.Message, "Could not confirm") {
					t.Errorf("open calls %v, reply %+v, want no focus and a message", launcher.calls, reply)
				}
				if after.CompletedAt != before.CompletedAt || after.SeenAt != before.SeenAt {
					t.Errorf("stale placement changed marks: before %+v, after %+v", before, after)
				}
			}

			launcher.calls = nil
			if err := TitleTab(context.Background(), c, processes, launcher, "New title"); err != nil {
				t.Errorf("title error: %v", err)
			}
			wantTitle := ""
			if tc.switches {
				wantTitle = "title Ghostty /dev/ttys002 New title"
			}
			if got := strings.Join(launcher.calls, "; "); got != wantTitle {
				t.Errorf("title calls %q, want %q", got, wantTitle)
			}
		})
	}
}
