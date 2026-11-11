package conversations

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// archivedAt is when deleteScanner archives its sessions; their files are dated before it.
var archivedAt = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

// sessionFiles writes a Claude Code session with every location Claude
// Code keeps for it, dated an hour before the archive, and returns each
// location with the bytes it holds.
func sessionFiles(t *testing.T, stores Stores, id string) map[string]string {
	t.Helper()
	writeMarkedSession(t, stores, id, "2026-11-07T11:00:00.000Z")
	project := filepath.Join(stores.ClaudeDir, "projects", "-tmp-app")
	files := map[string]string{
		filepath.Join(project, id, "subagents", "agent-1.jsonl"):              "subagent transcript\n",
		filepath.Join(project, id, "tool-results", "result-1.txt"):            "saved tool output\n",
		filepath.Join(stores.ClaudeDir, "file-history", id, "backup@v1"):      "file backup\n",
		filepath.Join(stores.ClaudeDir, "session-env", id, "hook-1.sh"):       "export A=1\n",
		filepath.Join(stores.ClaudeDir, "uploads", id, "screenshot-1.png"):    "image bytes\n",
		filepath.Join(stores.ClaudeDir, "tasks", "session-"+id[:8], "1.json"): "{}\n",
	}
	for path, content := range files {
		write(t, path, content)
	}
	transcript := filepath.Join(project, id+".jsonl")
	data, err := os.ReadFile(transcript)
	if err != nil {
		t.Fatal(err)
	}
	files[transcript] = string(data)
	dateBefore(t, stores.ClaudeDir, archivedAt.Add(-time.Hour))
	return files
}

// dateBefore dates every file and folder under root at, so a mark made after it stands.
func dateBefore(t *testing.T, root string, at time.Time) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Chtimes(path, at, at)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// deleteScanner lists two Claude Code sessions in one project, both
// archived, and the files that belong to neither.
func deleteScanner(t *testing.T, processes Processes) (scanner *Scanner, stores Stores, target, other map[string]string) {
	t.Helper()
	scanner, stores = testScanner(t, noSQLite, processes)
	scanner.MarksPath = filepath.Join(t.TempDir(), "state", "conversation-marks.json")
	now := archivedAt.Add(-time.Minute)
	scanner.Now = func() time.Time { return now }
	target = sessionFiles(t, stores, idSafari)
	other = sessionFiles(t, stores, idCache)
	shared := map[string]string{
		filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", "memory", "MEMORY.md"): "- project memory\n",
		filepath.Join(stores.ClaudeDir, "history.jsonl"):                               `{"display":"Fix the parser","sessionId":"` + idSafari + `"}` + "\n",
	}
	for path, content := range shared {
		write(t, path, content)
		other[path] = content
	}
	for path, content := range target {
		if strings.Contains(path, "tasks") {
			delete(target, path)
			other[path] = content
		}
	}
	dateBefore(t, stores.ClaudeDir, archivedAt.Add(-time.Hour))
	scanner.Scan(context.Background(), ScanOptions{})
	now = archivedAt
	for _, id := range []string{idSafari, idCache} {
		if err := scanner.Rename(ToolClaude, id, "Named "+id[:4]); err != nil {
			t.Fatal(err)
		}
		if err := scanner.Mark(ToolClaude, id, MarkCompleted); err != nil {
			t.Fatal(err)
		}
	}
	now = archivedAt.Add(time.Minute)
	return scanner, stores, target, other
}

func assertFiles(t *testing.T, files map[string]string, exist bool) {
	t.Helper()
	for path, content := range files {
		data, err := os.ReadFile(path)
		switch {
		case exist && err != nil:
			t.Errorf("%s is gone: %v", path, err)
		case exist && string(data) != content:
			t.Errorf("%s changed: %q", path, data)
		case !exist && !errors.Is(err, os.ErrNotExist):
			t.Errorf("%s is still there (%v)", path, err)
		}
	}
}

func deleteRefusal(t *testing.T, err error) *DeleteError {
	t.Helper()
	var refusal *DeleteError
	if !errors.As(err, &refusal) {
		t.Fatalf("error = %v, want a *DeleteError", err)
	}
	return refusal
}

