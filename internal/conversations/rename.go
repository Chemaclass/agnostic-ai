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
)

// A name the person chooses is saved in the marks file first (see
// Scanner.Rename). Renamer then writes it into the tool that keeps the
// conversation, so the tool's own list shows it too. Each tool takes it
// its own way: Claude Code from a line in the session's transcript,
// Codex from its app-server, Cursor from its databases, and ChatGPT from
// chatgpt.com.

// RenameReason is why a name could not be written into a tool. The list
// is closed: each reason has one sentence for the log.
type RenameReason string

const (
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

// renameReasonTexts is what the log says for each reason; %s is the tool.
var renameReasonTexts = map[RenameReason]string{
	RenameNoProgram:   "the program that writes names into %s is not installed",
	RenameSignedOut:   "no sign-in that may rename %s chats was found, or it expired",
	RenameInUse:       "%s has the conversation open and would write its old name back",
	RenameRefused:     "%s did not take the name",
	RenameUnreachable: "%s did not answer in time",
	RenameUnexpected:  "%s keeps the conversation in a way this version does not know",
}

// RenameError is a name that could not be written into its tool. Error
// is the sentence for the log; Cause keeps what the tool or the system
// said. Neither ever holds a credential.
type RenameError struct {
	Tool   Tool
	Reason RenameReason
	Cause  error
}

func (e *RenameError) Error() string {
	text, known := renameReasonTexts[e.Reason]
	if !known {
		return fmt.Sprintf("%s: an unknown problem", toolLabels[e.Tool])
	}
	return fmt.Sprintf(text, toolLabels[e.Tool])
}

func (e *RenameError) Unwrap() error { return e.Cause }

// codexThreadNamer sets a Codex thread's name.
type codexThreadNamer interface {
	SetName(ctx context.Context, threadID, name string) error
}

// chatGPTTitler sets a ChatGPT chat's title.
type chatGPTTitler interface {
	SetTitle(ctx context.Context, chatID, title string) error
}

// Renamer writes a name the person chose into the tool that keeps the
// conversation.
type Renamer struct {
	// CursorDB is Cursor's global state database.
	CursorDB string
	sqlite   SQLiteWrite
	codex    codexThreadNamer
	chatGPT  chatGPTTitler
}

// NewRenamer writes names with the programs and sign-ins on this machine.
func NewRenamer(stores Stores) *Renamer {
	return &Renamer{
		CursorDB: stores.CursorDB,
		sqlite:   RunSQLiteWrite,
		codex:    newCodexAppServer(),
		chatGPT:  newChatGPTTitles(),
	}
}

// Rename writes title into the tool that keeps c. It returns nil once
// the tool has it, and also for a conversation whose name stays in the
// marks file only, which each case below names.
func (r *Renamer) Rename(ctx context.Context, c Conversation, title string) error {
	switch c.Tool {
	case ToolClaude:
		switch c.App {
		case AppCLI, AppIDE, AppDesktop, AppBackground:
			return appendClaudeTitle(c.file, c.ID, title)
		case AppChat, AppCowork, AppCloud:
			// Kept in the marks file only: claude.ai refuses the sign-in
			// this server can read.
			return nil
		}
	case ToolCodex:
		switch c.App {
		case AppCLI, AppIDE, AppDesktop:
			return r.codex.SetName(ctx, c.ID, title)
		case AppCloud:
			// Kept in the marks file only: the app-server names the
			// threads on this machine.
			return nil
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
	return fmt.Errorf("no way to rename a %s %s conversation", c.Tool, c.App)
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
// only the name field of both, and only when both are there. A chat
// Cursor has loaded may get its old name back when Cursor next saves it.
func (r *Renamer) renameCursorEditorChat(ctx context.Context, id, title string) error {
	if !cursorIDPattern.MatchString(id) {
		return &RenameError{Tool: ToolCursor, Reason: RenameUnexpected, Cause: fmt.Errorf("%q is not a Cursor chat id", id)}
	}
	name := sqlText(title)
	script := fmt.Sprintf(`BEGIN IMMEDIATE;
UPDATE composerHeaders SET value = json_set(value, '$.name', %[1]s)
  WHERE composerId = '%[2]s' AND EXISTS (SELECT 1 FROM cursorDiskKV WHERE key = 'composerData:%[2]s')
  RETURNING 'composerHeaders' AS store;
UPDATE cursorDiskKV SET value = json_set(value, '$.name', %[1]s)
  WHERE key = 'composerData:%[2]s' AND EXISTS (SELECT 1 FROM composerHeaders WHERE composerId = '%[2]s')
  RETURNING 'composerData' AS store;
COMMIT;`, name, id)
	return r.writeCursor(ctx, r.CursorDB, script, 2)
}

// renameCursorAgentChat sets the name in a cursor-agent chat's store.db,
// whose meta row holds the chat's metadata as hex-encoded JSON. A
// running chat writes its metadata back from memory, so it is left alone.
func (r *Renamer) renameCursorAgentChat(ctx context.Context, c Conversation, title string) error {
	if c.Live {
		return &RenameError{Tool: ToolCursor, Reason: RenameInUse, Cause: fmt.Errorf("cursor-agent runs the chat %s", c.ID)}
	}
	if c.store == "" || filepath.Base(c.store) != "store.db" {
		return &RenameError{Tool: ToolCursor, Reason: RenameUnexpected, Cause: fmt.Errorf("the chat %s has no store.db", c.ID)}
	}
	script := fmt.Sprintf(`BEGIN IMMEDIATE;
UPDATE meta SET value = lower(hex(json_set(CAST(unhex(value) AS TEXT), '$.name', %s)))
  WHERE key = '0' AND json_valid(CAST(unhex(value) AS TEXT))
  RETURNING 'meta' AS store;
COMMIT;`, sqlText(title))
	return r.writeCursor(ctx, c.store, script, 1)
}

// writeCursor runs a Cursor write script and checks that it changed the
// rows it should, each reported by a RETURNING clause.
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
	return nil
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
