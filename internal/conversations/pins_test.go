package conversations

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func pinTimeInJSON(t *testing.T, value any) string {
	t.Helper()
	var pin struct {
		PinnedAt string `json:"pinnedAt"`
	}
	if err := json.Unmarshal([]byte(asJSON(t, value)), &pin); err != nil {
		t.Fatal(err)
	}
	return pin.PinnedAt
}

func TestMark_PinKeepsUnreadAndCompletedMarksAcrossARestart(t *testing.T) {
	scanner, stores, now := renamingScanner(t)
	ctx := context.Background()
	scanner.Scan(ctx, ScanOptions{})
	writeMarkedSession(t, stores, idSafari, "2026-11-07T12:10:00Z")
	*now = now.Add(30*time.Minute + 125*time.Millisecond)
	before := byID(t, scanner.Scan(ctx, ScanOptions{}), idSafari)
	if before.State != StateUnread || before.SeenAt != "" || before.CompletedAt != "" {
		t.Fatalf("fixture must be unread: %s", asJSON(t, before))
	}

	if err := scanner.Mark(ToolClaude, idSafari, Mark("pinned")); err != nil {
		t.Fatal(err)
	}
	wantPin := now.Format(time.RFC3339Nano)
	entry := storedMarks(t, scanner.MarksPath).Marks[markKey(ToolClaude, idSafari)]
	if entry.SeenAt != "" || entry.CompletedAt != "" || pinTimeInJSON(t, entry) != wantPin {
		t.Errorf("pin changed the marks: %s", asJSON(t, entry))
	}

	*now = now.Add(time.Second)
	if err := scanner.Mark(ToolClaude, idSafari, Mark("pinned")); err != nil {
		t.Fatal(err)
	}
	scanner = &Scanner{Stores: stores, SQLite: noSQLite, Processes: fakeProcesses{}, Now: scanner.Now, MarksPath: scanner.MarksPath}
	after := byID(t, scanner.Scan(ctx, ScanOptions{}), idSafari)
	if after.State != before.State || after.Unread != before.Unread || after.SeenAt != before.SeenAt || after.CompletedAt != before.CompletedAt ||
		pinTimeInJSON(t, after) != wantPin {
		t.Errorf("pin did not survive a retry and restart without changing state: %s", asJSON(t, after))
	}

	if err := scanner.Mark(ToolClaude, idSafari, MarkCompleted); err != nil {
		t.Fatal(err)
	}
	completed := byID(t, scanner.Scan(ctx, ScanOptions{}), idSafari)
	if completed.State != StateCompleted || pinTimeInJSON(t, completed) != wantPin {
		t.Errorf("completing cleared the pin: %s", asJSON(t, completed))
	}

	if err := scanner.Mark(ToolClaude, idSafari, Mark("unpinned")); err != nil {
		t.Fatal(err)
	}
	unpinned := byID(t, scanner.Scan(ctx, ScanOptions{}), idSafari)
	if unpinned.State != StateCompleted || unpinned.SeenAt != completed.SeenAt || unpinned.CompletedAt != completed.CompletedAt ||
		strings.Contains(asJSON(t, unpinned), `"pinnedAt"`) {
		t.Errorf("unpin changed seen or completed marks, or kept pinnedAt: %s", asJSON(t, unpinned))
	}
}

func TestMark_PinsAndUnpinsTheCloudTwinAndClearsItsPinOnHide(t *testing.T) {
	scanner, stores, now := sharedScanner(t)
	const cloudID = "cse_01AbCd"
	check := func(want string) {
		t.Helper()
		marks := storedMarks(t, scanner.MarksPath).Marks
		for _, id := range []string{idSafari, cloudID} {
			if got := pinTimeInJSON(t, marks[markKey(ToolClaude, id)]); got != want {
				t.Errorf("%s pin = %q, want %q", id, got, want)
			}
		}
	}

	if err := scanner.Mark(ToolClaude, idSafari, Mark("pinned")); err != nil {
		t.Fatal(err)
	}
	want := now.Format(time.RFC3339Nano)
	check(want)
	if pin := pinTimeInJSON(t, byID(t, restartedScanner(scanner, stores).Scan(context.Background(), ScanOptions{}), cloudID)); pin != want {
		t.Errorf("cloud twin shown apart has pin %q, want %q", pin, want)
	}
	if err := scanner.Mark(ToolClaude, idSafari, Mark("unpinned")); err != nil {
		t.Fatal(err)
	}
	check("")

	if err := scanner.Mark(ToolClaude, idSafari, Mark("pinned")); err != nil {
		t.Fatal(err)
	}
	if err := scanner.Mark(ToolClaude, idSafari, MarkHidden); err != nil {
		t.Fatal(err)
	}
	check("")
	twin := storedMarks(t, scanner.MarksPath).Marks[markKey(ToolClaude, cloudID)]
	if twin.HiddenAt != "" || twin.SeenAt != "" {
		t.Errorf("hiding the local row also hid or marked its twin seen: %s", asJSON(t, twin))
	}
}

