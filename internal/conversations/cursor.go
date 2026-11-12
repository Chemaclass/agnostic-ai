package conversations

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Cursor indexes its editor chats in the composerHeaders table of its
// global state database; the messages sit in the same database's
// cursorDiskKV table, which holds gigabytes and is only read by key.

var cursorIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

// cursorHeader is one chat from composerHeaders: everything a row needs
// except its prompts.
type cursorHeader struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	CreatedAt   int64  `json:"created_at"`
	Recency     int64  `json:"recency"`
	Name        string `json:"name"`
	Folder      string `json:"folder"`
	Branch      string `json:"branch"`
}

// Chats with a name have messages; subagent and archived chats are left out.
const cursorHeadersQuery = `SELECT composerId AS id, coalesce(workspaceId, '') AS workspace_id, createdAt AS created_at, recency,
  json_extract(value, '$.name') AS name,
  coalesce(json_extract(value, '$.workspaceIdentifier.uri.fsPath'), '') AS folder,
  coalesce(json_extract(value, '$.createdOnBranch'), '') AS branch
FROM composerHeaders
WHERE isSubagent = 0 AND isArchived = 0 AND json_extract(value, '$.name') IS NOT NULL
ORDER BY recency DESC`

type cursorPrompts struct {
	ID    string `json:"id"`
	First string `json:"first_text"`
	Last  string `json:"last_text"`
	replyEnds
}

// cursorPromptsQuery finds the first and last user message of the given
// chats, and the agent's latest message with text after the last one.
// Older chats list their message headers inline; newer ones keep them in
// composerConvHeaders:<id>:<chunk> keys. A header whose grouping lacks
// hasText is a tool call or thinking, so its bubble is not read.
var cursorPromptsQuery = `WITH chosen(composerId) AS (VALUES %s),
headers AS (
  SELECT c.composerId, '' AS chunk, CAST(h.key AS INTEGER) AS seq, json_extract(h.value, '$.type') AS type, json_extract(h.value, '$.bubbleId') AS bubbleId
  FROM chosen c
  JOIN cursorDiskKV d ON d.key = 'composerData:' || c.composerId,
       json_each(d.value, '$.fullConversationHeadersOnly') h
  WHERE json_extract(h.value, '$.type') = 1 OR (json_extract(h.value, '$.type') = 2 AND coalesce(json_extract(h.value, '$.grouping.hasText'), json_type(h.value, '$.grouping') IS NULL))
  UNION ALL
  SELECT c.composerId, k.key AS chunk, CAST(h.key AS INTEGER) AS seq, json_extract(h.value, '$.type') AS type, json_extract(h.value, '$.bubbleId') AS bubbleId
  FROM chosen c
  JOIN cursorDiskKV k ON k.key >= 'composerConvHeaders:' || c.composerId || ':'
                     AND k.key <  'composerConvHeaders:' || c.composerId || ';',
       json_each(k.value) h
  WHERE json_extract(h.value, '$.type') = 1 OR (json_extract(h.value, '$.type') = 2 AND coalesce(json_extract(h.value, '$.grouping.hasText'), json_type(h.value, '$.grouping') IS NULL))
),
prompts AS (
  SELECT composerId,
    (SELECT bubbleId FROM headers h2 WHERE h2.composerId = h.composerId AND h2.type = 1 ORDER BY chunk, seq LIMIT 1) AS firstBubble,
    (SELECT bubbleId FROM headers h2 WHERE h2.composerId = h.composerId AND h2.type = 1 ORDER BY chunk DESC, seq DESC LIMIT 1) AS lastBubble,
    (SELECT chunk FROM headers h2 WHERE h2.composerId = h.composerId AND h2.type = 1 ORDER BY chunk DESC, seq DESC LIMIT 1) AS lastChunk,
    (SELECT seq FROM headers h2 WHERE h2.composerId = h.composerId AND h2.type = 1 ORDER BY chunk DESC, seq DESC LIMIT 1) AS lastSeq
  FROM headers h GROUP BY composerId
),
ends AS (
  SELECT p.*,
    (SELECT json_extract(b.value, '$.text') FROM headers h2
     JOIN cursorDiskKV b ON b.key = 'bubbleId:' || h2.composerId || ':' || h2.bubbleId
     WHERE h2.composerId = p.composerId AND h2.type = 2
       AND (p.lastBubble IS NULL OR h2.chunk > p.lastChunk OR (h2.chunk = p.lastChunk AND h2.seq > p.lastSeq))
       AND coalesce(json_extract(b.value, '$.text'), '') <> ''
     ORDER BY h2.chunk DESC, h2.seq DESC LIMIT 1) AS reply
  FROM prompts p
)
SELECT e.composerId AS id,
  coalesce(json_extract(fb.value, '$.text'), '') AS first_text,
  coalesce(json_extract(lb.value, '$.text'), '') AS last_text,
  ` + replyEndsColumns(`coalesce(e.reply, '')`) + `
FROM ends e
LEFT JOIN cursorDiskKV fb ON fb.key = 'bubbleId:' || e.composerId || ':' || e.firstBubble
LEFT JOIN cursorDiskKV lb ON lb.key = 'bubbleId:' || e.composerId || ':' || e.lastBubble`

