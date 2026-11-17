package conversations

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/chromeidb/chromeidbtest"
)

const (
	appChatID  = "6dad25fc-d7d3-413d-92de-b34e6d3aed07"
	appChat2ID = "97689660-5cd5-44a4-ac73-b2d27269f9d3"
	appTaskID  = "local_ffcf90de-f3fc-4a2a-a29c-c56f314c3fd7"
	appCloudID = "cse_01RhRk64JVj8UziXydyKaHdH"
)

// millis is a time as the Claude app's Cowork lists store it.
func millis(at string) float64 {
	parsed, err := time.Parse(time.RFC3339, at)
	if err != nil {
		panic(err)
	}
	return float64(parsed.UnixMilli())
}

func chatListQuery(filter obj, chats ...any) obj {
	return obj{
		"queryKey": []any{"chat_conversation_list", obj{"orgUuid": "7d23479f-7a92-47ac-b877-9bf9f9f85a13"}, "infinite", filter},
		"state":    obj{"data": obj{"pageParams": []any{nil}, "pages": []any{obj{"has_more": false, "data": chats}}}},
	}
}

func chat(id, name, updated string, liveStatus any) obj {
	return obj{"uuid": id, "name": name, "created_at": "2026-11-07T08:00:00.000Z", "updated_at": updated, "last_settled_at": updated, "live_status": liveStatus}
}

func coworkTask(id, title, active string, running, archived bool) obj {
	return obj{"sessionId": id, "title": title, "cwd": "", "createdAt": millis("2026-11-06T10:00:00Z"), "lastActivityAt": millis(active), "isRunning": running, "isArchived": archived}
}

// writeClaudeApp writes the Claude app's cached queries where the
// scanner looks for them.
func writeClaudeApp(t *testing.T, stores Stores, queries ...any) {
	t.Helper()
	dir := filepath.Join(stores.ClaudeAppDir, "IndexedDB", "https_claude.ai_0.indexeddb.leveldb")
	cache := obj{"timestamp": 1.0, "buster": "", "clientState": obj{"mutations": []any{}, "queries": queries}}
	chromeidbtest.WriteRecord(t, dir, "keyval-store", "keyval", "react-query-cache", cache)
}

func appRows(view View) map[string]Conversation {
	rows := map[string]Conversation{}
	for _, c := range view.Conversations {
		if c.App == AppChat || c.App == AppCowork {
			rows[c.ID] = c
		}
	}
	return rows
}

func TestScan_ListsTheClaudeAppsChatsAndCoworkTasks(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	writeClaudeApp(t, stores,
		obj{"queryKey": []any{"current_account"}, "state": obj{"data": obj{"uuid": "a"}}},
		chatListQuery(obj{"limit": 30, "starred": nil, "archived": false},
			chat(appChatID, "Vague request", "2026-11-07T11:00:32.285Z", nil),
			chat(appChat2ID, "Firmenrad costs", "2026-11-07T10:00:00.000Z", "streaming"),
			chat("not-a-uuid", "Broken", "2026-11-07T10:00:00.000Z", nil),
		),
		obj{"queryKey": []any{"cowork-local-sessions", "org"}, "state": obj{"data": obj{"v": 1.0, "rows": []any{
			coworkTask(appTaskID, "Rome documentaries", "2026-11-07T09:00:00Z", false, false),
			coworkTask("local_11111111-1111-4111-8111-111111111111", "Archived task", "2026-11-07T09:00:00Z", false, true),
		}}}},
		obj{"queryKey": []any{"cowork-remote-sessions", "org", true}, "state": obj{"data": []any{
			coworkTask(appCloudID, "Haiku", "2026-11-07T08:00:00Z", true, false),
		}}},
	)
	view := scanner.Scan(context.Background(), ScanOptions{})
	for _, source := range view.Tools {
		if source.Tool == ToolClaude && !source.Found {
			t.Errorf("Claude app chats are listed but the source is unavailable: %+v", source)
		}
	}
	rows := appRows(view)
	if len(rows) != 5 {
		t.Fatalf("rows = %s", asJSON(t, rows))
	}
	got := rows[appChatID]
	if got.Tool != ToolClaude || got.Title != "Vague request" || got.UpdatedAt != "2026-11-07T11:00:32Z" || got.StartedAt != "2026-11-07T08:00:00Z" || got.Live {
		t.Fatalf("chat = %s", asJSON(t, got))
	}
	if got.Open != (Route{Kind: RouteLink, Label: "Open in Claude app", Link: "claude://claude.ai/chat/" + appChatID}) {
		t.Fatalf("chat route = %+v", got.Open)
	}
	if answering := rows[appChat2ID]; !answering.Live || answering.Activity != ActivityWorking {
		t.Fatalf("answering chat = %s", asJSON(t, answering))
	}
	task := rows[appTaskID]
	if task.App != AppCowork || task.Title != "Rome documentaries" || task.UpdatedAt != "2026-11-07T09:00:00Z" || task.Open.Link != "claude://claude.ai/cowork/"+appTaskID {
		t.Fatalf("task = %s", asJSON(t, task))
	}
	if archived := rows["local_11111111-1111-4111-8111-111111111111"]; archived.State != StateDormant || task.State != StateActive {
		t.Fatalf("a task archived in the app is %s and one not archived is %s, want dormant and active", archived.State, task.State)
	}
	if remote := rows[appCloudID]; !remote.Live || remote.Activity != ActivityWorking || remote.Open.Link != "claude://claude.ai/cowork/"+appCloudID {
		t.Fatalf("remote task = %s", asJSON(t, remote))
	}
}

