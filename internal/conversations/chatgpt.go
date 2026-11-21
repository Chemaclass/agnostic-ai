package conversations

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
)

// This file is the only code that knows how the ChatGPT desktop app
// keeps its chat list. The app (bundle com.openai.codex) writes a
// catalog of the person's chats to a SQLite database in Codex's home,
// so they are read from this machine with no network call. What it
// reads was measured on one machine on 2026-11-12 and can change with
// any release of the app. It reads titles and times only, never a
// chat's messages. A chat started on chatgpt.com or a phone shows once
// the app has synced it.

// chatGPTCatalogPath is the app's database, inside Codex's home.
func chatGPTCatalogPath(codexDir string) string {
	return filepath.Join(codexDir, "sqlite", "codex.db")
}

// chatGPTChatsQuery lists the app's ChatGPT chats. Rows of another kind
// (cli, vscode) are Codex threads, which the Codex reader lists. A row
// marked missing_candidate is one the app no longer found on its last
// pass. running says whether active_chat_turns holds the chat.
const chatGPTChatsQuery = `SELECT c.thread_id, c.display_title, c.source_created_at, c.source_updated_at,
       c.chatgpt_async_status,
       EXISTS (SELECT 1 FROM active_chat_turns a WHERE a.thread_id = c.thread_id) AS running
FROM local_thread_catalog c
WHERE c.source_kind = 'chatgpt' AND c.missing_candidate = 0`

// chatGPTLink opens a chat on chatgpt.com.
const chatGPTLink = "https://chatgpt.com/c/"

// chatGPTIDPattern is a chat id, shaped as a UUID.
var chatGPTIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// chatGPTLinkPattern is the one chatgpt.com page the server opens.
var chatGPTLinkPattern = regexp.MustCompile(`^https://chatgpt\.com/c/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type chatGPTRow struct {
	ID    string `json:"thread_id"`
	Title string `json:"display_title"`
	// CreatedAt and UpdatedAt are seconds since 1970, with a fraction.
	CreatedAt float64 `json:"source_created_at"`
	UpdatedAt float64 `json:"source_updated_at"`
	// AsyncStatus is set while an answer runs in the background. Only
	// null was seen; the meaning of a set one is inferred from its name.
	AsyncStatus *int64 `json:"chatgpt_async_status"`
	// Running is 1 when active_chat_turns holds the chat. That table
	// was only seen empty; that it holds the chats whose reply is being
	// written is inferred from its name and columns.
	Running int `json:"running"`
}

// chatGPTFiles are the files a ChatGPT read depends on.
func (s *Scanner) chatGPTFiles() []string {
	return withWAL(chatGPTCatalogPath(s.Stores.CodexDir))
}

// hasChatGPTApp says whether the app's database is on this machine.
// Without it ChatGPT is left out of the tools, with nothing said.
func (s *Scanner) hasChatGPTApp() bool {
	_, err := os.Stat(chatGPTCatalogPath(s.Stores.CodexDir))
	return err == nil
}

// scanChatGPT reads the app's chats. Folder scope lists none: a chat
// belongs to no folder.
func (s *Scanner) scanChatGPT(ctx context.Context, opts ScanOptions) sourceResult {
	if len(opts.Within) > 0 {
		return found(ToolChatGPT, nil)
	}
	path := chatGPTCatalogPath(s.Stores.CodexDir)
	rows, err := queryRows[chatGPTRow](ctx, s.SQLite, path, chatGPTChatsQuery)
	if errors.Is(err, errNoSQLite) {
		return needsSQLite(ToolChatGPT, "Reading ChatGPT chats needs the sqlite3 command")
	}
	if err != nil {
		return missing(ToolChatGPT, "Could not read the ChatGPT app's chat list "+filepath.Base(path))
	}
	conversations := make([]Conversation, 0, len(rows))
	for _, row := range rows {
		if !chatGPTIDPattern.MatchString(row.ID) {
			continue
		}
		conversations = append(conversations, s.chatGPTConversation(row))
	}
	return found(ToolChatGPT, conversations)
}

// chatGPTConversation builds a chat's row. Its title goes through the
// same credential check as every other reader's texts.
func (s *Scanner) chatGPTConversation(row chatGPTRow) Conversation {
	c := Conversation{
		Tool: ToolChatGPT, App: AppChat, ID: row.ID,
		StartedAt: timeFromMillis(int64(row.CreatedAt * 1000)), UpdatedAt: timeFromMillis(int64(row.UpdatedAt * 1000)),
		Open: Route{Kind: RouteLink, Label: "Open on chatgpt.com", Link: chatGPTLink + row.ID},
	}
	if row.Running == 1 || row.AsyncStatus != nil {
		c.Live, c.Activity = true, ActivityWorking
	}
	s.fill(&c, texts{title: row.Title})
	return c
}