func (s *Scanner) scanCursor(ctx context.Context, opts ScanOptions) sourceResult {
	if _, err := os.Stat(s.Stores.CursorDB); err != nil {
		return missing(ToolCursor, "No Cursor chat index at "+s.Stores.CursorDB)
	}
	headers, err := queryRows[cursorHeader](ctx, s.SQLite, s.Stores.CursorDB, cursorHeadersQuery)
	if errors.Is(err, errNoSQLite) {
		return missing(ToolCursor, "Reading Cursor chats needs the sqlite3 command")
	}
	if err != nil {
		return missing(ToolCursor, "Could not read Cursor's chat index")
	}

	open := s.cursorOpenChats(ctx, headers)
	folders := map[string]bool{}
	var editorChats []Conversation
	for _, header := range headers {
		if !cursorIDPattern.MatchString(header.ID) {
			continue
		}
		folders[header.Folder] = true
		editorChats = append(editorChats, Conversation{
			Tool: ToolCursor, App: AppIDE, ID: header.ID, Folder: header.Folder, Branch: header.Branch,
			StartedAt: timeFromMillis(header.CreatedAt), UpdatedAt: timeFromMillis(header.Recency), Live: open[header.ID],
			Open:    Route{Kind: RouteLink, Label: "Open in Cursor", Link: "cursor://anysphere.cursor-deeplink/agent?id=" + header.ID},
			pending: &texts{title: header.Name},
		})
	}
	all := append(within(editorChats, opts.Within), within(s.cursorAgentChats(ctx, folders, opts.Within), opts.Within)...)
	return found(ToolCursor, all)
}

// fillPending reads the prompts of the page's Cursor editor chats, the
// only rows whose prompts wait for a page: the store holds thousands.
func (s *Scanner) fillPending(ctx context.Context, page []Conversation) {
	prompts := s.cursorEditorPrompts(ctx, pendingRows(page))
	for i := range page {
		c := &page[i]
		if c.pending != nil {
			t := withPrompts(*c.pending, prompts[c.ID])
			c.pending = nil
			s.fill(c, t)
		}
	}
}

// searchAll keeps the rows whose shown text holds every word. A pending
// Cursor chat is checked on its raw text first, a superset of what it
// shows, so only the chats that may match pay for the credential check;
// the match is then made on the shown text, so withheld text never matches.
func (s *Scanner) searchAll(ctx context.Context, all []Conversation, words []string) []Conversation {
	prompts := s.cursorEditorPrompts(ctx, pendingRows(all))
	var maybe []Conversation
	for _, c := range all {
		if c.pending != nil {
			t := withPrompts(*c.pending, prompts[c.ID])
			if !holdsAll(strings.Join([]string{c.renamed, t.title, t.firstPrompt, t.lastPrompt, t.reply, t.question, c.Folder, c.Branch, toolLabels[c.Tool], string(c.App)}, "\n"), words) {
				continue
			}
			c.pending = nil
			s.fill(&c, t)
		}
		maybe = append(maybe, c)
	}
	return matching(maybe, words)
}

func pendingRows(list []Conversation) []Conversation {
	var pending []Conversation
	for _, c := range list {
		if c.pending != nil {
			pending = append(pending, c)
		}
	}
	return pending
}

func withPrompts(t texts, prompts cursorPrompts) texts {
	t.firstPrompt, t.lastPrompt = cursorTyped(prompts.First), cursorTyped(prompts.Last)
	t.reply, t.question = agentSaid(prompts.Start, prompts.wholeLines(), "")
	return t
}

