package conversations

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"
)

// writeBridgedSession writes a running session's sessions/<pid>.json with
// the bridge id Remote Control adds; an empty bridge leaves the field out.
func writeBridgedSession(t *testing.T, stores Stores, pid int, id, status, bridge string) {
	t.Helper()
	entry := map[string]any{"pid": pid, "sessionId": id, "status": status}
	if bridge != "" {
		entry["bridgeSessionId"] = bridge
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(stores.ClaudeDir, "sessions", strconv.Itoa(pid)+".json"), string(data))
}

type bridgedSession struct {
	pid    int
	id     string
	bridge string
	alive  bool
}

func oneCloudList(rows ...Conversation) *countingCloud {
	return &countingCloud{answers: []func() ([]Conversation, error){
		func() ([]Conversation, error) { return rows, nil },
	}}
}

func idsOf(view View) []string {
	var ids []string
	for _, c := range view.Conversations {
		ids = append(ids, c.ID)
	}
	return ids
}

func TestScan_ShowsARemoteControlSessionOnce(t *testing.T) {
	cases := []struct {
		name string
		// sessions are the live session files: id, bridge id and whether its pid runs.
		sessions  []bridgedSession
		cloud     []Conversation
		wantIDs   []string
		sharedID  string
		wantCloud *CloudRef
	}{
		{
			name:      "a live session whose bridge id is listed merges",
			sessions:  []bridgedSession{{71, idSafari, "session_01AbCd", true}},
			cloud:     []Conversation{cloudRow("cse_01AbCd", "Fix the parser", "2026-11-07T11:30:00Z")},
			wantIDs:   []string{idSafari},
			sharedID:  idSafari,
			wantCloud: &CloudRef{ID: "cse_01AbCd", Link: "https://claude.ai/code/session_01AbCd"},
		},
		{
			name:     "a live session without a bridge id changes nothing",
			sessions: []bridgedSession{{71, idSafari, "", true}},
			cloud:    []Conversation{cloudRow("cse_01AbCd", "Fix the parser", "2026-11-07T11:30:00Z")},
			wantIDs:  []string{"cse_01AbCd", idSafari},
		},
		{
			name:     "a malformed bridge id changes nothing",
			sessions: []bridgedSession{{71, idSafari, "cse_01AbCd", true}},
			cloud:    []Conversation{cloudRow("cse_01AbCd", "Fix the parser", "2026-11-07T11:30:00Z")},
			wantIDs:  []string{"cse_01AbCd", idSafari},
		},
		{
			name:     "a bridge id with characters an id never holds changes nothing",
			sessions: []bridgedSession{{71, idSafari, "session_01Ab/Cd", true}},
			cloud:    []Conversation{cloudRow("cse_01AbCd", "Fix the parser", "2026-11-07T11:30:00Z")},
			wantIDs:  []string{"cse_01AbCd", idSafari},
		},
		{
			name:     "a bridge id the cloud does not list changes nothing",
			sessions: []bridgedSession{{71, idSafari, "session_09Zz", true}},
			cloud:    []Conversation{cloudRow("cse_01AbCd", "Fix the parser", "2026-11-07T11:30:00Z")},
			wantIDs:  []string{"cse_01AbCd", idSafari},
		},
		{
			name:     "two live sessions that claim one cloud id merge neither",
			sessions: []bridgedSession{{71, idSafari, "session_01AbCd", true}, {72, idCache, "session_01AbCd", true}},
			cloud:    []Conversation{cloudRow("cse_01AbCd", "Fix the parser", "2026-11-07T11:30:00Z")},
			wantIDs:  []string{"cse_01AbCd", idCache, idSafari},
		},
		{
			name:     "an ended session leaves the plain cloud row",
			sessions: []bridgedSession{{71, idSafari, "session_01AbCd", false}},
			cloud:    []Conversation{cloudRow("cse_01AbCd", "Fix the parser", "2026-11-07T11:30:00Z")},
			wantIDs:  []string{"cse_01AbCd", idSafari},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			alive := map[int]bool{}
			for _, s := range tc.sessions {
				alive[s.pid] = s.alive
			}
			scanner, stores := testScanner(t, noSQLite, fakeProcesses{alive: alive})
			scanner.Now = func() time.Time { return time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC) }
			for i, s := range tc.sessions {
				writeMarkedSession(t, stores, s.id, time.Date(2026, 11, 7, 11, i, 0, 0, time.UTC).Format(time.RFC3339))
				writeBridgedSession(t, stores, s.pid, s.id, "waiting", s.bridge)
			}
			scanner.Cloud = oneCloudList(tc.cloud...)

			view := scanner.Scan(context.Background(), ScanOptions{})

			ids := idsOf(view)
			if !slices.Equal(slices.Sorted(slices.Values(ids)), slices.Sorted(slices.Values(tc.wantIDs))) {
				t.Errorf("rows = %v, want %v", ids, tc.wantIDs)
			}
			for _, c := range view.Conversations {
				if c.ID == tc.sharedID {
					if asJSON(t, c.Cloud) != asJSON(t, tc.wantCloud) {
						t.Errorf("shared row cloud = %s, want %s", asJSON(t, c.Cloud), asJSON(t, tc.wantCloud))
					}
					continue
				}
				if c.Cloud != nil {
					t.Errorf("row %s claims cloud %s", c.ID, asJSON(t, c.Cloud))
				}
			}
			if view.Tools[0].Count != len(tc.wantIDs) {
				t.Errorf("Claude Code counts %d rows, want %d", view.Tools[0].Count, len(tc.wantIDs))
			}
		})
	}
}

