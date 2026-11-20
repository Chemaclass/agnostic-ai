package conversations

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type shellTestSignal struct {
	group int
	force bool
}

func recordShellSignals(scanner *Scanner) *[]shellTestSignal {
	var signals []shellTestSignal
	scanner.shellSignal = func(group int, force bool) error {
		signals = append(signals, shellTestSignal{group: group, force: force})
		return nil
	}
	return &signals
}

func TestStopShell_SignalsOnlyTheGroupAndReportsADetachedChild(t *testing.T) {
	if !canStopShells() {
		t.Skip("requires process groups")
	}
	scanner, processes, _, _ := forgottenShellScanner(t)
	signals := recordShellSignals(scanner)
	calledWait := false
	scanner.shellWait = func(_ context.Context, duration time.Duration) error {
		calledWait = true
		if duration != 5*time.Second || !reflect.DeepEqual(*signals, []shellTestSignal{{group: 4243}}) {
			t.Errorf("before wait: duration = %v, signals = %+v", duration, *signals)
		}
		processes.table[4].PPID = 1
		return nil
	}
	prior := scanner.Stamp()
	reply, err := scanner.StopShell(context.Background(), ToolClaude, idSafari,
		ProcessRef{PID: 4243, ProcStart: processes.table[1].Started}, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	wantSignals := []shellTestSignal{{group: 4243}, {group: 4243, force: true}}
	if !calledWait || !reflect.DeepEqual(*signals, wantSignals) {
		t.Errorf("signals = %+v, waited = %v", *signals, calledWait)
	}
	if want := []ProcessRef{{PID: 4246, ProcStart: processes.table[4].Started}}; !reflect.DeepEqual(reply.StillRunning, want) {
		t.Errorf("still running = %+v, want %+v", reply.StillRunning, want)
	}
	if scanner.Stamp() == prior {
		t.Error("stop left the list tag unchanged")
	}
}

func TestStopShell_RefusesBeforeSignaling(t *testing.T) {
	cases := map[string]func(*fakeProcesses, *ProcessRef, *ScanOptions){
		"target start changed": func(_ *fakeProcesses, target *ProcessRef, _ *ScanOptions) { target.ProcStart = "reused" },
		"session closed":       func(p *fakeProcesses, _ *ProcessRef, _ *ScanOptions) { delete(p.alive, 4242) },
		"parent start changed": func(p *fakeProcesses, _ *ProcessRef, _ *ScanOptions) { p.table[0].Started = p.table[1].Started },
		"session group":        func(p *fakeProcesses, _ *ProcessRef, _ *ScanOptions) { p.table[0].PGID = 4243 },
		"session pid": func(p *fakeProcesses, target *ProcessRef, _ *ScanOptions) {
			*target = ProcessRef{PID: 4242, ProcStart: p.table[0].Started}
		},
		"unrelated tool": func(p *fakeProcesses, target *ProcessRef, _ *ScanOptions) {
			*target = ProcessRef{PID: 4245, ProcStart: p.table[3].Started}
		},
		"out of scope": func(_ *fakeProcesses, _ *ProcessRef, opts *ScanOptions) { opts.Within = []string{"/other"} },
		"disabled":     func(_ *fakeProcesses, _ *ProcessRef, opts *ScanOptions) { opts.StuckShellAfter = new(time.Duration) },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			scanner, processes, _, _ := forgottenShellScanner(t)
			signals := recordShellSignals(scanner)
			target := ProcessRef{PID: 4243, ProcStart: processes.table[1].Started}
			opts := ScanOptions{}
			change(processes, &target, &opts)
			_, err := scanner.StopShell(context.Background(), ToolClaude, idSafari, target, opts)
			if !errors.Is(err, ErrShellRefused) || len(*signals) != 0 {
				t.Errorf("err = %v, signals = %+v", err, *signals)
			}
		})
	}
}

func TestStopShell_RechecksGroupIdentityBeforeForce(t *testing.T) {
	if !canStopShells() {
		t.Skip("requires process groups")
	}
	cases := map[string]func([]ProcessInfo){
		"leader reused":     func(table []ProcessInfo) { table[1].Started = "reused" },
		"parent reused":     func(table []ProcessInfo) { table[0].Started = "reused" },
		"gained new parent": func(table []ProcessInfo) { table[1].PPID = 1 },
		"changed groups":    func(table []ProcessInfo) { table[1].PGID = 4000 },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			scanner, processes, _, _ := forgottenShellScanner(t)
			signals := recordShellSignals(scanner)
			target := ProcessRef{PID: 4243, ProcStart: processes.table[1].Started}
			scanner.shellWait = func(context.Context, time.Duration) error {
				change(processes.table)
				return nil
			}
			_, err := scanner.StopShell(context.Background(), ToolClaude, idSafari, target, ScanOptions{})
			if err != nil || !reflect.DeepEqual(*signals, []shellTestSignal{{group: 4243}}) {
				t.Errorf("err = %v, signals = %+v", err, *signals)
			}
		})
	}
}

func TestStopShell_EscalatesAfterRequestCancellation(t *testing.T) {
	if !canStopShells() {
		t.Skip("requires process groups")
	}
	scanner, processes, _, _ := forgottenShellScanner(t)
	signals := recordShellSignals(scanner)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scanner.shellWait = func(grace context.Context, _ time.Duration) error {
		cancel()
		return grace.Err()
	}
	_, err := scanner.StopShell(ctx, ToolClaude, idSafari, ProcessRef{PID: 4243, ProcStart: processes.table[1].Started}, ScanOptions{})
	if err != nil || len(*signals) != 2 || !(*signals)[1].force {
		t.Errorf("err = %v, signals = %+v", err, *signals)
	}
}

func TestStopShell_DoesNotSignalAProcessReusedAfterTheRescan(t *testing.T) {
	if !canStopShells() {
		t.Skip("requires process groups")
	}
	scanner, processes, _, _ := forgottenShellScanner(t)
	target := ProcessRef{PID: 4243, ProcStart: processes.table[1].Started}
	signals := recordShellSignals(scanner)
	scanner.Processes = &shellTableReads{fakeProcesses: processes, beforeRead: func(reads int) {
		if reads == 2 {
			processes.table[1].Started = "different"
		}
	}}
	_, err := scanner.StopShell(context.Background(), ToolClaude, idSafari, target, ScanOptions{})
	if !errors.Is(err, ErrShellRefused) || len(*signals) != 0 {
		t.Errorf("err = %v, signals = %+v", err, *signals)
	}
}

func TestStopShell_ReportsGroupMembersWhenLeaderExitsDuringGrace(t *testing.T) {
	if !canStopShells() {
		t.Skip("requires process groups")
	}
	scanner, processes, _, _ := forgottenShellScanner(t)
	target := ProcessRef{PID: 4243, ProcStart: processes.table[1].Started}
	signals := recordShellSignals(scanner)
	scanner.shellWait = func(context.Context, time.Duration) error {
		processes.table = append(processes.table[:1], processes.table[2:]...)
		return nil
	}
	reply, err := scanner.StopShell(context.Background(), ToolClaude, idSafari, target, ScanOptions{})
	if err != nil || len(*signals) != 1 {
		t.Errorf("err = %v, signals = %+v", err, *signals)
	}
	want := []ProcessRef{
		{PID: 4244, ProcStart: target.ProcStart},
		{PID: 4246, ProcStart: target.ProcStart},
	}
	if !reflect.DeepEqual(reply.StillRunning, want) {
		t.Errorf("still running = %+v, want %+v", reply.StillRunning, want)
	}
}