var cursorQuery = regexp.MustCompile(`(?s)<user_query>(.*?)</user_query>`)

// cursorTyped is what the person typed in a Cursor prompt. Cursor puts
// timestamps, notifications and attachments beside <user_query> in the
// same text, so a prompt that has one keeps only what is inside it.
func cursorTyped(text string) string {
	found := cursorQuery.FindAllStringSubmatch(text, -1)
	if len(found) == 0 {
		return text
	}
	queries := make([]string, len(found))
	for i, match := range found {
		queries[i] = match[1]
	}
	return strings.Join(queries, "\n")
}

var cursorWorkspacePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// cursorOpenChats returns the editor chats open right now: Cursor runs,
// a window shows the chat's folder, and that workspace has the chat
// selected. Each workspace keeps its selection in its own database.
func (s *Scanner) cursorOpenChats(ctx context.Context, headers []cursorHeader) map[string]bool {
	open := map[string]bool{}
	if !s.Processes.AppRunning("Cursor") {
		return open
	}
	userDir := filepath.Dir(filepath.Dir(s.Stores.CursorDB))
	windows := openWindowFolders(filepath.Join(filepath.Dir(s.Stores.CursorDB), "storage.json"))
	workspaces := map[string]bool{}
	for _, header := range headers {
		if header.Folder != "" && windows[folderKey(header.Folder)] && cursorWorkspacePattern.MatchString(header.WorkspaceID) {
			workspaces[header.WorkspaceID] = true
		}
	}
	type selection struct {
		Value string `json:"value"`
	}
	for workspace := range workspaces {
		rows, err := queryRows[selection](ctx, s.SQLite, filepath.Join(userDir, "workspaceStorage", workspace, "state.vscdb"), `SELECT value FROM ItemTable WHERE key = 'composer.composerData'`)
		if err != nil || len(rows) == 0 {
			continue
		}
		var state struct {
			SelectedComposerIDs []string `json:"selectedComposerIds"`
		}
		if json.Unmarshal([]byte(rows[0].Value), &state) != nil {
			continue
		}
		for _, id := range state.SelectedComposerIDs {
			open[id] = true
		}
	}
	return open
}

// cursorPromptBatch bounds how many chats one prompts query names.
const cursorPromptBatch = 300

// cursorEditorPrompts reads the first and last prompt of the editor
// chats among list. A chat's prompts are kept until its last activity
// time changes, so a search over thousands of chats queries only the
// ones that are new or changed.
func (s *Scanner) cursorEditorPrompts(ctx context.Context, list []Conversation) map[string]cursorPrompts {
	if s.cursorPrompts == nil {
		s.cursorPrompts = map[string]cached[cursorPrompts]{}
	}
	prompts := map[string]cursorPrompts{}
	var missing []string
	for _, c := range list {
		if c.App != AppIDE {
			continue
		}
		if hit, ok := s.cursorPrompts[c.ID]; ok && hit.stamp == c.UpdatedAt {
			prompts[c.ID] = hit.value
			continue
		}
		missing = append(missing, c.ID)
	}
	stamps := map[string]string{}
	for _, c := range list {
		stamps[c.ID] = c.UpdatedAt
	}
	for start := 0; start < len(missing); start += cursorPromptBatch {
		batch := missing[start:min(start+cursorPromptBatch, len(missing))]
		values := make([]string, len(batch))
		for i, id := range batch {
			// Ids match cursorIDPattern, so they need no escaping.
			values[i] = "('" + id + "')"
		}
		rows, err := queryRows[cursorPrompts](ctx, s.SQLite, s.Stores.CursorDB, fmt.Sprintf(cursorPromptsQuery, strings.Join(values, ",")))
		if err != nil {
			continue
		}
		found := map[string]cursorPrompts{}
		for _, row := range rows {
			found[row.ID] = row
		}
		// A chat with no prompt rows is cached too, so it is not asked again.
		for _, id := range batch {
			prompts[id] = found[id]
			s.cursorPrompts[id] = cached[cursorPrompts]{stamp: stamps[id], value: found[id]}
		}
	}
	return prompts
}

// cursorAgentMeta is the hex-encoded JSON under key "0" of a cursor-agent
// chat's store.db.
type cursorAgentMeta struct {
	AgentID   string `json:"agentId"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"createdAt"`
}

