package conversations

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	writeSessionSaying(t, stores, id, timestamp, "Fix the parser")
}

func writeSessionSaying(t *testing.T, stores Stores, id, timestamp, prompt string) {
	t.Helper()
	line, err := json.Marshal(map[string]any{
		"type": "user", "cwd": "/tmp/app", "entrypoint": "cli", "timestamp": timestamp,
		"message": map[string]any{"role": "user", "content": prompt},
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

func TestMark_TwoServersSharingTheMarksFileLoseNoMark(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conversation-marks.json")
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	server := func() *Scanner {
		scanner, _ := testScanner(t, noSQLite, fakeProcesses{})
		scanner.MarksPath = path
		scanner.Now = func() time.Time { return now }
		return scanner
	}
	// The companion's server and the dashboard's: two scanners, one file.
	companion, dashboard := server(), server()
	const each = 40
	var wait sync.WaitGroup
	failed := make(chan error, 2*each)
	for i := range each {
		wait.Go(func() {
			if err := companion.Mark(ToolClaude, fmt.Sprintf("companion-%d", i), MarkCompleted); err != nil {
				failed <- err
			}
		})
		wait.Go(func() {
			if err := dashboard.Mark(ToolCodex, fmt.Sprintf("dashboard-%d", i), MarkHidden); err != nil {
				failed <- err
			}
		})
	}
	wait.Wait()
	close(failed)
	for err := range failed {
		t.Fatal(err)
	}

	kept := storedMarks(t, path).Marks
	for i := range each {
		if kept[markKey(ToolClaude, fmt.Sprintf("companion-%d", i))].CompletedAt == "" {
			t.Errorf("the companion's completion %d was lost", i)
		}
		if kept[markKey(ToolCodex, fmt.Sprintf("dashboard-%d", i))].HiddenAt == "" {
			t.Errorf("the dashboard's hiding %d was lost", i)
		}
	}
}

func TestLockMarks_WaitsForTheOtherHolderAndFreesTheLockWhenReleased(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conversation-marks.json")
	release, err := lockMarks(path)
	if err != nil {
		t.Fatal(err)
	}
	taken := make(chan struct{})
	go func() {
		again, err := lockMarks(path)
		if err == nil {
			again()
		}
		close(taken)
	}()
	select {
	case <-taken:
		t.Fatal("a second holder took the lock while the first held it")
	case <-time.After(100 * time.Millisecond):
	}
	release()
	select {
	case <-taken:
	case <-time.After(2 * time.Second):
		t.Fatal("the lock was not taken after its holder released it")
	}
}

func renamingScanner(t *testing.T) (*Scanner, Stores, *time.Time) {
	t.Helper()
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	scanner.Now = func() time.Time { return now }
	return scanner, stores, &now
}

func TestRename_ShowsTheChosenTitleAndKeepsItAfterANewMessage(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	scanner, stores, now := renamingScanner(t)
	writeSessionSaying(t, stores, id, "2026-11-07T11:00:00.000Z", "/clear")
	scanner.Scan(context.Background(), ScanOptions{})

	if err := scanner.Rename(ToolClaude, id, "Parser fix"); err != nil {
		t.Fatal(err)
	}

	if c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), id); c.Title != "Parser fix" {
		t.Fatalf("title after renaming = %q", c.Title)
	}
	*now = now.Add(time.Hour)
	writeSessionSaying(t, stores, id, "2026-11-07T12:30:00.000Z", "/clear")
	c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), id)
	if c.Title != "Parser fix" || c.State != StateUnread {
		t.Errorf("after a new message: title %q, state %s", c.Title, c.State)
	}
	if entry := storedMarks(t, scanner.MarksPath).Marks[markKey(ToolClaude, id)]; entry.Title != "Parser fix" {
		t.Errorf("stored entry = %+v", entry)
	}
}

func TestRename_KeepsTheStateSeenAndCompletedMarks(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	scanner, stores, now := renamingScanner(t)
	writeMarkedSession(t, stores, id, "2026-11-07T11:00:00.000Z")
	scanner.Scan(context.Background(), ScanOptions{})
	if err := scanner.Mark(ToolClaude, id, MarkCompleted); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(time.Minute)

	if err := scanner.Rename(ToolClaude, id, "Parser fix"); err != nil {
		t.Fatal(err)
	}

	c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), id)
	if c.Title != "Parser fix" || c.State != StateCompleted || c.SeenAt != "2026-11-07T12:00:00Z" || c.CompletedAt != "2026-11-07T12:00:00Z" {
		t.Errorf("after renaming a completed conversation: %s", asJSON(t, c))
	}
}