func listed(view View, id string) bool {
	for _, c := range view.Conversations {
		if c.ID == id {
			return true
		}
	}
	return false
}

func TestDelete_RemovesAnArchivedClaudeSessionItsSideFilesAndItsMarks(t *testing.T) {
	scanner, stores, target, other := deleteScanner(t, fakeProcesses{})
	tabs := filepath.Join(filepath.Dir(scanner.MarksPath), "conversation-tabs.json")
	write(t, tabs, `{"version":1,"tabs":{"claude:`+idSafari+`":{"host":{"app":"Terminal","tty":"/dev/ttys004"},"at":"2026-11-07T11:00:00Z"},"claude:`+idCache+`":{"host":{"app":"Terminal","tty":"/dev/ttys005"},"at":"2026-11-07T11:00:00Z"}}}`)
	marksBefore := storedMarks(t, scanner.MarksPath)
	freed := 0
	for _, content := range target {
		freed += len(content)
	}

	reply, err := scanner.Delete(context.Background(), ToolClaude, idSafari, ScanOptions{})

	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !reply.Deleted || reply.FreedBytes != int64(freed) || reply.Note != "" {
		t.Errorf("reply = %s, want %d bytes freed", asJSON(t, reply), freed)
	}
	assertFiles(t, target, false)
	for _, folder := range []string{
		filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", idSafari),
		filepath.Join(stores.ClaudeDir, "file-history", idSafari),
		filepath.Join(stores.ClaudeDir, "session-env", idSafari),
		filepath.Join(stores.ClaudeDir, "uploads", idSafari),
	} {
		if _, err := os.Lstat(folder); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s is still there (%v)", folder, err)
		}
	}
	assertFiles(t, other, true)
	marks := storedMarks(t, scanner.MarksPath)
	if _, kept := marks.Marks[markKey(ToolClaude, idSafari)]; kept {
		t.Errorf("the deleted session's marks stay: %+v", marks.Marks)
	}
	if marks.Marks[markKey(ToolClaude, idCache)] != marksBefore.Marks[markKey(ToolClaude, idCache)] || marks.Since != marksBefore.Since {
		t.Errorf("the other session's marks changed: %+v", marks)
	}
	if remembered := readTabs(tabs).Tabs; len(remembered) != 1 || remembered[markKey(ToolClaude, idCache)].Host.TTY != "/dev/ttys005" {
		t.Errorf("remembered tabs = %+v", remembered)
	}
	if listed(scanner.Scan(context.Background(), ScanOptions{}), idSafari) {
		t.Error("the deleted session is still listed")
	}
	fresh := &Scanner{Stores: stores, SQLite: noSQLite, Processes: fakeProcesses{}, Now: scanner.Now, MarksPath: scanner.MarksPath}
	view := fresh.Scan(context.Background(), ScanOptions{})
	if listed(view, idSafari) || byID(t, view, idCache).State != StateCompleted {
		t.Errorf("a new server lists %s", asJSON(t, view.Conversations))
	}
}

func TestDelete_RefusesAConversationThatIsNotArchived(t *testing.T) {
	for name, tc := range map[string]struct {
		marks []Mark
		kind  DeleteRefusal
	}{
		"never marked":            {nil, DeleteNotArchived},
		"seen only":               {[]Mark{MarkSeen}, DeleteNotArchived},
		"archive taken back":      {[]Mark{MarkCompleted, MarkOpen}, DeleteNotArchived},
		"hidden, so not in lists": {[]Mark{MarkCompleted, MarkHidden}, DeleteNotListed},
	} {
		t.Run(name, func(t *testing.T) {
			scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
			scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
			scanner.Now = func() time.Time { return archivedAt }
			files := sessionFiles(t, stores, idSafari)
			for _, mark := range tc.marks {
				if err := scanner.Mark(ToolClaude, idSafari, mark); err != nil {
					t.Fatal(err)
				}
			}

			reply, err := scanner.Delete(context.Background(), ToolClaude, idSafari, ScanOptions{})

			if refusal := deleteRefusal(t, err); refusal.Kind != tc.kind || reply.Deleted {
				t.Errorf("= %s %+v", asJSON(t, reply), refusal)
			}
			assertFiles(t, files, true)
		})
	}
}

