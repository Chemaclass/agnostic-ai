package conversations

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"testing"
)

const (
	chatGPTDone    = "6abcd7fe-837c-83ed-919e-a699ea6b4809"
	chatGPTRunning = "6abb7da9-6cb8-83eb-b92e-2d0f4933976b"
	chatGPTAsync   = "6aab3f27-d45c-83ed-bf93-781c3d286c0d"
)

// chatGPTCatalog answers the chat-list query with rows, as sqlite3
// would, and fails any other query.
func chatGPTCatalog(t *testing.T, rows string) SQLiteQuery {
	return fakeStores(t, map[string]string{"codex.db|local_thread_catalog": rows})
}

// writeChatGPTApp puts the app's database where the scanner looks.
func writeChatGPTApp(t *testing.T, stores Stores) {
	t.Helper()
	write(t, filepath.Join(stores.CodexDir, "sqlite", "codex.db"), "")
}

func chatGPTSource(t *testing.T, view View) (Source, bool) {
	t.Helper()
	for _, source := range view.Tools {
		if source.Tool == ToolChatGPT {
			return source, true
		}
	}
	return Source{}, false
}

func chatGPTRows(view View) map[string]Conversation {
	rows := map[string]Conversation{}
	for _, c := range view.Conversations {
		if c.Tool == ToolChatGPT {
			rows[c.ID] = c
		}
	}
	return rows
}

func TestScan_ListsTheChatGPTAppsChats(t *testing.T) {
	scanner, stores := testScanner(t, chatGPTCatalog(t, `[
		{"thread_id":"`+chatGPTDone+`","display_title":"Lunch near the office","source_created_at":1762505759.41,"source_updated_at":1762509394.5,"chatgpt_async_status":null,"running":0},
		{"thread_id":"`+chatGPTRunning+`","display_title":"Long research","source_created_at":1762509000,"source_updated_at":1762510000,"chatgpt_async_status":null,"running":1},
		{"thread_id":"`+chatGPTAsync+`","display_title":"Deep research","source_created_at":1762509000,"source_updated_at":1762510000,"chatgpt_async_status":2,"running":0},
		{"thread_id":"not-a-chat-id","display_title":"Broken","source_created_at":1762509000,"source_updated_at":1762510000,"chatgpt_async_status":null,"running":0}
	]`), fakeProcesses{})
	writeChatGPTApp(t, stores)

	view := scanner.Scan(context.Background(), ScanOptions{})

	rows := chatGPTRows(view)
	if len(rows) != 3 {
		t.Fatalf("rows = %s", asJSON(t, view.Conversations))
	}
	done := rows[chatGPTDone]
	if done.App != AppChat || done.Title != "Lunch near the office" || done.StartedAt != "2025-11-07T08:55:59Z" || done.UpdatedAt != "2025-11-07T09:56:34Z" || done.Live {
		t.Errorf("done = %s", asJSON(t, done))
	}
	if done.Open != (Route{Kind: RouteLink, Label: "Open on chatgpt.com", Link: "https://chatgpt.com/c/" + chatGPTDone}) {
		t.Errorf("open = %s", asJSON(t, done.Open))
	}
	for _, id := range []string{chatGPTRunning, chatGPTAsync} {
		if running := rows[id]; !running.Live || running.Activity != ActivityWorking {
			t.Errorf("%s = %s", id, asJSON(t, running))
		}
	}
	if source, ok := chatGPTSource(t, view); !ok || !source.Found {
		t.Errorf("source = %s", asJSON(t, view.Tools))
	}
}

func TestScan_WithholdsAChatGPTTitleThatMayHoldACredential(t *testing.T) {
	scanner, stores := testScanner(t, chatGPTCatalog(t, `[
		{"thread_id":"`+chatGPTDone+`","display_title":"Debug ghp_abcdefghijklmnopqrstuvwxyz0123456789","source_created_at":1762505759,"source_updated_at":1762509394,"chatgpt_async_status":null,"running":0}
	]`), fakeProcesses{})
	writeChatGPTApp(t, stores)

	view := scanner.Scan(context.Background(), ScanOptions{})

	row := chatGPTRows(view)[chatGPTDone]
	if row.Title != "" || !slices.Equal(row.Withheld, []string{FieldTitle}) {
		t.Errorf("row = %s", asJSON(t, row))
	}
}

func TestScan_LeavesChatGPTOutWithoutTheApp(t *testing.T) {
	scanner, _ := testScanner(t, noSQLite, fakeProcesses{})

	view := scanner.Scan(context.Background(), ScanOptions{})

	if source, ok := chatGPTSource(t, view); ok {
		t.Errorf("source = %s", asJSON(t, source))
	}
}

func TestScan_SaysWhyTheChatGPTAppCouldNotBeRead(t *testing.T) {
	cases := []struct {
		name   string
		sqlite SQLiteQuery
		note   string
	}{
		{"no sqlite3 command", noSQLite, "Reading ChatGPT chats needs the sqlite3 command"},
		{"a database the query fails on", func(context.Context, string, string) ([]json.RawMessage, error) {
			return nil, errors.New("sqlite3 codex.db: no such table: local_thread_catalog")
		}, "Could not read the ChatGPT app's chat list codex.db"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scanner, stores := testScanner(t, tc.sqlite, fakeProcesses{})
			writeChatGPTApp(t, stores)

			view := scanner.Scan(context.Background(), ScanOptions{})

			source, ok := chatGPTSource(t, view)
			if !ok || source.Found || source.Note != tc.note {
				t.Errorf("source = %s", asJSON(t, source))
			}
		})
	}
}

func TestScan_ListsNoChatGPTChatsInAFolder(t *testing.T) {
	scanner, stores := testScanner(t, chatGPTCatalog(t, `[
		{"thread_id":"`+chatGPTDone+`","display_title":"Lunch","source_created_at":1762505759,"source_updated_at":1762509394,"chatgpt_async_status":null,"running":0}
	]`), fakeProcesses{})
	writeChatGPTApp(t, stores)

	view := scanner.Scan(context.Background(), ScanOptions{Within: []string{stores.Home}})

	if rows := chatGPTRows(view); len(rows) != 0 {
		t.Errorf("rows = %s", asJSON(t, rows))
	}
}

func TestStamp_ChangesWhenTheChatGPTAppWrites(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	writeChatGPTApp(t, stores)
	before := scanner.Stamp()

	write(t, filepath.Join(stores.CodexDir, "sqlite", "codex.db-wal"), "new pages")

	if scanner.Stamp() == before {
		t.Error("the stamp held after the app wrote its log")
	}
}
