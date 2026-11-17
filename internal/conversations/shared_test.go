package conversations

import (
	"context"
	"encoding/json"
	"os"
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
	for name, end := range map[string]func(t *testing.T, scanner *Scanner, stores Stores) *Scanner{
		// The server that saw them shared keeps them one row; a new one lists both.
		"the session ends and the server restarts": func(t *testing.T, scanner *Scanner, stores Stores) *Scanner {
			return restartedScanner(scanner, stores)
		},
		"Remote Control is turned off": func(t *testing.T, scanner *Scanner, stores Stores) *Scanner {
			writeBridgedSession(t, stores, 71, idSafari, "idle", "")
			return scanner
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
			view := end(t, scanner, stores).Scan(context.Background(), ScanOptions{})

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
	scanner, stores, at := sharedScanner(t)
	*at = time.Date(2026, 11, 7, 11, 15, 0, 0, time.UTC)
	if err := scanner.Mark(ToolClaude, idSafari, MarkCompleted); err != nil {
		t.Fatal(err)
	}
	if err := scanner.Mark(ToolClaude, idSafari, MarkOpen); err != nil {
		t.Fatal(err)
	}

	view := restartedScanner(scanner, stores).Scan(context.Background(), ScanOptions{})

	if cloud := byID(t, view, "cse_01AbCd"); cloud.CompletedAt != "" {
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
	if cloud := byID(t, restartedScanner(scanner, stores).Scan(context.Background(), ScanOptions{}), "cse_01AbCd"); cloud.State != StateActive || cloud.SeenAt != "2026-11-07T12:00:00Z" {
		t.Errorf("cloud row = %s", asJSON(t, cloud))
	}
}

func TestFind_DoesNotListTheCloudIDOfASharedRow(t *testing.T) {
	scanner, _, _ := sharedScanner(t)
	if _, ok := scanner.Find(context.Background(), ToolClaude, "cse_01AbCd", ScanOptions{}); ok {
		t.Error("a mark or a rename could name a row nobody sees")
	}
}

// restartedScanner is a new server on the same stores, marks and cloud
// once the session ended: it never saw the session shared.
func restartedScanner(scanner *Scanner, stores Stores) *Scanner {
	return &Scanner{Stores: stores, SQLite: noSQLite, Processes: fakeProcesses{}, Now: scanner.Now, MarksPath: scanner.MarksPath,
		Cloud: oneCloudList(cloudRow("cse_01AbCd", "Fix the parser", "2026-11-07T11:30:00Z"))}
}

// closedTwinScanner is sharedScanner once the local session ended with
// a reply the person has not read.
func closedTwinScanner(t *testing.T) (*Scanner, Stores, *time.Time) {
	t.Helper()
	scanner, stores, at := sharedScanner(t)
	scanner.Processes = fakeProcesses{}
	return scanner, stores, at
}

func TestScan_AClosedSharedSessionStaysOneRowWithItsTwin(t *testing.T) {
	scanner, _, _ := closedTwinScanner(t)

	view := scanner.Scan(context.Background(), ScanOptions{})

	if ids := idsOf(view); !slices.Equal(ids, []string{idSafari}) {
		t.Fatalf("rows = %v, want the local row only", ids)
	}
	row := byID(t, view, idSafari)
	if row.State != StateDormant || !row.Unread || row.Cloud == nil || row.Cloud.ID != "cse_01AbCd" {
		t.Errorf("row = %s", asJSON(t, row))
	}
	if view.NeedsYou != 0 {
		t.Errorf("needs you = %d, want the twin to add nothing", view.NeedsYou)
	}
}

func TestReopen_TheTwinOfAClosedSessionClearsTheLocalRowAndItsDeleteRefusal(t *testing.T) {
	scanner, _, _ := closedTwinScanner(t)
	scanner.Scan(context.Background(), ScanOptions{})
	if reply, found := scanner.Reopen(context.Background(), ToolClaude, "cse_01AbCd", ScanOptions{}, &fakeLauncher{}); !found || !reply.Opened {
		t.Fatalf("found %v, reply %+v", found, reply)
	}

	view := scanner.Scan(context.Background(), ScanOptions{})

	if row := byID(t, view, idSafari); row.Unread || row.SeenAt != "2026-11-07T12:00:00Z" {
		t.Errorf("local row = %s", asJSON(t, row))
	}
	// The fixture's files carry today's times, so the delete stops later, at the write check.
	if _, err := scanner.Delete(context.Background(), ToolClaude, idSafari, ScanOptions{}); deleteRefusal(t, err).Kind == DeleteUnread {
		t.Errorf("delete still refused as unread: %v", err)
	}
}

func TestMark_TheLocalRowOfAClosedSessionAlsoMarksItsTwin(t *testing.T) {
	scanner, stores, _ := closedTwinScanner(t)
	scanner.Scan(context.Background(), ScanOptions{})
	if err := scanner.Mark(ToolClaude, idSafari, MarkSeen); err != nil {
		t.Fatal(err)
	}
	view := restartedScanner(scanner, stores).Scan(context.Background(), ScanOptions{})

	if cloud := byID(t, view, "cse_01AbCd"); cloud.State != StateActive || cloud.SeenAt != "2026-11-07T12:00:00Z" {
		t.Errorf("cloud row = %s", asJSON(t, cloud))
	}
	if view.NeedsYou != 0 {
		t.Errorf("needs you = %d", view.NeedsYou)
	}
}

func TestScan_AReplyOnTheTwinAfterTheSessionClosedCountsAsACloudChat(t *testing.T) {
	scanner, _, at := closedTwinScanner(t)
	*at = time.Date(2026, 11, 7, 13, 0, 0, 0, time.UTC)
	scanner.Cloud = oneCloudList(cloudRow("cse_01AbCd", "Fix the parser", "2026-11-07T12:30:00Z"))

	view := scanner.Scan(context.Background(), ScanOptions{})

	if cloud := byID(t, view, "cse_01AbCd"); cloud.State != StateUnread {
		t.Errorf("cloud row = %s", asJSON(t, cloud))
	}
	if row := byID(t, view, idSafari); row.Cloud != nil || row.State != StateDormant {
		t.Errorf("local row = %s", asJSON(t, row))
	}
	if view.NeedsYou != 1 {
		t.Errorf("needs you = %d, want the new cloud reply", view.NeedsYou)
	}
}

// The cloud list is asked once a minute, so the cloud's copy of the last
// reply can come after the last read that saw the session run.
func TestScan_TheTwinsCopyOfTheLastReplyKeepsItFolded(t *testing.T) {
	scanner, _, at := closedTwinScanner(t)
	*at = at.Add(time.Minute)
	scanner.Cloud = oneCloudList(cloudRow("cse_01AbCd", "Fix the parser", "2026-11-07T11:31:30Z"))

	view := scanner.Scan(context.Background(), ScanOptions{})

	if !slices.Equal(idsOf(view), []string{idSafari}) || view.NeedsYou != 0 {
		t.Errorf("needs you %d, rows %s", view.NeedsYou, asJSON(t, view.Conversations))
	}
}

// A cloud write past the window after the local session's last write is
// new work from the cloud side.
func TestScan_APhoneReplyAfterTheWindowCounts(t *testing.T) {
	scanner, stores, at := closedTwinScanner(t)
	writeMarkedSession(t, stores, idSafari, "2026-11-07T11:50:00Z")
	*at = time.Date(2026, 11, 7, 11, 50, 10, 0, time.UTC)
	if err := scanner.Mark(ToolClaude, idSafari, MarkSeen); err != nil {
		t.Fatal(err)
	}
	*at = time.Date(2026, 11, 7, 12, 1, 0, 0, time.UTC)
	scanner.Cloud = oneCloudList(cloudRow("cse_01AbCd", "Fix the parser", "2026-11-07T11:53:00Z"))

	view := scanner.Scan(context.Background(), ScanOptions{})

	if cloud := byID(t, view, "cse_01AbCd"); cloud.State != StateUnread || view.NeedsYou != 1 {
		t.Errorf("needs you %d, cloud row %s", view.NeedsYou, asJSON(t, cloud))
	}
}

// A reply inside the window stays folded, and the closed row shows it
// unread, so it is never counted nowhere: not while folded, and not on
// the twin once it shows apart.
func TestScan_APhoneReplyInsideTheWindowIsUnreadOnExactlyOneRow(t *testing.T) {
	phone := cloudRow("cse_01AbCd", "Fix the parser", "2026-11-07T11:31:00Z")
	unreadRows := func(view View) []string {
		var ids []string
		for _, c := range view.Conversations {
			if c.Unread || needsPerson(c.State) {
				ids = append(ids, c.ID)
			}
		}
		return ids
	}
	for name, after := range map[string]func(t *testing.T, scanner *Scanner, stores Stores) *Scanner{
		"while folded": func(*testing.T, *Scanner, Stores) *Scanner { return nil },
		"after a restart": func(_ *testing.T, scanner *Scanner, stores Stores) *Scanner {
			return &Scanner{Stores: stores, SQLite: noSQLite, Processes: fakeProcesses{}, Now: scanner.Now, MarksPath: scanner.MarksPath, Cloud: oneCloudList(phone)}
		},
		"after a resume without Remote Control": func(t *testing.T, scanner *Scanner, stores Stores) *Scanner {
			scanner.Processes = fakeProcesses{alive: map[int]bool{71: true}}
			writeBridgedSession(t, stores, 71, idSafari, "idle", "")
			return scanner
		},
	} {
		t.Run(name, func(t *testing.T) {
			scanner, stores, at := closedTwinScanner(t)
			*at = time.Date(2026, 11, 7, 11, 30, 30, 0, time.UTC)
			if err := scanner.Mark(ToolClaude, idSafari, MarkSeen); err != nil {
				t.Fatal(err)
			}
			*at = time.Date(2026, 11, 7, 12, 1, 0, 0, time.UTC)
			scanner.Cloud = oneCloudList(phone)
			view := scanner.Scan(context.Background(), ScanOptions{})
			if row := byID(t, view, idSafari); !slices.Equal(idsOf(view), []string{idSafari}) || !row.Unread || row.UpdatedAt != "2026-11-07T11:31:00Z" {
				t.Fatalf("folded: rows %s", asJSON(t, view.Conversations))
			}
			next := after(t, scanner, stores)
			if next == nil {
				return
			}
			*at = at.Add(time.Minute)

			view = next.Scan(context.Background(), ScanOptions{})

			if got := unreadRows(view); !slices.Equal(got, []string{"cse_01AbCd"}) {
				t.Errorf("unread rows = %v, want the twin only: %s", got, asJSON(t, view.Conversations))
			}
		})
	}
}

// The list is asked only when someone looks: the last read that saw the
// session run can be long before its last reply and its close.
func TestScan_ATwinStaysFoldedAfterAGapBetweenTheLastLiveReadAndTheClose(t *testing.T) {
	scanner, stores, at := closedTwinScanner(t)
	writeMarkedSession(t, stores, idSafari, "2026-11-07T13:00:00Z")
	*at = time.Date(2026, 11, 7, 13, 5, 0, 0, time.UTC)
	scanner.Cloud = oneCloudList(cloudRow("cse_01AbCd", "Fix the parser", "2026-11-07T13:00:20Z"))

	view := scanner.Scan(context.Background(), ScanOptions{})

	if !slices.Equal(idsOf(view), []string{idSafari}) || view.NeedsYou != 0 {
		t.Errorf("needs you %d, rows %s", view.NeedsYou, asJSON(t, view.Conversations))
	}
}

func TestScan_ARestartedServerListsTheTwinApartWithoutCountingTheOldReply(t *testing.T) {
	scanner, stores, _ := closedTwinScanner(t)
	scanner.Scan(context.Background(), ScanOptions{})

	view := restartedScanner(scanner, stores).Scan(context.Background(), ScanOptions{})

	if ids := idsOf(view); len(ids) != 2 {
		t.Errorf("rows = %v, want the local row and its twin apart", ids)
	}
	if cloud := byID(t, view, "cse_01AbCd"); cloud.State != StateActive || cloud.Unread || view.NeedsYou != 0 {
		t.Errorf("needs you %d, cloud row %s", view.NeedsYou, asJSON(t, cloud))
	}
	if row := byID(t, view, idSafari); !row.Unread {
		t.Errorf("local row = %s", asJSON(t, row))
	}
}

func TestScan_ASessionResumedWithoutItsLinkDoesNotCountTheOldReplyOnItsTwin(t *testing.T) {
	for name, bridge := range map[string]string{"Remote Control off": "", "a new bridge id": "session_09Zz"} {
		t.Run(name, func(t *testing.T) {
			scanner, stores, _ := closedTwinScanner(t)
			scanner.Scan(context.Background(), ScanOptions{})
			scanner.Processes = fakeProcesses{alive: map[int]bool{71: true}}
			writeBridgedSession(t, stores, 71, idSafari, "idle", bridge)

			view := scanner.Scan(context.Background(), ScanOptions{})

			if cloud := byID(t, view, "cse_01AbCd"); cloud.State != StateActive || view.NeedsYou != 0 {
				t.Errorf("needs you %d, rows %s", view.NeedsYou, asJSON(t, view.Conversations))
			}
		})
	}
}

func TestReopen_TheTwinOfAClosedSessionClearsTheLocalRowAfterARestart(t *testing.T) {
	scanner, stores, _ := closedTwinScanner(t)
	scanner.Scan(context.Background(), ScanOptions{})
	if reply, found := scanner.Reopen(context.Background(), ToolClaude, "cse_01AbCd", ScanOptions{}, &fakeLauncher{}); !found || !reply.Opened {
		t.Fatalf("found %v, reply %+v", found, reply)
	}
	restarted := restartedScanner(scanner, stores)

	if row := byID(t, restarted.Scan(context.Background(), ScanOptions{}), idSafari); row.Unread {
		t.Errorf("local row = %s", asJSON(t, row))
	}
	if _, err := restarted.Delete(context.Background(), ToolClaude, idSafari, ScanOptions{}); deleteRefusal(t, err).Kind == DeleteUnread {
		t.Errorf("delete refused as unread: %v", err)
	}
}

// claude.ai still keeps the cloud session, so it stays listed.
func TestDelete_AFoldedClosedSessionLeavesItsTwinListedWithoutTheOldReply(t *testing.T) {
	scanner, stores, at := closedTwinScanner(t)
	*at = time.Now().UTC().Truncate(time.Second)
	scanner.Scan(context.Background(), ScanOptions{})
	if err := scanner.Mark(ToolClaude, idSafari, MarkSeen); err != nil {
		t.Fatal(err)
	}
	*at = at.Add(time.Second)
	scanner.Scan(context.Background(), ScanOptions{})

	reply, err := scanner.Delete(context.Background(), ToolClaude, idSafari, ScanOptions{})
	if err != nil || !reply.Deleted {
		t.Fatalf("delete = %s, %v", asJSON(t, reply), err)
	}
	if _, err := os.Stat(filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", idSafari+".jsonl")); !os.IsNotExist(err) {
		t.Errorf("transcript stays: %v", err)
	}

	view := scanner.Scan(context.Background(), ScanOptions{})

	if !slices.Equal(idsOf(view), []string{"cse_01AbCd"}) || view.NeedsYou != 0 {
		t.Errorf("needs you %d, rows %s", view.NeedsYou, asJSON(t, view.Conversations))
	}
}

func TestScan_AFailedReadKeepsTheTwinLink(t *testing.T) {
	scanner, _, at := closedTwinScanner(t)
	scanner.Scan(context.Background(), ScanOptions{})
	twinRow := func() ([]Conversation, error) {
		return []Conversation{cloudRow("cse_01AbCd", "Fix the parser", "2026-11-07T11:30:00Z")}, nil
	}
	scanner.Cloud = &countingCloud{answers: []func() ([]Conversation, error){
		func() ([]Conversation, error) {
			return nil, &CloudError{Tool: ToolClaude, Reason: CloudUnreachable, Cause: context.DeadlineExceeded}
		},
		twinRow,
	}}
	*at = at.Add(cloudRideSpan + time.Minute)
	if view := scanner.Scan(context.Background(), ScanOptions{}); !slices.Equal(idsOf(view), []string{idSafari}) {
		t.Fatalf("while the cloud fails: rows %v", idsOf(view))
	}
	scanner.Processes = fakeProcesses{readFails: true}
	*at = at.Add(time.Minute)
	if view := scanner.Scan(context.Background(), ScanOptions{}); !slices.Equal(idsOf(view), []string{idSafari}) || byID(t, view, idSafari).Cloud == nil {
		t.Fatalf("while the process read fails: rows %s", asJSON(t, view.Conversations))
	}
	scanner.Processes = fakeProcesses{}
	*at = at.Add(time.Minute)

	view := scanner.Scan(context.Background(), ScanOptions{})

	if !slices.Equal(idsOf(view), []string{idSafari}) || byID(t, view, idSafari).Cloud == nil {
		t.Errorf("after the reads work again: rows %s", asJSON(t, view.Conversations))
	}
}

func TestScan_DropsATwinLinkOnceEitherRowIsGone(t *testing.T) {
	for name, gone := range map[string]func(t *testing.T, scanner *Scanner, stores Stores){
		"the local session is gone": func(t *testing.T, _ *Scanner, stores Stores) {
			if err := os.Remove(filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", idSafari+".jsonl")); err != nil {
				t.Fatal(err)
			}
		},
		"the cloud session is gone": func(_ *testing.T, scanner *Scanner, _ Stores) {
			scanner.Cloud = oneCloudList()
		},
	} {
		t.Run(name, func(t *testing.T) {
			scanner, stores, at := closedTwinScanner(t)
			scanner.Scan(context.Background(), ScanOptions{})
			gone(t, scanner, stores)
			*at = at.Add(time.Minute)

			scanner.Scan(context.Background(), ScanOptions{})

			if len(scanner.twins) != 0 {
				t.Errorf("twins = %+v", scanner.twins)
			}
		})
	}
}

func TestRememberTwin_KeepsAtMostMaxTwinsAndDropsTheOldest(t *testing.T) {
	scanner := &Scanner{}
	start := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	for i := range maxTwins + 5 {
		scanner.rememberTwin("claude:"+strconv.Itoa(i), "cse_"+strconv.Itoa(i), start.Add(time.Duration(i)*time.Second))
	}
	if len(scanner.twins) != maxTwins {
		t.Errorf("twins = %d, want %d", len(scanner.twins), maxTwins)
	}
	if _, kept := scanner.twins["claude:0"]; kept {
		t.Error("the oldest link stays")
	}
	if _, kept := scanner.twins["claude:"+strconv.Itoa(maxTwins+4)]; !kept {
		t.Error("the newest link is gone")
	}
}