func TestDelete_SaysWhyAConversationThatIsNotArchivedStays(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	sessionFiles(t, stores, idSafari)

	_, err := scanner.Delete(context.Background(), ToolClaude, idSafari, ScanOptions{})

	if err == nil || err.Error() != "Only an archived conversation can be deleted. This one is not archived any more: something was written in it, or it was opened. Refresh the list." {
		t.Errorf("error = %v", err)
	}
	scanner.MarksPath = ""
	if _, err := scanner.Delete(context.Background(), ToolClaude, idSafari, ScanOptions{}); deleteRefusal(t, err).Kind != DeleteNotArchived || err.Error() != "This server keeps no conversation marks, so nothing is archived." {
		t.Errorf("without a marks file: %v", err)
	}
}

func TestDelete_RefusesWhatThisServerDoesNotList(t *testing.T) {
	scanner, _, target, _ := deleteScanner(t, fakeProcesses{})

	_, outside := scanner.Delete(context.Background(), ToolClaude, idSafari, ScanOptions{Within: []string{"/tmp/other"}})
	_, unknown := scanner.Delete(context.Background(), ToolClaude, "55555555-5555-4555-8555-555555555555", ScanOptions{})

	for name, err := range map[string]error{"outside the project": outside, "unknown id": unknown} {
		if refusal := deleteRefusal(t, err); refusal.Kind != DeleteNotListed || refusal.Message != "That conversation is not in this list any more. Refresh the page." {
			t.Errorf("%s: %+v", name, refusal)
		}
	}
	assertFiles(t, target, true)
}

func TestDelete_RefusesARunningSessionEvenWhenArchived(t *testing.T) {
	for name, start := range map[string]func(t *testing.T, stores Stores){
		"an idle process holds it": func(t *testing.T, stores Stores) {
			writeClaudeProcess(t, stores, 4242, idSafari, "idle")
		},
		"a background job holds it": func(t *testing.T, stores Stores) {
			write(t, filepath.Join(stores.ClaudeDir, "jobs", "5f12e206", "state.json"), `{"sessionId":"`+idSafari+`","state":"idle"}`)
		},
	} {
		t.Run(name, func(t *testing.T) {
			scanner, stores, target, _ := deleteScanner(t, fakeProcesses{alive: map[int]bool{4242: true}})
			start(t, stores)

			_, err := scanner.Delete(context.Background(), ToolClaude, idSafari, ScanOptions{})

			if refusal := deleteRefusal(t, err); refusal.Kind != DeleteRunning || refusal.Message != "This conversation is still running. Close it in Claude Code, then delete it." {
				t.Errorf("refusal = %+v", refusal)
			}
			assertFiles(t, target, true)
			if _, kept := storedMarks(t, scanner.MarksPath).Marks[markKey(ToolClaude, idSafari)]; !kept {
				t.Error("a refused delete cleared the marks")
			}
		})
	}
}

func TestDelete_RefusesASessionThatWroteAfterItWasArchived(t *testing.T) {
	scanner, stores, target, _ := deleteScanner(t, fakeProcesses{})
	writeSessionSaying(t, stores, idSafari, "2026-11-07T12:00:30.000Z", "One more thing")
	transcript := filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", idSafari+".jsonl")
	data, err := os.ReadFile(transcript)
	if err != nil {
		t.Fatal(err)
	}
	target[transcript] = string(data)

	_, err = scanner.Delete(context.Background(), ToolClaude, idSafari, ScanOptions{})

	if refusal := deleteRefusal(t, err); refusal.Kind != DeleteNotArchived {
		t.Errorf("refusal = %+v", refusal)
	}
	assertFiles(t, target, true)
}

