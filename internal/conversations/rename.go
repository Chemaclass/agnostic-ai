package conversations

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/preview"
)

// A name the person chooses is written into the tool that keeps the
// conversation first, so the tool's own list shows it too; the marks
// file saves it only once the tool has it (see Scanner.Rename). Each
// tool takes it its own way: Claude Code from a line in the session's
// transcript, Codex from its app-server, Cursor from its databases,
// ChatGPT from chatgpt.com and Claude Code's cloud from its session API.

// RenameReason is why a name could not be written into a tool. The list
// is closed: each reason has one sentence for the log and one for the
// person.
type RenameReason string

const (
	// RenameNotSupported: this version has no way to rename this kind of
	// conversation in its tool.
	RenameNotSupported RenameReason = "not-supported"
	// RenameNoProgram: the program the write needs is not installed.
	RenameNoProgram RenameReason = "no-program"
	// RenameSignedOut: no sign-in that may rename the chat was found, or
	// the tool no longer accepts it.
	RenameSignedOut RenameReason = "signed-out"
	// RenameInUse: a running program holds the conversation and would
	// write its old name back.
	RenameInUse RenameReason = "in-use"
	// RenameRefused: the tool answered, but did not take the name.
	RenameRefused RenameReason = "refused"
	// RenameUnreachable: no answer in time.
	RenameUnreachable RenameReason = "unreachable"
	// RenameUnexpected: the store or the answer is not the one this
	// version knows.
	RenameUnexpected RenameReason = "unexpected"
)

// renameReasonTexts is what the log says for each reason; %s is the
// tool, or for RenameNotSupported the kind of conversation.
var renameReasonTexts = map[RenameReason]string{
	RenameNotSupported: "renaming %s is not supported",
	RenameNoProgram:    "the program that writes names into %s is not installed",
	RenameSignedOut:    "no sign-in that may rename %s chats was found, or it expired",
	RenameInUse:        "%s has the conversation open and would write its old name back",
	RenameRefused:      "%s did not take the name",
	RenameUnreachable:  "%s did not answer in time",
	RenameUnexpected:   "%s keeps the conversation in a way this version does not know",
}

// renameMissingProgram is what the person reads when a tool's program
// is missing.
var renameMissingProgram = map[Tool]string{
	ToolClaude:  "Claude Code is not installed. Install it and try again.",
	ToolCodex:   "Codex is not installed, and renaming a Codex chat needs its codex command. Install Codex and try again.",
	ToolCursor:  "Renaming a Cursor chat needs the sqlite3 command, which is not installed. Install it and try again.",
	ToolChatGPT: "ChatGPT chats are renamed with the Codex login, and Codex is not installed. Install Codex and try again.",
}

// renameSignIn is what the person reads when a tool's sign-in is
// missing or expired.
var renameSignIn = map[Tool]string{
	ToolClaude:  "Claude Code is signed out. Run claude, sign in with /login, and try again.",
	ToolCodex:   "Codex is signed out. Sign in to Codex and try again.",
	ToolCursor:  "Cursor is signed out. Sign in to Cursor and try again.",
	ToolChatGPT: "Sign in to ChatGPT in the Codex app, or run codex login, and try again.",
}

// RenameError is a name that could not be written into its tool. Error
// is the sentence for the log and Message the one for the person; Cause
// keeps what the tool or the system said. None of them ever holds a
// credential.
type RenameError struct {
	Tool   Tool
	Reason RenameReason
	// Kind names the kind of conversation, such as "Claude app chats";
	// RenameNotSupported reads it.
	Kind  string
	Cause error
}

func (e *RenameError) Error() string {
	text, known := renameReasonTexts[e.Reason]
	if !known {
		return fmt.Sprintf("%s: an unknown problem", toolLabels[e.Tool])
	}
	if e.Reason == RenameNotSupported {
		return fmt.Sprintf(text, e.Kind)
	}
	return fmt.Sprintf(text, toolLabels[e.Tool])
}

func (e *RenameError) Unwrap() error { return e.Cause }

// Message says, for the person, why the name was not saved and what to
// do. It never holds what the tool said.
func (e *RenameError) Message() string {
	tool := toolLabels[e.Tool]
	switch e.Reason {
	case RenameNotSupported:
		return fmt.Sprintf("Renaming %s is not supported yet. The name was not changed.", e.Kind)
	case RenameNoProgram:
		return renameMissingProgram[e.Tool]
	case RenameSignedOut:
		return renameSignIn[e.Tool]
	case RenameInUse:
		return fmt.Sprintf("%s has this conversation open and would put the old name back. Stop or close it there, then try again.", tool)
	case RenameRefused:
		return fmt.Sprintf("%s did not take the new name. Try again, or rename it in %s.", tool, tool)
	case RenameUnreachable:
		return fmt.Sprintf("%s did not answer in time. Try again.", tool)
	case RenameUnexpected:
		return fmt.Sprintf("This version cannot rename this %s conversation. Rename it in %s.", tool, tool)
	}
	return fmt.Sprintf("%s did not take the new name.", tool)
}

