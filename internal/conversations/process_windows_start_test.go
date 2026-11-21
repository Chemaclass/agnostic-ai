package conversations

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

// A process that started 2026-11-21 10:00:00.1234567 UTC.
const (
	startFiletime     = "134397288001234567"
	startFiletimeText = "134397288001234560"
)

func TestWindowsProcStart_ReadsTheNativeCreationTime(t *testing.T) {
	if got := windowsProcStart("", startFiletime, time.UTC); got != startFiletimeText {
		t.Errorf("got %q, want %q", got, startFiletimeText)
	}
}

func TestWindowsProcStart_ReadsPowerShellTicksInLocalTime(t *testing.T) {
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Skip("no time zone data")
	}
	// Ticks of 2026-11-21 11:00:00.123456 in Madrid (UTC+1), as CIM
	// gives the creation time: in local time, to the microsecond.
	ticks := "639308556001234560"
	if got := windowsProcStart(ticks, "", madrid); got != startFiletimeText {
		t.Errorf("got %q, want %q", got, startFiletimeText)
	}
}

func TestWindowsProcStart_KeepsWhatItCannotRead(t *testing.T) {
	for _, recorded := range []string{"", "Sat Nov 21 10:00:00 2026", "12"} {
		if got := windowsProcStart(recorded, "", time.UTC); got != recorded {
			t.Errorf("%q became %q", recorded, got)
		}
	}
}

func TestFiletimeText_DropsTheLastDigit(t *testing.T) {
	if got := filetimeText(134397288001234567); got != startFiletimeText {
		t.Errorf("got %q", got)
	}
}

func TestClaudeLive_OnWindowsReadsTheNativeStart(t *testing.T) {
	defer func(saved nativeReader) { nativeProcesses = saved }(nativeProcesses)
	nativeProcesses = fakeNative{}
	var entry claudeLive
	if err := json.Unmarshal([]byte(`{"pid":7,"sessionId":"s","procStartFt":"`+startFiletime+`","status":"idle"}`), &entry); err != nil {
		t.Fatal(err)
	}
	if entry.ProcStart != startFiletimeText || entry.PID != 7 || entry.Status != "idle" {
		t.Errorf("entry %+v", entry)
	}
}

func TestClaudeLive_ElsewhereKeepsProcStart(t *testing.T) {
	defer func(saved nativeReader) { nativeProcesses = saved }(nativeProcesses)
	nativeProcesses = nil
	var entry claudeLive
	if err := json.Unmarshal([]byte(`{"pid":7,"procStart":"Sat Nov 21 10:00:00 2026","procStartFt":"1"}`), &entry); err != nil {
		t.Fatal(err)
	}
	if entry.ProcStart != "Sat Nov 21 10:00:00 2026" {
		t.Errorf("entry %+v", entry)
	}
}

func TestHoldersByHalves_SplitsOnlyGroupsSomeoneHolds(t *testing.T) {
	holders := map[string]int{"c": 30, "f": 60}
	asked := 0
	list := func(group []string) ([]int, error) {
		asked++
		var pids []int
		for _, path := range group {
			if pid, ok := holders[path]; ok {
				pids = append(pids, pid)
			}
		}
		return pids, nil
	}

	got, err := holdersByHalves([]string{"a", "b", "c", "d", "e", "f", "g", "h"}, list)

	if err != nil || len(got) != 2 || got["c"] != 30 || got["f"] != 60 {
		t.Errorf("got %v err %v", got, err)
	}
	// Asking about every group of a full split takes 15 questions; a
	// group nobody holds is not split.
	if asked != 11 {
		t.Errorf("asked %d times for 8 paths and 2 holders", asked)
	}
}

func TestHoldersByHalves_ReturnsTheError(t *testing.T) {
	failed := errors.New("RmGetList: access denied")
	if _, err := holdersByHalves([]string{"a"}, func([]string) ([]int, error) { return nil, failed }); !errors.Is(err, failed) {
		t.Errorf("err %v", err)
	}
}