func TestDelete_RefusesASessionWhoseFilesChangedAfterTheArchive(t *testing.T) {
	scanner, stores, target, _ := deleteScanner(t, fakeProcesses{})
	// A bookkeeping write moves no message time; only the file's own time says it happened.
	later := archivedAt.Add(30 * time.Second)
	if err := os.Chtimes(filepath.Join(stores.ClaudeDir, "file-history", idSafari, "backup@v1"), later, later); err != nil {
		t.Fatal(err)
	}

	_, err := scanner.Delete(context.Background(), ToolClaude, idSafari, ScanOptions{})

	if refusal := deleteRefusal(t, err); refusal.Kind != DeleteChanged || refusal.Message != "Something was written in this conversation after you archived it. Archive it again if you still want to delete it." {
		t.Errorf("refusal = %+v", refusal)
	}
	assertFiles(t, target, true)
}

// startsLater lists pid in its process table from question number from
// on, and no process before.
type startsLater struct {
	fakeProcesses
	mu    sync.Mutex
	asked int
	pid   int
	from  int
}

func (s *startsLater) Table() ([]ProcessInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.asked++
	if s.asked < s.from {
		return nil, nil
	}
	return []ProcessInfo{{PID: s.pid, Command: "claude"}}, nil
}

func TestDelete_StopsBeforeTheTranscriptWhenTheSessionStartsDuringTheRemoval(t *testing.T) {
	scanner, stores, target, _ := deleteScanner(t, fakeProcesses{})
	writeClaudeProcess(t, stores, 4242, idSafari, "idle")
	// The check before the removal finds nothing; the one before the transcript sees it run.
	scanner.Processes = &startsLater{pid: 4242, from: 2}
	transcript := filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", idSafari+".jsonl")

	reply, err := scanner.Delete(context.Background(), ToolClaude, idSafari, ScanOptions{})

	refusal := deleteRefusal(t, err)
	if refusal.Kind != DeleteFailed || reply.Deleted || len(refusal.Removed) != 4 || strings.Join(refusal.Remaining, ",") != transcript {
		t.Fatalf("= %s %+v", asJSON(t, reply), refusal)
	}
	if !strings.HasPrefix(refusal.Message, "This conversation started running while it was being deleted, so agnostic-ai stopped before its transcript. Deleted 4 of 5 locations. Still on disk: "+transcript+". Its marks were kept.") {
		t.Errorf("message = %q", refusal.Message)
	}
	assertFiles(t, map[string]string{transcript: target[transcript]}, true)
	if _, kept := storedMarks(t, scanner.MarksPath).Marks[markKey(ToolClaude, idSafari)]; !kept {
		t.Error("the marks were cleared")
	}
}

// unreadableTable cannot read the process table.
type unreadableTable struct{ fakeProcesses }

func (unreadableTable) Table() ([]ProcessInfo, error) {
	return nil, errors.New("ps: operation not permitted")
}

func TestDelete_RefusesWhenItCannotTellWhetherTheSessionRuns(t *testing.T) {
	scanner, _, target, _ := deleteScanner(t, fakeProcesses{})
	scanner.Processes = unreadableTable{}

	reply, err := scanner.Delete(context.Background(), ToolClaude, idSafari, ScanOptions{})

	refusal := deleteRefusal(t, err)
	if refusal.Kind != DeleteFailed || reply.Deleted || len(refusal.Removed) != 0 || len(refusal.Remaining) != 0 ||
		refusal.Message != "agnostic-ai could not check whether this session still runs, so it deleted nothing." {
		t.Errorf("= %s %+v", asJSON(t, reply), refusal)
	}
	assertFiles(t, target, true)
}

func TestDelete_RefusesAWriteInTheSecondOfTheArchive(t *testing.T) {
	scanner, stores, target, _ := deleteScanner(t, fakeProcesses{})
	// Marks keep whole seconds: a write later in the archive's own second may have come after it.
	same := archivedAt.Add(800 * time.Millisecond)
	if err := os.Chtimes(filepath.Join(stores.ClaudeDir, "session-env", idSafari, "hook-1.sh"), same, same); err != nil {
		t.Fatal(err)
	}

	_, err := scanner.Delete(context.Background(), ToolClaude, idSafari, ScanOptions{})

	if refusal := deleteRefusal(t, err); refusal.Kind != DeleteChanged {
		t.Errorf("refusal = %+v", refusal)
	}
	assertFiles(t, target, true)
}