// maxRenameAnswerBytes bounds what is read of a provider's answer to a
// rename: its acknowledgement, or a refusal whose body is redacted
// whole before it is cut for the log.
const maxRenameAnswerBytes = 64 << 10

// refusalCause is what the log keeps of a provider's refusal: its
// status and the start of its body. The credentials sent with the
// request are redacted before the body is cut, so an echoed one is
// never kept, not even in part.
func refusalCause(status int, body io.Reader, credentials ...string) error {
	said, err := io.ReadAll(io.LimitReader(body, maxRenameAnswerBytes))
	if err != nil {
		return fmt.Errorf("HTTP %d, and its body could not be read: %w", status, err)
	}
	shown := strings.TrimSpace(preview.RedactText(string(said), credentials...))
	if len(shown) > maxCloudCauseBytes {
		shown = strings.ToValidUTF8(shown[:maxCloudCauseBytes], "")
	}
	return fmt.Errorf("HTTP %d: %s", status, shown)
}

// codexThreadNamer sets a Codex thread's name.
type codexThreadNamer interface {
	SetName(ctx context.Context, threadID, name string) error
}

// chatGPTTitler sets a ChatGPT chat's title.
type chatGPTTitler interface {
	SetTitle(ctx context.Context, chatID, title string) error
}

// claudeSessionTitler sets a Claude Code cloud session's title.
type claudeSessionTitler interface {
	SetTitle(ctx context.Context, sessionID, title string) error
}

// storeHolders finds the running processes that hold files open.
type storeHolders interface {
	Holders(paths []string) (map[string]int, error)
}

// Renamer writes a name the person chose into the tool that keeps the
// conversation.
type Renamer struct {
	// CursorDB is Cursor's global state database.
	CursorDB    string
	sqlite      SQLiteWrite
	holders     storeHolders
	codex       codexThreadNamer
	chatGPT     chatGPTTitler
	claudeCloud claudeSessionTitler
}

// NewRenamer writes names with the programs and sign-ins on this
// machine; processes says which running program holds a store.
func NewRenamer(stores Stores, processes Processes) *Renamer {
	return &Renamer{
		CursorDB:    stores.CursorDB,
		sqlite:      RunSQLiteWrite,
		holders:     processes,
		codex:       newCodexAppServer(),
		chatGPT:     newChatGPTTitles(),
		claudeCloud: NewClaudeCloud(),
	}
}

// Rename writes title into the tool that keeps c. It returns nil once
// the tool has it, and a *RenameError otherwise, also for a kind of
// conversation no tool can rename from here, which each case names.
func (r *Renamer) Rename(ctx context.Context, c Conversation, title string) error {
	switch c.Tool {
	case ToolClaude:
		switch c.App {
		case AppCLI, AppIDE, AppDesktop, AppBackground:
			return appendClaudeTitle(c.file, c.ID, title)
		case AppCloud:
			return r.claudeCloud.SetTitle(ctx, c.ID, title)
		case AppChat:
			// claude.ai refuses the sign-in this server can read.
			return &RenameError{Tool: ToolClaude, Reason: RenameNotSupported, Kind: "Claude app chats"}
		case AppCowork:
			// Kept by the Claude app, which takes no rename from outside.
			return &RenameError{Tool: ToolClaude, Reason: RenameNotSupported, Kind: "Cowork tasks"}
		}
	case ToolCodex:
		switch c.App {
		case AppCLI, AppIDE, AppDesktop:
			return r.codex.SetName(ctx, c.ID, title)
		case AppCloud:
			// The local app-server finds no rollout for a cloud thread, and
			// the Codex app renames one over the cloud's own app-server
			// connection, which this version does not speak.
			return &RenameError{Tool: ToolCodex, Reason: RenameNotSupported, Kind: "Codex cloud threads"}
		}
	case ToolCursor:
		switch c.App {
		case AppIDE:
			return r.renameCursorEditorChat(ctx, c.ID, title)
		case AppCLI:
			return r.renameCursorAgentChat(ctx, c, title)
		}
	case ToolChatGPT:
		switch c.App {
		case AppChat:
			return r.chatGPT.SetTitle(ctx, c.ID, title)
		}
	}
	return &RenameError{Tool: c.Tool, Reason: RenameNotSupported, Kind: fmt.Sprintf("%s %s conversations", toolLabels[c.Tool], c.App)}
}