func TestRename_KeepsAHiddenConversationHidden(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	scanner, stores, _ := renamingScanner(t)
	writeMarkedSession(t, stores, id, "2026-11-07T11:00:00.000Z")
	if err := scanner.Mark(ToolClaude, id, MarkHidden); err != nil {
		t.Fatal(err)
	}

	if err := scanner.Rename(ToolClaude, id, "Parser fix"); err != nil {
		t.Fatal(err)
	}

	if view := scanner.Scan(context.Background(), ScanOptions{}); len(view.Conversations) != 0 {
		t.Errorf("a rename brought a hidden conversation back: %s", asJSON(t, view.Conversations))
	}
}

func TestCleanTitle_KeepsOneLineAndRefusesWhatItCannotShow(t *testing.T) {
	if title, err := CleanTitle("  Parser \n\t fix  "); err != nil || title != "Parser fix" {
		t.Errorf("CleanTitle = %q, %v", title, err)
	}
	if title, err := CleanTitle(strings.Repeat("é", titleRunes)); err != nil || title != strings.Repeat("é", titleRunes) {
		t.Errorf("a title of %d characters = %q, %v", titleRunes, title, err)
	}
	for typed, refusal := range map[string]string{
		"":                          "the name is empty",
		" \n\t ":                    "the name is empty",
		strings.Repeat("a", 121):    "the name is longer than 120 characters",
		"Use password hunter2 here": "the name may hold a password or key, so it is not saved",
	} {
		if _, err := CleanTitle(typed); err == nil || err.Error() != refusal {
			t.Errorf("CleanTitle(%.20q) = %v, want %q", typed, err, refusal)
		}
	}
}

func TestRename_ShowsATitleThatWasWithheld(t *testing.T) {
	const renamed, twin = "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
	scanner, stores, _ := renamingScanner(t)
	// Two chats with the same texts share what the scan worked out for them.
	writeSessionSaying(t, stores, renamed, "2026-11-07T11:00:00.000Z", "Use password hunter2 for the staging login")
	writeSessionSaying(t, stores, twin, "2026-11-07T11:00:00.000Z", "Use password hunter2 for the staging login")

	if err := scanner.Rename(ToolClaude, renamed, "Staging login"); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		view := scanner.Scan(context.Background(), ScanOptions{})
		if c := byID(t, view, renamed); c.Title != "Staging login" || strings.Join(c.Withheld, ",") != "opening" {
			t.Errorf("the renamed chat: title %q, withheld %v", c.Title, c.Withheld)
		}
		if c := byID(t, view, twin); c.Title != "" || strings.Join(c.Withheld, ",") != "title,opening" {
			t.Errorf("the other chat: title %q, withheld %v", c.Title, c.Withheld)
		}
	}
}

func TestScan_SearchFindsARenamedConversationByItsNewName(t *testing.T) {
	const claude, recentCursor, oldCursor = "11111111-1111-4111-8111-111111111111", "88888888-8888-4888-8888-888888888888", "99999999-9999-4999-8999-999999999999"
	scanner, stores := testScanner(t, fakeStores(t, map[string]string{
		"state.vscdb|FROM composerHeaders": `[
			{"id":"` + recentCursor + `","workspace_id":"ws1","created_at":1794049200000,"recency":1794049200000,"name":"Cursor chat","folder":"/tmp/app","branch":""},
			{"id":"` + oldCursor + `","workspace_id":"ws1","created_at":1700000000000,"recency":1700000000000,"name":"Old chat","folder":"/tmp/app","branch":""}
		]`,
		"state.vscdb|chosen(composerId)": `[
			{"id":"` + recentCursor + `","first_text":"Start here","last_text":"Go on"},
			{"id":"` + oldCursor + `","first_text":"Start there","last_text":"Keep going"}
		]`,
	}), fakeProcesses{})
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	write(t, stores.CursorDB, "")
	writeSessionSaying(t, stores, claude, "2026-11-07T11:00:00.000Z", "/clear")
	for id, title := range map[string]string{recentCursor: "Billing webhooks", oldCursor: "Invoice export"} {
		if err := scanner.Rename(ToolCursor, id, title); err != nil {
			t.Fatal(err)
		}
	}
	if err := scanner.Rename(ToolClaude, claude, "Parser fix"); err != nil {
		t.Fatal(err)
	}

	if c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), recentCursor); c.Title != "Billing webhooks" {
		t.Errorf("a Cursor chat whose prompts are read for the page shows %q", c.Title)
	}
	for query, id := range map[string]string{"parser fix": claude, "billing webhooks": recentCursor, "invoice export": oldCursor} {
		view := scanner.Scan(context.Background(), ScanOptions{Query: query})
		if *view.Matches != 1 || view.Conversations[0].ID != id {
			t.Errorf("search %q = %s", query, asJSON(t, view.Conversations))
		}
	}
}

