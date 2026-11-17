package conversations

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestStateOf_AKnownClosedTerminalSessionIsArchivedWhateverItsReply(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	since := now.Add(-48 * time.Hour)
	at := func(ago time.Duration) string { return now.Add(-ago).Format(time.RFC3339) }
	one := 1
	cases := []struct {
		name  string
		c     Conversation
		entry markEntry
		want  State
	}{
		{"known closed with a reply not seen", Conversation{UpdatedAt: at(time.Minute), liveRead: true}, markEntry{SeenAt: at(time.Hour)}, StateDormant},
		{"known closed, never seen, written after the marks began", Conversation{UpdatedAt: at(time.Minute), liveRead: true}, markEntry{}, StateDormant},
		{"known closed with subagents counted working", Conversation{UpdatedAt: at(time.Minute), liveRead: true, WorkingSubagents: &one}, markEntry{SeenAt: at(time.Hour)}, StateDormant},
		{"known closed and completed stays completed", Conversation{UpdatedAt: at(time.Hour), liveRead: true}, markEntry{SeenAt: at(time.Minute), CompletedAt: at(time.Minute)}, StateCompleted},
		{"unknown, with a reply not seen, stays unread", Conversation{UpdatedAt: at(time.Minute)}, markEntry{SeenAt: at(time.Hour)}, StateUnread},
		{"resumed with the reply read while closed", Conversation{UpdatedAt: at(time.Hour), Live: true, Activity: ActivityIdle, liveRead: true}, markEntry{SeenAt: at(2 * time.Hour), ClosedWrittenAt: at(time.Hour)}, StateActive},
		{"resumed and waiting on the reply read while closed", Conversation{UpdatedAt: at(time.Hour), Live: true, Activity: ActivityWaiting, liveRead: true}, markEntry{SeenAt: at(2 * time.Hour), ClosedWrittenAt: at(time.Hour)}, StateActive},
		{"resumed with a later reply", Conversation{UpdatedAt: at(time.Minute), Live: true, Activity: ActivityIdle, liveRead: true}, markEntry{SeenAt: at(2 * time.Hour), ClosedWrittenAt: at(time.Hour)}, StateUnread},
		{"resumed and waiting after a later write", Conversation{UpdatedAt: at(time.Minute), Live: true, Activity: ActivityWaiting, liveRead: true}, markEntry{SeenAt: at(2 * time.Hour), ClosedWrittenAt: at(time.Hour)}, StateBlocked},
		{"unknown read with a reply read while closed stays unread", Conversation{UpdatedAt: at(time.Hour)}, markEntry{SeenAt: at(2 * time.Hour), ClosedWrittenAt: at(time.Hour)}, StateUnread},
		{"live, first seen open, with a reply never read while closed", Conversation{UpdatedAt: at(time.Hour), Live: true, Activity: ActivityIdle, liveRead: true}, markEntry{SeenAt: at(2 * time.Hour)}, StateUnread},
	}
	for _, tc := range cases {
		if got := stateOf(tc.c, tc.entry, since, now); got != tc.want {
			t.Errorf("%s: state = %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestScan_AClosedUnreadSessionIsArchivedAndAResumeKeepsItsOldReplyRead(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	processes := fakeProcesses{alive: map[int]bool{}}
	scanner, stores := testScanner(t, noSQLite, processes)
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	scanner.Now = func() time.Time { return now }
	writeMarkedSession(t, stores, id, "2026-11-07T11:00:00.000Z")
	writeClaudeProcess(t, stores, 4242, id, "idle")
	processes.alive[4242] = true
	scanner.Scan(context.Background(), ScanOptions{})

	now = now.Add(time.Hour)
	writeMarkedSession(t, stores, id, "2026-11-07T12:30:00.000Z")
	view := scanner.Scan(context.Background(), ScanOptions{})
	if c := byID(t, view, id); c.State != StateUnread || c.Unread || view.NeedsYou != 1 {
		t.Fatalf("an open session with a new reply needs the person: %s, needsYou %d", asJSON(t, c), view.NeedsYou)
	}

	now = now.Add(freshSpan)
	processes.alive[4242] = false
	view = scanner.Scan(context.Background(), ScanOptions{})
	if c := byID(t, view, id); c.State != StateDormant || !c.Unread || view.NeedsYou != 0 {
		t.Fatalf("a closed session with an unread reply is archived with its marker: %s, needsYou %d", asJSON(t, c), view.NeedsYou)
	}
	if entry := storedMarks(t, scanner.MarksPath).Marks[markKey(ToolClaude, id)]; entry.SeenAt != "" || entry.ClosedWrittenAt != "2026-11-07T12:30:00Z" {
		t.Fatalf("the reply read while closed is kept, and not marked seen: %+v", entry)
	}

	now = now.Add(freshSpan)
	processes.alive[4242] = true
	view = scanner.Scan(context.Background(), ScanOptions{})
	if c := byID(t, view, id); c.State != StateActive || !c.Unread || view.NeedsYou != 0 {
		t.Fatalf("a manual resume does not count the old reply as new, and keeps its marker: %s, needsYou %d", asJSON(t, c), view.NeedsYou)
	}

	now = now.Add(freshSpan)
	writeMarkedSession(t, stores, id, "2026-11-07T13:05:00.000Z")
	view = scanner.Scan(context.Background(), ScanOptions{})
	if c := byID(t, view, id); c.State != StateUnread || view.NeedsYou != 1 {
		t.Fatalf("a reply after the resume needs the person: %s, needsYou %d", asJSON(t, c), view.NeedsYou)
	}
}

func TestScan_AFailedProcessReadKeepsAnUnreadSessionUnread(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	processes := fakeProcesses{readFails: true}
	scanner, stores := testScanner(t, noSQLite, processes)
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	scanner.Now = func() time.Time { return now }
	writeMarkedSession(t, stores, id, "2026-11-07T11:00:00.000Z")
	writeClaudeProcess(t, stores, 4242, id, "idle")
	scanner.Scan(context.Background(), ScanOptions{})

	now = now.Add(time.Hour)
	writeMarkedSession(t, stores, id, "2026-11-07T12:30:00.000Z")
	view := scanner.Scan(context.Background(), ScanOptions{})
	if c := byID(t, view, id); c.State != StateUnread || c.Unread || view.NeedsYou != 1 {
		t.Fatalf("a failed read proves no closure: %s, needsYou %d", asJSON(t, c), view.NeedsYou)
	}
	if entry := storedMarks(t, scanner.MarksPath).Marks[markKey(ToolClaude, id)]; entry.ClosedWrittenAt != "" {
		t.Fatalf("a failed read records no closed reply: %+v", entry)
	}
}

func TestStateOf_AppConversationsStayOpenWhateverTheirAge(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	since := now.Add(-90 * 24 * time.Hour)
	old := now.Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	seen := markEntry{SeenAt: now.Add(-time.Hour).Format(time.RFC3339)}
	for app, want := range map[App]State{
		AppDesktop: StateActive, AppChat: StateActive, AppCowork: StateActive, AppCloud: StateActive,
		AppIDE: StateDormant, AppBackground: StateDormant, AppCLI: StateDormant,
	} {
		if got := stateOf(Conversation{App: app, UpdatedAt: old}, seen, since, now); got != want {
			t.Errorf("a %s conversation quiet for a month: state = %s, want %s", app, got, want)
		}
	}
	archived := Conversation{App: AppDesktop, UpdatedAt: old, underAppArchive: true}
	if got := stateOf(archived, seen, since, now); got != StateDormant {
		t.Errorf("an app archive still archives: state = %s", got)
	}
	completed := markEntry{SeenAt: seen.SeenAt, CompletedAt: seen.SeenAt}
	if got := stateOf(Conversation{App: AppChat, UpdatedAt: old}, completed, since, now); got != StateCompleted {
		t.Errorf("an archive made here still archives: state = %s", got)
	}
}

func TestScan_AStaleCloudStatusStopsWorkingWithoutBeingArchived(t *testing.T) {
	const id = "cse_0123456789abcdef"
	scanner, _ := testScanner(t, noSQLite, fakeProcesses{})
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	scanner.Now = func() time.Time { return now }
	scanner.Cloud = listedCloud{{
		Tool: ToolClaude, App: AppCloud, ID: id, Title: "Port the billing job", UpdatedAt: "2026-11-01T09:00:00Z",
		Live: true, Activity: ActivityWorking, Open: Route{Kind: RouteLink, Label: "Open on claude.ai", Link: "https://claude.ai/code/" + id},
	}}
	c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), id)
	if c.Live || c.Activity != "" || c.State != StateActive {
		t.Errorf("a cloud session that says it works but wrote nothing for days is active, not working or archived: %s", asJSON(t, c))
	}
}

func TestScan_AWorkingHelperStaysListedWhenItsParentIsArchived(t *testing.T) {
	stores, marksPath := codexRunStores(t)
	writeClaudeProcess(t, stores, 5151, idCache, "idle")
	view := codexRunScanner(t, stores, marksPath, runningSeen()).Scan(context.Background(), ScanOptions{})
	parent := byID(t, view, idSafari)
	if parent.State != StateDormant || !parent.liveRead {
		t.Fatalf("the parent session no process holds is archived: %s", asJSON(t, parent))
	}
	if helpers := parent.Helpers; len(helpers) != 1 || helpers[0].ID != runSeen || helpers[0].State != helperWorking {
		t.Errorf("its running helper stays listed under it: %s", asJSON(t, helpers))
	}
}

func TestScan_AFailedReadAfterAClosureKeepsTheReplyUnread(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{alive: map[int]bool{}})
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	scanner.Now = func() time.Time { return now }
	writeMarkedSession(t, stores, id, "2026-11-07T11:00:00.000Z")
	writeClaudeProcess(t, stores, 4242, id, "idle")
	scanner.Scan(context.Background(), ScanOptions{})
	now = now.Add(time.Hour)
	writeMarkedSession(t, stores, id, "2026-11-07T12:30:00.000Z")
	if c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), id); c.State != StateDormant || !c.Unread {
		t.Fatalf("closed with an unread reply: %s", asJSON(t, c))
	}

	now = now.Add(freshSpan)
	scanner.Processes = fakeProcesses{readFails: true}
	view := scanner.Scan(context.Background(), ScanOptions{})
	if c := byID(t, view, id); c.State != StateUnread || view.NeedsYou != 1 {
		t.Errorf("a failed read keeps the reply unread and counted: %s, needsYou %d", asJSON(t, c), view.NeedsYou)
	}
}