// claudeTitleLine is the line Claude Code's own /rename appends to a
// session's transcript; its session list shows the last one.
type claudeTitleLine struct {
	Type        string `json:"type"`
	CustomTitle string `json:"customTitle"`
	SessionID   string `json:"sessionId"`
}

// appendClaudeTitle appends a custom-title line to the session's
// transcript in one write. It never creates the file, and refuses one
// whose last line is unfinished, since the new line would join it.
func appendClaudeTitle(file, id, title string) error {
	if !sessionIDPattern.MatchString(id) || filepath.Base(file) != id+".jsonl" {
		return &RenameError{Tool: ToolClaude, Reason: RenameUnexpected, Cause: fmt.Errorf("the transcript %q is not named after the session %q", file, id)}
	}
	ended, err := endsWithNewline(file)
	if err != nil {
		return &RenameError{Tool: ToolClaude, Reason: RenameUnexpected, Cause: err}
	}
	if !ended {
		return &RenameError{Tool: ToolClaude, Reason: RenameInUse, Cause: fmt.Errorf("the last line of %s is unfinished", file)}
	}
	// Claude Code writes <, > and & as they are; the encoder ends the line.
	var line bytes.Buffer
	encoder := json.NewEncoder(&line)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(claudeTitleLine{Type: "custom-title", CustomTitle: title, SessionID: id}); err != nil {
		return &RenameError{Tool: ToolClaude, Reason: RenameUnexpected, Cause: err}
	}
	transcript, err := os.OpenFile(file, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return &RenameError{Tool: ToolClaude, Reason: RenameUnexpected, Cause: err}
	}
	_, writeErr := transcript.Write(line.Bytes())
	closeErr := transcript.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return &RenameError{Tool: ToolClaude, Reason: RenameUnexpected, Cause: err}
	}
	return nil
}

// endsWithNewline reports whether a regular file is empty or ends with
// a newline.
func endsWithNewline(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("%s is not a regular file", path)
	}
	if info.Size() == 0 {
		return true, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer func() { _ = file.Close() }()
	last := make([]byte, 1)
	if _, err := file.ReadAt(last, info.Size()-1); err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	return last[0] == '\n', nil
}

// sqlText is title as a SQL text value written in hex, so no character
// of it is read as SQL.
func sqlText(text string) string {
	return "CAST(X'" + hex.EncodeToString([]byte(text)) + "' AS TEXT)"
}

// renameCursorEditorChat sets the name of a Cursor editor chat. Cursor
// keeps it twice: in the chat's composerHeaders row, which its chat
// list reads, and in its composerData entry. One transaction changes
// only the name field of both, and only when both hold a JSON object:
// each update also checks the other row, so either both change or
// neither does. A chat Cursor has loaded may get its old name back when
// Cursor next saves it.
func (r *Renamer) renameCursorEditorChat(ctx context.Context, id, title string) error {
	if !cursorIDPattern.MatchString(id) {
		return &RenameError{Tool: ToolCursor, Reason: RenameUnexpected, Cause: fmt.Errorf("%q is not a Cursor chat id", id)}
	}
	name := sqlText(title)
	script := fmt.Sprintf(`BEGIN IMMEDIATE;
UPDATE composerHeaders SET value = json_set(value, '$.name', %[1]s)
  WHERE composerId = '%[2]s' AND %[3]s
    AND EXISTS (SELECT 1 FROM cursorDiskKV WHERE key = 'composerData:%[2]s' AND %[3]s)
  RETURNING json_extract(value, '$.name') = %[1]s AS named;
UPDATE cursorDiskKV SET value = json_set(value, '$.name', %[1]s)
  WHERE key = 'composerData:%[2]s' AND %[3]s
    AND EXISTS (SELECT 1 FROM composerHeaders WHERE composerId = '%[2]s' AND %[3]s)
  RETURNING json_extract(value, '$.name') = %[1]s AS named;
COMMIT;`, name, id, sqlJSONObject("value"))
	return r.writeCursor(ctx, r.CursorDB, script, 2)
}

// sqlJSONObject is a SQL condition that holds when column is a JSON
// object, and is false, never an error, for NULL, other JSON and text
// that is not JSON.
func sqlJSONObject(column string) string {
	return fmt.Sprintf("(CASE WHEN json_valid(%[1]s) THEN json_type(%[1]s) = 'object' ELSE 0 END)", column)
}

