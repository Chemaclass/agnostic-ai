package conversations

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// heldLauncher holds every new terminal until release closes, and counts them.
type heldLauncher struct {
	fakeLauncher
	mu        sync.Mutex
	terminals []string
	started   chan string
	release   chan struct{}
}

func newHeldLauncher() *heldLauncher {
	return &heldLauncher{started: make(chan string, 8), release: make(chan struct{})}
}

func (l *heldLauncher) RunInNewTerminal(ctx context.Context, command string) error {
	l.mu.Lock()
	l.terminals = append(l.terminals, command)
	l.mu.Unlock()
	l.started <- command
	select {
	case <-l.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (l *heldLauncher) launched() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.terminals...)
}

type openAnswer struct {
	reply OpenReply
	found bool
}

func reopenIn(scanner *Scanner, id string, launcher Launcher) <-chan openAnswer {
	answered := make(chan openAnswer, 1)
	go func() {
		reply, found := scanner.Reopen(context.Background(), ToolClaude, id, ScanOptions{}, launcher)
		answered <- openAnswer{reply, found}
	}()
	return answered
}

func answerOf(t *testing.T, answered <-chan openAnswer) openAnswer {
	t.Helper()
	select {
	case got := <-answered:
		return got
	case <-time.After(10 * time.Second):
		t.Fatal("the open never answered")
		return openAnswer{}
	}
}

// sharedWait is how long a second open has to reach the first one before it ends.
const sharedWait = 500 * time.Millisecond

func TestReopen_TwoOpensOfOneEndedChatStartOneTerminalAndShareTheReply(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	writeMarkedSession(t, stores, id, "2026-11-07T11:00:00.000Z")
	scanner.Scan(context.Background(), ScanOptions{})
	launcher := newHeldLauncher()

	first := reopenIn(scanner, id, launcher)
	<-launcher.started
	second := reopenIn(scanner, id, launcher)
	time.Sleep(sharedWait)
	close(launcher.release)

	one, two := answerOf(t, first), answerOf(t, second)
	if got := launcher.launched(); len(got) != 1 {
		t.Fatalf("two opens of one ended chat started %d terminals: %q", len(got), got)
	}
	if !one.found || !one.reply.Opened || one != two {
		t.Errorf("the opens answered %+v and %+v; want the same reply", one, two)
	}
}

func TestReopen_ASecondOpenSharesTheFirstOnesFailure(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	writeMarkedSession(t, stores, id, "2026-11-07T11:00:00.000Z")
	scanner.Scan(context.Background(), ScanOptions{})
	launcher := newHeldLauncher()
	t.Cleanup(func() { close(launcher.release) })
	ctx, cancel := context.WithCancel(context.Background())

	first := make(chan openAnswer, 1)
	go func() {
		reply, found := scanner.Reopen(ctx, ToolClaude, id, ScanOptions{}, launcher)
		first <- openAnswer{reply, found}
	}()
	<-launcher.started
	second := reopenIn(scanner, id, launcher)
	time.Sleep(sharedWait)
	cancel()

	one, two := answerOf(t, first), answerOf(t, second)
	if one.reply.Opened || one != two {
		t.Errorf("the opens answered %+v and %+v; want the first one's failure twice", one, two)
	}
	if got := launcher.launched(); len(got) != 1 {
		t.Errorf("the failed open was tried again: %q", got)
	}
	if again := answerOf(t, reopenIn(scanner, id, &fakeLauncher{})); !again.reply.Opened {
		t.Errorf("an open after the failed one = %+v; the failure must not stay behind", again)
	}
}

