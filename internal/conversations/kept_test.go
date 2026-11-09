package conversations

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScan_ReadsCursorAgainOnlyWhenItsFilesChangeOrTheResultIsOld(t *testing.T) {
	reads := 0
	stores := fakeStores(t, map[string]string{
		"state.vscdb|FROM composerHeaders": `[{"id":"88888888-8888-4888-8888-888888888888","workspace_id":"ws1","created_at":1700000000000,"recency":1700000000000,"name":"Old chat","folder":"/tmp/app","branch":""}]`,
		"state.vscdb|chosen(composerId)":   `[]`,
	})
	counting := func(ctx context.Context, path, query string) ([]json.RawMessage, error) {
		if strings.Contains(query, "FROM composerHeaders") {
			reads++
		}
		return stores(ctx, path, query)
	}
	scanner, paths := testScanner(t, counting, fakeProcesses{})
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	scanner.Now = func() time.Time { return now }
	write(t, paths.CursorDB, "")
	scan := func() int {
		t.Helper()
		view := scanner.Scan(context.Background(), ScanOptions{Query: "old chat"})
		if len(view.Conversations) != 1 || view.Conversations[0].Title != "Old chat" {
			t.Fatalf("view = %s", asJSON(t, view))
		}
		return reads
	}

	if scan(); scan() != 1 {
		t.Fatalf("an unchanged store is read once, read %d times", reads)
	}
	now = now.Add(keptMaxAge - time.Second)
	if scan() != 1 {
		t.Fatalf("a result younger than %s is kept, read %d times", keptMaxAge, reads)
	}
	now = now.Add(time.Second)
	if scan() != 2 {
		t.Fatalf("a result as old as %s is read again, read %d times", keptMaxAge, reads)
	}
	write(t, paths.CursorDB+"-wal", "a write Cursor has not folded in yet")
	if scan() != 3 {
		t.Fatalf("a changed store is read again, read %d times", reads)
	}
	if scanner.Scan(context.Background(), ScanOptions{Query: "old chat", Editor: "vscode"}); reads != 4 {
		t.Fatalf("another editor changes the routes, so the store is read again, read %d times", reads)
	}
}

func TestScan_DoesNotKeepAReadThatWasCanceled(t *testing.T) {
	reads := 0
	stores := fakeStores(t, map[string]string{"state.vscdb|FROM composerHeaders": `[]`})
	counting := func(ctx context.Context, path, query string) ([]json.RawMessage, error) {
		reads++
		return stores(ctx, path, query)
	}
	scanner, paths := testScanner(t, counting, fakeProcesses{})
	write(t, paths.CursorDB, "")
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	scanner.Scan(canceled, ScanOptions{})
	scanner.Scan(context.Background(), ScanOptions{})

	if reads != 2 {
		t.Errorf("the scan after a canceled one reads the store again, read %d times", reads)
	}
}

func TestScan_GivesEachScanItsOwnRowsFromAKeptResult(t *testing.T) {
	stores := fakeStores(t, map[string]string{
		"state.vscdb|FROM composerHeaders": `[{"id":"88888888-8888-4888-8888-888888888888","workspace_id":"ws1","created_at":1700000000000,"recency":1700000000000,"name":"Old chat","folder":"/tmp/app","branch":""}]`,
		"state.vscdb|chosen(composerId)":   `[]`,
	})
	scanner, paths := testScanner(t, stores, fakeProcesses{})
	write(t, paths.CursorDB, "")

	first := scanner.Scan(context.Background(), ScanOptions{Query: "old chat"})
	first.Conversations[0].Title = "changed by the caller"
	second := scanner.Scan(context.Background(), ScanOptions{Query: "old chat"})

	if second.Conversations[0].Title != "Old chat" {
		t.Errorf("a kept row carried a caller's change: %q", second.Conversations[0].Title)
	}
}

func TestStamp_ChangesWhenAStoreOrTheMarksChange(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	writeMarkedSession(t, stores, "11111111-1111-4111-8111-111111111111", "2026-11-07T11:00:00.000Z")
	before := scanner.Stamp()
	if again := scanner.Stamp(); again != before {
		t.Fatal("the stamp of unchanged files changed")
	}

	changes := map[string]func(){
		"a Claude Code session grows": func() {
			write(t, filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", "11111111-1111-4111-8111-111111111111.jsonl"), "{}\n{}\n")
		},
		"a Claude Code process reports its state": func() { write(t, filepath.Join(stores.ClaudeDir, "sessions", "412.json"), `{"status":"busy"}`) },
		"a new Claude Code session": func() {
			writeMarkedSession(t, stores, "22222222-2222-4222-8222-222222222222", "2026-11-07T11:30:00.000Z")
		},
		"Cursor writes to its log":     func() { write(t, stores.CursorDB+"-wal", "pending write") },
		"a Codex thread index appears": func() { write(t, filepath.Join(stores.CodexDir, "state_5.sqlite"), "x") },
		"a conversation is marked":     func() { write(t, scanner.MarksPath, "{}") },
	}
	for name, change := range changes {
		change()
		after := scanner.Stamp()
		if after == before {
			t.Errorf("%s: the stamp did not change", name)
		}
		before = after
	}
}