func TestScan_ReadsOnlyTheClaudeAppsFullChatList(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	starredOnly := chatListQuery(obj{"limit": 30, "starred": true, "archived": false}, chat(appChatID, "Starred", "2026-11-07T11:00:00.000Z", nil))
	archived := chatListQuery(obj{"limit": 30, "starred": nil, "archived": true}, chat(appChat2ID, "Archived", "2026-11-07T11:00:00.000Z", nil))
	writeClaudeApp(t, stores, starredOnly, archived)
	if rows := appRows(scanner.Scan(context.Background(), ScanOptions{})); len(rows) != 0 {
		t.Fatalf("rows = %s", asJSON(t, rows))
	}
}

func TestScan_ListsNoClaudeAppRowsWithoutTheApp(t *testing.T) {
	scanner, _ := testScanner(t, noSQLite, fakeProcesses{})
	view := scanner.Scan(context.Background(), ScanOptions{})
	if rows := appRows(view); len(rows) != 0 {
		t.Fatalf("rows = %s", asJSON(t, rows))
	}
	for _, source := range view.Tools {
		if source.Tool == ToolClaude && source.Note != "No Claude Code sessions in "+filepath.Join(scanner.Stores.ClaudeDir, "projects") {
			t.Fatalf("claude source = %+v", source)
		}
	}
}

func TestScan_KeepsTheClaudeAppsChatsOutOfAFolderScope(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	writeClaudeApp(t, stores, chatListQuery(obj{"limit": 30, "starred": nil, "archived": false}, chat(appChatID, "Vague request", "2026-11-07T11:00:00.000Z", nil)))
	if rows := appRows(scanner.Scan(context.Background(), ScanOptions{Within: []string{stores.Home}})); len(rows) != 0 {
		t.Fatalf("rows = %s", asJSON(t, rows))
	}
}

func TestScan_ListsACoworkTaskInsideTheFolderScope(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	inside := coworkTask(appTaskID, "Inside", "2026-11-07T09:00:00Z", false, false)
	inside["cwd"] = filepath.Join(stores.Home, "project")
	outside := coworkTask("local_11111111-1111-4111-8111-111111111111", "Outside", "2026-11-07T09:00:00Z", false, false)
	outside["cwd"] = filepath.Join(t.TempDir(), "elsewhere")
	writeClaudeApp(t, stores,
		chatListQuery(obj{"limit": 30, "starred": nil, "archived": false}, chat(appChatID, "Vague request", "2026-11-07T11:00:00.000Z", nil)),
		obj{"queryKey": []any{"cowork-local-sessions", "org"}, "state": obj{"data": obj{"v": 1.0, "rows": []any{inside, outside}}}},
	)
	rows := appRows(scanner.Scan(context.Background(), ScanOptions{Within: []string{stores.Home}}))
	if len(rows) != 1 || rows[appTaskID].Folder != filepath.Join(stores.Home, "project") {
		t.Fatalf("rows = %s", asJSON(t, rows))
	}
}

func claudeSourceNote(t *testing.T, view View) string {
	t.Helper()
	for _, source := range view.Tools {
		if source.Tool == ToolClaude {
			return source.Note
		}
	}
	t.Fatalf("no Claude source in %s", asJSON(t, view.Tools))
	return ""
}