func TestSystemProcesses_OnWindowsReadsTheNativeTable(t *testing.T) {
	defer func(saved nativeReader) { nativeProcesses = saved }(nativeProcesses)
	nativeProcesses = fakeNative{table: []ProcessInfo{{PID: 7, PPID: 1, Started: startFiletimeText, Command: "claude.exe"}, {PID: 9, Started: "1", Command: "Cursor.exe"}}}
	processes := SystemProcesses{}

	if !processes.Alive(7, startFiletimeText) || processes.Alive(7, "1") || processes.Alive(8, "") {
		t.Error("Alive does not follow the table")
	}
	alive, err := processes.AliveAmong([]ProcessRef{{PID: 7, ProcStart: startFiletimeText}, {PID: 9, ProcStart: "2"}, {PID: 8}})
	if err != nil || !alive[7] || alive[9] || alive[8] {
		t.Errorf("alive %v err %v", alive, err)
	}
	if !processes.AppRunning("Cursor") || processes.AppRunning("Code") {
		t.Error("AppRunning does not follow the table")
	}
	if held, err := processes.Holders([]string{`C:\a.lock`}); err != nil || held[`C:\a.lock`] != 7 {
		t.Errorf("held %v err %v", held, err)
	}
	if args := processes.Args([]int{7, 8}); !slices.Equal([]string{args[7]}, []string{"claude --resume x"}) || len(args) != 1 {
		t.Errorf("args %v", args)
	}
}

type fakeNative struct{ table []ProcessInfo }

func (f fakeNative) processes() ([]ProcessInfo, error) { return f.table, nil }

func (f fakeNative) args(pids []int) map[int]string {
	args := map[int]string{}
	for _, pid := range pids {
		if pid == 7 {
			args[pid] = "claude --resume x"
		}
	}
	return args
}

func (f fakeNative) holders(paths []string) (map[string]int, error) {
	held := map[string]int{}
	for _, path := range paths {
		held[path] = 7
	}
	return held, nil
}

func TestWindowsProcStart_ReadsAFileTimeInProcStart(t *testing.T) {
	// claude.exe 2.1.330 on Windows Server 2025 wrote this procStart.
	if got := windowsProcStart("134397694254382773", "", time.UTC); got != "134397694254382770" {
		t.Errorf("got %q", got)
	}
}

func TestSystemProcesses_OnWindowsAProcessThatHidesItsStartRuns(t *testing.T) {
	defer func(saved nativeReader) { nativeProcesses = saved }(nativeProcesses)
	nativeProcesses = fakeNative{table: []ProcessInfo{{PID: 7, Command: "claude"}}}

	if alive, err := (SystemProcesses{}).AliveAmong([]ProcessRef{{PID: 7, ProcStart: startFiletimeText}}); err != nil || !alive[7] {
		t.Errorf("alive %v err %v", alive, err)
	}
}

func TestCommandName_ReadsTheProgramOfACommandLine(t *testing.T) {
	for line, want := range map[string]string{
		`/usr/local/bin/node /x/cursor-agent.js`:                        "node",
		`cursor-agent --resume x`:                                       "cursor-agent",
		`"C:\Program Files\nodejs\node.exe" C:\x\cursor-agent\index.js`: "node",
		`C:\Users\dev\AppData\Local\cursor-agent\cursor-agent.EXE -p`:   "cursor-agent",
	} {
		if got := commandName(line); got != want {
			t.Errorf("%q: got %q, want %q", line, got, want)
		}
	}
}

func TestWithoutExe_KeepsANameWithSpaces(t *testing.T) {
	if got := withoutExe("Code - Insiders.EXE"); got != "Code - Insiders" {
		t.Errorf("got %q", got)
	}
}

func TestFocusTerminal_OnWindowsSaysTheWindowCannotBeBroughtForward(t *testing.T) {
	defer func(saved string) { shellSystem = saved }(shellSystem)
	shellSystem = "windows"
	c := Conversation{Tool: ToolClaude, Open: Route{Kind: RouteFocus, Command: "claude --resume x"}, process: &liveProcess{PID: 42}}

	reply := focusTerminal(context.Background(), c, fakeProcesses{alive: map[int]bool{42: true}}, windowsLauncher{})

	if reply.Opened || reply.Message != "It is open in another window (process 42), and agnostic-ai cannot bring a window forward on Windows. Switch to it there. To resume it elsewhere, end that process first, then run this command." || reply.Command == "" {
		t.Errorf("reply %+v", reply)
	}
}

func TestFocusTerminal_OnWindowsACodexThreadGetsTheSameReply(t *testing.T) {
	defer func(saved string) { shellSystem = saved }(shellSystem)
	shellSystem = "windows"
	c := Conversation{Tool: ToolCodex, Open: Route{Kind: RouteFocus, Command: "codex resume x"}, process: &liveProcess{PID: 42}}

	reply := focusTerminal(context.Background(), c, fakeProcesses{alive: map[int]bool{42: true}}, windowsLauncher{})

	if !strings.HasPrefix(reply.Message, "It is open in another window (process 42)") {
		t.Errorf("reply %+v", reply)
	}
}
