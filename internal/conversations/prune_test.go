package conversations

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	keptID = "11111111-1111-4111-8111-111111111111"
	goneID = "22222222-2222-4222-8222-222222222222"
)

func inventoryOf(results []sourceResult, source markSource) (map[string]bool, bool) {
	for _, result := range results {
		for _, inv := range result.inventories {
			if inv.source == source {
				ids := map[string]bool{}
				for id := range inv.files {
					ids[id] = true
				}
				return ids, true
			}
		}
	}
	return nil, false
}

func readEvery(t *testing.T, scanner *Scanner, ctx context.Context, opts ScanOptions) []sourceResult {
	t.Helper()
	scanner.mu.Lock()
	defer scanner.mu.Unlock()
	results, _ := scanner.readAll(ctx, opts)
	return results
}

func TestReadAll_ClaudeCodeProvesAFullInventoryOnlyWhenEveryFolderWasRead(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	writeMarkedSession(t, stores, keptID, "2026-11-07T11:00:00.000Z")
	// A session in another folder, and one with no text, which no row shows.
	other := filepath.Join(stores.ClaudeDir, "projects", "-tmp-other", goneID+".jsonl")
	write(t, other, `{"type":"user","cwd":"/tmp/other","timestamp":"2026-11-07T11:00:00.000Z","message":{"role":"user","content":""}}`+"\n")

	ids, complete := inventoryOf(readEvery(t, scanner, context.Background(), ScanOptions{}), sourceClaudeCode)
	if !complete || !ids[keptID] || !ids[goneID] {
		t.Fatalf("a full read: complete %v ids %v, want both sessions", complete, ids)
	}

	// A folder filter narrows the rows, never the inventory.
	ids, complete = inventoryOf(readEvery(t, scanner, context.Background(), ScanOptions{Within: []string{"/tmp/app"}}), sourceClaudeCode)
	if !complete || !ids[goneID] {
		t.Fatalf("a filtered read: complete %v ids %v, want the session outside the folder", complete, ids)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, complete := inventoryOf(readEvery(t, scanner, ctx, ScanOptions{}), sourceClaudeCode); complete {
		t.Error("a canceled read proved a full inventory")
	}

	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		return
	}
	unreadable := filepath.Dir(other)
	if err := os.Chmod(unreadable, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o755) })
	if _, complete := inventoryOf(readEvery(t, scanner, context.Background(), ScanOptions{}), sourceClaudeCode); complete {
		t.Error("a read that could not open a project folder proved a full inventory")
	}
}

func TestReadAll_ClaudeCodeWithNoProjectsFolderProvesNothing(t *testing.T) {
	scanner, _ := testScanner(t, noSQLite, fakeProcesses{})
	if _, complete := inventoryOf(readEvery(t, scanner, context.Background(), ScanOptions{}), sourceClaudeCode); complete {
		t.Error("a missing store proved a full inventory")
	}
}

func codexThreadsFixture(ids ...string) string {
	rows := make([]string, len(ids))
	for i, id := range ids {
		rows[i] = `{"id":"` + id + `","cwd":"/tmp/app","created_at_ms":1794045600000,"updated_at_ms":1794049200000,"originator":"codex-tui","source":"cli","name":"Plan"}`
	}
	return "[" + strings.Join(rows, ",") + "]"
}

