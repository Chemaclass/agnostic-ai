package conversations

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStateOf_PlacesAConversationByToolReportThenMarksThenAge(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	since := now.Add(-48 * time.Hour)
	at := func(ago time.Duration) string { return now.Add(-ago).Format(time.RFC3339) }
	cases := []struct {
		name  string
		c     Conversation
		entry markEntry
		want  State
	}{
		{"a live chat on a prompt nobody looked at is blocked", Conversation{UpdatedAt: at(time.Hour), Live: true, Activity: ActivityWaiting}, markEntry{SeenAt: at(2 * time.Hour)}, StateBlocked},
		{"a live chat on a prompt, never seen and written after the marks began, is blocked", Conversation{UpdatedAt: at(time.Hour), Live: true, Activity: ActivityWaiting}, markEntry{}, StateBlocked},
		{"a prompt the person looked at no longer needs them", Conversation{UpdatedAt: at(time.Hour), Live: true, Activity: ActivityWaiting}, markEntry{SeenAt: at(time.Minute)}, StateActive},
		{"a prompt older than the marks no longer needs them", Conversation{UpdatedAt: at(72 * time.Hour), Live: true, Activity: ActivityWaiting}, markEntry{}, StateActive},
		{"archiving a waiting chat holds until its next write", Conversation{UpdatedAt: at(time.Hour), Live: true, Activity: ActivityWaiting}, markEntry{SeenAt: at(time.Minute), CompletedAt: at(time.Minute)}, StateCompleted},
		{"a live busy chat is working, not unread", Conversation{UpdatedAt: at(time.Minute), Live: true, Activity: ActivityWorking}, markEntry{SeenAt: at(time.Hour)}, StateWorking},
		{"completed with nothing written since", Conversation{UpdatedAt: at(2 * time.Hour)}, markEntry{SeenAt: at(time.Hour), CompletedAt: at(time.Hour)}, StateCompleted},
		{"completed in the second of its last write", Conversation{UpdatedAt: at(time.Hour)}, markEntry{CompletedAt: at(time.Hour)}, StateCompleted},
		{"written after it was seen", Conversation{UpdatedAt: at(time.Minute)}, markEntry{SeenAt: at(time.Hour)}, StateUnread},
		{"never seen, written after the marks began", Conversation{UpdatedAt: at(47 * time.Hour)}, markEntry{}, StateUnread},
		{"never seen, written before the marks began", Conversation{UpdatedAt: at(72 * time.Hour)}, markEntry{}, StateDormant},
		{"seen and open in an idle process", Conversation{UpdatedAt: at(72 * time.Hour), Live: true, Activity: ActivityIdle}, markEntry{SeenAt: at(time.Hour)}, StateActive},
		{"seen, closed and written within a day", Conversation{UpdatedAt: at(23 * time.Hour)}, markEntry{SeenAt: at(time.Hour)}, StateActive},
		{"seen, closed and quiet for a day", Conversation{UpdatedAt: at(24 * time.Hour)}, markEntry{SeenAt: at(time.Hour)}, StateDormant},
	}
	for _, tc := range cases {
		if got := stateOf(tc.c, tc.entry, since, now); got != tc.want {
			t.Errorf("%s: state = %s, want %s", tc.name, got, tc.want)
		}
	}
	if got := stateOf(Conversation{UpdatedAt: at(time.Hour)}, markEntry{SeenAt: at(47 * time.Hour)}, since.Add(47*time.Hour+30*time.Minute), now); got != StateActive {
		t.Errorf("a baseline later than the seen mark counts: state = %s", got)
	}
	if got := stateOf(Conversation{UpdatedAt: at(time.Minute)}, markEntry{}, time.Time{}, now); got != StateActive {
		t.Errorf("without a marks file nothing is unread: state = %s", got)
	}
	if got := stateOf(Conversation{UpdatedAt: at(time.Hour), Live: true, Activity: ActivityWaiting}, markEntry{}, time.Time{}, now); got != StateBlocked {
		t.Errorf("without a marks file a waiting chat is still blocked: state = %s", got)
	}
}

func writeMarkedSession(t *testing.T, stores Stores, id, timestamp string) {
	t.Helper()
	line, err := json.Marshal(map[string]any{
		"type": "user", "cwd": "/tmp/app", "entrypoint": "cli", "timestamp": timestamp,
		"message": map[string]any{"role": "user", "content": "Fix the parser"},
	})
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", id+".jsonl"), string(line)+"\n")
}

