package conversations

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

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
	scanner, processes, _, _ := forgottenShellScanner(t)
	signals := recordSessionSignals(scanner)
	var waited []time.Duration
	scanner.shellWait = func(_ context.Context, duration time.Duration) error {
		waited = append(waited, duration)
		processes.table = processes.table[1:]
		return nil
	}
	prior := scanner.Stamp()
	reply, err := scanner.StopSession(context.Background(), ToolClaude, idSafari)
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
	scanner, _, _, _ := forgottenShellScanner(t)
	signals := recordSessionSignals(scanner)
	scanner.shellWait = func(context.Context, time.Duration) error { return nil }
	reply, err := scanner.StopSession(context.Background(), ToolClaude, idSafari)
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
	scanner, processes, _, _ := forgottenShellScanner(t)
	signals := recordSessionSignals(scanner)
	scanner.shellWait = func(context.Context, time.Duration) error {
		processes.table[0].Started = "reused"
		return nil
	}
	reply, err := scanner.StopSession(context.Background(), ToolClaude, idSafari)
	if err != nil {
		t.Fatal(err)
	}
	if want := []shellTestSignal{{group: 4242}}; !reflect.DeepEqual(*signals, want) || reply.Forced {
		t.Errorf("signals = %+v, reply = %+v", *signals, reply)
	}
}

func TestStopSession_RefusesBeforeSignaling(t *testing.T) {
	cases := map[string]func(*fakeProcesses, *Tool, *string){
		"other tool":        func(_ *fakeProcesses, tool *Tool, _ *string) { *tool = ToolCodex },
		"unknown session":   func(_ *fakeProcesses, _ *Tool, id *string) { *id = "00000000-0000-4000-8000-000000000000" },
		"not a session id":  func(_ *fakeProcesses, _ *Tool, id *string) { *id = "../4242" },
		"process ended":     func(processes *fakeProcesses, _ *Tool, _ *string) { processes.alive = map[int]bool{} },
		"process restarted": func(processes *fakeProcesses, _ *Tool, _ *string) { processes.table[0].Started = "reused" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			scanner, processes, _, _ := forgottenShellScanner(t)
			signals := recordSessionSignals(scanner)
			tool, id := ToolClaude, idSafari
			change(processes, &tool, &id)
			_, err := scanner.StopSession(context.Background(), tool, id)
			if !errors.Is(err, ErrSessionRefused) || len(*signals) != 0 {
				t.Errorf("err = %v, signals = %+v", err, *signals)
			}
		})
	}
}

func TestScan_MarksALiveClaudeSessionStoppable(t *testing.T) {
	scanner, processes, _, _ := forgottenShellScanner(t)
	c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), idSafari)
	if c.Stoppable != canStopShells() {
		t.Errorf("live row stoppable = %v", c.Stoppable)
	}
	processes.alive = map[int]bool{}
	if c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), idSafari); c.Stoppable {
		t.Error("ended row is stoppable")
	}
}
