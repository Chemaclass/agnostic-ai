package conversations

import (
	"context"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// Chats archived in their own app show under Archived (state dormant)
// until something is written in them or their tool says they work.

const archivedTaskID = "local_11111111-1111-4111-8111-111111111111"

// writeCoworkTask stores one Cowork task in the Claude app's cache.
func writeCoworkTask(t *testing.T, stores Stores, active string, running, archived bool) {
	t.Helper()
	writeClaudeApp(t, stores, obj{"queryKey": []any{"cowork-local-sessions", "org"}, "state": obj{"data": obj{"v": 1.0, "rows": []any{
		coworkTask(archivedTaskID, "Archived task", active, running, archived),
	}}}})
}

// scanAt scans as of at, past the span in which a read is kept.
func scanAt(t *testing.T, scanner *Scanner, at time.Time) View {
	t.Helper()
	scanner.Now = func() time.Time { return at }
	return scanner.Scan(context.Background(), ScanOptions{})
}

func TestScan_AChatArchivedInItsAppStaysArchivedWhileNothingIsWritten(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	start := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	writeCoworkTask(t, stores, "2026-11-07T11:59:00Z", false, true)

	for i := range 3 {
		if c := byID(t, scanAt(t, scanner, start.Add(time.Duration(i)*2*time.Minute)), archivedTaskID); c.State != StateDormant {
			t.Fatalf("read %d: a task archived in the app is %s, want dormant", i+1, c.State)
		}
	}
}

func TestScan_ANewMessageTakesAChatOutOfItsAppsArchive(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	start := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	writeCoworkTask(t, stores, "2026-11-07T11:59:00Z", false, true)
	if c := byID(t, scanAt(t, scanner, start), archivedTaskID); c.State != StateDormant {
		t.Fatalf("before the message: %s, want dormant", c.State)
	}

	// The app still says archived, but a message came after the archive.
	writeCoworkTask(t, stores, "2026-11-07T12:01:00Z", false, true)
	if c := byID(t, scanAt(t, scanner, start.Add(2*time.Minute)), archivedTaskID); c.State != StateActive {
		t.Fatalf("after the message: %s, want active", c.State)
	}
	if c := byID(t, scanAt(t, scanner, start.Add(4*time.Minute)), archivedTaskID); c.State != StateActive {
		t.Fatalf("a later read put it back under Archived: %s", c.State)
	}
}

func TestScan_WorkBeatsAnAppArchiveAndEndsIt(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	start := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	writeCoworkTask(t, stores, "2026-11-07T11:59:00Z", false, true)
	scanAt(t, scanner, start)

	writeCoworkTask(t, stores, "2026-11-07T11:59:00Z", true, true)
	if c := byID(t, scanAt(t, scanner, start.Add(2*time.Minute)), archivedTaskID); c.State != StateWorking {
		t.Fatalf("while it works: %s, want working", c.State)
	}
	writeCoworkTask(t, stores, "2026-11-07T11:59:00Z", false, true)
	if c := byID(t, scanAt(t, scanner, start.Add(4*time.Minute)), archivedTaskID); c.State != StateActive {
		t.Fatalf("after the work: %s, want active, as after a completion the work cleared", c.State)
	}
}

func TestScan_UnarchivingInTheAppAndArchivingAgainFollowsTheApp(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	start := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	writeCoworkTask(t, stores, "2026-11-07T11:59:00Z", false, true)
	scanAt(t, scanner, start)
	writeCoworkTask(t, stores, "2026-11-07T12:01:00Z", false, true)
	scanAt(t, scanner, start.Add(2*time.Minute))

	writeCoworkTask(t, stores, "2026-11-07T12:01:00Z", false, false)
	if c := byID(t, scanAt(t, scanner, start.Add(4*time.Minute)), archivedTaskID); c.State != StateActive {
		t.Fatalf("unarchived in the app: %s, want active", c.State)
	}
	writeCoworkTask(t, stores, "2026-11-07T12:01:00Z", false, true)
	if c := byID(t, scanAt(t, scanner, start.Add(6*time.Minute)), archivedTaskID); c.State != StateDormant {
		t.Fatalf("archived again in the app: %s, want dormant", c.State)
	}
}

func TestScan_AnUnarchiveSeenOnlyByALookupStillStartsANewArchive(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	start := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	writeCoworkTask(t, stores, "2026-11-07T11:59:00Z", false, true)
	scanAt(t, scanner, start)
	writeCoworkTask(t, stores, "2026-11-07T12:01:00Z", false, true)
	scanAt(t, scanner, start.Add(2*time.Minute))

	// Only the lookup behind an open, mark or rename reads the unarchive.
	writeCoworkTask(t, stores, "2026-11-07T12:01:00Z", false, false)
	scanner.Now = func() time.Time { return start.Add(4 * time.Minute) }
	if _, found := scanner.Find(context.Background(), ToolClaude, archivedTaskID, ScanOptions{}); !found {
		t.Fatal("the task is not found")
	}
	writeCoworkTask(t, stores, "2026-11-07T12:01:00Z", false, true)
	if c := byID(t, scanAt(t, scanner, start.Add(6*time.Minute)), archivedTaskID); c.State != StateDormant {
		t.Fatalf("archived again after an unarchive only a lookup saw: %s, want dormant", c.State)
	}
}

func TestScan_ACodexThreadComparesWritesWithItsOwnArchiveTime(t *testing.T) {
	quiet, written := "55555555-5555-4555-8555-555555555555", "66666666-6666-4666-8666-666666666666"
	archivedAt := time.Date(2026, 11, 7, 11, 0, 0, 0, time.UTC)
	thread := func(id string, updated time.Time) string {
		return `{"id":"` + id + `","cwd":"/tmp/app","created_at_ms":1762500000000,"updated_at_ms":` + strconv.FormatInt(updated.UnixMilli(), 10) +
			`,"originator":"Codex Desktop","source":"vscode","name":"Thread","archived":1,"archived_at":` + strconv.FormatInt(archivedAt.Unix(), 10) + `}`
	}
	scanner, stores := testScanner(t, fakeStores(t, map[string]string{
		// Codex keeps the archive in whole seconds: a write in its own second came before it.
		"state_5.sqlite|FROM threads":          "[" + thread(quiet, archivedAt.Add(400*time.Millisecond)) + "," + thread(written, archivedAt.Add(30*time.Minute)) + "]",
		"thread_history_1.sqlite|thread_items": `[]`,
	}), fakeProcesses{})
	write(t, filepath.Join(stores.CodexDir, "state_5.sqlite"), "")
	write(t, filepath.Join(stores.CodexDir, "thread_history_1.sqlite"), "")

	// Both were first read after the write, so only Codex's time tells them apart.
	view := scanner.Scan(context.Background(), ScanOptions{})
	if c := byID(t, view, quiet); c.State != StateDormant {
		t.Errorf("thread archived after its last write: %s, want dormant", c.State)
	}
	if c := byID(t, view, written); c.State != StateActive {
		t.Errorf("thread written after Codex archived it: %s, want active", c.State)
	}
}

func TestScan_ListsCursorChatsArchivedInCursorAsArchived(t *testing.T) {
	scanner, stores := testScanner(t, nil, fakeProcesses{})
	scanner.SQLite = RunSQLite
	recency := time.Date(2026, 11, 7, 11, 0, 0, 0, time.UTC).UnixMilli()
	cursorEditorStore(t, stores.CursorDB, recency, map[string][]cursorBubble{
		"a1111111": {cursorUser("Kept"), cursorAgent("Done.")},
		"b1111111": {cursorUser("Archived"), cursorAgent("Done.")},
	})
	if out, err := exec.Command("sqlite3", stores.CursorDB, `UPDATE composerHeaders SET isArchived = 1 WHERE composerId = 'b1111111'`).CombinedOutput(); err != nil {
		t.Fatalf("sqlite3: %v: %s", err, out)
	}

	view := scanner.Scan(context.Background(), ScanOptions{})
	if c := byID(t, view, "a1111111"); c.State != StateActive {
		t.Errorf("chat not archived: %s, want active", c.State)
	}
	if c := byID(t, view, "b1111111"); c.State != StateDormant {
		t.Errorf("chat archived in Cursor: %s, want dormant", c.State)
	}
}