func storedMarks(t *testing.T, path string) marksFile {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var file marksFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestScan_ANewMessageDeletesTheCompletedMarkAndCountsAsUnread(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	scanner.MarksPath = filepath.Join(t.TempDir(), "state", "conversation-marks.json")
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	scanner.Now = func() time.Time { return now }
	writeMarkedSession(t, stores, id, "2026-11-07T11:00:00.000Z")

	// The first scan starts the marks file: what was written before it counts as seen.
	view := scanner.Scan(context.Background(), ScanOptions{})
	if c := byID(t, view, id); c.State != StateActive || view.NeedsYou != 0 {
		t.Fatalf("before any mark: state = %s, needsYou = %d", c.State, view.NeedsYou)
	}
	if since := storedMarks(t, scanner.MarksPath).Since; since != "2026-11-07T12:00:00Z" {
		t.Fatalf("since = %q", since)
	}

	now = now.Add(time.Minute)
	if err := scanner.Mark(ToolClaude, id, MarkCompleted); err != nil {
		t.Fatal(err)
	}
	view = scanner.Scan(context.Background(), ScanOptions{})
	c := byID(t, view, id)
	if c.State != StateCompleted || c.CompletedAt != "2026-11-07T12:01:00Z" || c.SeenAt != "2026-11-07T12:01:00Z" || view.NeedsYou != 0 {
		t.Fatalf("after completing: %s", asJSON(t, c))
	}

	// The agent writes again: the completion no longer stands.
	now = now.Add(time.Hour)
	writeMarkedSession(t, stores, id, "2026-11-07T12:30:00.000Z")
	view = scanner.Scan(context.Background(), ScanOptions{})
	c = byID(t, view, id)
	if c.State != StateUnread || c.CompletedAt != "" || view.NeedsYou != 1 {
		t.Fatalf("after a new message: %s, needsYou = %d", asJSON(t, c), view.NeedsYou)
	}
	entry := storedMarks(t, scanner.MarksPath).Marks[markKey(ToolClaude, id)]
	if entry.CompletedAt != "" || entry.SeenAt != "2026-11-07T12:01:00Z" {
		t.Fatalf("stored mark after a new message = %+v", entry)
	}

	if err := scanner.Mark(ToolClaude, id, MarkSeen); err != nil {
		t.Fatal(err)
	}
	view = scanner.Scan(context.Background(), ScanOptions{})
	if c := byID(t, view, id); c.State != StateActive || view.NeedsYou != 0 {
		t.Fatalf("after seeing it: state = %s, needsYou = %d", c.State, view.NeedsYou)
	}
}

func TestMark_OpenTakesACompletionBack(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	writeMarkedSession(t, stores, id, "2026-11-07T11:00:00.000Z")
	for _, mark := range []Mark{MarkCompleted, MarkOpen} {
		if err := scanner.Mark(ToolClaude, id, mark); err != nil {
			t.Fatal(err)
		}
	}
	if c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), id); c.State != StateActive || c.CompletedAt != "" {
		t.Fatalf("after reopening: %s", asJSON(t, c))
	}
}

func TestScan_SaysWhenTheMarksFileCannotBeRead(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	write(t, scanner.MarksPath, "not json")
	writeMarkedSession(t, stores, "11111111-1111-4111-8111-111111111111", "2026-11-07T11:00:00.000Z")
	view := scanner.Scan(context.Background(), ScanOptions{})
	if view.MarksNote == "" || len(view.Conversations) != 1 || view.Conversations[0].State != StateActive {
		t.Fatalf("view = %s", asJSON(t, view))
	}
	if raw, _ := os.ReadFile(scanner.MarksPath); string(raw) != "not json" {
		t.Fatalf("an unreadable marks file was overwritten: %q", raw)
	}
	if err := scanner.Mark(ToolClaude, "11111111-1111-4111-8111-111111111111", MarkSeen); err == nil {
		t.Fatal("marking wrote over an unreadable marks file")
	}
}

func TestScan_LeavesOutAHiddenConversationUntilItsNextMessage(t *testing.T) {
	const hidden, other = "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	scanner.Now = func() time.Time { return now }
	writeMarkedSession(t, stores, hidden, "2026-11-07T11:00:00.000Z")
	writeMarkedSession(t, stores, other, "2026-11-07T11:30:00.000Z")
	scanner.Scan(context.Background(), ScanOptions{})

	if err := scanner.Mark(ToolClaude, hidden, MarkHidden); err != nil {
		t.Fatal(err)
	}

	listed := func(view View) []string {
		var ids []string
		for _, c := range view.Conversations {
			ids = append(ids, c.ID)
		}
		return ids
	}
	view := scanner.Scan(context.Background(), ScanOptions{})
	if ids := listed(view); len(ids) != 1 || ids[0] != other {
		t.Fatalf("the list after hiding = %v", ids)
	}
	if view.Tools[0].Count != 1 {
		t.Errorf("the tool counts %d conversations, want 1: a hidden one is not counted", view.Tools[0].Count)
	}
	search := scanner.Scan(context.Background(), ScanOptions{Query: "parser"})
	if ids := listed(search); len(ids) != 1 || *search.Matches != 1 {
		t.Errorf("a search after hiding = %v, matches %d", ids, *search.Matches)
	}
	if _, found := scanner.Find(context.Background(), ToolClaude, hidden, ScanOptions{}); !found {
		t.Error("the tool's own store lost the hidden conversation")
	}

	writeMarkedSession(t, stores, hidden, "2026-11-07T12:20:00.000Z")
	now = now.Add(30 * time.Minute)
	view = scanner.Scan(context.Background(), ScanOptions{})
	if c := byID(t, view, hidden); c.State != StateUnread || view.NeedsYou != 1 {
		t.Fatalf("after a new message: state %s, needsYou %d", c.State, view.NeedsYou)
	}
	if entry := storedMarks(t, scanner.MarksPath).Marks[markKey(ToolClaude, hidden)]; entry.HiddenAt != "" {
		t.Errorf("the file still hides it at %q after a new message", entry.HiddenAt)
	}
}

func TestReopen_TakesACompletionBack(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	scanner.Now = func() time.Time { return now }
	writeMarkedSession(t, stores, id, "2026-11-07T11:00:00.000Z")
	if err := scanner.Mark(ToolClaude, id, MarkCompleted); err != nil {
		t.Fatal(err)
	}

	reply, found := scanner.Reopen(context.Background(), ToolClaude, id, ScanOptions{}, &fakeLauncher{})

	if !found || !reply.Opened {
		t.Fatalf("reopen = %+v, found %t", reply, found)
	}
	if c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), id); c.State != StateActive || c.CompletedAt != "" || c.SeenAt != "2026-11-07T12:00:00Z" {
		t.Errorf("after opening a completed conversation: state %s, completedAt %q, seenAt %q", c.State, c.CompletedAt, c.SeenAt)
	}
}