func TestDelete_RefusesASessionThatStartsBetweenTheReadAndTheRemoval(t *testing.T) {
	scanner, stores, target, _ := deleteScanner(t, fakeProcesses{})
	writeClaudeProcess(t, stores, 4242, idSafari, "idle")
	scanner.Processes = &startsLater{pid: 4242, from: 1}

	_, err := scanner.Delete(context.Background(), ToolClaude, idSafari, ScanOptions{})

	if refusal := deleteRefusal(t, err); refusal.Kind != DeleteRunning {
		t.Errorf("refusal = %+v", refusal)
	}
	assertFiles(t, target, true)
}

// recordingSQLite answers as fakeStores does and keeps every query it was asked.
type recordingSQLite struct {
	answer  SQLiteQuery
	mu      sync.Mutex
	queries []string
}

func (r *recordingSQLite) query(ctx context.Context, path, query string) ([]json.RawMessage, error) {
	r.mu.Lock()
	r.queries = append(r.queries, query)
	r.mu.Unlock()
	return r.answer(ctx, path, query)
}

// listedCloud is a cloud that always lists the same conversations.
type listedCloud []Conversation

func (l listedCloud) List(context.Context) ([]Conversation, error) { return l, nil }

func TestDelete_SaysWhichStoresItCannotDelete(t *testing.T) {
	const codexThread, cursorChat = "55555555-5555-4555-8555-555555555555", "88888888-8888-4888-8888-888888888888"
	at := "2026-11-07T11:00:00Z"
	sqlite := &recordingSQLite{answer: fakeStores(t, map[string]string{
		"state_5.sqlite|FROM threads":      `[{"id":"` + codexThread + `","cwd":"/tmp/app","created_at_ms":1794045600000,"updated_at_ms":1794049200000,"originator":"codex-tui","source":"cli","name":"Plan the release"}]`,
		"state.vscdb|FROM composerHeaders": `[{"id":"` + cursorChat + `","created_at":1794045600000,"recency":1794049200000,"name":"Parser refactor","folder":"/tmp/app","branch":""}]`,
		"state.vscdb|chosen(composerId)":   `[]`,
	})}
	scanner, stores := testScanner(t, sqlite.query, fakeProcesses{})
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	scanner.Now = func() time.Time { return archivedAt }
	scanner.Cloud = listedCloud{cloudRow("cse_a1", "Fix the parser", at)}
	write(t, filepath.Join(stores.CodexDir, "state_5.sqlite"), "codex threads")
	write(t, stores.CursorDB, "cursor chats")
	for _, row := range []struct {
		tool Tool
		id   string
	}{{ToolCodex, codexThread}, {ToolCursor, cursorChat}, {ToolClaude, "cse_a1"}} {
		if err := scanner.Mark(row.tool, row.id, MarkCompleted); err != nil {
			t.Fatal(err)
		}
	}
	scanner.Now = func() time.Time { return archivedAt.Add(time.Minute) }

	for _, tc := range []struct {
		tool   Tool
		id     string
		reason string
	}{
		{ToolCodex, codexThread, "Deleting Codex threads from here is not supported yet. To delete it, run this in a terminal: codex delete " + codexThread},
		{ToolCursor, cursorChat, "Cursor keeps every chat in one shared database and has no command to delete one. Delete it in Cursor."},
		{ToolClaude, "cse_a1", "This conversation is stored in the tool's cloud, not on this computer. Delete it there."},
	} {
		reply, err := scanner.Delete(context.Background(), tc.tool, tc.id, ScanOptions{})
		if refusal := deleteRefusal(t, err); refusal.Kind != DeleteUnsupported || refusal.Message != tc.reason || reply.Deleted {
			t.Errorf("%s %s: %s %+v", tc.tool, tc.id, asJSON(t, reply), refusal)
		}
	}
	for path, content := range map[string]string{filepath.Join(stores.CodexDir, "state_5.sqlite"): "codex threads", stores.CursorDB: "cursor chats"} {
		if data, err := os.ReadFile(path); err != nil || string(data) != content {
			t.Errorf("%s = %q (%v)", path, data, err)
		}
	}
	for _, query := range sqlite.queries {
		upper := strings.ToUpper(query)
		for _, write := range []string{"DELETE", "UPDATE", "INSERT", "DROP"} {
			if strings.Contains(upper, write+" ") {
				t.Errorf("a refused delete wrote with %q", query)
			}
		}
	}
}

