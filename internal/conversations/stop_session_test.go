package conversations

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func stopSessionScanner(t *testing.T) (*Scanner, *fakeProcesses) {
	t.Helper()
	scanner, processes, _, _ := forgottenShellScanner(t)
	processes.table[0].TTY = "/dev/ttys004"
	return scanner, processes
}

func sessionRef(processes *fakeProcesses) ProcessRef {
	return ProcessRef{PID: 4242, ProcStart: processes.table[0].Started}
}

func recordSessionSignals(scanner *Scanner) *[]shellTestSignal {
	var signals []shellTestSignal
	scanner.sessionSignal = func(pid int, force bool) error {
		signals = append(signals, shellTestSignal{group: pid, force: force})
		return nil
	}
	return &signals
}

func TestStopSession_AsksTheProcessToEndFirst(t *testing.T) {
	if !canStopShells() {
		t.Skip("requires signals")
	}
	scanner, processes := stopSessionScanner(t)
	signals := recordSessionSignals(scanner)
	var waited []time.Duration
	scanner.shellWait = func(_ context.Context, duration time.Duration) error {
		waited = append(waited, duration)
		processes.table = processes.table[1:]
		return nil
	}
	prior := scanner.Stamp()
	reply, err := scanner.StopSession(context.Background(), ToolClaude, idSafari, sessionRef(processes), ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if reply != (StopSessionReply{}) {
		t.Errorf("reply = %+v", reply)
	}
	if want := []shellTestSignal{{group: 4242}}; !reflect.DeepEqual(*signals, want) {
		t.Errorf("signals = %+v, want %+v", *signals, want)
	}
	if want := []time.Duration{5 * time.Second}; !reflect.DeepEqual(waited, want) {
		t.Errorf("waited = %v", waited)
	}
	if scanner.Stamp() == prior {
		t.Error("stop left the list tag unchanged")
	}
}

func TestStopSession_KillsTheSameProcessWhenItKeepsRunning(t *testing.T) {
	if !canStopShells() {
		t.Skip("requires signals")
	}
	scanner, processes := stopSessionScanner(t)
	signals := recordSessionSignals(scanner)
	scanner.shellWait = func(context.Context, time.Duration) error { return nil }
	reply, err := scanner.StopSession(context.Background(), ToolClaude, idSafari, sessionRef(processes), ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if want := []shellTestSignal{{group: 4242}, {group: 4242, force: true}}; !reflect.DeepEqual(*signals, want) {
		t.Errorf("signals = %+v, want %+v", *signals, want)
	}
	if want := (StopSessionReply{Forced: true, StillRunning: true}); reply != want {
		t.Errorf("reply = %+v, want %+v", reply, want)
	}
}

func TestStopSession_DoesNotKillAProcessThatReusedThePID(t *testing.T) {
	if !canStopShells() {
		t.Skip("requires signals")
	}
	scanner, processes := stopSessionScanner(t)
	signals := recordSessionSignals(scanner)
	scanner.shellWait = func(context.Context, time.Duration) error {
		processes.table[0].Started = "reused"
		return nil
	}
	reply, err := scanner.StopSession(context.Background(), ToolClaude, idSafari, sessionRef(processes), ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if want := []shellTestSignal{{group: 4242}}; !reflect.DeepEqual(*signals, want) || reply.Forced {
		t.Errorf("signals = %+v, reply = %+v", *signals, reply)
	}
}

func TestStopSession_RefusesBeforeSignaling(t *testing.T) {
	cases := map[string]func(*Scanner, *fakeProcesses, *Tool, *string, *ProcessRef){
		"other tool": func(_ *Scanner, _ *fakeProcesses, tool *Tool, _ *string, _ *ProcessRef) { *tool = ToolCodex },
		"unknown session": func(_ *Scanner, _ *fakeProcesses, _ *Tool, id *string, _ *ProcessRef) {
			*id = "00000000-0000-4000-8000-000000000000"
		},
		"not a session id": func(_ *Scanner, _ *fakeProcesses, _ *Tool, id *string, _ *ProcessRef) { *id = "../4242" },
		"process ended": func(_ *Scanner, processes *fakeProcesses, _ *Tool, _ *string, _ *ProcessRef) {
			processes.alive = map[int]bool{}
		},
		"process restarted": func(_ *Scanner, processes *fakeProcesses, _ *Tool, _ *string, _ *ProcessRef) {
			processes.table[0].Started = "reused"
		},
		"no terminal": func(_ *Scanner, processes *fakeProcesses, _ *Tool, _ *string, _ *ProcessRef) {
			processes.table[0].TTY = ""
		},
		"table unreadable": func(_ *Scanner, processes *fakeProcesses, _ *Tool, _ *string, _ *ProcessRef) {
			processes.tableFails = true
		},
		"runs this server": func(_ *Scanner, processes *fakeProcesses, _ *Tool, _ *string, _ *ProcessRef) {
			processes.table = append(processes.table, ProcessInfo{PID: os.Getpid(), PPID: 4242})
		},
		"already stopping": func(scanner *Scanner, _ *fakeProcesses, _ *Tool, id *string, _ *ProcessRef) {
			scanner.stoppingSessions.Store(*id, struct{}{})
		},
		"session file names another chat": func(scanner *Scanner, processes *fakeProcesses, _ *Tool, _ *string, _ *ProcessRef) {
			write(t, filepath.Join(scanner.Stores.ClaudeDir, "sessions", "4242.json"), lines(obj{"sessionId": "00000000-0000-4000-8000-000000000000", "pid": 4242, "procStart": processes.table[0].Started, "status": "shell"}))
		},
		"stale confirmation": func(_ *Scanner, _ *fakeProcesses, _ *Tool, _ *string, target *ProcessRef) {
			target.ProcStart = "an earlier process"
		},
		"background job": func(scanner *Scanner, _ *fakeProcesses, _ *Tool, _ *string, _ *ProcessRef) {
			write(t, filepath.Join(scanner.Stores.ClaudeDir, "jobs", "e3469759", "state.json"), lines(obj{"sessionId": idSafari, "state": "running"}))
		},
		"editor session": func(scanner *Scanner, processes *fakeProcesses, _ *Tool, _ *string, _ *ProcessRef) {
			write(t, filepath.Join(scanner.Stores.ClaudeDir, "sessions", "4242.json"), lines(obj{"sessionId": idSafari, "pid": 4242, "procStart": processes.table[0].Started, "status": "shell", "entrypoint": "claude-vscode"}))
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			scanner, processes := stopSessionScanner(t)
			signals := recordSessionSignals(scanner)
			tool, id, target := ToolClaude, idSafari, sessionRef(processes)
			change(scanner, processes, &tool, &id, &target)
			_, err := scanner.StopSession(context.Background(), tool, id, target, ScanOptions{})
			if err == nil || len(*signals) != 0 {
				t.Errorf("err = %v, signals = %+v", err, *signals)
			}
		})
	}
}

func TestScan_OffersStopOnlyForAConfirmedTerminalProcess(t *testing.T) {
	scanner, processes := stopSessionScanner(t)
	c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), idSafari)
	if canStopShells() {
		if want := sessionRef(processes); c.Stoppable == nil || *c.Stoppable != want {
			t.Errorf("live row stoppable = %+v, want %+v", c.Stoppable, want)
		}
	} else if c.Stoppable != nil {
		t.Errorf("unsupported system showed a stop target: %+v", c.Stoppable)
	}

	tty := processes.table[0].TTY
	processes.table[0].TTY = ""
	if c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), idSafari); c.Stoppable != nil {
		t.Error("row without a terminal offered stop")
	}
	processes.table[0].TTY = tty
	processes.tableFails = true
	if c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), idSafari); c.Stoppable != nil {
		t.Error("row with an unreadable process table offered stop")
	}
	processes.tableFails = false
	processes.alive = map[int]bool{}
	if c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), idSafari); c.Stoppable != nil {
		t.Error("ended row is stoppable")
	}
}