func TestScan_ShowsOldPinsAndDescendantsOnTheFirstMatchingPage(t *testing.T) {
	scanner, stores, _ := renamingScanner(t)
	ctx := context.Background()
	writeSessionSaying(t, stores, idParent, "2026-10-01T10:00:00Z", "Release root")
	writeSessionSaying(t, stores, idChild, "2026-10-10T10:00:00Z", "Continues claude:"+idParent+"\n\nRelease child")
	writeSessionSaying(t, stores, idSide, "2026-10-20T10:00:00Z", "Continues claude:"+idChild+"\n\nRelease grandchild")
	writeSessionSaying(t, stores, idSafari, "2026-11-06T10:00:00Z", "Release recent")
	writeSessionSaying(t, stores, idCache, "2026-10-17T10:00:00Z", "Release other")
	if err := scanner.Mark(ToolClaude, idParent, Mark("pinned")); err != nil {
		t.Fatal(err)
	}

	first := scanner.Scan(ctx, ScanOptions{Query: "release"})
	if len(first.Conversations) != 4 || first.Next == "" || listed(first, idCache) {
		t.Fatalf("first search page = %s", asJSON(t, first))
	}
	for _, id := range []string{idParent, idChild, idSide, idSafari} {
		if !listed(first, id) {
			t.Errorf("the first page missed %s", id)
		}
	}
	before, err := time.Parse(time.RFC3339, first.Next)
	if err != nil {
		t.Fatal(err)
	}
	second := scanner.Scan(ctx, ScanOptions{Query: "release", Before: before})
	if len(second.Conversations) != 1 || !listed(second, idCache) || second.Next != "" {
		t.Errorf("second search page duplicated a pin or missed the other row: %s", asJSON(t, second))
	}
	q := scanner.Scan(ctx, ScanOptions{Query: "grandchild"})
	if len(q.Conversations) != 1 || !listed(q, idSide) {
		t.Errorf("a pin pulled non-matching ancestors into search: %s", asJSON(t, q))
	}
}

func TestScan_PrunesAPinWithItsMissingConversation(t *testing.T) {
	p := newPruning(t)
	p.scan(t, context.Background())
	if err := p.scanner.Mark(ToolClaude, goneID, Mark("pinned")); err != nil {
		t.Fatal(err)
	}
	if entry, ok := p.entry(t, ToolClaude, goneID); !ok || pinTimeInJSON(t, entry) == "" {
		t.Fatal("the pin was not stored before the transcript disappeared")
	}
	if err := os.Remove(p.transcript(goneID)); err != nil {
		t.Fatal(err)
	}
	p.daily(t, 1)
	if !p.marked(t, goneID) {
		t.Fatal("the first miss pruned a pin")
	}
	p.daily(t, 30)
	if p.marked(t, goneID) {
		t.Error("a pin remained after thirty days of complete reads without its transcript")
	}
}

func TestMark_PinRepairsAnInvalidTimeInTheMarksFile(t *testing.T) {
	scanner, stores, now := renamingScanner(t)
	ctx := context.Background()
	writeSessionSaying(t, stores, idSafari, "2026-10-01T10:00:00Z", "An older conversation")
	writeSessionSaying(t, stores, idCache, "2026-11-06T10:00:00Z", "A recent conversation")
	write(t, scanner.MarksPath, `{"schemaVersion":"conversation-marks/v1","since":"2026-11-07T00:00:00Z","marks":{"claude:`+idSafari+`":{"pinnedAt":"yesterday","futureAt":"untouched"}}}`)

	first := scanner.Scan(ctx, ScanOptions{})
	if listed(first, idSafari) || first.Next == "" {
		t.Fatalf("an invalid pin brought an old conversation to the first page: %s", asJSON(t, first))
	}
	older, err := time.Parse(time.RFC3339, first.Next)
	if err != nil {
		t.Fatal(err)
	}
	if row := byID(t, scanner.Scan(ctx, ScanOptions{Before: older}), idSafari); pinTimeInJSON(t, row) != "" {
		t.Errorf("the list sent an invalid pin time: %s", asJSON(t, row))
	}

	if err := scanner.Mark(ToolClaude, idSafari, MarkPinned); err != nil {
		t.Fatal(err)
	}
	if row := byID(t, scanner.Scan(ctx, ScanOptions{}), idSafari); pinTimeInJSON(t, row) != now.Format(time.RFC3339Nano) {
		t.Errorf("pin did not replace the invalid time: %s", asJSON(t, row))
	}
	if err := scanner.Mark(ToolClaude, idSafari, MarkUnpinned); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(scanner.MarksPath)
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Marks map[string]map[string]json.RawMessage
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	entry := file.Marks[markKey(ToolClaude, idSafari)]
	if _, pinned := entry["pinnedAt"]; pinned || string(entry["futureAt"]) != `"untouched"` {
		t.Errorf("unpin did not clear only the pin: %s", asJSON(t, entry))
	}
}

func TestScan_PublishesPinTimesInUTCWithoutLosingPrecision(t *testing.T) {
	scanner, stores, _ := renamingScanner(t)
	writeSessionSaying(t, stores, idSafari, "2026-10-01T10:00:00Z", "An older conversation")
	write(t, scanner.MarksPath, `{"schemaVersion":"conversation-marks/v1","since":"2026-11-07T00:00:00Z","marks":{"claude:`+idSafari+`":{"pinnedAt":"2026-11-01T08:00:00.123456789+01:00"}}}`)

	row := byID(t, scanner.Scan(context.Background(), ScanOptions{}), idSafari)
	if got := pinTimeInJSON(t, row); got != "2026-11-01T07:00:00.123456789Z" {
		t.Errorf("pin time = %q, want the equivalent time in UTC", got)
	}
}