func TestReadAll_CodexProvesAFullInventoryOnlyFromASuccessfulRead(t *testing.T) {
	threads := codexThreadsFixture(keptID, goneID)
	failing := false
	sqlite := func(ctx context.Context, path, query string) ([]json.RawMessage, error) {
		if failing && strings.Contains(query, "FROM threads") {
			return nil, errors.New("database is locked")
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return fakeStores(t, map[string]string{
			"state_5.sqlite|FROM threads":          threads,
			"thread_history_1.sqlite|thread_items": `[]`,
		})(ctx, path, query)
	}
	scanner, stores := testScanner(t, sqlite, fakeProcesses{})
	write(t, filepath.Join(stores.CodexDir, "state_5.sqlite"), "")
	write(t, filepath.Join(stores.CodexDir, "thread_history_1.sqlite"), "")

	ids, complete := inventoryOf(readEvery(t, scanner, context.Background(), ScanOptions{Within: []string{"/elsewhere"}}), sourceCodex)
	if !complete || !ids[keptID] || !ids[goneID] {
		t.Fatalf("a full read: complete %v ids %v", complete, ids)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	scanner.keptSources = nil
	if _, complete := inventoryOf(readEvery(t, scanner, ctx, ScanOptions{}), sourceCodex); complete {
		t.Error("a canceled read proved a full inventory")
	}

	failing = true
	scanner.keptSources = nil
	if _, complete := inventoryOf(readEvery(t, scanner, context.Background(), ScanOptions{}), sourceCodex); complete {
		t.Error("a failed read proved a full inventory")
	}
}

func TestReadAll_CursorAgentProvesAFullInventoryButEditorChatsNever(t *testing.T) {
	agent := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	meta := hex.EncodeToString([]byte(`{"agentId":"` + agent + `","name":"Fix","createdAt":1762500000000}`))
	metaFails := false
	sqlite := func(ctx context.Context, path, query string) ([]json.RawMessage, error) {
		if metaFails && strings.Contains(query, "FROM meta") {
			return nil, errors.New("disk I/O error")
		}
		return fakeStores(t, map[string]string{
			"state.vscdb|FROM composerHeaders": `[{"id":"b1111111","created_at":1762500000000,"recency":1762600000000,"name":"Editor chat","folder":"/tmp/app","branch":""}]`,
			"store.db|FROM meta":               `[{"value":"` + meta + `"}]`,
		})(ctx, path, query)
	}
	scanner, stores := testScanner(t, sqlite, fakeProcesses{})
	write(t, stores.CursorDB, "")
	write(t, filepath.Join(stores.CursorDir, "chats", md5Hex("/tmp/app"), agent, "store.db"), "")

	results := readEvery(t, scanner, context.Background(), ScanOptions{})
	ids, complete := inventoryOf(results, sourceCursorAgent)
	if !complete || !ids[agent] {
		t.Fatalf("a full read: complete %v ids %v", complete, ids)
	}
	if ids["b1111111"] {
		t.Error("an editor chat counted in the cursor-agent inventory")
	}
	for _, result := range results {
		for _, inv := range result.inventories {
			if !provable[inv.source] {
				t.Errorf("source %q claims a full inventory", inv.source)
			}
		}
	}

	metaFails = true
	scanner.keptSources = nil
	scanner.cursorAgents = nil
	if _, complete := inventoryOf(readEvery(t, scanner, context.Background(), ScanOptions{}), sourceCursorAgent); complete {
		t.Error("a read whose chat store could not be read proved a full inventory")
	}
}

func TestProvable_OnlyStoresThatListEveryConversationProveDeletion(t *testing.T) {
	for source, want := range map[markSource]bool{
		sourceClaudeCode: true, sourceCodex: true, sourceCursorAgent: true,
		"claude-app": false, "claude-cloud": false, "codex-cloud": false, "cursor-editor": false, "chatgpt": false,
	} {
		if provable[source] != want {
			t.Errorf("provable[%s] = %v, want %v", source, provable[source], want)
		}
	}
}

// pruning is a scanner over Claude Code transcripts with a marks file
// and a clock the test moves.
type pruning struct {
	scanner *Scanner
	stores  Stores
	now     time.Time
}

func newPruning(t *testing.T) *pruning {
	t.Helper()
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{readFails: true})
	p := &pruning{scanner: scanner, stores: stores, now: time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)}
	scanner.MarksPath = filepath.Join(t.TempDir(), "state", "conversation-marks.json")
	scanner.Now = func() time.Time { return p.now }
	writeMarkedSession(t, stores, keptID, "2026-11-07T11:00:00.000Z")
	writeMarkedSession(t, stores, goneID, "2026-11-07T11:00:00.000Z")
	return p
}

func (p *pruning) transcript(id string) string {
	return filepath.Join(p.stores.ClaudeDir, "projects", "-tmp-app", id+".jsonl")
}

func (p *pruning) scan(t *testing.T, ctx context.Context) {
	t.Helper()
	if view := p.scanner.Scan(ctx, ScanOptions{}); view.MarksNote != "" {
		t.Fatal(view.MarksNote)
	}
}

// daily moves the clock a day at a time and scans after each move.
func (p *pruning) daily(t *testing.T, days int) {
	t.Helper()
	for range days {
		p.now = p.now.Add(24 * time.Hour)
		p.scan(t, context.Background())
	}
}

func (p *pruning) entry(t *testing.T, tool Tool, id string) (markEntry, bool) {
	t.Helper()
	entry, ok := storedMarks(t, p.scanner.MarksPath).Marks[markKey(tool, id)]
	return entry, ok
}

func (p *pruning) marked(t *testing.T, id string) bool {
	t.Helper()
	_, ok := p.entry(t, ToolClaude, id)
	return ok
}

func markAll(t *testing.T, scanner *Scanner, tool Tool, ids ...string) {
	t.Helper()
	for _, id := range ids {
		if err := scanner.Mark(tool, id, MarkSeen); err != nil {
			t.Fatal(err)
		}
	}
}

