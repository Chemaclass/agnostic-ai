package conversations

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"regexp"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/chromeidb"
)

// This file is the only code that knows how the Claude desktop app
// caches the person's chats and Cowork tasks. The app is claude.ai in
// a window: its web page keeps the lists it last fetched in IndexedDB,
// as a persisted react-query cache. What it reads was measured on the
// app on 2026-11-12 and can change with any app release. It reads the
// lists only, never a chat's messages, and sends nothing anywhere.

// The cache is one record in the page's keyval store.
const (
	claudeAppOrigin   = "https_claude.ai_0.indexeddb.leveldb"
	claudeAppDatabase = "keyval-store"
	claudeAppStore    = "keyval"
	claudeAppCacheKey = "react-query-cache"
)

// The cached queries the list is built from.
const (
	claudeChatListQuery     = "chat_conversation_list"
	claudeCoworkLocalQuery  = "cowork-local-sessions"
	claudeCoworkRemoteQuery = "cowork-remote-sessions"
	// claudeInfinite marks the chat list the sidebar pages through, as
	// against a one-page list such as the starred chats.
	claudeInfinite = "infinite"
)

// Ids the app's links accept: a chat is a UUID; a Cowork task is a
// local_ UUID on this machine or a cse_ session in the cloud.
var (
	claudeChatIDPattern   = sessionIDPattern
	claudeCoworkIDPattern = regexp.MustCompile(`^(local_[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|cse_[A-Za-z0-9]{1,64})$`)
)

type claudeAppCache struct {
	ClientState struct {
		Queries []claudeAppQuery `json:"queries"`
	} `json:"clientState"`
}

type claudeAppQuery struct {
	QueryKey []json.RawMessage `json:"queryKey"`
	State    struct {
		Data json.RawMessage `json:"data"`
	} `json:"state"`
}

// claudeChatListFilter is the last part of the chat list's query key.
type claudeChatListFilter struct {
	Starred  *bool `json:"starred"`
	Archived *bool `json:"archived"`
}

type claudeChatListData struct {
	Pages []struct {
		Data []claudeChat `json:"data"`
	} `json:"pages"`
}