func TestScan_ASharedRowTakesItsStateFromTheLocalSessionAndCountsOnce(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{alive: map[int]bool{71: true}})
	scanner.Now = func() time.Time { return time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC) }
	writeMarkedSession(t, stores, idSafari, "2026-11-07T11:00:00Z")
	writeBridgedSession(t, stores, 71, idSafari, "waiting", "session_01AbCd")
	cloud := cloudRow("cse_01AbCd", "Fix the parser", "2026-11-07T11:30:00Z")
	cloud.Live, cloud.Activity = true, ActivityWorking
	scanner.Cloud = oneCloudList(cloud)

	view := scanner.Scan(context.Background(), ScanOptions{})

	row := byID(t, view, idSafari)
	if row.State != StateBlocked || row.App != AppCLI || row.Open.Kind != RouteFocus {
		t.Errorf("shared row = %s", asJSON(t, row))
	}
	if view.NeedsYou != 1 {
		t.Errorf("needs you = %d, want the shared row once", view.NeedsYou)
	}
}

func TestFind_ASharedRowsCloudIDOpensItsClaudeAIPage(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{alive: map[int]bool{71: true}})
	scanner.Now = func() time.Time { return time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC) }
	writeMarkedSession(t, stores, idSafari, "2026-11-07T11:00:00Z")
	writeBridgedSession(t, stores, 71, idSafari, "idle", "session_01AbCd")
	scanner.Cloud = oneCloudList(cloudRow("cse_01AbCd", "Fix the parser", "2026-11-07T11:30:00Z"))

	local, ok := scanner.Find(context.Background(), ToolClaude, idSafari, ScanOptions{})
	if !ok || local.App != AppCLI {
		t.Fatalf("local = %v %s", ok, asJSON(t, local))
	}
	launcher := &fakeLauncher{}
	reply, found := scanner.Reopen(context.Background(), ToolClaude, "cse_01AbCd", ScanOptions{}, launcher)
	if !found || !reply.Opened || len(launcher.calls) != 1 || launcher.calls[0] != "link https://claude.ai/code/session_01AbCd" {
		t.Errorf("reply %+v after %v", reply, launcher.calls)
	}
}

// A mark on the cloud row before the merge said the person saw the
// conversation; the shared row keeps that seen time. Completion, hiding
// and a chosen name belonged to the duplicate and do not carry over.
func TestScan_ASharedRowKeepsTheSeenMarkOfItsCloudRow(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{alive: map[int]bool{71: true}})
	scanner.MarksPath = filepath.Join(t.TempDir(), "marks.json")
	at := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	scanner.Now = func() time.Time { return at }
	writeMarkedSession(t, stores, idSafari, "2026-11-07T11:00:00Z")
	scanner.Cloud = oneCloudList(cloudRow("cse_01AbCd", "Fix the parser", "2026-11-07T11:30:00Z"))

	// The marks file starts before the conversation's last write, so it is unread.
	if view := scanner.Scan(context.Background(), ScanOptions{}); byID(t, view, idSafari).State != StateUnread {
		t.Fatalf("before the mark: %s", asJSON(t, view.Conversations))
	}
	at = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	if err := scanner.Mark(ToolClaude, "cse_01AbCd", MarkHidden); err != nil {
		t.Fatal(err)
	}
	writeBridgedSession(t, stores, 71, idSafari, "idle", "session_01AbCd")

	view := scanner.Scan(context.Background(), ScanOptions{})

	row := byID(t, view, idSafari)
	if row.State != StateActive || row.SeenAt != "2026-11-07T12:00:00Z" || row.Cloud == nil {
		t.Errorf("shared row = %s", asJSON(t, row))
	}
	if view.NeedsYou != 0 || len(view.Conversations) != 1 {
		t.Errorf("needs you %d, rows %s", view.NeedsYou, asJSON(t, view.Conversations))
	}
}

func TestScan_HidingASharedRowHidesBoth(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{alive: map[int]bool{71: true}})
	scanner.MarksPath = filepath.Join(t.TempDir(), "marks.json")
	scanner.Now = func() time.Time { return time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC) }
	writeMarkedSession(t, stores, idSafari, "2026-11-07T11:00:00Z")
	writeBridgedSession(t, stores, 71, idSafari, "idle", "session_01AbCd")
	scanner.Cloud = oneCloudList(cloudRow("cse_01AbCd", "Fix the parser", "2026-11-07T11:30:00Z"))
	if err := scanner.Mark(ToolClaude, idSafari, MarkHidden); err != nil {
		t.Fatal(err)
	}
	if view := scanner.Scan(context.Background(), ScanOptions{}); len(view.Conversations) != 0 {
		t.Errorf("rows = %s", asJSON(t, view.Conversations))
	}
}