func TestRename_TwoServersSharingTheMarksFileAgree(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	path := filepath.Join(t.TempDir(), "conversation-marks.json")
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	companion, stores := testScanner(t, noSQLite, fakeProcesses{})
	dashboard := &Scanner{Stores: stores, SQLite: noSQLite, Processes: fakeProcesses{}}
	for _, scanner := range []*Scanner{companion, dashboard} {
		scanner.MarksPath = path
		scanner.Now = func() time.Time { return now }
	}
	writeSessionSaying(t, stores, id, "2026-11-07T11:00:00.000Z", "/clear")
	dashboard.Scan(context.Background(), ScanOptions{})

	if err := companion.Rename(ToolClaude, id, "Parser fix"); err != nil {
		t.Fatal(err)
	}
	if c := byID(t, dashboard.Scan(context.Background(), ScanOptions{}), id); c.Title != "Parser fix" {
		t.Fatalf("the other server shows %q", c.Title)
	}
	if err := dashboard.Mark(ToolClaude, id, MarkCompleted); err != nil {
		t.Fatal(err)
	}

	c := byID(t, companion.Scan(context.Background(), ScanOptions{}), id)
	if c.Title != "Parser fix" || c.State != StateCompleted {
		t.Errorf("after the other server's completion: title %q, state %s", c.Title, c.State)
	}
	if view := dashboard.Scan(context.Background(), ScanOptions{Query: "parser fix"}); *view.Matches != 1 {
		t.Errorf("the other server's search found %d", *view.Matches)
	}
}

func writeClaudeProcess(t *testing.T, stores Stores, pid int, id, status string) {
	t.Helper()
	write(t, filepath.Join(stores.ClaudeDir, "sessions", fmt.Sprintf("%d.json", pid)), fmt.Sprintf(`{"pid":%d,"sessionId":%q,"status":%q}`, pid, id, status))
}

func TestScan_AWorkingConversationLeavesTheArchiveOnEveryServer(t *testing.T) {
	const resumed, other = "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
	path := filepath.Join(t.TempDir(), "conversation-marks.json")
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	processes := fakeProcesses{alive: map[int]bool{4242: true}}
	companion, stores := testScanner(t, noSQLite, processes)
	dashboard := &Scanner{Stores: stores, SQLite: noSQLite, Processes: processes}
	for _, scanner := range []*Scanner{companion, dashboard} {
		scanner.MarksPath = path
		scanner.Now = func() time.Time { return now }
	}
	writeMarkedSession(t, stores, resumed, "2026-11-07T11:00:00.000Z")
	writeMarkedSession(t, stores, other, "2026-11-07T11:30:00.000Z")
	companion.Scan(context.Background(), ScanOptions{})
	if err := companion.Rename(ToolClaude, resumed, "Parser fix"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	for _, id := range []string{resumed, other} {
		if err := companion.Mark(ToolClaude, id, MarkCompleted); err != nil {
			t.Fatal(err)
		}
	}
	archived := storedMarks(t, path).Marks

	now = now.Add(time.Minute)
	writeClaudeProcess(t, stores, 4242, resumed, "busy")
	view := companion.Scan(context.Background(), ScanOptions{})
	if c := byID(t, view, resumed); c.State != StateWorking || c.CompletedAt != "" {
		t.Fatalf("the resumed conversation on the companion: %s", asJSON(t, c))
	}
	if c := byID(t, view, other); c.State != StateCompleted {
		t.Errorf("the other archived conversation is %s", c.State)
	}
	marks := storedMarks(t, path).Marks
	entry := marks[markKey(ToolClaude, resumed)]
	if entry.CompletedAt != "" || entry.SeenAt != archived[markKey(ToolClaude, resumed)].SeenAt || entry.Title != "Parser fix" {
		t.Errorf("the resumed conversation's marks: %+v", entry)
	}
	if marks[markKey(ToolClaude, other)] != archived[markKey(ToolClaude, other)] {
		t.Errorf("the other conversation's marks changed: %+v", marks[markKey(ToolClaude, other)])
	}

	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	dashboard.Scan(context.Background(), ScanOptions{})
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Error("a scan while the conversation still works wrote the marks file again")
	}

	writeClaudeProcess(t, stores, 4242, resumed, "idle")
	for name, scanner := range map[string]*Scanner{"companion": companion, "dashboard": dashboard} {
		c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), resumed)
		if c.State != StateActive || c.CompletedAt != "" || c.Title != "Parser fix" {
			t.Errorf("after the work ends on the %s: %s", name, asJSON(t, c))
		}
	}
}