// cursorAgentDefaultName is the name cursor-agent gives every chat.
const cursorAgentDefaultName = "New Agent"

// cursorAgentChats lists cursor-agent CLI chats. They live under
// chats/<md5 of the folder>/<chat id>, so the folder is found by
// hashing the folders Cursor's editor chats and the project use.
func (s *Scanner) cursorAgentChats(ctx context.Context, folders map[string]bool, roots []string) []Conversation {
	stores, _ := filepath.Glob(filepath.Join(s.Stores.CursorDir, "chats", "*", "*", "store.db"))
	if len(stores) == 0 {
		return nil
	}
	byHash := map[string]string{}
	for folder := range folders {
		byHash[md5Hex(folder)] = folder
	}
	for _, root := range roots {
		byHash[md5Hex(root)] = root
	}
	if s.cursorAgents == nil {
		s.cursorAgents = map[string]cached[cursorAgentMeta]{}
		s.cursorTranscripts = map[string]cached[cursorAgentTexts]{}
	}
	seen := map[string]bool{}
	var chats []Conversation
	for _, store := range stores {
		seen[store] = true
		meta, ok := s.cursorAgentMeta(ctx, store)
		if !ok {
			continue
		}
		folder := byHash[filepath.Base(filepath.Dir(filepath.Dir(store)))]
		updated := modTime(store)
		transcript := s.cursorAgentTranscript(meta.AgentID, seen)
		title := meta.Name
		if title == cursorAgentDefaultName {
			title = ""
		}
		c := Conversation{
			Tool: ToolCursor, App: AppCLI, ID: meta.AgentID, Folder: folder,
			StartedAt: timeFromMillis(meta.CreatedAt), UpdatedAt: updated,
			Open:  Route{Kind: RouteTerminal, Label: "Open in Terminal", Command: resumeIn(folder, "cursor-agent --resume "+meta.AgentID)},
			store: store,
		}
		s.fill(&c, texts{title: title, firstPrompt: transcript.first, lastPrompt: transcript.last, reply: transcript.reply, question: transcript.question})
		chats = append(chats, c)
	}
	if ctx.Err() == nil {
		forget(s.cursorAgents, seen)
		forget(s.cursorTranscripts, seen)
	}
	s.markLiveCursorAgentChats(chats)
	return chats
}

// markLiveCursorAgentChats marks the chats a running cursor-agent has
// open. It keeps the chat's store.db open while the chat runs, and the
// file stays after it exits, so the test is whether a process still has
// it open. A running chat switches to its terminal, since a second
// resume would compete with it.
func (s *Scanner) markLiveCursorAgentChats(chats []Conversation) {
	cutoff := s.Now().Add(-liveWindow).UTC().Format(time.RFC3339)
	chatOf := map[string]int{}
	var recent []string
	for i, c := range chats {
		if c.UpdatedAt >= cutoff {
			chatOf[c.store] = i
			recent = append(recent, c.store)
		}
	}
	for store, pid := range s.Processes.Holders(recent) {
		i, ok := chatOf[store]
		if !ok {
			continue
		}
		c := &chats[i]
		c.Live = true
		c.process = &liveProcess{PID: pid}
		c.Open = Route{Kind: RouteFocus, Label: "Open in Terminal", Command: c.Open.Command}
	}
}

// cursorAgentMeta reads the chat's metadata from its store.db, or from
// the cache while the store and its WAL are unchanged. A store without
// usable metadata is cached too, so it is not queried again; a failed
// query is not, since sqlite3 may fail only for now.
func (s *Scanner) cursorAgentMeta(ctx context.Context, store string) (cursorAgentMeta, bool) {
	stamp := fileStamp(store, store+"-wal")
	if entry, ok := s.cursorAgents[store]; ok && entry.stamp == stamp {
		return entry.value, entry.value.AgentID != ""
	}
	rows, err := queryRows[cursorAgentMetaRow](ctx, s.SQLite, store, `SELECT value FROM meta WHERE key = '0'`)
	if err != nil {
		return cursorAgentMeta{}, false
	}
	meta := decodeCursorAgentMeta(rows)
	s.cursorAgents[store] = cached[cursorAgentMeta]{stamp: stamp, value: meta}
	return meta, meta.AgentID != ""
}