func TestStopSession_ReportsTheInitialProcessReadError(t *testing.T) {
	if !canStopShells() {
		t.Skip("requires signals")
	}
	scanner, processes := stopSessionScanner(t)
	signals := recordSessionSignals(scanner)
	processes.readFails = true
	_, err := scanner.StopSession(context.Background(), ToolClaude, idSafari, sessionRef(processes), ScanOptions{})
	if !errors.Is(err, errProcessRead) || len(*signals) != 0 {
		t.Errorf("stop err = %v, signals = %+v; want the initial read error", err, *signals)
	}
}

func TestStopSession_DoesNotForceStopWhenEligibilityChanges(t *testing.T) {
	if !canStopShells() {
		t.Skip("requires signals")
	}
	for name, change := range map[string]func(*fakeProcesses){
		"left terminal":    func(processes *fakeProcesses) { processes.table[0].TTY = "" },
		"changed terminal": func(processes *fakeProcesses) { processes.table[0].TTY = "/dev/ttys005" },
		"needed by server": func(processes *fakeProcesses) {
			processes.table = append(processes.table, ProcessInfo{PID: os.Getpid(), PPID: 4242})
		},
	} {
		t.Run(name, func(t *testing.T) {
			scanner, processes := stopSessionScanner(t)
			signals := recordSessionSignals(scanner)
			scanner.shellWait = func(context.Context, time.Duration) error {
				change(processes)
				return nil
			}
			reply, err := scanner.StopSession(context.Background(), ToolClaude, idSafari, sessionRef(processes), ScanOptions{})
			if err != nil || reply != (StopSessionReply{StillRunning: true}) {
				t.Errorf("stop reply = %+v, err = %v; want still running without force", reply, err)
			}
			if want := []shellTestSignal{{group: 4242}}; !reflect.DeepEqual(*signals, want) {
				t.Errorf("signals = %+v, want %+v", *signals, want)
			}
		})
	}
}

func TestStopSession_SaysSoWhenItCannotTellTheChatEnded(t *testing.T) {
	if !canStopShells() {
		t.Skip("requires signals")
	}
	scanner, processes := stopSessionScanner(t)
	signals := recordSessionSignals(scanner)
	scanner.shellWait = func(context.Context, time.Duration) error {
		processes.tableFails = true
		return nil
	}
	if _, err := scanner.StopSession(context.Background(), ToolClaude, idSafari, sessionRef(processes), ScanOptions{}); err == nil || errors.Is(err, ErrSessionRefused) {
		t.Errorf("err = %v, want a failure to check", err)
	}
	if want := []shellTestSignal{{group: 4242}}; !reflect.DeepEqual(*signals, want) {
		t.Errorf("signals = %+v", *signals)
	}
}
