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
				return inv.ids, true
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

func (p *pruning) marked(t *testing.T, id string) bool {
	t.Helper()
	_, ok := storedMarks(t, p.scanner.MarksPath).Marks[markKey(ToolClaude, id)]
	return ok
}

// markBothAndRemove marks both sessions, lists them in a complete read,
// then removes the gone one's transcript.
func (p *pruning) markBothAndRemove(t *testing.T) {
	t.Helper()
	for _, id := range []string{keptID, goneID} {
		if err := p.scanner.Mark(ToolClaude, id, MarkSeen); err != nil {
			t.Fatal(err)
		}
	}
	p.scan(t, context.Background())
	if err := os.Remove(p.transcript(goneID)); err != nil {
		t.Fatal(err)
	}
}

func TestScan_PrunesAMarkAfterThirtyDaysOfCompleteReadsWithoutItsConversation(t *testing.T) {
	p := newPruning(t)
	p.markBothAndRemove(t)

	p.now = p.now.Add(time.Hour)
	p.scan(t, context.Background())
	if entry := storedMarks(t, p.scanner.MarksPath).Marks[markKey(ToolClaude, goneID)]; entry.MissingSince != stamp(p.now) || entry.Source != sourceClaudeCode {
		t.Fatalf("first complete miss: %+v", entry)
	}

	p.now = p.now.Add(29 * 24 * time.Hour)
	p.scan(t, context.Background())
	if !p.marked(t, goneID) {
		t.Fatal("pruned after 29 days")
	}

	p.now = p.now.Add(24 * time.Hour)
	p.scan(t, context.Background())
	if p.marked(t, goneID) {
		t.Error("kept after 30 days of complete reads without it")
	}
	if !p.marked(t, keptID) {
		t.Error("pruned a mark whose conversation is still there")
	}
}

func TestScan_AFailedOrPartialReadNeitherRemovesNorAgesAMark(t *testing.T) {
	p := newPruning(t)
	p.markBothAndRemove(t)

	// A canceled read, then a read that could not open a folder, for 40 days.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p.now = p.now.Add(20 * 24 * time.Hour)
	p.scan(t, ctx)
	if !runtimeCanLockFolders() {
		p.now = p.now.Add(20 * 24 * time.Hour)
		p.scan(t, ctx)
	} else {
		unreadable := filepath.Join(p.stores.ClaudeDir, "projects", "-tmp-locked")
		if err := os.MkdirAll(unreadable, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(unreadable, 0); err != nil {
			t.Fatal(err)
		}
		p.now = p.now.Add(20 * 24 * time.Hour)
		p.scan(t, context.Background())
		if err := os.Chmod(unreadable, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if entry := storedMarks(t, p.scanner.MarksPath).Marks[markKey(ToolClaude, goneID)]; entry.MissingSince != "" {
		t.Fatalf("an incomplete read started the clock: %+v", entry)
	}

	// The first complete read starts it; 29 days later the mark still stands.
	p.scan(t, context.Background())
	p.now = p.now.Add(29 * 24 * time.Hour)
	p.scan(t, context.Background())
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

	p.now = p.now.Add(20 * 24 * time.Hour)
	writeMarkedSession(t, p.stores, goneID, "2026-11-07T11:00:00.000Z")
	p.scan(t, context.Background())
	if entry := storedMarks(t, p.scanner.MarksPath).Marks[markKey(ToolClaude, goneID)]; entry.MissingSince != "" {
		t.Fatalf("a listed conversation still ages: %+v", entry)
	}

	if err := os.Remove(p.transcript(goneID)); err != nil {
		t.Fatal(err)
	}
	p.now = p.now.Add(time.Hour)
	p.scan(t, context.Background())
	p.now = p.now.Add(29 * 24 * time.Hour)
	p.scan(t, context.Background())
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
	for _, id := range []string{goneID, cloudID} {
		if err := p.scanner.Mark(ToolClaude, id, MarkSeen); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(p.transcript(goneID)); err != nil {
		t.Fatal(err)
	}
	p.scan(t, context.Background())
	p.scanner.Cloud = listedCloud{}
	for range 3 {
		p.now = p.now.Add(31 * 24 * time.Hour)
		p.scan(t, context.Background())
	}
	if !p.marked(t, goneID) {
		t.Error("pruned a mark whose source is unknown")
	}
	if !p.marked(t, cloudID) {
		t.Error("pruned a cloud mark: a bounded list proves no deletion")
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
	file["marks"].(map[string]any)[markKey(ToolClaude, keptID)].(map[string]any)["pinnedAt"] = "2026-11-07T12:00:00Z"
	raw, err = json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	write(t, p.scanner.MarksPath, string(raw))

	p.now = p.now.Add(time.Hour)
	p.scan(t, context.Background())
	p.now = p.now.Add(30 * 24 * time.Hour)
	p.scan(t, context.Background())
	if p.marked(t, goneID) {
		t.Fatal("not pruned")
	}
	raw, err = os.ReadFile(p.scanner.MarksPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"later"`) || !strings.Contains(string(raw), `"pinnedAt": "2026-11-07T12:00:00Z"`) {
		t.Errorf("unknown fields lost:\n%s", raw)
	}
}

func TestScan_AMarkMadeWhilePruningIsKept(t *testing.T) {
	p := newPruning(t)
	p.markBothAndRemove(t)
	p.now = p.now.Add(time.Hour)
	p.scan(t, context.Background())
	p.now = p.now.Add(30 * 24 * time.Hour)

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

func TestScan_AFreshMarkOnAMissingConversationHoldsThePrune(t *testing.T) {
	p := newPruning(t)
	p.markBothAndRemove(t)
	p.now = p.now.Add(time.Hour)
	p.scan(t, context.Background())

	p.now = p.now.Add(10 * 24 * time.Hour)
	if err := p.scanner.Mark(ToolClaude, goneID, MarkCompleted); err != nil {
		t.Fatal(err)
	}
	p.now = p.now.Add(21 * 24 * time.Hour)
	p.scan(t, context.Background())
	if !p.marked(t, goneID) {
		t.Error("pruned a mark made after its conversation went missing")
	}
}

func TestMarksFile_KeepsUnknownFieldsThroughAReadAndWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conversation-marks.json")
	write(t, path, `{"schemaVersion":"conversation-marks/v1","since":"2026-11-07T12:00:00Z","later":[1,2],"marks":{"claude:a":{"seenAt":"2026-11-07T12:00:00Z","pinnedAt":"x","nested":{"a":1}}}}`)
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
	if back["later"] == nil || entry["pinnedAt"] != "x" || entry["nested"] == nil || entry["seenAt"] != "2026-11-07T12:00:00Z" {
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