func TestScan_SaysOnAnArchivedRowWhyItCannotBeDeleted(t *testing.T) {
	const codexDone, codexOpen = "55555555-5555-4555-8555-555555555555", "66666666-6666-4666-8666-666666666666"
	scanner, stores := testScanner(t, fakeStores(t, map[string]string{
		"state_5.sqlite|FROM threads": `[
			{"id":"` + codexDone + `","cwd":"/tmp/app","created_at_ms":1794045600000,"updated_at_ms":1794049200000,"originator":"codex-tui","source":"cli","name":"Done"},
			{"id":"` + codexOpen + `","cwd":"/tmp/app","created_at_ms":1794045600000,"updated_at_ms":1794049200000,"originator":"codex-tui","source":"cli","name":"Open"}
		]`,
	}), fakeProcesses{})
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	scanner.Now = func() time.Time { return archivedAt }
	write(t, filepath.Join(stores.CodexDir, "state_5.sqlite"), "")
	sessionFiles(t, stores, idSafari)
	for _, row := range []struct {
		tool Tool
		id   string
	}{{ToolCodex, codexDone}, {ToolClaude, idSafari}} {
		if err := scanner.Mark(row.tool, row.id, MarkCompleted); err != nil {
			t.Fatal(err)
		}
	}

	view := scanner.Scan(context.Background(), ScanOptions{})

	if c := byID(t, view, codexDone); c.State != StateCompleted || !strings.HasPrefix(c.DeleteUnsupported, "Deleting Codex threads from here is not supported yet.") {
		t.Errorf("archived Codex thread: %s", asJSON(t, c))
	}
	for _, id := range []string{codexOpen, idSafari} {
		if c := byID(t, view, id); c.DeleteUnsupported != "" {
			t.Errorf("%s (%s) says %q", id, c.State, c.DeleteUnsupported)
		}
	}
}

func TestDelete_StopsAtTheFirstFailureAndSaysWhatRemains(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs a folder this user cannot write in")
	}
	scanner, stores, target, _ := deleteScanner(t, fakeProcesses{})
	locked := filepath.Join(stores.ClaudeDir, "file-history", idSafari, "deep")
	write(t, filepath.Join(locked, "backup@v2"), "kept by the lock\n")
	dateBefore(t, stores.ClaudeDir, archivedAt.Add(-time.Hour))
	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	project := filepath.Join(stores.ClaudeDir, "projects", "-tmp-app")

	reply, err := scanner.Delete(context.Background(), ToolClaude, idSafari, ScanOptions{})

	refusal := deleteRefusal(t, err)
	if refusal.Kind != DeleteFailed || reply.Deleted {
		t.Fatalf("= %s %+v", asJSON(t, reply), refusal)
	}
	if removed := strings.Join(refusal.Removed, ","); removed != filepath.Join(stores.ClaudeDir, "session-env", idSafari) {
		t.Errorf("removed = %s", removed)
	}
	remaining := []string{
		filepath.Join(stores.ClaudeDir, "file-history", idSafari),
		filepath.Join(stores.ClaudeDir, "uploads", idSafari),
		filepath.Join(project, idSafari),
		filepath.Join(project, idSafari+".jsonl"),
	}
	if actual := strings.Join(refusal.Remaining, ","); actual != strings.Join(remaining, ",") {
		t.Errorf("remaining = %s", actual)
	}
	if !strings.HasPrefix(refusal.Message, "Deleted 1 of 5 locations, then stopped: ") || !strings.HasSuffix(refusal.Message, ". Still on disk: "+strings.Join(remaining, ", ")+". Its marks were kept.") {
		t.Errorf("message = %q", refusal.Message)
	}
	if _, err := os.Stat(filepath.Join(project, idSafari+".jsonl")); err != nil {
		t.Errorf("the transcript went before the failure was handled: %v", err)
	}
	gone := map[string]string{}
	for _, path := range []string{filepath.Join(stores.ClaudeDir, "session-env", idSafari, "hook-1.sh"), filepath.Join(stores.ClaudeDir, "file-history", idSafari, "backup@v1")} {
		gone[path] = target[path]
		delete(target, path)
	}
	assertFiles(t, gone, false)
	assertFiles(t, target, true)
	if c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), idSafari); c.State != StateCompleted {
		t.Errorf("after a failed delete the row is %s", c.State)
	}

	// Removing files moved the folders' own times; a retry is not refused as changed.
	if err := os.Chmod(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if reply, err := scanner.Delete(context.Background(), ToolClaude, idSafari, ScanOptions{}); err != nil || !reply.Deleted {
		t.Fatalf("retry = %s %v", asJSON(t, reply), err)
	}
	assertFiles(t, target, false)
}