func TestScan_AResumedSessionKeepsItsUnreadMarkerUntilRead(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	processes := fakeProcesses{alive: map[int]bool{}}
	scanner, stores := testScanner(t, noSQLite, processes)
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	scanner.Now = func() time.Time { return now }
	writeMarkedSession(t, stores, id, "2026-11-07T11:00:00.000Z")
	writeClaudeProcess(t, stores, 4242, id, "idle")
	scanner.Scan(context.Background(), ScanOptions{})
	now = now.Add(time.Hour)
	writeMarkedSession(t, stores, id, "2026-11-07T12:30:00.000Z")
	scanner.Scan(context.Background(), ScanOptions{})

	now = now.Add(freshSpan)
	processes.alive[4242] = true
	view := scanner.Scan(context.Background(), ScanOptions{})
	if c := byID(t, view, id); c.State != StateActive || !c.Unread || view.NeedsYou != 0 {
		t.Fatalf("resumed by hand: active, still marked unread, not counted: %s, needsYou %d", asJSON(t, c), view.NeedsYou)
	}

	now = now.Add(freshSpan)
	processes.alive[4242] = false
	if c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), id); c.State != StateDormant || !c.Unread {
		t.Fatalf("closed again with nothing new: archived and still unread: %s", asJSON(t, c))
	}

	if err := scanner.Mark(ToolClaude, id, MarkSeen); err != nil {
		t.Fatal(err)
	}
	now = now.Add(freshSpan)
	if c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), id); c.State != StateDormant || c.Unread {
		t.Errorf("marked read: archived without the marker: %s", asJSON(t, c))
	}
}