func TestScan_AWorkingConversationLeavesHiding(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{alive: map[int]bool{4242: true}})
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	scanner.Now = func() time.Time { return now }
	writeMarkedSession(t, stores, id, "2026-11-07T11:00:00.000Z")
	scanner.Scan(context.Background(), ScanOptions{})
	for _, mark := range []Mark{MarkCompleted, MarkHidden} {
		if err := scanner.Mark(ToolClaude, id, mark); err != nil {
			t.Fatal(err)
		}
	}
	if view := scanner.Scan(context.Background(), ScanOptions{}); len(view.Conversations) != 0 {
		t.Fatalf("the hidden conversation is listed: %s", asJSON(t, view.Conversations))
	}

	now = now.Add(time.Minute)
	writeClaudeProcess(t, stores, 4242, id, "busy")
	if c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), id); c.State != StateWorking {
		t.Fatalf("the resumed conversation is %s", c.State)
	}
	if entry := storedMarks(t, scanner.MarksPath).Marks[markKey(ToolClaude, id)]; entry.HiddenAt != "" || entry.CompletedAt != "" || entry.SeenAt == "" {
		t.Errorf("the resumed conversation's marks: %+v", entry)
	}
}

// duringRead runs read once, while the scan asks which processes are alive.
type duringRead struct {
	fakeProcesses
	read func()
}

func (d *duringRead) AliveAmong(processes []ProcessRef) map[int]bool {
	if d.read != nil {
		d.read()
		d.read = nil
	}
	return d.fakeProcesses.AliveAmong(processes)
}

func TestScan_KeepsAnArchiveMadeAfterItsReadBegan(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	path := filepath.Join(t.TempDir(), "conversation-marks.json")
	now := time.Date(2026, 11, 7, 12, 0, 0, 500_000_000, time.UTC)
	processes := &duringRead{fakeProcesses: fakeProcesses{alive: map[int]bool{4242: true}}}
	dashboard, stores := testScanner(t, noSQLite, processes)
	companion := &Scanner{Stores: stores, SQLite: noSQLite, Processes: fakeProcesses{}}
	for _, scanner := range []*Scanner{companion, dashboard} {
		scanner.MarksPath = path
		scanner.Now = func() time.Time { return now }
	}
	writeMarkedSession(t, stores, id, "2026-11-07T11:00:00.000Z")
	writeClaudeProcess(t, stores, 4242, id, "busy")
	dashboard.Scan(context.Background(), ScanOptions{})

	now = now.Add(time.Minute)
	processes.read = func() {
		now = now.Add(time.Second)
		if err := companion.Mark(ToolClaude, id, MarkCompleted); err != nil {
			t.Fatal(err)
		}
	}
	dashboard.Scan(context.Background(), ScanOptions{})
	if entry := storedMarks(t, path).Marks[markKey(ToolClaude, id)]; entry.CompletedAt != "2026-11-07T12:01:01Z" {
		t.Fatalf("a scan that read the tools before the archive cleared it: %+v", entry)
	}

	now = now.Add(time.Second)
	dashboard.Scan(context.Background(), ScanOptions{})
	if entry := storedMarks(t, path).Marks[markKey(ToolClaude, id)]; entry.CompletedAt != "" {
		t.Errorf("a scan that read the tools after the archive kept it: %+v", entry)
	}
}

func TestScan_KeepsAnArchiveMadeAfterAKeptWorkingRowWasRead(t *testing.T) {
	scanner, _ := testScanner(t, noSQLite, fakeProcesses{})
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	now := time.Date(2026, 11, 7, 12, 0, 1, 0, time.UTC)
	scanner.Now = func() time.Time { return now }
	working := func() ([]Conversation, error) {
		row := cloudRow("cse_a1", "Fix the parser", "2026-11-07T11:59:00Z")
		row.Live, row.Activity = true, ActivityWorking
		return []Conversation{row}, nil
	}
	scanner.Cloud = &countingCloud{answers: []func() ([]Conversation, error){working, working}}
	scanner.Scan(context.Background(), ScanOptions{})

	now = now.Add(9 * time.Second)
	if err := scanner.Mark(ToolClaude, "cse_a1", MarkCompleted); err != nil {
		t.Fatal(err)
	}
	now = now.Add(20 * time.Second)
	scanner.Scan(context.Background(), ScanOptions{})
	if entry := storedMarks(t, scanner.MarksPath).Marks[markKey(ToolClaude, "cse_a1")]; entry.CompletedAt != "2026-11-07T12:00:10Z" {
		t.Fatalf("a working row read before the archive cleared it: %+v", entry)
	}

	now = now.Add(time.Minute)
	if c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), "cse_a1"); c.State != StateWorking {
		t.Fatalf("the fresh read shows %s", c.State)
	}
	if entry := storedMarks(t, scanner.MarksPath).Marks[markKey(ToolClaude, "cse_a1")]; entry.CompletedAt != "" {
		t.Errorf("a working row read after the archive kept it: %+v", entry)
	}
}