func TestReopen_OpensOfDifferentChatsDoNotWaitForEachOther(t *testing.T) {
	const slow, quick = "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	writeMarkedSession(t, stores, slow, "2026-11-07T11:00:00.000Z")
	write(t, filepath.Join(stores.ClaudeDir, "projects", "-tmp-other", quick+".jsonl"), `{"type":"user","cwd":"/tmp/other","entrypoint":"cli","timestamp":"2026-11-07T11:10:00.000Z","message":{"role":"user","content":"Other work"}}`+"\n")
	scanner.Scan(context.Background(), ScanOptions{})
	launcher := newHeldLauncher()
	t.Cleanup(func() { close(launcher.release) })

	held := reopenIn(scanner, slow, launcher)
	<-launcher.started
	var quickCalls atomic.Int32
	other := reopenIn(scanner, quick, countingLauncher{&quickCalls})
	select {
	case got := <-other:
		if !got.reply.Opened || quickCalls.Load() != 1 {
			t.Errorf("the other chat's open = %+v after %d launches", got, quickCalls.Load())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("an open of another chat waited for the held one")
	}
	select {
	case got := <-held:
		t.Errorf("the held open answered before its terminal opened: %+v", got)
	default:
	}
}

// countingLauncher opens a terminal at once and counts it.
type countingLauncher struct{ terminals *atomic.Int32 }

func (l countingLauncher) RunInNewTerminal(context.Context, string) error {
	l.terminals.Add(1)
	return nil
}
func (countingLauncher) OpenLink(context.Context, string) error           { return nil }
func (countingLauncher) OpenFolder(context.Context, string, string) error { return nil }
func (countingLauncher) ActivateApp(context.Context, string) error        { return nil }
func (countingLauncher) FocusTab(context.Context, string, string) (bool, error) {
	return false, nil
}
func (countingLauncher) TitleTab(context.Context, string, string, string) (bool, error) {
	return false, nil
}
func (countingLauncher) TypeInTab(context.Context, string, string, string, bool) (bool, error) {
	return false, nil
}
func (countingLauncher) RunInNewTab(context.Context, string, string) error { return nil }

func TestReopen_WaitsForTheRunningScanBeforeItFollowsTheLinkOfAClosedDesktopSession(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	alive := map[int]bool{4242: false}
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{alive: alive})
	var clock atomic.Int64
	clock.Store(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC).Unix())
	scanner.Now = func() time.Time { return time.Unix(clock.Load(), 0).UTC() }
	write(t, filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", id+".jsonl"), lines(
		obj{"type": "user", "cwd": "/tmp/app", "entrypoint": "claude-desktop", "timestamp": "2026-11-07T10:00:00.000Z", "message": obj{"role": "user", "content": "Plan the release"}},
		obj{"type": "user", "cwd": "/tmp/app", "entrypoint": "cli", "timestamp": "2026-11-07T11:00:00.000Z", "message": obj{"role": "user", "content": "Go on from the terminal"}},
	))
	write(t, filepath.Join(stores.ClaudeDir, "sessions", "4242.json"), `{"pid":4242,"sessionId":"`+id+`","status":"idle","entrypoint":"cli"}`)
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
	if c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), id); c.Open.Kind != RouteLink || c.Live {
		t.Fatalf("the closed session's row = %s; this test needs a link to a closed session", asJSON(t, c))
	}
	clock.Add(61)
	scanned := make(chan struct{})
	go func() {
		defer close(scanned)
		scanner.Scan(context.Background(), ScanOptions{})
	}()
	<-asked
	// The person resumed it in a terminal after the running scan looked at the processes.
	alive[4242] = true
	launcher := &fakeLauncher{focusOK: true}
	opened := make(chan OpenReply, 1)
	go func() {
		reply, _ := scanner.Reopen(context.Background(), ToolClaude, id, ScanOptions{}, launcher)
		opened <- reply
	}()
	time.Sleep(sharedWait)
	close(release)
	<-scanned
	reply := <-opened

	for _, call := range launcher.calls {
		if strings.HasPrefix(call, "link ") {
			t.Fatalf("the open followed the old link while the session runs in a terminal: %q, reply %+v", call, reply)
		}
	}
}

// panickingLauncher fails the way a bug would.
type panickingLauncher struct{ fakeLauncher }

func (panickingLauncher) RunInNewTerminal(context.Context, string) error { panic("launcher bug") }

func TestReopen_AnOpenThatPanicsLeavesNothingBehind(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	writeMarkedSession(t, stores, id, "2026-11-07T11:00:00.000Z")
	scanner.Scan(context.Background(), ScanOptions{})

	func() {
		defer func() { _ = recover() }()
		scanner.Reopen(context.Background(), ToolClaude, id, ScanOptions{}, &panickingLauncher{})
	}()

	if again := answerOf(t, reopenIn(scanner, id, &fakeLauncher{})); !again.reply.Opened {
		t.Errorf("an open after one that panicked = %+v", again)
	}
}