func TestScan_ASeenMarkInTheSecondOfTheReplyCoversIt(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{alive: map[int]bool{}})
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	now := time.Date(2026, 11, 7, 11, 0, 0, 0, time.UTC)
	scanner.Now = func() time.Time { return now }
	writeClaudeProcess(t, stores, 4242, idCache, "idle")
	writeMarkedSession(t, stores, id, "2026-11-07T10:00:00.000Z")
	scanner.Scan(context.Background(), ScanOptions{})
	writeMarkedSession(t, stores, id, "2026-11-07T12:00:00.500Z")
	now = time.Date(2026, 11, 7, 12, 0, 0, 900_000_000, time.UTC)
	if err := scanner.Mark(ToolClaude, id, MarkSeen); err != nil {
		t.Fatal(err)
	}
	now = now.Add(freshSpan)
	view := scanner.Scan(context.Background(), ScanOptions{})
	if c := byID(t, view, id); c.State != StateDormant || c.Unread || view.NeedsYou != 0 {
		t.Errorf("a seen mark in the reply's own second covers it: %s, needsYou %d", asJSON(t, c), view.NeedsYou)
	}
}

func TestScan_TheFirstPageHoldsAnOldClosedUnreadSession(t *testing.T) {
	const old, recent = "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{alive: map[int]bool{}})
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	if err := writeMarks(scanner.MarksPath, marksFile{SchemaVersion: marksSchemaVersion, Since: "2026-10-01T00:00:00Z", Marks: map[string]markEntry{}}); err != nil {
		t.Fatal(err)
	}
	writeMarkedSession(t, stores, old, "2026-10-20T10:00:00.000Z")
	writeMarkedSession(t, stores, recent, "2026-11-07T10:00:00.000Z")
	writeClaudeProcess(t, stores, 4242, idCache, "idle")

	view := scanner.Scan(context.Background(), ScanOptions{})
	if c := byID(t, view, old); c.State != StateDormant || !c.Unread {
		t.Errorf("the old closed session is archived with its marker on the first page: %s", asJSON(t, c))
	}
	before, err := time.Parse(time.RFC3339, view.From)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range scanner.Scan(context.Background(), ScanOptions{Before: before}).Conversations {
		if c.ID == old {
			t.Error("a later page lists it again")
		}
	}
}