// claudeChat is one row of claude.ai's chat list.
type claudeChat struct {
	UUID      string `json:"uuid"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	// LiveStatus is set while the assistant answers, and empty once it
	// is done. Only empty values were seen; that a set one means an
	// answer in progress is inferred from its name. last_settled_at
	// cannot tell: the server touches updated_at a second after it.
	LiveStatus json.RawMessage `json:"live_status"`
}

// claudeCowork is one Cowork task, local or in the cloud.
type claudeCowork struct {
	SessionID      string  `json:"sessionId"`
	Title          string  `json:"title"`
	Cwd            string  `json:"cwd"`
	CreatedAt      float64 `json:"createdAt"`
	LastActivityAt float64 `json:"lastActivityAt"`
	IsRunning      bool    `json:"isRunning"`
	IsArchived     bool    `json:"isArchived"`
}

type claudeCoworkLocalData struct {
	Rows []claudeCowork `json:"rows"`
}

// What the Claude source's note says when the app's lists could not be
// read. The reader's own error text stays out of it.
const (
	claudeAppNoteMissing    = "Could not find the chat list in the Claude app's storage."
	claudeAppNoteUnreadable = "Could not read the chat list in the Claude app's storage."
)

// claudeAppRead is one read of the app's cached lists: the rows, or the
// note that says why there are none.
type claudeAppRead struct {
	rows []Conversation
	note string
}

// claudeAppRows returns the chats and Cowork tasks the Claude app has
// cached, and a note when they could not be read. A read, failed or
// not, is kept until the app's storage files change, so the next write
// reads them again. An app that was never used gives no rows and no
// note.
func (s *Scanner) claudeAppRows(opts ScanOptions) ([]Conversation, string) {
	if s.Stores.ClaudeAppDir == "" {
		return nil, ""
	}
	stamp := fileStamp(s.claudeAppFiles()...)
	if s.claudeApp.stamp != stamp {
		s.claudeApp = cached[claudeAppRead]{stamp: stamp, value: s.readClaudeApp(s.claudeAppDB())}
	}
	return within(slices.Clone(s.claudeApp.value.rows), opts.Within), s.claudeApp.value.note
}

// claudeAppDB is the LevelDB folder of the app's web storage.
func (s *Scanner) claudeAppDB() string {
	return filepath.Join(s.Stores.ClaudeAppDir, "IndexedDB", claudeAppOrigin)
}

// claudeAppFiles are the files the app's cached lists are read from.
// A large record lives in the .blob folder, but writing one also
// writes the LevelDB log, so the LevelDB files tell every change.
func (s *Scanner) claudeAppFiles() []string {
	if s.Stores.ClaudeAppDir == "" {
		return nil
	}
	files, _ := filepath.Glob(filepath.Join(s.claudeAppDB(), "*"))
	return files
}

// claudeAppNotes is what the note says for each reason a read failed.
var claudeAppNotes = map[chromeidb.Reason]string{
	chromeidb.ReasonMissing:    claudeAppNoteMissing,
	chromeidb.ReasonUnreadable: claudeAppNoteUnreadable,
}

func (s *Scanner) readClaudeApp(dir string) claudeAppRead {
	folder, err := chromeidb.Open(dir)
	if claudeAppReason(err) == chromeidb.ReasonMissing {
		// No storage folder: the app was never used, which is no failure.
		return claudeAppRead{}
	}
	if err != nil {
		return claudeAppRead{note: claudeAppNotes[claudeAppReason(err)]}
	}
	value, err := folder.Get(claudeAppDatabase, claudeAppStore, claudeAppCacheKey)
	if err != nil {
		return claudeAppRead{note: claudeAppNotes[claudeAppReason(err)]}
	}
	// The decoded value is plain maps and slices; JSON gives it types.
	raw, err := claudeAppCacheJSON(value)
	if err != nil {
		return claudeAppRead{note: claudeAppNoteUnreadable}
	}
	var cache claudeAppCache
	if json.Unmarshal(raw, &cache) != nil {
		return claudeAppRead{note: claudeAppNoteUnreadable}
	}
	var rows []Conversation
	seen := map[string]bool{}
	keep := func(c Conversation) {
		if seen[c.ID] {
			return
		}
		seen[c.ID] = true
		rows = append(rows, c)
	}
	for _, query := range cache.ClientState.Queries {
		switch queryName(query) {
		case claudeChatListQuery:
			for _, chat := range claudeChatsOf(query) {
				if c, ok := s.claudeChatRow(chat); ok {
					keep(c)
				}
			}
		case claudeCoworkLocalQuery:
			var data claudeCoworkLocalData
			if json.Unmarshal(query.State.Data, &data) == nil {
				for _, task := range data.Rows {
					if c, ok := s.claudeCoworkRow(task); ok {
						keep(c)
					}
				}
			}
		case claudeCoworkRemoteQuery:
			var tasks []claudeCowork
			if json.Unmarshal(query.State.Data, &tasks) == nil {
				for _, task := range tasks {
					if c, ok := s.claudeCoworkRow(task); ok {
						keep(c)
					}
				}
			}
		}
	}
	return claudeAppRead{rows: rows}
}

// claudeAppReason is why the reader failed with err, or no reason for
// no error. The reader returns only its own error type.
func claudeAppReason(err error) chromeidb.Reason {
	var readErr *chromeidb.Error
	if !errors.As(err, &readErr) {
		return ""
	}
	return readErr.Reason
}

func queryName(query claudeAppQuery) string {
	if len(query.QueryKey) == 0 {
		return ""
	}
	var name string
	if json.Unmarshal(query.QueryKey[0], &name) != nil {
		return ""
	}
	return name
}

// claudeChatsOf returns the chats of the sidebar's list: the paged one
// that holds every chat not archived, starred or not. The app keeps no
// list of archived chats on this machine (none was cached on
// 2026-11-17), so a chat archived in the app leaves this list.
func claudeChatsOf(query claudeAppQuery) []claudeChat {
	key := query.QueryKey
	if len(key) != 4 {
		return nil
	}
	var marker string
	var filter claudeChatListFilter
	if json.Unmarshal(key[2], &marker) != nil || marker != claudeInfinite || json.Unmarshal(key[3], &filter) != nil {
		return nil
	}
	if filter.Starred != nil || filter.Archived == nil || *filter.Archived {
		return nil
	}
	var data claudeChatListData
	if json.Unmarshal(query.State.Data, &data) != nil {
		return nil
	}
	var chats []claudeChat
	for _, page := range data.Pages {
		chats = append(chats, page.Data...)
	}
	return chats
}

func (s *Scanner) claudeChatRow(chat claudeChat) (Conversation, bool) {
	updated := isoTime(chat.UpdatedAt)
	if !claudeChatIDPattern.MatchString(chat.UUID) || updated == "" {
		return Conversation{}, false
	}
	c := Conversation{
		Tool: ToolClaude, App: AppChat, ID: chat.UUID,
		StartedAt: isoTime(chat.CreatedAt), UpdatedAt: updated,
		Open: Route{Kind: RouteLink, Label: "Open in Claude app", Link: "claude://claude.ai/chat/" + chat.UUID},
	}
	if answering(chat.LiveStatus) {
		c.Live, c.Activity = true, ActivityWorking
	}
	s.fill(&c, texts{title: chat.Name})
	return c, true
}

// answering reports whether a chat's live status is set: anything but
// absent, null or an empty string.
func answering(status json.RawMessage) bool {
	switch string(status) {
	case "", "null", `""`:
		return false
	default:
		return true
	}
}

func (s *Scanner) claudeCoworkRow(task claudeCowork) (Conversation, bool) {
	updated := timeFromMillis(int64(task.LastActivityAt))
	if !claudeCoworkIDPattern.MatchString(task.SessionID) || updated == "" {
		return Conversation{}, false
	}
	// The app keeps no time for when a task was archived.
	c := Conversation{
		Tool: ToolClaude, App: AppCowork, ID: task.SessionID, Folder: task.Cwd,
		StartedAt: timeFromMillis(int64(task.CreatedAt)), UpdatedAt: updated,
		Open:        Route{Kind: RouteLink, Label: "Open in Claude app", Link: "claude://claude.ai/cowork/" + task.SessionID},
		appArchived: task.IsArchived,
	}
	if task.IsRunning {
		c.Live, c.Activity = true, ActivityWorking
	}
	s.fill(&c, texts{title: task.Title})
	return c, true
}
