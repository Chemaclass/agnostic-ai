package conversations

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	bgSession       = "e3469759-52bb-4f80-934a-b76b45a2339f"
	bgShortID       = "e3469759"
	bgName          = "Payroll leaf docs"
	terminalAppPath = "/System/Applications/Utilities/Terminal.app/Contents/MacOS/Terminal"
)

// claudeDaemonTable is a claude --bg session as ps shows it: Claude's
// daemon, the bg-pty-host that gives the session its terminal, and the
// session itself on ttys004, with no terminal app above them.
func claudeDaemonTable(extra ...ProcessInfo) []ProcessInfo {
	table := []ProcessInfo{
		{PID: 77652, PPID: 1, Stat: "Ss", Started: "Mon Nov 16 16:25:43 2026", Command: "/Users/me/.local/bin/claude"},
		{PID: 77661, PPID: 77652, Stat: "SNs", Started: "Mon Nov 16 16:25:44 2026", Command: "claude bg-pty-host"},
		{PID: 77663, PPID: 77661, Stat: "SN", TTY: "/dev/ttys004", Started: "Mon Nov 16 16:25:44 2026", Command: "claude bg-spare"},
		{PID: 22657, PPID: 1, Stat: "S", Started: "Mon Nov 16 09:00:00 2026", Command: terminalAppPath},
	}
	return append(table, extra...)
}

// terminalAttach is a Terminal tab on tty whose shell runs claude
// attach as pid, started at started.
func terminalAttach(tty string, pid int, started string) []ProcessInfo {
	return []ProcessInfo{
		{PID: pid - 2, PPID: 22657, Stat: "Ss", TTY: tty, Started: started, Command: "login"},
		{PID: pid - 1, PPID: pid - 2, Stat: "S", TTY: tty, Started: started, Command: "-fish"},
		{PID: pid, PPID: pid - 1, Stat: "S+", TTY: tty, Started: started, Command: "claude"},
	}
}

func backgroundConversation() Conversation {
	return Conversation{
		Tool: ToolClaude, App: AppCLI, ID: bgSession, Folder: "/tmp/app",
		Open:    Route{Kind: RouteFocus},
		process: &liveProcess{PID: 77663},
		placed:  &placement{process: liveProcess{PID: 77663}, host: terminalHost{TTY: "/dev/ttys004"}, claudeBackground: true},
	}
}

var bgJob = ClaudeJob{ShortID: bgShortID, SessionID: bgSession, Name: bgName}

func TestListClaudeJobs_KeepsBackgroundSessions(t *testing.T) {
	out := `[
		{"pid": 63223, "cwd": "/Users/me", "kind": "interactive", "sessionId": "9032303f-8963-4562-ae9e-d7aee990a2de", "name": "me-39", "status": "idle"},
		{"pid": 77663, "id": "e3469759", "cwd": "/tmp/app", "kind": "background", "sessionId": "` + bgSession + `", "name": "Payroll leaf docs", "status": "idle", "state": "working"},
		{"id": "5f12e206", "kind": "background", "sessionId": "5f12e206-38ed-48ec-90c8-ba8cba28f7fa", "name": "", "state": "blocked"}
	]`
	var asked []string
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		asked = append(asked, name+" "+strings.Join(args, " "))
		return []byte(out), nil
	}

	locate := func() (string, error) { return "/Users/me/.local/bin/claude", nil }

	jobs, err := listClaudeJobs(context.Background(), locate, run)

	if err != nil {
		t.Fatal(err)
	}
	want := []ClaudeJob{bgJob, {ShortID: "5f12e206", SessionID: "5f12e206-38ed-48ec-90c8-ba8cba28f7fa"}}
	if fmt.Sprint(jobs) != fmt.Sprint(want) {
		t.Errorf("jobs = %+v, want %+v", jobs, want)
	}
	if strings.Join(asked, "; ") != "/Users/me/.local/bin/claude agents --json" {
		t.Errorf("ran %v", asked)
	}
}

