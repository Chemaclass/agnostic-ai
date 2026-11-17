package conversations

import (
	"context"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestReopen_DoesNotWaitForARunningScan(t *testing.T) {
	const id = "cse_a1"
	scanner, _ := testScanner(t, noSQLite, fakeProcesses{readFails: true})
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	var clock atomic.Int64
	clock.Store(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC).Unix())
	scanner.Now = func() time.Time { return time.Unix(clock.Load(), 0).UTC() }
	rows := []Conversation{cloudRow(id, "Fix the parser", "2026-11-07T11:30:00Z")}
	asked, release := make(chan struct{}), make(chan struct{})
	cloud := &countingCloud{answers: []func() ([]Conversation, error){
		func() ([]Conversation, error) { return rows, nil },
		// A cloud that is slow to answer holds the scan that asked it.
		func() ([]Conversation, error) {
			close(asked)
			<-release
			return rows, nil
		},
	}}
	scanner.Cloud = cloud
	scanner.Scan(context.Background(), ScanOptions{})
	// A new minute: the next scan asks the cloud again.
	clock.Add(61)
	scanned := make(chan struct{})
	go func() {
		defer close(scanned)
		scanner.Scan(context.Background(), ScanOptions{})
	}()
	<-asked

	type answer struct {
		reply OpenReply
		found bool
	}
	opened := make(chan answer, 1)
	go func() {
		reply, found := scanner.Reopen(context.Background(), ToolClaude, id, ScanOptions{}, &fakeLauncher{})
		opened <- answer{reply, found}
	}()

	select {
	case got := <-opened:
		if !got.found || !got.reply.Opened {
			t.Errorf("reopen while a scan runs = %+v, found %t", got.reply, got.found)
		}
		if seen := storedMarks(t, scanner.MarksPath).Marks[markKey(ToolClaude, id)].SeenAt; seen != "2026-11-07T12:01:01Z" {
			t.Errorf("seenAt while the scan still runs = %q; the mark must not wait for it either", seen)
		}
	case <-time.After(5 * time.Second):
		t.Error("the open waited for the running scan")
	}
	close(release)
	<-scanned
	if cloud.calls != 2 {
		t.Errorf("the cloud was asked %d times; want 2: the open reads nothing again", cloud.calls)
	}
}

func TestReopen_WaitsForTheRunningScanBeforeItResumesAnEndedSession(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{readFails: true})
	var clock atomic.Int64
	clock.Store(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC).Unix())
	scanner.Now = func() time.Time { return time.Unix(clock.Load(), 0).UTC() }
	writeMarkedSession(t, stores, id, "2026-11-07T11:00:00.000Z")
	asked, release := make(chan struct{}), make(chan struct{})
	scanner.Cloud = &countingCloud{answers: []func() ([]Conversation, error){
		func() ([]Conversation, error) { return nil, nil },
		func() ([]Conversation, error) {
			close(asked)
			<-release
			return nil, nil
		},
	}}
	scanner.Scan(context.Background(), ScanOptions{})
	clock.Add(61)
	scanned := make(chan struct{})
	go func() {
		defer close(scanned)
		scanner.Scan(context.Background(), ScanOptions{})
	}()
	<-asked
	opened := make(chan OpenReply, 1)
	go func() {
		reply, _ := scanner.Reopen(context.Background(), ToolClaude, id, ScanOptions{}, &fakeLauncher{})
		opened <- reply
	}()
	select {
	case reply := <-opened:
		t.Fatalf("the open answered %+v from the older read; the session may have started again since", reply)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	<-scanned

	if reply := <-opened; !reply.Opened {
		t.Errorf("after the scan ended the open = %+v", reply)
	}
}

func TestReopen_ReadsAgainWhenTheScanItWaitedForLookedAtProcessesBeforeTheClick(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	alive := map[int]bool{4242: false}
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{alive: alive})
	var clock atomic.Int64
	clock.Store(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC).Unix())
	scanner.Now = func() time.Time { return time.Unix(clock.Load(), 0).UTC() }
	writeMarkedSession(t, stores, id, "2026-11-07T11:00:00.000Z")
	writeLiveSessionFile(t, stores, "4242", "", id)
	asked, release := make(chan struct{}), make(chan struct{})
	scanner.Cloud = &countingCloud{answers: []func() ([]Conversation, error){
		func() ([]Conversation, error) { return nil, nil },
		func() ([]Conversation, error) {
			close(asked)
			<-release
			return nil, nil
		},
		func() ([]Conversation, error) { return nil, nil },
	}}
	if c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), id); c.Open.Kind != RouteTerminal {
		t.Fatalf("the ended session's route = %q; this test needs a row that resumes it", c.Open.Kind)
	}
	clock.Add(61)
	scanned := make(chan struct{})
	go func() {
		defer close(scanned)
		scanner.Scan(context.Background(), ScanOptions{})
	}()
	<-asked
	// The running scan has looked at the processes; the session starts again after that.
	alive[4242] = true
	launcher := &fakeLauncher{focusOK: true}
	opened := make(chan OpenReply, 1)
	go func() {
		reply, _ := scanner.Reopen(context.Background(), ToolClaude, id, ScanOptions{}, launcher)
		opened <- reply
	}()
	time.Sleep(50 * time.Millisecond)
	close(release)
	<-scanned
	reply := <-opened

	for _, call := range launcher.calls {
		if strings.HasPrefix(call, "terminal ") {
			t.Fatalf("the open resumed a session that runs again: %q, reply %+v", call, reply)
		}
	}
}

func TestReopen_WaitsForTheRunningScanWhenTheLiveSessionItKnewHasEnded(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	alive := map[int]bool{4242: true}
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{alive: alive})
	var clock atomic.Int64
	clock.Store(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC).Unix())
	scanner.Now = func() time.Time { return time.Unix(clock.Load(), 0).UTC() }
	writeMarkedSession(t, stores, id, "2026-11-07T11:00:00.000Z")
	writeLiveSessionFile(t, stores, "4242", "", id)
	asked, release := make(chan struct{}), make(chan struct{})
	scanner.Cloud = &countingCloud{answers: []func() ([]Conversation, error){
		func() ([]Conversation, error) { return nil, nil },
		func() ([]Conversation, error) {
			close(asked)
			<-release
			return nil, nil
		},
	}}
	if c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), id); c.Open.Kind != RouteFocus {
		t.Fatalf("the running session's route = %q; this test needs a row that switches to it", c.Open.Kind)
	}
	// The session ends; it may have been started again under another process.
	alive[4242] = false
	clock.Add(61)
	scanned := make(chan struct{})
	go func() {
		defer close(scanned)
		scanner.Scan(context.Background(), ScanOptions{})
	}()
	<-asked
	opened := make(chan OpenReply, 1)
	go func() {
		reply, _ := scanner.Reopen(context.Background(), ToolClaude, id, ScanOptions{}, &fakeLauncher{})
		opened <- reply
	}()
	select {
	case reply := <-opened:
		t.Fatalf("the open answered %+v from the older read, which resumes a session that may run again", reply)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	<-scanned
	<-opened
}