type cursorAgentMetaRow struct {
	Value string `json:"value"`
}

// decodeCursorAgentMeta returns the metadata in rows, or none when it
// is missing or its id is not a plain chat id.
func decodeCursorAgentMeta(rows []cursorAgentMetaRow) cursorAgentMeta {
	var meta cursorAgentMeta
	if len(rows) == 0 {
		return meta
	}
	decoded, err := hex.DecodeString(rows[0].Value)
	if err != nil || json.Unmarshal(decoded, &meta) != nil || !cursorIDPattern.MatchString(meta.AgentID) {
		return cursorAgentMeta{}
	}
	return meta
}

// cursorAgentTexts are what a row keeps of a cursor-agent transcript.
type cursorAgentTexts struct {
	first, last     string
	reply, question string
}

// cursorAgentLine is one line of a cursor-agent transcript.
type cursorAgentLine struct {
	Role    string `json:"role"`
	Message struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"message"`
}

// cursorAgentTranscript reads the first and last prompt from the chat's
// transcript, where cursor-agent wraps what was typed in <user_query>,
// and the agent's latest message after the last prompt, or all of them
// from the cache while the transcript is unchanged. It adds the
// transcript to seen.
func (s *Scanner) cursorAgentTranscript(id string, seen map[string]bool) cursorAgentTexts {
	matches, _ := filepath.Glob(filepath.Join(s.Stores.CursorDir, "projects", "*", "agent-transcripts", id, id+".jsonl"))
	if len(matches) == 0 {
		return cursorAgentTexts{}
	}
	transcript := matches[0]
	seen[transcript] = true
	stamp := fileStamp(transcript)
	if entry, ok := s.cursorTranscripts[transcript]; ok && entry.stamp == stamp {
		return entry.value
	}
	info, err := os.Stat(transcript)
	if err != nil {
		return cursorAgentTexts{}
	}
	head, tail, err := readEnds(transcript, info.Size(), claudeWindow)
	if err != nil {
		return cursorAgentTexts{}
	}
	var kept cursorAgentTexts
	if prompts, _ := cursorAgentLines(head); len(prompts) > 0 {
		kept.first = prompts[0]
	}
	prompts, message := cursorAgentLines(tail)
	if len(prompts) > 0 {
		kept.last = prompts[len(prompts)-1]
	}
	kept.reply, kept.question = agentSaid(message, message, "")
	s.cursorTranscripts[transcript] = cached[cursorAgentTexts]{stamp: stamp, value: kept}
	return kept
}

// cursorAgentLines returns the prompts in data and the agent's latest
// message after the last of them.
func cursorAgentLines(data []byte) (prompts []string, message string) {
	for _, raw := range bytes.Split(data, []byte("\n")) {
		var line cursorAgentLine
		if json.Unmarshal(raw, &line) != nil {
			continue
		}
		var said []string
		for _, part := range line.Message.Content {
			switch {
			case part.Type != "text":
			case line.Role == "user" && strings.Contains(part.Text, "<user_query>"):
				prompts = append(prompts, cursorTyped(part.Text))
				message = ""
			case line.Role == "assistant" && strings.TrimSpace(part.Text) != "":
				said = append(said, part.Text)
			}
		}
		if len(said) > 0 {
			message = strings.Join(said, "\n")
		}
	}
	return prompts, message
}

// cached is a value read from files, kept while their fileStamp holds.
type cached[T any] struct {
	stamp string
	value T
}

// fileStamp tells whether files changed, by their sizes and modification times.
func fileStamp(paths ...string) string {
	var stamp strings.Builder
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil {
			fmt.Fprintf(&stamp, "%d@%d;", info.Size(), info.ModTime().UnixNano())
		} else {
			stamp.WriteString("-;")
		}
	}
	return stamp.String()
}

// forget drops the entries of files a full scan no longer found.
func forget[T any](cache map[string]cached[T], seen map[string]bool) {
	for path := range cache {
		if !seen[path] {
			delete(cache, path)
		}
	}
}

func md5Hex(value string) string {
	sum := md5.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}

func modTime(path string) string {
	newest := time.Time{}
	for _, file := range []string{path, path + "-wal"} {
		if info, err := os.Stat(file); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	if newest.IsZero() {
		return ""
	}
	return newest.UTC().Format(time.RFC3339)
}