func TestListClaudeJobs_TurnsEachFailureIntoOneReason(t *testing.T) {
	for name, tc := range map[string]struct {
		locateErr error
		out       string
		err       error
		message   string
	}{
		"claude not found": {locateErr: exec.ErrNotFound, message: "no claude command on PATH or in its usual install folders"},
		"claude gone":      {err: &exec.Error{Name: "claude", Err: exec.ErrNotFound}, message: "no claude command on PATH or in its usual install folders"},
		"command failed":   {err: errors.New("exit status 1: unknown command agents"), message: "claude agents --json failed"},
		"output not JSON":  {out: "Usage: claude agents", message: "claude agents --json printed what this version cannot read"},
		"output not array": {out: `{"id": "e3469759"}`, message: "claude agents --json printed what this version cannot read"},
	} {
		t.Run(name, func(t *testing.T) {
			locate := func() (string, error) { return "/usr/local/bin/claude", tc.locateErr }
			run := func(context.Context, string, ...string) ([]byte, error) { return []byte(tc.out), tc.err }

			jobs, err := listClaudeJobs(context.Background(), locate, run)

			var failure *claudeJobsError
			if !errors.As(err, &failure) || err.Error() != tc.message || jobs != nil {
				t.Fatalf("jobs %v, err %v, want %q", jobs, err, tc.message)
			}
			if failure.cause == nil {
				t.Error("the command's own error must stay in cause")
			}
		})
	}
}

// fakeExecutable writes an executable file named name in folder.
func fakeExecutable(t *testing.T, folder, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(folder, name)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFindClaude_LooksInTheInstallFoldersADesktopPathLeavesOut(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	// The PATH an app started from the desktop gets, with no claude on it.
	t.Setenv("PATH", t.TempDir())
	installed := fakeExecutable(t, filepath.Join(home, ".local", "bin"), "claude")

	found, err := findClaude(claudeFolders())

	if err != nil || found != installed {
		t.Errorf("found %q, %v, want %q", found, err, installed)
	}
}

func TestFindClaude_PathComesFirstAndNoneFoundIsNotFound(t *testing.T) {
	onPath := t.TempDir()
	t.Setenv("PATH", onPath)
	folder := t.TempDir()
	fakeExecutable(t, folder, "claude")
	fromPath := fakeExecutable(t, onPath, "claude")

	found, err := findClaude([]string{folder})
	if err != nil || found != fromPath {
		t.Errorf("found %q, %v, want %q", found, err, fromPath)
	}

	t.Setenv("PATH", t.TempDir())
	found, err = findClaude([]string{t.TempDir()})
	if !errors.Is(err, exec.ErrNotFound) || found != "" {
		t.Errorf("found %q, %v, want exec.ErrNotFound", found, err)
	}
}

func TestClaudeFolders_ListsTheNativeInstallFirst(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	want := []string{filepath.Join(home, ".local", "bin"), filepath.Join(home, ".claude", "local"), "/opt/homebrew/bin", "/usr/local/bin"}
	if folders := claudeFolders(); fmt.Sprint(folders) != fmt.Sprint(want) {
		t.Errorf("folders = %v, want %v", folders, want)
	}
}

func TestScan_ClaudeBackgroundSessionOpensWithClaudeAttach(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, nil)
	write(t, filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", bgSession+".jsonl"), lines(
		obj{"type": "user", "message": obj{"role": "user", "content": "Bring the docs up to date"}, "timestamp": "2026-11-16T15:25:44.000Z", "cwd": "/tmp/app"},
	))
	write(t, filepath.Join(stores.ClaudeDir, "sessions", "77663.json"), `{"pid":77663,"sessionId":"`+bgSession+`","status":"idle"}`)
	processes := processesWithTable(claudeDaemonTable())
	processes.jobs = []ClaudeJob{bgJob}
	scanner.Processes = processes

	c, found := scanner.Find(context.Background(), ToolClaude, bgSession, ScanOptions{})
	if !found || c.placed == nil || !c.placed.claudeBackground {
		t.Fatalf("the scan must see a session under claude bg-pty-host as a background session: found %v, placed %+v", found, c.placed)
	}
	launcher := &fakeLauncher{}
	reply := Open(context.Background(), c, scanner.Processes, launcher)

	if want := (OpenReply{Opened: true, Message: "Opened it in a new Terminal window with claude attach."}); reply != want {
		t.Errorf("reply = %+v, want %+v", reply, want)
	}
	if want := "terminal cd '/tmp/app' && claude attach e3469759"; strings.Join(launcher.calls, "; ") != want {
		t.Errorf("calls = %v, want %q", launcher.calls, want)
	}
}