// sharedScanner lists idSafari, live as pid 71 with Remote Control on,
// beside its cloud row cse_01AbCd written at 11:30.
func sharedScanner(t *testing.T) (*Scanner, Stores, *time.Time) {
	t.Helper()
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{alive: map[int]bool{71: true}})
	scanner.MarksPath = filepath.Join(t.TempDir(), "marks.json")
	at := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	scanner.Now = func() time.Time { return at }
	writeMarkedSession(t, stores, idSafari, "2026-11-07T11:00:00Z")
	writeBridgedSession(t, stores, 71, idSafari, "idle", "session_01AbCd")
	scanner.Cloud = oneCloudList(cloudRow("cse_01AbCd", "Fix the parser", "2026-11-07T11:30:00Z"))
	scanner.Scan(context.Background(), ScanOptions{})
	at = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	return scanner, stores, &at
}

func TestMark_OnASharedRowAlsoMarksItsCloudRow(t *testing.T) {
	for name, end := range map[string]func(t *testing.T, scanner *Scanner, stores Stores){
		"the session ends": func(t *testing.T, scanner *Scanner, _ Stores) {
			scanner.Processes = fakeProcesses{}
		},
		"Remote Control is turned off": func(t *testing.T, _ *Scanner, stores Stores) {
			writeBridgedSession(t, stores, 71, idSafari, "idle", "")
		},
	} {
		t.Run(name, func(t *testing.T) {
			scanner, stores, _ := sharedScanner(t)
			if view := scanner.Scan(context.Background(), ScanOptions{}); view.NeedsYou != 1 || byID(t, view, idSafari).Cloud == nil {
				t.Fatalf("before the mark: needs you %d, rows %s", view.NeedsYou, asJSON(t, view.Conversations))
			}
			if err := scanner.Mark(ToolClaude, idSafari, MarkSeen); err != nil {
				t.Fatal(err)
			}
			if view := scanner.Scan(context.Background(), ScanOptions{}); view.NeedsYou != 0 {
				t.Fatalf("after the mark: needs you %d", view.NeedsYou)
			}
			end(t, scanner, stores)

			view := scanner.Scan(context.Background(), ScanOptions{})

			if cloud := byID(t, view, "cse_01AbCd"); cloud.State != StateActive || cloud.SeenAt != "2026-11-07T12:00:00Z" {
				t.Errorf("cloud row = %s", asJSON(t, cloud))
			}
			if view.NeedsYou != 0 {
				t.Errorf("needs you = %d", view.NeedsYou)
			}
		})
	}
}

func TestMark_TakingACompletionBackOnASharedRowTakesItBackOnItsCloudRow(t *testing.T) {
	scanner, _, at := sharedScanner(t)
	*at = time.Date(2026, 11, 7, 11, 15, 0, 0, time.UTC)
	if err := scanner.Mark(ToolClaude, idSafari, MarkCompleted); err != nil {
		t.Fatal(err)
	}
	if err := scanner.Mark(ToolClaude, idSafari, MarkOpen); err != nil {
		t.Fatal(err)
	}
	scanner.Processes = fakeProcesses{}

	view := scanner.Scan(context.Background(), ScanOptions{})

	if cloud := byID(t, view, "cse_01AbCd"); cloud.State != StateUnread || cloud.CompletedAt != "" {
		t.Errorf("cloud row = %s", asJSON(t, cloud))
	}
}

func TestReopen_ASharedRowMarksItsCloudRowSeen(t *testing.T) {
	scanner, stores, _ := sharedScanner(t)
	// The desktop app opens it by link, which the fake launcher takes.
	write(t, filepath.Join(stores.ClaudeDir, "sessions", "71.json"), `{"pid":71,"sessionId":"`+idSafari+`","status":"idle","entrypoint":"claude-desktop","bridgeSessionId":"session_01AbCd"}`)
	if reply, found := scanner.Reopen(context.Background(), ToolClaude, idSafari, ScanOptions{}, &fakeLauncher{}); !found || !reply.Opened {
		t.Fatalf("found %v, reply %+v", found, reply)
	}
	scanner.Processes = fakeProcesses{}

	if cloud := byID(t, scanner.Scan(context.Background(), ScanOptions{}), "cse_01AbCd"); cloud.State != StateActive || cloud.SeenAt != "2026-11-07T12:00:00Z" {
		t.Errorf("cloud row = %s", asJSON(t, cloud))
	}
}

func TestFind_DoesNotListTheCloudIDOfASharedRow(t *testing.T) {
	scanner, _, _ := sharedScanner(t)
	if _, ok := scanner.Find(context.Background(), ToolClaude, "cse_01AbCd", ScanOptions{}); ok {
		t.Error("a mark or a rename could name a row nobody sees")
	}
}