func TestScan_SaysWhyTheClaudeAppsChatsCouldNotBeRead(t *testing.T) {
	noSessions := "No Claude Code sessions in "
	for name, tc := range map[string]struct {
		write func(t *testing.T, dir string)
		note  string
	}{
		"a log in a shape the reader does not know": {func(t *testing.T, dir string) {
			// One log record of an unknown type, 9.
			write(t, filepath.Join(dir, "000003.log"), "\x00\x00\x00\x00\x01\x00\x09x")
		}, claudeAppNoteUnreadable},
		"a store with no chat list": {func(t *testing.T, dir string) {
			chromeidbtest.WriteRecord(t, dir, "keyval-store", "keyval", "something-else", obj{})
		}, claudeAppNoteMissing},
	} {
		t.Run(name, func(t *testing.T) {
			scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
			dir := filepath.Join(stores.ClaudeAppDir, "IndexedDB", "https_claude.ai_0.indexeddb.leveldb")
			tc.write(t, dir)

			view := scanner.Scan(context.Background(), ScanOptions{})

			if rows := appRows(view); len(rows) != 0 {
				t.Fatalf("rows = %s", asJSON(t, rows))
			}
			note := claudeSourceNote(t, view)
			if !strings.HasPrefix(note, noSessions) || !strings.HasSuffix(note, " "+tc.note) {
				t.Fatalf("note = %q", note)
			}

			// The app writes its cache again, and the next scan reads it.
			if err := os.RemoveAll(dir); err != nil {
				t.Fatal(err)
			}
			writeClaudeApp(t, stores, chatListQuery(obj{"limit": 30, "starred": nil, "archived": false}, chat(appChatID, "Vague request", "2026-11-07T11:00:00.000Z", nil)))

			view = scanner.Scan(context.Background(), ScanOptions{})

			if rows := appRows(view); len(rows) != 1 {
				t.Fatalf("rows = %s", asJSON(t, rows))
			}
			if note := claudeSourceNote(t, view); strings.Contains(note, "Claude app") {
				t.Fatalf("note = %q", note)
			}
		})
	}
}

func TestScan_CountsAClaudeChatTheAssistantAnsweredUntilSeen(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	scanner.Now = func() time.Time { return now }
	list := func(updated string) {
		writeClaudeApp(t, stores, chatListQuery(obj{"limit": 30, "starred": nil, "archived": false}, chat(appChatID, "Vague request", updated, nil)))
	}
	list("2026-11-07T11:00:00.000Z")
	// The first scan starts the marks file: what was written before counts as seen.
	if view := scanner.Scan(context.Background(), ScanOptions{}); view.NeedsYou != 0 {
		t.Fatalf("before a reply: needsYou = %d", view.NeedsYou)
	}

	now = now.Add(time.Hour)
	list("2026-11-07T12:30:00.000Z")
	view := scanner.Scan(context.Background(), ScanOptions{})
	if c := byID(t, view, appChatID); c.State != StateUnread || view.NeedsYou != 1 {
		t.Fatalf("after a reply: %s, needsYou = %d", asJSON(t, c), view.NeedsYou)
	}

	if err := scanner.Mark(ToolClaude, appChatID, MarkSeen); err != nil {
		t.Fatal(err)
	}
	if view := scanner.Scan(context.Background(), ScanOptions{}); view.NeedsYou != 0 {
		t.Fatalf("after seeing it: needsYou = %d", view.NeedsYou)
	}
}

func TestStamp_ChangesWhenTheClaudeAppWritesItsCache(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	before := scanner.Stamp()
	writeClaudeApp(t, stores, chatListQuery(obj{"limit": 30, "starred": nil, "archived": false}, chat(appChatID, "Vague request", "2026-11-07T11:00:00.000Z", nil)))
	if scanner.Stamp() == before {
		t.Fatal("the stamp stayed the same after the app wrote its cache")
	}
}

func TestDeleteUnsupported_SendsAClaudeAppConversationToTheApp(t *testing.T) {
	scanner, _ := testScanner(t, noSQLite, fakeProcesses{})
	for _, app := range []App{AppChat, AppCowork} {
		got := scanner.deleteUnsupported(Conversation{Tool: ToolClaude, App: app, ID: appChatID})
		if got != "This conversation is kept by its app, not in a file here. Delete it in the app." {
			t.Fatalf("%s: %q", app, got)
		}
	}
}