func TestScan_ClaudeBackgroundSessionAttachesInTheTabAppItRanIn(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, nil)
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	write(t, filepath.Join(filepath.Dir(scanner.MarksPath), "conversation-tabs.json"),
		`{"version":1,"tabs":{"claude:`+bgSession+`":{"host":{"app":"Ghostty","tty":"/dev/ttys030"},"at":"2026-11-16T15:00:00Z"}}}`)
	write(t, filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", bgSession+".jsonl"), lines(
		obj{"type": "user", "message": obj{"role": "user", "content": "Bring the docs up to date"}, "timestamp": "2026-11-16T15:25:44.000Z", "cwd": "/tmp/app"},
	))
	write(t, filepath.Join(stores.ClaudeDir, "sessions", "77663.json"), `{"pid":77663,"sessionId":"`+bgSession+`","status":"idle"}`)
	processes := processesWithTable(claudeDaemonTable())
	processes.jobs = []ClaudeJob{bgJob}
	scanner.Processes = processes

	c, _ := scanner.Find(context.Background(), ToolClaude, bgSession, ScanOptions{})
	launcher := &fakeLauncher{}
	reply := Open(context.Background(), c, scanner.Processes, launcher)

	if want := (OpenReply{Opened: true, Message: "Opened it in a new Ghostty tab with claude attach."}); reply != want {
		t.Errorf("reply = %+v, want %+v", reply, want)
	}
	if want := "new tab Ghostty cd '/tmp/app' && claude attach e3469759"; strings.Join(launcher.calls, "; ") != want {
		t.Errorf("calls = %v, want %q", launcher.calls, want)
	}
}

func TestPlace_OnlyASessionUnderClaudeBgPtyHostIsABackgroundSession(t *testing.T) {
	linuxTable := claudeDaemonTable()
	// Linux ps comm keeps 15 characters of the parent's name.
	linuxTable[1].Command = "claude bg-pty-h"
	ptyHostArgs := map[int]string{77661: "claude bg-pty-host --bg-pty-host /tmp/cc-daemon-1000/c0f11278/spare/ac5ee99d.pty.sock 200 50 -- /home/me/.local/share/claude/versions/2.1.325 --bg-spare /tmp/cc-daemon-1000/c0f11278/spare/ac5ee99d.claim.sock"}
	for name, tc := range map[string]struct {
		table []ProcessInfo
		args  map[int]string
		tool  Tool
		want  bool
	}{
		"under claude bg-pty-host":                       {table: claudeDaemonTable(), tool: ToolClaude, want: true},
		"Linux, name cut, command line confirms":         {table: linuxTable, args: ptyHostArgs, tool: ToolClaude, want: true},
		"Linux, name cut, another command line":          {table: linuxTable, args: map[int]string{77661: "claude bg-pty-helper --watch"}, tool: ToolClaude},
		"Linux, name cut, command line no longer listed": {table: linuxTable, tool: ToolClaude},
		"Codex under a process named the same":           {table: claudeDaemonTable(), tool: ToolCodex},
		"terminal with no known app": {table: []ProcessInfo{
			{PID: 500, PPID: 1, Stat: "S", Started: "Mon Nov 16 16:25:43 2026", Command: "/usr/bin/tmux"},
			{PID: 77663, PPID: 500, Stat: "S+", TTY: "/dev/ttys004", Started: "Mon Nov 16 16:25:44 2026", Command: "claude"},
		}, tool: ToolClaude},
	} {
		t.Run(name, func(t *testing.T) {
			c := Conversation{Tool: tc.tool, process: &liveProcess{PID: 77663}}

			place(&c, nil, tc.table, fakeProcesses{args: tc.args}.Args)

			if c.placed == nil || c.placed.claudeBackground != tc.want {
				t.Errorf("placed = %+v, want claudeBackground %v", c.placed, tc.want)
			}
		})
	}
}

