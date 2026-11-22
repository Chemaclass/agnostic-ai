package conversations

import (
	"context"
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