func TestScan_ADesktopSessionResumedInATerminalFollowsTheDesktopRuleOnceClosed(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	processes := fakeProcesses{alive: map[int]bool{4242: true}}
	scanner, stores := testScanner(t, noSQLite, processes)
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	scanner.Now = func() time.Time { return now }
	write(t, filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", id+".jsonl"), lines(
		obj{"type": "user", "cwd": "/tmp/app", "entrypoint": "claude-desktop", "timestamp": "2026-11-07T10:00:00.000Z", "message": obj{"role": "user", "content": "Plan the release"}},
		obj{"type": "user", "cwd": "/tmp/app", "entrypoint": "cli", "timestamp": "2026-11-07T11:00:00.000Z", "message": obj{"role": "user", "content": "Go on from the terminal"}},
	))
	write(t, filepath.Join(stores.ClaudeDir, "sessions", "4242.json"), `{"pid":4242,"sessionId":"`+id+`","status":"idle","entrypoint":"cli"}`)
	if c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), id); c.App != AppCLI || !c.Live {
		t.Fatalf("while the terminal holds it, it is a terminal session: %s", asJSON(t, c))
	}
	now = now.Add(freshSpan)
	processes.alive[4242] = false
	if c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), id); c.App != AppDesktop || c.State != StateActive || c.Open.Kind != RouteLink {
		t.Errorf("once the terminal closes it follows the desktop rule: %s", asJSON(t, c))
	}
}