func TestDelete_RefusesAnIDFoundInTwoProjectFolders(t *testing.T) {
	scanner, stores, target, _ := deleteScanner(t, fakeProcesses{})
	copyPath := filepath.Join(stores.ClaudeDir, "projects", "-tmp-copy", idSafari+".jsonl")
	write(t, copyPath, target[filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", idSafari+".jsonl")])
	dateBefore(t, stores.ClaudeDir, archivedAt.Add(-time.Hour))

	_, err := scanner.Delete(context.Background(), ToolClaude, idSafari, ScanOptions{})

	if refusal := deleteRefusal(t, err); refusal.Kind != DeleteUnsupported || !strings.Contains(refusal.Message, "cannot verify which files belong to it") {
		t.Errorf("refusal = %+v", refusal)
	}
	assertFiles(t, target, true)
}

func TestDelete_RefusesALocationThatIsALink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need extra rights on Windows")
	}
	scanner, stores, target, _ := deleteScanner(t, fakeProcesses{})
	outside := filepath.Join(t.TempDir(), "other-disk")
	write(t, filepath.Join(outside, "backup@v1"), "file backup on another disk\n")
	link := filepath.Join(stores.ClaudeDir, "file-history", idSafari)
	if err := os.RemoveAll(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	delete(target, filepath.Join(link, "backup@v1"))
	dateBefore(t, stores.ClaudeDir, archivedAt.Add(-time.Hour))

	_, err := scanner.Delete(context.Background(), ToolClaude, idSafari, ScanOptions{})

	if refusal := deleteRefusal(t, err); refusal.Kind != DeleteUnsupported || !strings.Contains(refusal.Message, "cannot verify which files belong to it") || !strings.Contains(refusal.Message, link) {
		t.Errorf("refusal = %+v", refusal)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the link is gone: %v", err)
	}
	assertFiles(t, map[string]string{filepath.Join(outside, "backup@v1"): "file backup on another disk\n"}, true)
	assertFiles(t, target, true)
}

func TestDelete_RefusesALinkInsideTheSessionFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need extra rights on Windows")
	}
	scanner, stores, target, _ := deleteScanner(t, fakeProcesses{})
	outside := filepath.Join(t.TempDir(), "other-disk")
	write(t, filepath.Join(outside, "output.txt"), "tool output on another disk\n")
	dateBefore(t, outside, archivedAt.Add(-time.Hour))
	link := filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", idSafari, "linked-results")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}

	_, err := scanner.Delete(context.Background(), ToolClaude, idSafari, ScanOptions{})

	if refusal := deleteRefusal(t, err); refusal.Kind != DeleteUnsupported || !strings.Contains(refusal.Message, link) {
		t.Errorf("refusal = %+v", refusal)
	}
	assertFiles(t, map[string]string{filepath.Join(outside, "output.txt"): "tool output on another disk\n"}, true)
	assertFiles(t, target, true)
}

