package conversations

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The Claude app caches only its list of chats not archived, so a chat
// that leaves it is read as archived when it cannot have just fallen
// off the loaded pages.

const appChat3ID = "c3c3c3c3-3c3c-43c3-83c3-c3c3c3c3c3c3"

// writeClaudeChats stores the app's sidebar list with chats, newest first.
func writeClaudeChats(t *testing.T, stores Stores, chats ...any) {
	t.Helper()
	writeClaudeApp(t, stores, chatListQuery(obj{"limit": 30, "starred": nil, "archived": false}, chats...))
}

// replaceClaudeApp removes the app's storage, so the next write starts it again.
func replaceClaudeApp(t *testing.T, stores Stores) string {
	t.Helper()
	dir := filepath.Join(stores.ClaudeAppDir, "IndexedDB", "https_claude.ai_0.indexeddb.leveldb")
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestScan_AClaudeChatThatLeavesTheAppsListIsArchived(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	start := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	writeClaudeChats(t, stores,
		chat(appChatID, "Newest", "2026-11-07T11:00:00.000Z", nil),
		chat(appChat2ID, "Middle", "2026-11-07T10:00:00.000Z", nil),
		chat(appChat3ID, "Oldest", "2026-11-07T09:00:00.000Z", nil),
	)
	scanAt(t, scanner, start)

	// Middle is archived in the app: it leaves the list, and Oldest is still there.
	replaceClaudeApp(t, stores)
	writeClaudeChats(t, stores,
		chat(appChatID, "Newest", "2026-11-07T11:00:00.000Z", nil),
		chat(appChat3ID, "Oldest", "2026-11-07T09:00:00.000Z", nil),
	)
	for i := 1; i <= 2; i++ {
		c := byID(t, scanAt(t, scanner, start.Add(time.Duration(i)*2*time.Minute)), appChat2ID)
		if c.State != StateDormant || c.Title != "Middle" || c.App != AppChat || c.UpdatedAt != "2026-11-07T10:00:00Z" {
			t.Fatalf("read %d: chat gone from the list = %s, want it dormant as it was", i, asJSON(t, c))
		}
	}
	if c := byID(t, scanAt(t, scanner, start.Add(6*time.Minute)), appChatID); c.State != StateActive {
		t.Fatalf("chat still in the list: %s, want active", c.State)
	}

	// Un-archived in the app, it is back in the list and a normal row.
	replaceClaudeApp(t, stores)
	writeClaudeChats(t, stores,
		chat(appChatID, "Newest", "2026-11-07T11:00:00.000Z", nil),
		chat(appChat2ID, "Middle", "2026-11-07T10:00:00.000Z", nil),
		chat(appChat3ID, "Oldest", "2026-11-07T09:00:00.000Z", nil),
	)
	if c := byID(t, scanAt(t, scanner, start.Add(8*time.Minute)), appChat2ID); c.State != StateActive {
		t.Fatalf("chat back in the list: %s, want active", c.State)
	}
}

func TestScan_AClaudeChatOlderThanTheCachedListLeavesOurList(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	start := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	writeClaudeChats(t, stores,
		chat(appChatID, "Newest", "2026-11-07T11:00:00.000Z", nil),
		chat(appChat3ID, "Oldest", "2026-11-07T09:00:00.000Z", nil),
	)
	scanAt(t, scanner, start)

	// Oldest may only sit on a page the app no longer caches.
	replaceClaudeApp(t, stores)
	writeClaudeChats(t, stores, chat(appChatID, "Newest", "2026-11-07T11:00:00.000Z", nil))
	if _, listed := appRows(scanAt(t, scanner, start.Add(2*time.Minute)))[appChat3ID]; listed {
		t.Fatal("a chat older than every cached one is listed")
	}

	// It is forgotten, so a list that reaches further back does not bring it back as archived.
	replaceClaudeApp(t, stores)
	writeClaudeChats(t, stores,
		chat(appChatID, "Newest", "2026-11-07T11:00:00.000Z", nil),
		chat(appChat2ID, "Older still", "2026-11-07T08:00:00.000Z", nil),
	)
	if _, listed := appRows(scanAt(t, scanner, start.Add(4*time.Minute)))[appChat3ID]; listed {
		t.Fatal("a forgotten chat came back")
	}
}

func TestScan_AFailedReadOfTheClaudeAppInfersNoArchive(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	start := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	writeClaudeChats(t, stores,
		chat(appChatID, "Newest", "2026-11-07T11:00:00.000Z", nil),
		chat(appChat2ID, "Middle", "2026-11-07T10:00:00.000Z", nil),
	)
	scanAt(t, scanner, start)

	dir := replaceClaudeApp(t, stores)
	// One log record of an unknown type, 9.
	write(t, filepath.Join(dir, "000003.log"), "\x00\x00\x00\x00\x01\x00\x09x")
	if rows := appRows(scanAt(t, scanner, start.Add(2*time.Minute))); len(rows) != 0 {
		t.Fatalf("a failed read listed %s", asJSON(t, rows))
	}

	// The failed read changed nothing: Middle leaving the next good read still counts.
	replaceClaudeApp(t, stores)
	writeClaudeChats(t, stores,
		chat(appChatID, "Newest", "2026-11-07T11:00:00.000Z", nil),
		chat(appChat3ID, "Oldest", "2026-11-07T09:00:00.000Z", nil),
	)
	if c := byID(t, scanAt(t, scanner, start.Add(4*time.Minute)), appChat2ID); c.State != StateDormant {
		t.Fatalf("after the failed read: %s, want dormant", c.State)
	}
}

func TestScan_AClaudeAppReadWithoutTheSidebarInfersNoArchive(t *testing.T) {
	for name, without := range map[string]func(t *testing.T, stores Stores){
		"a sidebar list that does not decode": func(t *testing.T, stores Stores) {
			t.Helper()
			writeClaudeApp(t, stores, obj{
				"queryKey": []any{"chat_conversation_list", obj{"orgUuid": "o"}, "infinite", obj{"limit": 30, "starred": nil, "archived": false}},
				"state":    obj{"data": "not pages"},
			})
		},
		"no storage folder": func(*testing.T, Stores) {},
	} {
		t.Run(name, func(t *testing.T) {
			scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
			start := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
			writeClaudeChats(t, stores,
				chat(appChatID, "Newest", "2026-11-07T11:00:00.000Z", nil),
				chat(appChat2ID, "Middle", "2026-11-07T10:00:00.000Z", nil),
			)
			scanAt(t, scanner, start)

			replaceClaudeApp(t, stores)
			without(t, stores)
			if rows := appRows(scanAt(t, scanner, start.Add(2*time.Minute))); len(rows) != 0 {
				t.Fatalf("a read without the sidebar listed %s", asJSON(t, rows))
			}

			replaceClaudeApp(t, stores)
			writeClaudeChats(t, stores,
				chat(appChatID, "Newest", "2026-11-07T11:00:00.000Z", nil),
				chat(appChat3ID, "Oldest", "2026-11-07T09:00:00.000Z", nil),
			)
			if c := byID(t, scanAt(t, scanner, start.Add(4*time.Minute)), appChat2ID); c.State != StateDormant {
				t.Fatalf("the read without the sidebar lost what was seen: %s, want dormant", c.State)
			}
		})
	}
}

func TestScan_AForgottenClaudeChatLeavesNoArchiveBehind(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	start := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	writeClaudeChats(t, stores,
		chat(appChatID, "Newest", "2026-11-07T11:00:00.000Z", nil),
		chat(appChat2ID, "Middle", "2026-11-07T10:00:00.000Z", nil),
		chat(appChat3ID, "Oldest", "2026-11-07T09:00:00.000Z", nil),
	)
	scanAt(t, scanner, start)
	replaceClaudeApp(t, stores)
	writeClaudeChats(t, stores,
		chat(appChatID, "Newest", "2026-11-07T11:00:00.000Z", nil),
		chat(appChat3ID, "Oldest", "2026-11-07T09:00:00.000Z", nil),
	)
	scanAt(t, scanner, start.Add(2*time.Minute))
	if _, kept := scanner.appArchives[markKey(ToolClaude, appChat2ID)]; !kept {
		t.Fatal("the archived chat has no archive")
	}

	// The list now starts after Middle, so it is forgotten, archive and all.
	replaceClaudeApp(t, stores)
	writeClaudeChats(t, stores, chat(appChatID, "Newest", "2026-11-07T11:00:00.000Z", nil))
	scanAt(t, scanner, start.Add(4*time.Minute))
	if _, kept := scanner.appArchives[markKey(ToolClaude, appChat2ID)]; kept {
		t.Fatal("a forgotten chat left its archive behind")
	}
}

func TestScan_ClaudeCodeSessionsAreNotArchivedByAbsence(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	start := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	writeClaudeSessions(t, stores)
	writeClaudeChats(t, stores, chat(appChatID, "Newest", "2026-11-07T11:00:00.000Z", nil))
	scanAt(t, scanner, start)

	// The session's transcript goes, as a delete in Claude Code does.
	if err := os.Remove(filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", idSafari+".jsonl")); err != nil {
		t.Fatal(err)
	}
	for _, c := range scanAt(t, scanner, start.Add(2*time.Minute)).Conversations {
		if c.ID == idSafari {
			t.Fatalf("a Claude Code session gone from its store is listed: %s", asJSON(t, c))
		}
		if c.Tool == ToolClaude && c.App != AppChat && c.App != AppCowork && c.State == StateDormant && c.underAppArchive {
			t.Fatalf("a Claude Code session is under an app archive: %s", asJSON(t, c))
		}
	}
}