// renameCursorAgentChat sets the name in a cursor-agent chat's store.db,
// whose meta row holds the chat's metadata as hex-encoded JSON. A
// running chat writes its metadata back from memory, so the write
// happens only when a look at the open files, made just before it,
// finds no process holding the store.
//
// The list dates a cursor-agent chat by its store's modification time,
// so the write would read as new activity and take back the chat's
// completed and seen marks. The store and its WAL get their times from
// before the write back: a name is not a message, and nothing else
// writes to a store no process holds.
func (r *Renamer) renameCursorAgentChat(ctx context.Context, c Conversation, title string) error {
	if c.store == "" || filepath.Base(c.store) != "store.db" {
		return &RenameError{Tool: ToolCursor, Reason: RenameUnexpected, Cause: fmt.Errorf("the chat %s has no store.db", c.ID)}
	}
	held, err := r.holders.Holders([]string{c.store})
	if err != nil {
		return &RenameError{Tool: ToolCursor, Reason: RenameRefused, Cause: fmt.Errorf("could not check whether cursor-agent runs the chat %s: %w", c.ID, err)}
	}
	if pid, running := held[c.store]; running {
		return &RenameError{Tool: ToolCursor, Reason: RenameInUse, Cause: fmt.Errorf("cursor-agent (pid %d) runs the chat %s", pid, c.ID)}
	}
	written, err := lastModified(c.store, c.store+"-wal")
	if err != nil {
		return &RenameError{Tool: ToolCursor, Reason: RenameUnexpected, Cause: err}
	}
	meta := "CAST(unhex(value) AS TEXT)"
	script := fmt.Sprintf(`BEGIN IMMEDIATE;
UPDATE meta SET value = lower(hex(json_set(%[1]s, '$.name', %[2]s)))
  WHERE key = '0' AND %[3]s
  RETURNING json_extract(CAST(unhex(value) AS TEXT), '$.name') = %[2]s AS named;
COMMIT;`, meta, sqlText(title), sqlJSONObject(meta))
	if err := r.writeCursor(ctx, c.store, script, 1); err != nil {
		return err
	}
	if err := keepModified(written, c.store, c.store+"-wal"); err != nil {
		return &RenameError{Tool: ToolCursor, Reason: RenameUnexpected, Cause: fmt.Errorf("the name was written, but the store's time could not be kept: %w", err)}
	}
	return nil
}

// lastModified is the latest modification time of the files that exist
// among paths; the first must exist.
func lastModified(paths ...string) (time.Time, error) {
	var latest time.Time
	for i, path := range paths {
		info, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) && i > 0 {
			continue
		}
		if err != nil {
			return time.Time{}, err
		}
		if info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	return latest, nil
}

// keepModified sets the modification time of the files that exist
// among paths to at, and leaves their access times as they are.
func keepModified(at time.Time, paths ...string) error {
	for _, path := range paths {
		err := os.Chtimes(path, time.Time{}, at)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// writeCursor runs a Cursor write script and checks that it changed the
// rows it should, each reported by a RETURNING clause that says whether
// the row now holds the new name.
func (r *Renamer) writeCursor(ctx context.Context, path, script string, rows int) error {
	changed, err := r.sqlite(ctx, path, script)
	switch {
	case errors.Is(err, errNoSQLite):
		return &RenameError{Tool: ToolCursor, Reason: RenameNoProgram, Cause: err}
	case errors.Is(err, errSQLiteBusy):
		return &RenameError{Tool: ToolCursor, Reason: RenameInUse, Cause: err}
	case err != nil:
		return &RenameError{Tool: ToolCursor, Reason: RenameRefused, Cause: err}
	case len(changed) != rows:
		return &RenameError{Tool: ToolCursor, Reason: RenameUnexpected, Cause: fmt.Errorf("%s: changed %d rows, want %d", path, len(changed), rows)}
	}
	for _, row := range changed {
		var named cursorNamedRow
		if err := json.Unmarshal(row, &named); err != nil || named.Named != 1 {
			return &RenameError{Tool: ToolCursor, Reason: RenameUnexpected, Cause: fmt.Errorf("%s: a changed row does not hold the new name: %s", path, row)}
		}
	}
	return nil
}

// cursorNamedRow is a row a Cursor write script returns: named is 1
// when the row holds the new name.
type cursorNamedRow struct {
	Named int `json:"named"`
}

// SQLiteWrite runs a script that writes to an existing database and
// returns the rows its statements report.
type SQLiteWrite func(ctx context.Context, path, script string) ([]json.RawMessage, error)

// decodeRowSets reads what sqlite3 -json prints for a script: one JSON
// array per statement that returned rows.
func decodeRowSets(out []byte) ([]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(out))
	var rows []json.RawMessage
	for {
		var set []json.RawMessage
		err := dec.Decode(&set)
		if errors.Is(err, io.EOF) {
			return rows, nil
		}
		if err != nil {
			return nil, err
		}
		rows = append(rows, set...)
	}
}