func TestDelete_SaysWhenTheMarksCouldNotBeClearedAfterTheData(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs a folder this user cannot write in")
	}
	scanner, _, target, _ := deleteScanner(t, fakeProcesses{})
	folder := filepath.Dir(scanner.MarksPath)
	// The lock file exists already, so the lock still works in a read-only folder.
	if err := os.Chmod(folder, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(folder, 0o755) })

	reply, err := scanner.Delete(context.Background(), ToolClaude, idSafari, ScanOptions{})

	if err != nil || !reply.Deleted || !strings.HasPrefix(reply.Note, "The session data is gone. Its entry in the marks file could not be cleared: ") {
		t.Errorf("= %s %v", asJSON(t, reply), err)
	}
	assertFiles(t, target, false)
}

func TestDelete_RefusesASessionThatWritesBetweenItsReads(t *testing.T) {
	scanner, stores, _, _ := deleteScanner(t, fakeProcesses{})
	transcript := filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", idSafari+".jsonl")
	// A resume that appends and exits leaves no process for the later checks to see.
	scanner.betweenReads = func() {
		resumed, err := os.OpenFile(transcript, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := resumed.WriteString(`{"type":"assistant"}` + "\n"); err != nil {
			t.Fatal(err)
		}
		if err := resumed.Close(); err != nil {
			t.Fatal(err)
		}
	}

	_, err := scanner.Delete(context.Background(), ToolClaude, idSafari, ScanOptions{})

	if refusal := deleteRefusal(t, err); refusal.Kind != DeleteChanged {
		t.Errorf("refusal = %+v", refusal)
	}
	if data, err := os.ReadFile(transcript); err != nil || !strings.HasSuffix(string(data), `{"type":"assistant"}`+"\n") {
		t.Errorf("the resumed transcript = %q (%v)", data, err)
	}
}

// tableOnly lists running processes in its table, but its lookup by
// pid always answers that none runs, as a failed ps does.
type tableOnly struct{ fakeProcesses }

func (tableOnly) AliveAmong([]ProcessRef) map[int]bool { return map[int]bool{} }

func TestDelete_DecidesWhetherTheSessionRunsFromTheTableItRead(t *testing.T) {
	scanner, stores, target, _ := deleteScanner(t, fakeProcesses{})
	write(t, filepath.Join(stores.ClaudeDir, "sessions", "4242.json"), `{"pid":4242,"sessionId":"`+idSafari+`","procStart":"Sat Nov  7 08:00:00 2026"}`)
	scanner.Processes = tableOnly{fakeProcesses{table: []ProcessInfo{{PID: 4242, Started: "Sat Nov  7 08:00:00 2026", Command: "claude"}}}}

	_, err := scanner.Delete(context.Background(), ToolClaude, idSafari, ScanOptions{})

	if refusal := deleteRefusal(t, err); refusal.Kind != DeleteRunning {
		t.Errorf("refusal = %+v", refusal)
	}
	assertFiles(t, target, true)
}

func TestDelete_ReadsAReusedPidAsAnotherProcess(t *testing.T) {
	scanner, stores, _, _ := deleteScanner(t, fakeProcesses{})
	write(t, filepath.Join(stores.ClaudeDir, "sessions", "4242.json"), `{"pid":4242,"sessionId":"`+idSafari+`","procStart":"Sat Nov  7 08:00:00 2026"}`)
	dateBefore(t, stores.ClaudeDir, archivedAt.Add(-time.Hour))
	scanner.Processes = tableOnly{fakeProcesses{table: []ProcessInfo{{PID: 4242, Started: "Sat Nov  7 11:30:00 2026", Command: "node"}}}}

	if reply, err := scanner.Delete(context.Background(), ToolClaude, idSafari, ScanOptions{}); err != nil || !reply.Deleted {
		t.Errorf("= %s %v", asJSON(t, reply), err)
	}
}