// markBothAndRemove marks both sessions, lists them in a complete read,
// then removes the gone one's transcript.
func (p *pruning) markBothAndRemove(t *testing.T) {
	t.Helper()
	markAll(t, p.scanner, ToolClaude, keptID, goneID)
	p.scan(t, context.Background())
	if err := os.Remove(p.transcript(goneID)); err != nil {
		t.Fatal(err)
	}
}

// fileWrites counts how often path was replaced; call it after each scan.
func fileWrites(t *testing.T, path string) func() int {
	t.Helper()
	last, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	writes := 0
	return func() int {
		now, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(last, now) || !last.ModTime().Equal(now.ModTime()) {
			writes++
		}
		last = now
		return writes
	}
}

func TestScan_PrunesAMarkAfterThirtyDaysOfDailyCompleteReadsWithoutItsConversation(t *testing.T) {
	p := newPruning(t)
	p.markBothAndRemove(t)

	p.now = p.now.Add(time.Hour)
	p.scan(t, context.Background())
	entry, _ := p.entry(t, ToolClaude, goneID)
	if entry.MissingSince != stamp(p.now) || entry.Source != sourceClaudeCode || entry.Store != storeKey(filepath.Join(p.stores.ClaudeDir, "projects")) || entry.File != filepath.Join("-tmp-app", goneID+".jsonl") {
		t.Fatalf("first complete miss: %+v", entry)
	}

	p.daily(t, 29)
	if !p.marked(t, goneID) {
		t.Fatal("pruned after 29 days")
	}
	p.daily(t, 1)
	if p.marked(t, goneID) {
		t.Error("kept after 30 days of complete reads without it")
	}
	if !p.marked(t, keptID) {
		t.Error("pruned a mark whose conversation is still there")
	}
}

func TestScan_SavesAMissingMarkAtMostOnceADay(t *testing.T) {
	p := newPruning(t)
	p.markBothAndRemove(t)
	writes := fileWrites(t, p.scanner.MarksPath)

	n := 0
	for range 10 {
		p.now = p.now.Add(time.Hour)
		p.scan(t, context.Background())
		n = writes()
	}
	if n != 1 {
		t.Fatalf("ten scans within a day wrote the marks %d times, want once for the first miss", n)
	}
	p.now = p.now.Add(15 * time.Hour)
	p.scan(t, context.Background())
	writes()
	p.scan(t, context.Background())
	if n := writes(); n != 2 {
		t.Errorf("the next day wrote the marks %d times in all, want 2", n)
	}
}

func TestScan_AMachineOffForAMonthPrunesNothingOnItsFirstReadBack(t *testing.T) {
	p := newPruning(t)
	p.markBothAndRemove(t)
	p.now = p.now.Add(time.Hour)
	p.scan(t, context.Background())

	p.now = p.now.Add(31 * 24 * time.Hour)
	p.scan(t, context.Background())
	if !p.marked(t, goneID) {
		t.Fatal("one read after a month away pruned the mark")
	}
	entry, _ := p.entry(t, ToolClaude, goneID)
	if entry.MissingSince != stamp(p.now) {
		t.Errorf("the 30 days did not start again: %+v", entry)
	}
	p.daily(t, 29)
	if !p.marked(t, goneID) {
		t.Error("pruned 29 days after the pause")
	}
}

func TestScan_AnUnreadableMissingTimeStartsTheThirtyDays(t *testing.T) {
	p := newPruning(t)
	p.markBothAndRemove(t)
	for _, broken := range []string{"", "yesterday"} {
		update := markUpdate{markKey(ToolClaude, goneID), func(entry *markEntry, now time.Time) {
			entry.MissingSince, entry.LastMissAt = broken, stamp(now)
		}}
		if err := p.scanner.changeMarks([]markUpdate{update}); err != nil {
			t.Fatal(err)
		}
		p.now = p.now.Add(time.Hour)
		p.scan(t, context.Background())
		entry, ok := p.entry(t, ToolClaude, goneID)
		if !ok || entry.MissingSince != stamp(p.now) {
			t.Errorf("missingSince %q: %+v, kept %v", broken, entry, ok)
		}
	}
}