func TestOpen_ClaudeBackgroundSession(t *testing.T) {
	alive := map[int]bool{77663: true}
	attachIn := func(pid int, target string) map[int]string { return map[int]string{pid: "claude attach " + target} }
	terminalTab := terminalHost{App: "Ghostty", TTY: "/dev/ttys030"}
	cursorAttach := []ProcessInfo{
		{PID: 900, PPID: 1, Stat: "S", Started: "Mon Nov 16 09:00:00 2026", Command: "/Applications/Cursor.app/Contents/Frameworks/Cursor Helper.app/Contents/MacOS/Cursor Helper"},
		{PID: 99267, PPID: 900, Stat: "Ss", TTY: "/dev/ttys040", Started: "Mon Nov 16 16:43:20 2026", Command: "/bin/zsh"},
		{PID: 99268, PPID: 99267, Stat: "S+", TTY: "/dev/ttys040", Started: "Mon Nov 16 16:43:20 2026", Command: "claude"},
	}
	ranInGhostty := backgroundConversation()
	ranInGhostty.lastTab = &terminalTab
	for name, tc := range map[string]struct {
		conversation Conversation
		processes    fakeProcesses
		launcher     fakeLauncher
		reply        OpenReply
		calls        string
	}{
		"a Terminal tab already attached by short id": {
			processes: fakeProcesses{alive: alive, jobs: []ClaudeJob{bgJob}, table: claudeDaemonTable(terminalAttach("/dev/ttys012", 99268, "Mon Nov 16 16:43:20 2026")...), args: attachIn(99268, bgShortID)},
			launcher:  fakeLauncher{focusOK: true},
			reply:     OpenReply{Opened: true, Message: "Switched to the Terminal tab that runs it."},
			calls:     "focus Terminal /dev/ttys012",
		},
		"a Terminal tab already attached by name": {
			processes: fakeProcesses{alive: alive, jobs: []ClaudeJob{bgJob}, table: claudeDaemonTable(terminalAttach("/dev/ttys012", 99268, "Mon Nov 16 16:43:20 2026")...), args: attachIn(99268, bgName)},
			launcher:  fakeLauncher{focusOK: true},
			reply:     OpenReply{Opened: true, Message: "Switched to the Terminal tab that runs it."},
			calls:     "focus Terminal /dev/ttys012",
		},
		"of two attached tabs, the latest started": {
			processes: fakeProcesses{
				alive: alive, jobs: []ClaudeJob{bgJob},
				table: claudeDaemonTable(append(terminalAttach("/dev/ttys012", 99268, "Mon Nov 16 16:43:20 2026"), terminalAttach("/dev/ttys014", 56541, "Mon Nov 16 17:02:41 2026")...)...),
				args:  map[int]string{99268: "claude attach " + bgShortID, 56541: "/Users/me/.local/bin/claude attach " + bgShortID},
			},
			launcher: fakeLauncher{focusOK: true},
			reply:    OpenReply{Opened: true, Message: "Switched to the Terminal tab that runs it."},
			calls:    "focus Terminal /dev/ttys014",
		},
		"attached tab no longer shown": {
			processes: fakeProcesses{alive: alive, jobs: []ClaudeJob{bgJob}, table: claudeDaemonTable(terminalAttach("/dev/ttys012", 99268, "Mon Nov 16 16:43:20 2026")...), args: attachIn(99268, bgShortID)},
			reply:     OpenReply{Message: "It runs in Terminal on ttys012 (process 99268), but no tab shows that terminal. Switch to it there.", Command: "cd '/tmp/app' && claude attach e3469759"},
			calls:     "focus Terminal /dev/ttys012",
		},
		"switching to the attached tab refused by macOS": {
			processes: fakeProcesses{alive: alive, jobs: []ClaudeJob{bgJob}, table: claudeDaemonTable(terminalAttach("/dev/ttys012", 99268, "Mon Nov 16 16:43:20 2026")...), args: attachIn(99268, bgShortID)},
			launcher:  fakeLauncher{err: errAutomationDenied},
			reply:     OpenReply{Message: "macOS blocked switching to Terminal. Allow Agnostic Companion, or the terminal that runs agnostic-ai serve, to control Terminal in System Settings › Privacy & Security › Automation, then click again.", Command: "cd '/tmp/app' && claude attach e3469759"},
			calls:     "focus Terminal /dev/ttys012",
		},
		"attached in Cursor's terminal in another project": {
			processes: fakeProcesses{alive: alive, jobs: []ClaudeJob{bgJob}, table: claudeDaemonTable(cursorAttach...), args: attachIn(99268, bgShortID), cwds: map[int]string{99268: "/tmp/viewer-project"}},
			reply:     OpenReply{Message: "Brought its Cursor window to the front, but the terminal inside it cannot be selected from outside. It runs on ttys040 (process 99268).", Command: "cd '/tmp/app' && claude attach e3469759"},
			calls:     "folder Cursor /tmp/viewer-project",
		},
		"attached in Cursor's terminal, its folder unknown": {
			processes: fakeProcesses{alive: alive, jobs: []ClaudeJob{bgJob}, table: claudeDaemonTable(cursorAttach...), args: attachIn(99268, bgShortID)},
			reply:     OpenReply{Message: "Brought Cursor to the front, but its tab cannot be selected from outside. It runs on ttys040 (process 99268).", Command: "cd '/tmp/app' && claude attach e3469759"},
			calls:     "activate Cursor",
		},
		"a tab attached to another session": {
			conversation: ranInGhostty,
			processes:    fakeProcesses{alive: alive, jobs: []ClaudeJob{bgJob}, table: claudeDaemonTable(terminalAttach("/dev/ttys012", 99268, "Mon Nov 16 16:43:20 2026")...), args: attachIn(99268, "787d5d52")},
			reply:        OpenReply{Opened: true, Message: "Opened it in a new Ghostty tab with claude attach."},
			calls:        "new tab Ghostty cd '/tmp/app' && claude attach e3469759",
		},
		"a tab running claude attach with the full session id, which it does not take": {
			processes: fakeProcesses{alive: alive, jobs: []ClaudeJob{bgJob}, table: claudeDaemonTable(terminalAttach("/dev/ttys012", 99268, "Mon Nov 16 16:43:20 2026")...), args: attachIn(99268, bgSession)},
			reply:     OpenReply{Opened: true, Message: "Opened it in a new Terminal window with claude attach."},
			calls:     "terminal cd '/tmp/app' && claude attach e3469759",
		},
		"no attached tab and no tab remembered": {
			processes: fakeProcesses{alive: alive, jobs: []ClaudeJob{bgJob}, table: claudeDaemonTable()},
			reply:     OpenReply{Opened: true, Message: "Opened it in a new Terminal window with claude attach."},
			calls:     "terminal cd '/tmp/app' && claude attach e3469759",
		},
		"new tab refused by macOS": {
			conversation: ranInGhostty,
			processes:    fakeProcesses{alive: alive, jobs: []ClaudeJob{bgJob}, table: claudeDaemonTable()},
			launcher:     fakeLauncher{err: errAutomationDenied},
			reply:        OpenReply{Message: "macOS blocked opening Ghostty. Allow Agnostic Companion, or the terminal that runs agnostic-ai serve, to control Ghostty in System Settings › Privacy & Security › Automation, or run this command yourself.", Command: "cd '/tmp/app' && claude attach e3469759"},
			calls:        "new tab Ghostty cd '/tmp/app' && claude attach e3469759",
		},
		"claude agents does not list it": {
			processes: fakeProcesses{alive: alive, jobs: []ClaudeJob{{ShortID: "787d5d52", SessionID: "787d5d52-b66b-40fd-83ed-c3dc6d025856"}}, table: claudeDaemonTable()},
			reply:     OpenReply{Message: "It runs as a Claude Code background session (process 77663), but claude agents does not list it, so it cannot be attached from here. Run claude agents to find its id, then claude attach <id>."},
		},
		"claude agents cannot run": {
			processes: fakeProcesses{alive: alive, jobsErr: &claudeJobsError{reason: claudeNotInstalled, cause: exec.ErrNotFound}, table: claudeDaemonTable()},
			reply:     OpenReply{Message: "It runs as a Claude Code background session (process 77663), but the list of those sessions could not be read: no claude command on PATH or in its usual install folders. Run claude agents to find its id, then claude attach <id>."},
		},
	} {
		t.Run(name, func(t *testing.T) {
			conversation := tc.conversation
			if conversation.ID == "" {
				conversation = backgroundConversation()
			}
			launcher := tc.launcher
			reply := Open(context.Background(), conversation, tc.processes, &launcher)
			if reply != tc.reply {
				t.Errorf("reply = %+v, want %+v", reply, tc.reply)
			}
			if actual := strings.Join(launcher.calls, "; "); actual != tc.calls {
				t.Errorf("calls = %q, want %q", actual, tc.calls)
			}
		})
	}
}

func TestClaudeAttachTarget_ReadsTheArgumentAsPsPrintsIt(t *testing.T) {
	for args, want := range map[string]string{
		"claude attach e3469759":                       "e3469759",
		"/Users/me/.local/bin/claude attach e3469759":  "e3469759",
		"claude attach [bot] SFTP write: first draft":  "[bot] SFTP write: first draft",
		"claude attach":                                "",
		"claude --resume e3469759":                     "",
		"claude bg-spare --bg-spare /tmp/x.claim.sock": "",
	} {
		target, ok := claudeAttachTarget(args)
		if target != want || ok != (want != "") {
			t.Errorf("claudeAttachTarget(%q) = %q, %v, want %q", args, target, ok, want)
		}
	}
}