func TestScan_AFailedOrPartialReadNeitherRemovesNorAgesAMark(t *testing.T) {
	p := newPruning(t)
	p.markBothAndRemove(t)

	// Canceled reads, then reads that could not open a folder, for 40 days.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for range 20 {
		p.now = p.now.Add(24 * time.Hour)
		p.scan(t, ctx)
	}
	if runtimeCanLockFolders() {
		unreadable := filepath.Join(p.stores.ClaudeDir, "projects", "-tmp-locked")
		if err := os.MkdirAll(unreadable, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(unreadable, 0); err != nil {
			t.Fatal(err)
		}
		p.daily(t, 20)
		if err := os.Chmod(unreadable, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if entry, _ := p.entry(t, ToolClaude, goneID); entry.MissingSince != "" {
		t.Fatalf("an incomplete read started the clock: %+v", entry)
	}

	// The first complete read starts it; 29 days later the mark still stands.
	p.scan(t, context.Background())
	p.daily(t, 29)
	if !p.marked(t, goneID) {
		t.Error("incomplete reads aged the mark")
	}
}

func runtimeCanLockFolders() bool { return runtime.GOOS != "windows" && os.Geteuid() != 0 }

func TestScan_AConversationThatReappearsResetsTheClock(t *testing.T) {
	p := newPruning(t)
	p.markBothAndRemove(t)
	p.now = p.now.Add(time.Hour)
	p.scan(t, context.Background())

	p.daily(t, 20)
	writeMarkedSession(t, p.stores, goneID, "2026-11-07T11:00:00.000Z")
	p.scan(t, context.Background())
	if entry, _ := p.entry(t, ToolClaude, goneID); entry.MissingSince != "" || entry.LastMissAt != "" {
		t.Fatalf("a listed conversation still ages: %+v", entry)
	}

	if err := os.Remove(p.transcript(goneID)); err != nil {
		t.Fatal(err)
	}
	p.daily(t, 29)
	if !p.marked(t, goneID) {
		t.Error("the clock went on from before the conversation came back")
	}
}

func TestScan_KeepsMarksWhoseSourceCannotBeTold(t *testing.T) {
	p := newPruning(t)
	// An old entry from before sources were kept, and a cloud session
	// a list showed once: neither was ever in a complete inventory.
	const cloudID = "session_01ABCDEFGHJKMNPQRSTVWXYZ0"
	p.scanner.Cloud = listedCloud{{Tool: ToolClaude, App: AppCloud, ID: cloudID, Title: "Cloud", UpdatedAt: "2026-11-07T11:00:00Z"}}
	markAll(t, p.scanner, ToolClaude, goneID, cloudID)
	if err := os.Remove(p.transcript(goneID)); err != nil {
		t.Fatal(err)
	}
	p.scan(t, context.Background())
	p.scanner.Cloud = listedCloud{}
	p.daily(t, 40)
	if !p.marked(t, goneID) {
		t.Error("pruned a mark whose source is unknown")
	}
	if !p.marked(t, cloudID) {
		t.Error("pruned a cloud mark: a bounded list proves no deletion")
	}
}

func TestScan_TwoServersReadingTwoClaudeStoresAgeNoMarkOfTheOther(t *testing.T) {
	p := newPruning(t)
	// The companion reads ~/.claude; a terminal's serve reads another
	// CLAUDE_CONFIG_DIR. Both share one marks file.
	other, otherStores := testScanner(t, noSQLite, fakeProcesses{readFails: true})
	other.MarksPath = p.scanner.MarksPath
	other.Now = func() time.Time { return p.now }
	const otherID = "33333333-3333-4333-8333-333333333333"
	writeMarkedSession(t, otherStores, otherID, "2026-11-07T11:00:00.000Z")
	markAll(t, p.scanner, ToolClaude, keptID)
	p.scan(t, context.Background())
	markAll(t, other, ToolClaude, otherID)
	other.Scan(context.Background(), ScanOptions{})
	p.scan(t, context.Background())

	writes := fileWrites(t, p.scanner.MarksPath)
	for range 40 {
		p.now = p.now.Add(24 * time.Hour)
		p.scan(t, context.Background())
		writes()
		other.Scan(context.Background(), ScanOptions{})
	}
	if n := writes(); n != 0 {
		t.Errorf("two servers rewrote the marks %d times", n)
	}
	for _, id := range []string{keptID, otherID} {
		if entry, ok := p.entry(t, ToolClaude, id); !ok || entry.MissingSince != "" {
			t.Errorf("%s aged by the other store's reads: %+v, kept %v", id, entry, ok)
		}
	}
}

func TestScan_AnEmptyStoreProvesNothing(t *testing.T) {
	p := newPruning(t)
	p.markBothAndRemove(t)
	// The store is recreated empty, as after a move or with a disk not mounted.
	if err := os.RemoveAll(filepath.Join(p.stores.ClaudeDir, "projects")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(p.stores.ClaudeDir, "projects", "-tmp-app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, complete := inventoryOf(readEvery(t, p.scanner, context.Background(), ScanOptions{}), sourceClaudeCode); complete {
		t.Fatal("an empty store proved a full inventory")
	}
	p.daily(t, 40)
	for _, id := range []string{keptID, goneID} {
		if entry, ok := p.entry(t, ToolClaude, id); !ok || entry.MissingSince != "" {
			t.Errorf("%s aged by an empty store: %+v, kept %v", id, entry, ok)
		}
	}
}

func TestScan_PruningKeepsFieldsItDoesNotKnow(t *testing.T) {
	p := newPruning(t)
	p.markBothAndRemove(t)
	raw, err := os.ReadFile(p.scanner.MarksPath)
	if err != nil {
		t.Fatal(err)
	}
	var file map[string]any
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	file["later"] = map[string]any{"kept": true}
	file["marks"].(map[string]any)[markKey(ToolClaude, keptID)].(map[string]any)["futureAt"] = "2026-11-07T12:00:00Z"
	raw, err = json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	write(t, p.scanner.MarksPath, string(raw))

	p.daily(t, 31)
	if p.marked(t, goneID) {
		t.Fatal("not pruned")
	}
	raw, err = os.ReadFile(p.scanner.MarksPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"later"`) || !strings.Contains(string(raw), `"futureAt": "2026-11-07T12:00:00Z"`) {
		t.Errorf("unknown fields lost:\n%s", raw)
	}
}

func TestScan_AMarkMadeWhilePruningIsKept(t *testing.T) {
	p := newPruning(t)
	p.markBothAndRemove(t)
	p.now = p.now.Add(time.Hour)
	p.scan(t, context.Background())
	p.daily(t, 29)
	p.now = p.now.Add(24 * time.Hour)

	// Another server shares the file and marks while this one prunes.
	other, _ := testScanner(t, noSQLite, fakeProcesses{})
	other.MarksPath = p.scanner.MarksPath
	now := p.now
	other.Now = func() time.Time { return now }
	const each = 20
	var wait sync.WaitGroup
	failed := make(chan error, each)
	for i := range each {
		wait.Go(func() {
			if err := other.Mark(ToolCodex, fmt.Sprintf("thread-%d", i), MarkCompleted); err != nil {
				failed <- err
			}
		})
	}
	wait.Go(func() { p.scanner.Scan(context.Background(), ScanOptions{}) })
	wait.Wait()
	close(failed)
	for err := range failed {
		t.Fatal(err)
	}

	marks := storedMarks(t, p.scanner.MarksPath).Marks
	for i := range each {
		if marks[markKey(ToolCodex, fmt.Sprintf("thread-%d", i))].CompletedAt == "" {
			t.Errorf("mark %d lost to the prune", i)
		}
	}
	if _, ok := marks[markKey(ToolClaude, goneID)]; ok {
		t.Error("the prune was lost to the marks")
	}
}

// codexStateStore writes a real Codex thread index with one user thread
// per id, each with its rollout under sessions/.
func codexStateStore(t *testing.T, stores Stores, ids ...string) {
	t.Helper()
	needSQLite(t)
	path := filepath.Join(stores.CodexDir, "state_5.sqlite")
	_ = os.Remove(path)
	script := `CREATE TABLE threads (id TEXT PRIMARY KEY, cwd TEXT, created_at_ms INTEGER, updated_at_ms INTEGER, originator TEXT, source TEXT,
  git_branch TEXT, name TEXT, title TEXT, first_user_message TEXT, rollout_path TEXT, archived INTEGER, archived_at INTEGER, thread_source TEXT);`
	var scriptSb583 strings.Builder
	for _, id := range ids {
		fmt.Fprintf(&scriptSb583, `INSERT INTO threads VALUES ('%s', '/tmp/app', 1794045600000, 1794049200000, 'codex-tui', 'cli', '', 'Plan', '', 'Plan it', '%s', 0, 0, 'user');`, id, codexRollout(stores, id))
	}
	script += scriptSb583.String()
	if err := os.MkdirAll(stores.CodexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sqliteExec(t, path, script)
}

func codexRollout(stores Stores, id string) string {
	return filepath.Join(stores.CodexDir, "sessions", "2026", "11", "07", "rollout-"+id+".jsonl")
}

func TestScan_PrunesAndKeepsCodexMarksFromARealIndex(t *testing.T) {
	needSQLite(t)
	p := newPruning(t)
	p.scanner.SQLite = RunSQLite
	const unknownPath = "44444444-4444-4444-8444-444444444444"
	for _, id := range []string{keptID, goneID} {
		write(t, codexRollout(p.stores, id), "{}\n")
	}
	codexStateStore(t, p.stores, keptID, goneID)
	sqliteExec(t, filepath.Join(p.stores.CodexDir, "state_5.sqlite"), `INSERT INTO threads VALUES ('`+unknownPath+`', '/tmp/app', 1794045600000, 1794049200000, 'codex-tui', 'cli', '', 'Plan', '', 'Plan it', '', 0, 0, 'user');`)
	markAll(t, p.scanner, ToolCodex, keptID, goneID, unknownPath)
	p.scan(t, context.Background())

	// The index loses both threads, but only one rollout is gone. The
	// third thread's index row named no rollout.
	codexStateStore(t, p.stores, keptID)
	if err := os.Remove(codexRollout(p.stores, goneID)); err != nil {
		t.Fatal(err)
	}
	p.now = p.now.Add(time.Hour)
	p.scan(t, context.Background())
	p.daily(t, 29)
	if _, ok := p.entry(t, ToolCodex, goneID); !ok {
		t.Fatal("pruned after 29 days")
	}
	p.daily(t, 1)
	if _, ok := p.entry(t, ToolCodex, goneID); ok {
		t.Error("a thread gone from the index and its rollout kept its mark")
	}
	if _, ok := p.entry(t, ToolCodex, keptID); !ok {
		t.Error("a listed thread lost its mark")
	}
	if _, ok := p.entry(t, ToolCodex, unknownPath); !ok {
		t.Error("a thread whose rollout was never known lost its mark")
	}
}

func TestScan_KeepsACodexMarkWhoseRolloutIsStillThere(t *testing.T) {
	needSQLite(t)
	p := newPruning(t)
	p.scanner.SQLite = RunSQLite
	for _, id := range []string{keptID, goneID} {
		write(t, codexRollout(p.stores, id), "{}\n")
	}
	codexStateStore(t, p.stores, keptID, goneID)
	markAll(t, p.scanner, ToolCodex, goneID)
	p.scan(t, context.Background())

	// A new index generation that Codex has only partly filled.
	codexStateStore(t, p.stores, keptID)
	p.daily(t, 31)
	entry, ok := p.entry(t, ToolCodex, goneID)
	if !ok {
		t.Fatal("pruned a thread whose rollout is still on disk")
	}
	if entry.MissingSince != "" && parseStamp(entry.MissingSince).Before(p.now.Add(-24*time.Hour)) {
		t.Errorf("the 30 days did not start again after the check: %+v", entry)
	}
}

func TestReadAll_CodexWithAnEmptyIndexProvesNothing(t *testing.T) {
	needSQLite(t)
	scanner, stores := testScanner(t, RunSQLite, fakeProcesses{})
	codexStateStore(t, stores)
	if _, complete := inventoryOf(readEvery(t, scanner, context.Background(), ScanOptions{}), sourceCodex); complete {
		t.Error("an index with no thread proved a full inventory")
	}
}

func markedAgentChat(t *testing.T, stores Stores, id string) string {
	t.Helper()
	folder := filepath.Join(stores.CursorDir, "chats", md5Hex("/tmp/app"), id)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := hex.EncodeToString([]byte(`{"agentId":"` + id + `","name":"Fix it","createdAt":1777203106986}`))
	sqliteExec(t, filepath.Join(folder, "store.db"), `CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT); INSERT INTO meta VALUES ('0', '`+meta+`');`)
	return folder
}

func TestScan_PrunesAndKeepsCursorAgentMarksFromRealStores(t *testing.T) {
	needSQLite(t)
	p := newPruning(t)
	p.scanner.SQLite = RunSQLite
	markedAgentChat(t, p.stores, keptID)
	gone := markedAgentChat(t, p.stores, goneID)
	markAll(t, p.scanner, ToolCursor, keptID, goneID)
	p.scan(t, context.Background())
	if entry, _ := p.entry(t, ToolCursor, goneID); entry.Source != sourceCursorAgent || entry.File != filepath.Join(md5Hex("/tmp/app"), goneID) {
		t.Fatalf("not attributed to its chat folder: %+v", entry)
	}

	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	p.now = p.now.Add(time.Hour)
	p.scan(t, context.Background())
	p.daily(t, 29)
	if _, ok := p.entry(t, ToolCursor, goneID); !ok {
		t.Fatal("pruned after 29 days")
	}
	p.daily(t, 1)
	if _, ok := p.entry(t, ToolCursor, goneID); ok {
		t.Error("a deleted cursor-agent chat kept its mark")
	}
	if _, ok := p.entry(t, ToolCursor, keptID); !ok {
		t.Error("a listed chat lost its mark")
	}
}

func TestReadAll_AnEmptyCursorAgentChatsFolderProvesNothing(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	if err := os.MkdirAll(filepath.Join(stores.CursorDir, "chats"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, ids := scanner.cursorAgentChats(context.Background(), nil, nil); provenBy(sourceCursorAgent, ToolCursor, "", ids) != nil {
		t.Error("an empty chats folder proved a full inventory")
	}
}

func TestMarksFile_KeepsUnknownFieldsThroughAReadAndWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conversation-marks.json")
	write(t, path, `{"schemaVersion":"conversation-marks/v1","since":"2026-11-07T12:00:00Z","later":[1,2],"marks":{"claude:a":{"seenAt":"2026-11-07T12:00:00Z","futureAt":"x","nested":{"a":1}}}}`)
	file, _, err := readMarks(path, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if file.Marks["claude:a"].SeenAt != "2026-11-07T12:00:00Z" {
		t.Fatalf("known field lost on read: %+v", file.Marks["claude:a"])
	}
	if err := writeMarks(path, file); err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	raw, _ := os.ReadFile(path)
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	entry := back["marks"].(map[string]any)["claude:a"].(map[string]any)
	if back["later"] == nil || entry["futureAt"] != "x" || entry["nested"] == nil || entry["seenAt"] != "2026-11-07T12:00:00Z" {
		t.Errorf("round trip lost a field:\n%s", raw)
	}
}

func TestMarksFile_KnownKeysAreTheFieldsItEncodes(t *testing.T) {
	for typ, known := range map[reflect.Type]map[string]bool{
		reflect.TypeFor[markEntryFields](): markEntryKeys,
		reflect.TypeFor[marksFileFields](): marksFileKeys,
	} {
		for field := range typ.Fields() {
			if name, _, _ := strings.Cut(field.Tag.Get("json"), ","); name != "" && !known[name] {
				t.Errorf("%s field %q is missing from its known keys, so a read would keep a second copy", typ, name)
			}
		}
	}
}

func TestListTree_FindsWhatGlobFinds(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a/x.jsonl", "a/y.json", "a/abc", "a/store.db", "b/c/store.db", "b/d/x.jsonl", ".h/z.jsonl"} {
		write(t, filepath.Join(root, name), "")
	}
	for _, tc := range []struct {
		levels  int
		pattern string
	}{{1, "*.jsonl"}, {1, "store.db"}, {1, "a?c"}, {2, "store.db"}, {2, "*.jsonl"}} {
		glob := filepath.Join(root, strings.Repeat("*"+string(filepath.Separator), tc.levels)+tc.pattern)
		want, _ := filepath.Glob(glob)
		got, complete := listTree(root, tc.levels, tc.pattern)
		if !complete || strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("%s: listTree %v (complete %v), Glob %v", glob, got, complete, want)
		}
	}
}

func TestScan_AMarkFollowsItsStoreToANewFolder(t *testing.T) {
	p := newPruning(t)
	markAll(t, p.scanner, ToolClaude, keptID, goneID)
	p.scan(t, context.Background())

	// The home folder was renamed: the same chats, under another path.
	moved := filepath.Join(t.TempDir(), "renamed-home", ".claude")
	if err := os.MkdirAll(filepath.Dir(moved), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(p.stores.ClaudeDir, moved); err != nil {
		t.Fatal(err)
	}
	p.stores.ClaudeDir, p.scanner.Stores.ClaudeDir = moved, moved
	p.now = p.now.Add(time.Hour)
	p.scan(t, context.Background())
	if entry, _ := p.entry(t, ToolClaude, keptID); entry.Store != storeKey(filepath.Join(moved, "projects")) {
		t.Fatalf("the mark kept the old store: %+v", entry)
	}

	if err := os.Remove(p.transcript(goneID)); err != nil {
		t.Fatal(err)
	}
	p.daily(t, 31)
	if p.marked(t, goneID) {
		t.Error("a chat deleted after its store moved kept its mark")
	}
	if !p.marked(t, keptID) {
		t.Error("a listed chat lost its mark")
	}
}

func TestScan_TwoServersWithACopiedStoreKeepTheFirstStore(t *testing.T) {
	p := newPruning(t)
	markAll(t, p.scanner, ToolClaude, keptID)
	p.scan(t, context.Background())
	first, _ := p.entry(t, ToolClaude, keptID)

	// Another server reads a copy of the same folder, which holds the same ids.
	other, otherStores := testScanner(t, noSQLite, fakeProcesses{readFails: true})
	other.MarksPath = p.scanner.MarksPath
	other.Now = func() time.Time { return p.now }
	writeMarkedSession(t, otherStores, keptID, "2026-11-07T11:00:00.000Z")
	writes := fileWrites(t, p.scanner.MarksPath)
	n := 0
	for range 10 {
		p.now = p.now.Add(24 * time.Hour)
		p.scan(t, context.Background())
		writes()
		other.Scan(context.Background(), ScanOptions{})
		n = writes()
	}
	if n != 0 {
		t.Errorf("two servers on copies of one store rewrote the marks %d times", n)
	}
	if entry, _ := p.entry(t, ToolClaude, keptID); entry.Store != first.Store {
		t.Errorf("the mark moved to the copy: %+v, first %+v", entry, first)
	}
}

func TestStoreKey_NamesAFolderTheSameThroughALink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("links need privileges on Windows")
	}
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if storeKey(link) != storeKey(real) {
		t.Error("a link to the store names another store")
	}
	if storeKey(filepath.Join(dir, "missing")) == storeKey(real) {
		t.Error("a missing folder named an existing one")
	}
}

func TestScan_ASixDayPauseAfterAnUnsavedMissKeepsTheThirtyDays(t *testing.T) {
	p := newPruning(t)
	p.markBothAndRemove(t)
	p.now = p.now.Add(time.Hour)
	p.scan(t, context.Background())
	started, _ := p.entry(t, ToolClaude, goneID)

	// A miss the next day is not saved; the next read comes six and a half days after it.
	p.now = p.now.Add(23 * time.Hour)
	p.scan(t, context.Background())
	p.now = p.now.Add(6*24*time.Hour + 12*time.Hour)
	p.scan(t, context.Background())
	if entry, _ := p.entry(t, ToolClaude, goneID); entry.MissingSince != started.MissingSince {
		t.Errorf("a pause under 7 days started the 30 days again: %+v, started %s", entry, started.MissingSince)
	}
}

func TestScan_SavesTheMissingMarksOfAStoreTogether(t *testing.T) {
	p := newPruning(t)
	ids := []string{}
	for i := range 5 {
		id := fmt.Sprintf("5555555%d-5555-4555-8555-555555555555", i)
		writeMarkedSession(t, p.stores, id, "2026-11-07T11:00:00.000Z")
		ids = append(ids, id)
	}
	markAll(t, p.scanner, ToolClaude, append(ids, keptID)...)
	p.scan(t, context.Background())
	// Each chat goes missing at another time of day.
	for _, id := range ids {
		p.now = p.now.Add(4 * time.Hour)
		if err := os.Remove(p.transcript(id)); err != nil {
			t.Fatal(err)
		}
		p.scan(t, context.Background())
	}

	writes := fileWrites(t, p.scanner.MarksPath)
	n := 0
	for range 4 * 8 {
		p.now = p.now.Add(3 * time.Hour)
		p.scan(t, context.Background())
		n = writes()
	}
	if n != 4 {
		t.Errorf("four days of scans wrote the marks %d times, want one a day", n)
	}
}

func TestScan_ACloudTwinMarkIsNeverTiedToAStoreOrPruned(t *testing.T) {
	p := newPruning(t)
	const cloudID = "session_01ABCDEFGHJKMNPQRSTVWXYZ0"
	markAll(t, p.scanner, ToolClaude, keptID)
	p.scan(t, context.Background())
	raw, err := os.ReadFile(p.scanner.MarksPath)
	if err != nil {
		t.Fatal(err)
	}
	var file map[string]any
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	// A shared cloud key carries only the twin's shown write, beside a
	// field a newer version wrote.
	file["marks"].(map[string]any)[markKey(ToolClaude, cloudID)] = map[string]any{"twinShownAt": "2026-11-07T11:00:00Z", "laterField": 1}
	raw, err = json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	write(t, p.scanner.MarksPath, string(raw))

	p.daily(t, 40)
	// A mark on another row makes the scan write the file again.
	markAll(t, p.scanner, ToolClaude, keptID)
	p.daily(t, 1)
	entry, ok := p.entry(t, ToolClaude, cloudID)
	if !ok || entry.TwinShownAt != "2026-11-07T11:00:00Z" || entry.Source != "" || entry.Store != "" || entry.MissingSince != "" {
		t.Fatalf("the cloud twin's mark: %+v, kept %v", entry, ok)
	}
	if raw, _ := os.ReadFile(p.scanner.MarksPath); !strings.Contains(string(raw), `"laterField": 1`) || !strings.Contains(string(raw), "\n") {
		t.Errorf("an unknown field beside twinShownAt was lost:\n%s", raw)
	}
}
