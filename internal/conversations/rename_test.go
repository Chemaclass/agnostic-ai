package conversations

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// renameError says what a rename error is, for comparing with a want.
func renameError(err error) string {
	if err == nil {
		return "<nil>"
	}
	var renameErr *RenameError
	if !errors.As(err, &renameErr) {
		return "not a RenameError: " + err.Error()
	}
	if renameErr.Cause == nil {
		return fmt.Sprintf("%s (%s)", renameErr.Error(), renameErr.Reason)
	}
	return fmt.Sprintf("%s (%s): %v", renameErr.Error(), renameErr.Reason, renameErr.Cause)
}

func TestRename_ClaudeCodeTakesTheNameFromItsTranscript(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	writeClaudeSessions(t, stores)
	c, ok := scanner.Find(context.Background(), ToolClaude, idCache, ScanOptions{})
	if !ok {
		t.Fatal("the session is not listed")
	}
	before, err := os.ReadFile(c.file)
	if err != nil {
		t.Fatal(err)
	}

	if err := (&Renamer{}).Rename(context.Background(), c, "Cache <notes> & more"); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(c.file)
	if err != nil {
		t.Fatal(err)
	}
	want := string(before) + `{"type":"custom-title","customTitle":"Cache <notes> & more","sessionId":"` + idCache + `"}` + "\n"
	if string(after) != want {
		t.Errorf("transcript ends with %q", strings.TrimPrefix(string(after), string(before)))
	}
	if renamed, _ := scanner.Find(context.Background(), ToolClaude, idCache, ScanOptions{}); renamed.Title != "Cache <notes> & more" {
		t.Errorf("the list reads the title %q", renamed.Title)
	}
}

func TestRename_ClaudeCodeLeavesATranscriptItCannotSafelyAppendTo(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, idSafari+".jsonl")
	_, missingErr := os.Lstat(missing)
	unfinished := filepath.Join(dir, idCache+".jsonl")
	write(t, unfinished, `{"type":"user"}`+"\n"+`{"type":"assi`)
	for name, tc := range map[string]struct {
		c    Conversation
		want string
	}{
		"an unfinished last line": {
			Conversation{Tool: ToolClaude, App: AppCLI, ID: idCache, file: unfinished},
			"Claude Code has the conversation open and would write its old name back (in-use): the last line of " + unfinished + " is unfinished",
		},
		"a missing transcript": {
			Conversation{Tool: ToolClaude, App: AppCLI, ID: idSafari, file: filepath.Join(dir, idSafari+".jsonl")},
			"Claude Code keeps the conversation in a way this version does not know (unexpected): " + missingErr.Error(),
		},
		"a file named after another session": {
			Conversation{Tool: ToolClaude, App: AppIDE, ID: idSafari, file: unfinished},
			fmt.Sprintf("Claude Code keeps the conversation in a way this version does not know (unexpected): the transcript %q is not named after the session %q", unfinished, idSafari),
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := renameError((&Renamer{}).Rename(context.Background(), tc.c, "New name")); got != tc.want {
				t.Errorf("got  %s\nwant %s", got, tc.want)
			}
		})
	}
	if data, _ := os.ReadFile(unfinished); string(data) != `{"type":"user"}`+"\n"+`{"type":"assi` {
		t.Errorf("the transcript changed: %q", data)
	}
	if _, err := os.Stat(filepath.Join(dir, idSafari+".jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a missing transcript was created: %v", err)
	}
}

func TestRename_SaysWhichConversationsCannotBeRenamedInTheirTool(t *testing.T) {
	r := &Renamer{}
	for _, tc := range []struct {
		c    Conversation
		want string
	}{
		{Conversation{Tool: ToolClaude, App: AppChat, ID: "chat"}, "Renaming Claude app chats is not supported yet. The name was not changed."},
		{Conversation{Tool: ToolClaude, App: AppCowork, ID: "task"}, "Renaming Cowork tasks is not supported yet. The name was not changed."},
		{Conversation{Tool: ToolCodex, App: AppCloud, ID: "thread"}, "Renaming Codex cloud threads is not supported yet. The name was not changed."},
		{Conversation{Tool: ToolChatGPT, App: AppCLI, ID: "x"}, "Renaming ChatGPT cli conversations is not supported yet. The name was not changed."},
	} {
		err := r.Rename(context.Background(), tc.c, "New name")
		var renameErr *RenameError
		if !errors.As(err, &renameErr) || renameErr.Reason != RenameNotSupported || renameErr.Message() != tc.want {
			t.Errorf("%s %s = %s\nwant %s", tc.c.Tool, tc.c.App, renameError(err), tc.want)
		}
	}
}

func TestRenameError_TellsThePersonWhatToDoForEveryReasonAndTool(t *testing.T) {
	cause := errors.New("what the tool said")
	for _, reason := range []RenameReason{RenameNotSupported, RenameNoProgram, RenameSignedOut, RenameInUse, RenameRefused, RenameUnreachable, RenameUnexpected} {
		for _, tool := range []Tool{ToolClaude, ToolCodex, ToolCursor, ToolChatGPT} {
			err := &RenameError{Tool: tool, Reason: reason, Kind: "these chats", Cause: cause}
			message := err.Message()
			if message == "" || strings.Contains(message, cause.Error()) || strings.Contains(message, "%!") {
				t.Errorf("%s %s: message %q", tool, reason, message)
			}
			if strings.Contains(err.Error(), "unknown problem") {
				t.Errorf("%s %s: no log sentence", tool, reason)
			}
		}
	}
}

// claudeCloudRenameAPI serves answer for a session rename from a fake
// API and refuses a request that lacks the headers the real one needs.
func claudeCloudRenameAPI(t *testing.T, answer func(w http.ResponseWriter, r *http.Request)) *ClaudeCloud {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/v1/code/sessions/cse_01Rename" {
			http.Error(w, "unexpected request "+r.Method+" "+r.URL.String(), http.StatusTeapot)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+fakeToken || r.Header.Get("anthropic-version") != "2023-06-01" || r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "missing the sign-in headers", http.StatusTeapot)
			return
		}
		answer(w, r)
	}))
	t.Cleanup(server.Close)
	return &ClaudeCloud{BaseURL: server.URL, Client: newCloudClient(), Credentials: fakeCredentials{token: fakeToken}}
}

func TestRename_ClaudeCloudSessionChangesItsTitleOnTheSessionAPI(t *testing.T) {
	var asked string
	cloud := claudeCloudRenameAPI(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		asked = string(body)
		_, _ = io.WriteString(w, `{"session":{"id":"cse_01Rename","title":"Trip plans, day 2","status_bucket":"completed"}}`)
	})

	err := (&Renamer{claudeCloud: cloud}).Rename(context.Background(), Conversation{Tool: ToolClaude, App: AppCloud, ID: "cse_01Rename"}, "Trip plans, day 2")

	if err != nil {
		t.Fatal(renameError(err))
	}
	if asked != `{"title":"Trip plans, day 2"}` {
		t.Errorf("sent %s", asked)
	}
}

func TestRename_ClaudeCloudSessionSaysWhyTheTitleDidNotChange(t *testing.T) {
	answer := func(status int, body string) func(http.ResponseWriter, *http.Request) {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, body)
		}
	}
	for name, tc := range map[string]struct {
		cloud *ClaudeCloud
		id    string
		want  string
	}{
		"an expired sign-in": {
			claudeCloudRenameAPI(t, answer(http.StatusUnauthorized, `{"error":{"type":"authentication_error"}}`)),
			"cse_01Rename",
			`no sign-in that may rename Claude Code chats was found, or it expired (signed-out): HTTP 401: {"error":{"type":"authentication_error"}}`,
		},
		"a refusal": {
			claudeCloudRenameAPI(t, answer(http.StatusNotFound, `{"error":{"type":"not_found_error"}}`)),
			"cse_01Rename",
			`Claude Code did not take the name (refused): HTTP 404: {"error":{"type":"not_found_error"}}`,
		},
		"a refusal that echoes the token": {
			claudeCloudRenameAPI(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, `{"error":{"message":"bad token `+strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")+`"}}`)
			}),
			"cse_01Rename",
			`no sign-in that may rename Claude Code chats was found, or it expired (signed-out): HTTP 401: {"error":{"message":"bad token <redacted>"}}`,
		},
		"an answer without the new title": {
			claudeCloudRenameAPI(t, answer(http.StatusOK, `{"session":{"title":"Old name"}}`)),
			"cse_01Rename",
			"Claude Code keeps the conversation in a way this version does not know (unexpected): the rename answer does not hold the new title",
		},
		"an answer that is not JSON": {
			claudeCloudRenameAPI(t, answer(http.StatusOK, `<html>`)),
			"cse_01Rename",
			"Claude Code keeps the conversation in a way this version does not know (unexpected): rename answer: invalid character '<' looking for beginning of value",
		},
		"no sign-in": {
			func() *ClaudeCloud {
				cloud := claudeCloudRenameAPI(t, answer(http.StatusOK, `{}`))
				cloud.Credentials = fakeCredentials{err: &CloudError{Tool: ToolClaude, Reason: CloudNoCredential, Cause: errors.New("no keychain entry")}}
				return cloud
			}(),
			"cse_01Rename",
			"no sign-in that may rename Claude Code chats was found, or it expired (signed-out): " + (&CloudError{Tool: ToolClaude, Reason: CloudNoCredential, Cause: errors.New("no keychain entry")}).Error(),
		},
		"an id that is not a session id": {
			claudeCloudRenameAPI(t, answer(http.StatusOK, `{}`)),
			"../admin",
			`Claude Code keeps the conversation in a way this version does not know (unexpected): "../admin" is not a cloud session id`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := (&Renamer{claudeCloud: tc.cloud}).Rename(context.Background(), Conversation{Tool: ToolClaude, App: AppCloud, ID: tc.id}, "New name")
			if got := renameError(err); got != tc.want {
				t.Errorf("got  %s\nwant %s", got, tc.want)
			}
			if strings.Contains(renameError(err), fakeToken) {
				t.Error("the error holds the token")
			}
		})
	}
}

// The fake app-server: a test run of this binary that answers as
// `codex app-server` does. FAKE_CODEX_APP_SERVER picks its answer and
// FAKE_CODEX_LOG names the file it copies its input to.
func TestFakeCodexAppServer(t *testing.T) {
	mode := os.Getenv("FAKE_CODEX_APP_SERVER")
	if mode == "" {
		t.Skip("run as a fake app-server only")
	}
	log, err := os.Create(os.Getenv("FAKE_CODEX_LOG"))
	if err != nil {
		os.Exit(3)
	}
	in := bufio.NewScanner(os.Stdin)
	for in.Scan() {
		_, _ = fmt.Fprintln(log, in.Text())
		var req struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		_ = json.Unmarshal(in.Bytes(), &req)
		switch {
		case req.Method == "initialize" && mode == "exit":
			os.Exit(1)
		case req.Method == "initialize" && mode == "hang":
			// A program it started keeps the output open, and no answer comes.
			child := exec.Command("sleep", "30")
			child.Stdout = os.Stdout
			if err := child.Start(); err != nil {
				os.Exit(4)
			}
			_ = os.WriteFile(os.Getenv("FAKE_CODEX_LOG")+".child", []byte(strconv.Itoa(child.Process.Pid)), 0o600)
			select {}
		case req.Method == "initialize" && mode == "init-without-result":
			fmt.Printf(`{"id":%d}`+"\n", req.ID)
		case req.Method == "initialize":
			fmt.Println(`{"method":"remoteControl/status/changed","params":{"status":"disabled"}}`)
			fmt.Printf(`{"id":%d,"result":{"userAgent":"fake"}}`+"\n", req.ID)
		case req.Method == "thread/name/set" && mode == "rename-without-result":
			fmt.Printf(`{"id":%d}`+"\n", req.ID)
		case req.Method == "thread/name/set" && mode == "rename-with-null-result":
			fmt.Printf(`{"id":%d,"result":null}`+"\n", req.ID)
		case req.Method == "thread/name/set" && mode == "refuse":
			fmt.Printf(`{"error":{"code":-32600,"message":"no rollout found for thread id x"},"id":%d}`+"\n", req.ID)
		case req.Method == "thread/name/set":
			fmt.Println(`{"method":"thread/name/updated","params":{"threadId":"x","threadName":"y"}}`)
			fmt.Printf(`{"id":%d,"result":{}}`+"\n", req.ID)
		}
	}
	os.Exit(0)
}

func fakeCodexAppServer(t *testing.T, mode string) (*codexAppServer, string) {
	t.Helper()
	log := filepath.Join(t.TempDir(), "input.jsonl")
	return &codexAppServer{command: func(ctx context.Context) (*exec.Cmd, error) {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestFakeCodexAppServer$")
		cmd.Env = append(os.Environ(), "FAKE_CODEX_APP_SERVER="+mode, "FAKE_CODEX_LOG="+log)
		return cmd, nil
	}}, log
}

const renamedCodexThread = "01a1d81d-6ea2-7f50-a7b0-8324b465ddfb"

func TestRename_CodexNamesTheThreadThroughItsAppServer(t *testing.T) {
	server, log := fakeCodexAppServer(t, "accept")
	r := &Renamer{codex: server}

	if err := r.Rename(context.Background(), Conversation{Tool: ToolCodex, App: AppDesktop, ID: renamedCodexThread}, "Release notes"); err != nil {
		t.Fatal(renameError(err))
	}

	sent, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":1,"method":"initialize","params":{"clientInfo":{"name":"agnostic-ai","version":"rename"}}}
{"method":"initialized"}
{"id":2,"method":"thread/name/set","params":{"threadId":"` + renamedCodexThread + `","name":"Release notes"}}
`
	if string(sent) != want {
		t.Errorf("the app-server was sent\n%s", sent)
	}
}

func TestRename_CodexSaysWhyTheAppServerDidNotTakeTheName(t *testing.T) {
	for name, tc := range map[string]struct {
		server codexThreadNamer
		want   string
	}{
		"a refusal": {
			func() codexThreadNamer { s, _ := fakeCodexAppServer(t, "refuse"); return s }(),
			"Codex did not take the name (refused): app-server error -32600: no rollout found for thread id x",
		},
		"an app-server that ends": {
			func() codexThreadNamer { s, _ := fakeCodexAppServer(t, "exit"); return s }(),
			"Codex did not answer in time (unreachable): the app-server ended before it answered",
		},
		"an initialize reply without a result": {
			func() codexThreadNamer { s, _ := fakeCodexAppServer(t, "init-without-result"); return s }(),
			`Codex keeps the conversation in a way this version does not know (unexpected): the app-server's reply to initialize holds no result: {"id":1}`,
		},
		"a rename reply without a result": {
			func() codexThreadNamer { s, _ := fakeCodexAppServer(t, "rename-without-result"); return s }(),
			`Codex keeps the conversation in a way this version does not know (unexpected): the app-server's reply to thread/name/set holds no result: {"id":2}`,
		},
		"a rename reply with a null result": {
			func() codexThreadNamer { s, _ := fakeCodexAppServer(t, "rename-with-null-result"); return s }(),
			`Codex keeps the conversation in a way this version does not know (unexpected): the app-server's reply to thread/name/set holds no result: {"id":2,"result":null}`,
		},
		"no codex command": {
			&codexAppServer{command: func(context.Context) (*exec.Cmd, error) { return nil, exec.ErrNotFound }},
			"the program that writes names into Codex is not installed (no-program): " + exec.ErrNotFound.Error(),
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := (&Renamer{codex: tc.server}).Rename(context.Background(), Conversation{Tool: ToolCodex, App: AppCLI, ID: renamedCodexThread}, "Release notes")
			if got := renameError(err); got != tc.want {
				t.Errorf("got  %s\nwant %s", got, tc.want)
			}
		})
	}
}

func TestRename_CodexStopsAtTheDeadlineWhenAProgramItStartedKeepsTheOutputOpen(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake starts sleep, and Windows has no process group to kill")
	}
	server, log := fakeCodexAppServer(t, "hang")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	started := time.Now()

	err := (&Renamer{codex: server}).Rename(ctx, Conversation{Tool: ToolCodex, App: AppCLI, ID: renamedCodexThread}, "Release notes")

	if took := time.Since(started); took > 3*time.Second {
		t.Errorf("the rename took %s past a 200ms deadline", took)
	}
	if got, want := renameError(err), "Codex did not answer in time (unreachable): context deadline exceeded"; got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	pid, readErr := os.ReadFile(log + ".child")
	if readErr != nil {
		t.Fatal(readErr)
	}
	child, _ := strconv.Atoi(string(pid))
	gone := false
	for wait := time.Now().Add(3 * time.Second); time.Now().Before(wait); time.Sleep(20 * time.Millisecond) {
		if exec.Command("kill", "-0", string(pid)).Run() != nil {
			gone = true
			break
		}
	}
	if !gone {
		_ = exec.Command("kill", "-9", string(pid)).Run()
		t.Errorf("the program the app-server started (pid %d) still runs", child)
	}
}

func needSQLite(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 is not installed")
	}
}

func sqliteExec(t *testing.T, path, script string) string {
	t.Helper()
	out, err := exec.Command("sqlite3", path, script).CombinedOutput()
	if err != nil {
		t.Fatalf("sqlite3: %v: %s", err, out)
	}
	return strings.TrimSpace(string(out))
}

const renamedCursorChat = "0aab8bc9-52b9-4f6c-b446-b535628310c6"

func cursorEditorDB(t *testing.T, withData bool) string {
	t.Helper()
	db := filepath.Join(t.TempDir(), "state.vscdb")
	script := `CREATE TABLE composerHeaders (composerId TEXT PRIMARY KEY, workspaceId TEXT, createdAt INTEGER, lastUpdatedAt INTEGER, isArchived INTEGER, isSubagent INTEGER, recency INTEGER, checkpointAt INTEGER, value TEXT, subagentTypeName TEXT);
CREATE TABLE cursorDiskKV (key TEXT UNIQUE ON CONFLICT REPLACE, value BLOB);
INSERT INTO composerHeaders (composerId, recency, value) VALUES ('` + renamedCursorChat + `', 7, '{"name":"Old name","createdOnBranch":"main","n":12345678901234567890}');
INSERT INTO composerHeaders (composerId, recency, value) VALUES ('other-chat-0001', 8, '{"name":"Other"}');
INSERT INTO cursorDiskKV VALUES ('composerData:other-chat-0001', '{"name":"Other"}');`
	if withData {
		script += `INSERT INTO cursorDiskKV VALUES ('composerData:` + renamedCursorChat + `', '{"composerId":"` + renamedCursorChat + `","name":"Old name","text":"it''s"}');`
	}
	sqliteExec(t, db, script)
	return db
}

func TestRename_CursorEditorChatChangesOnlyItsNameInBothPlaces(t *testing.T) {
	needSQLite(t)
	db := cursorEditorDB(t, true)
	r := &Renamer{CursorDB: db, sqlite: RunSQLiteWrite}

	if err := r.Rename(context.Background(), Conversation{Tool: ToolCursor, App: AppIDE, ID: renamedCursorChat}, "Don't ship; it's 'quoted'"); err != nil {
		t.Fatal(renameError(err))
	}

	got := sqliteExec(t, db, `SELECT value FROM composerHeaders ORDER BY recency; SELECT key || ' ' || value FROM cursorDiskKV ORDER BY key;`)
	want := `{"name":"Don't ship; it's 'quoted'","createdOnBranch":"main","n":12345678901234567890}
{"name":"Other"}
composerData:` + renamedCursorChat + ` {"composerId":"` + renamedCursorChat + `","name":"Don't ship; it's 'quoted'","text":"it's"}
composerData:other-chat-0001 {"name":"Other"}`
	if got != want {
		t.Errorf("the database holds\n%s", got)
	}
}

func TestRename_CursorEditorChatChangesNothingWhenOneOfItsPlacesIsMissing(t *testing.T) {
	needSQLite(t)
	db := cursorEditorDB(t, false)
	r := &Renamer{CursorDB: db, sqlite: RunSQLiteWrite}

	err := r.Rename(context.Background(), Conversation{Tool: ToolCursor, App: AppIDE, ID: renamedCursorChat}, "New name")

	if got, want := renameError(err), "Cursor keeps the conversation in a way this version does not know (unexpected): "+db+": changed 0 rows, want 2"; got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	if got := sqliteExec(t, db, `SELECT json_extract(value, '$.name') FROM composerHeaders WHERE composerId = '`+renamedCursorChat+`'`); got != "Old name" {
		t.Errorf("the header's name = %q", got)
	}
}

func TestRename_CursorEditorChatChangesNothingUnlessBothPlacesHoldAnObject(t *testing.T) {
	needSQLite(t)
	for name, change := range map[string]string{
		"NULL chat data":   `UPDATE cursorDiskKV SET value = NULL WHERE key = 'composerData:` + renamedCursorChat + `'`,
		"scalar chat data": `UPDATE cursorDiskKV SET value = '3' WHERE key = 'composerData:` + renamedCursorChat + `'`,
		"text chat data":   `UPDATE cursorDiskKV SET value = 'not json' WHERE key = 'composerData:` + renamedCursorChat + `'`,
		"NULL header":      `UPDATE composerHeaders SET value = NULL WHERE composerId = '` + renamedCursorChat + `'`,
		"array header":     `UPDATE composerHeaders SET value = '["Old name"]' WHERE composerId = '` + renamedCursorChat + `'`,
	} {
		t.Run(name, func(t *testing.T) {
			db := cursorEditorDB(t, true)
			sqliteExec(t, db, change)
			before := sqliteExec(t, db, `SELECT quote(value) FROM composerHeaders ORDER BY recency; SELECT quote(value) FROM cursorDiskKV ORDER BY key;`)
			r := &Renamer{CursorDB: db, sqlite: RunSQLiteWrite}

			err := r.Rename(context.Background(), Conversation{Tool: ToolCursor, App: AppIDE, ID: renamedCursorChat}, "New name")

			if got, want := renameError(err), "Cursor keeps the conversation in a way this version does not know (unexpected): "+db+": changed 0 rows, want 2"; got != want {
				t.Errorf("got  %s\nwant %s", got, want)
			}
			if after := sqliteExec(t, db, `SELECT quote(value) FROM composerHeaders ORDER BY recency; SELECT quote(value) FROM cursorDiskKV ORDER BY key;`); after != before {
				t.Errorf("the database changed:\n%s", after)
			}
		})
	}
}

func TestRename_CursorRefusesAWriteWhoseRowsDoNotHoldTheNewName(t *testing.T) {
	for name, rows := range map[string][]json.RawMessage{
		"a row without the name": {json.RawMessage(`{"named":1}`), json.RawMessage(`{"named":0}`)},
		"a row it cannot read":   {json.RawMessage(`{"named":1}`), json.RawMessage(`{"named":null}`)},
	} {
		t.Run(name, func(t *testing.T) {
			write := func(context.Context, string, string) ([]json.RawMessage, error) { return rows, nil }
			r := &Renamer{CursorDB: "state.vscdb", sqlite: write}

			err := r.Rename(context.Background(), Conversation{Tool: ToolCursor, App: AppIDE, ID: renamedCursorChat}, "New name")

			var renameErr *RenameError
			if !errors.As(err, &renameErr) || renameErr.Reason != RenameUnexpected {
				t.Errorf("= %s", renameError(err))
			}
		})
	}
}

func TestRename_CursorEditorChatSaysWhenTheDatabaseIsMissing(t *testing.T) {
	needSQLite(t)
	db := filepath.Join(t.TempDir(), "state.vscdb")
	r := &Renamer{CursorDB: db, sqlite: RunSQLiteWrite}

	err := r.Rename(context.Background(), Conversation{Tool: ToolCursor, App: AppIDE, ID: renamedCursorChat}, "New name")

	var renameErr *RenameError
	if !errors.As(err, &renameErr) || renameErr.Reason != RenameRefused {
		t.Errorf("= %s", renameError(err))
	}
	if _, statErr := os.Stat(db); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("the rename created the database: %v", statErr)
	}
}

func cursorAgentStore(t *testing.T) string {
	t.Helper()
	store := filepath.Join(t.TempDir(), "chats", "hash", "bc-chat", "store.db")
	if err := os.MkdirAll(filepath.Dir(store), 0o755); err != nil {
		t.Fatal(err)
	}
	meta := hex.EncodeToString([]byte(`{"agentId":"bc-adbb6bdc-fea5-4d14-aa4b-1adcd8d6c3a0","latestRootBlobId":"8cf1","name":"New Agent","mode":"background","createdAt":1777203106986}`))
	sqliteExec(t, store, `CREATE TABLE blobs (id TEXT PRIMARY KEY, data BLOB); CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT);
INSERT INTO meta VALUES ('0', '`+meta+`');`)
	return store
}

func TestRename_CursorAgentChatChangesOnlyItsName(t *testing.T) {
	needSQLite(t)
	store := cursorAgentStore(t)
	r := &Renamer{sqlite: RunSQLiteWrite, holders: fakeProcesses{}}

	if err := r.Rename(context.Background(), Conversation{Tool: ToolCursor, App: AppCLI, ID: "bc-adbb6bdc-fea5-4d14-aa4b-1adcd8d6c3a0", store: store}, "Release notes"); err != nil {
		t.Fatal(renameError(err))
	}

	stored := sqliteExec(t, store, `SELECT value FROM meta WHERE key = '0'`)
	decoded, err := hex.DecodeString(stored)
	if err != nil || stored != strings.ToLower(stored) {
		t.Fatalf("the meta row is not lowercase hex: %q", stored)
	}
	if want := `{"agentId":"bc-adbb6bdc-fea5-4d14-aa4b-1adcd8d6c3a0","latestRootBlobId":"8cf1","name":"Release notes","mode":"background","createdAt":1777203106986}`; string(decoded) != want {
		t.Errorf("the meta holds %s", decoded)
	}
	if meta := decodeCursorAgentMeta([]cursorAgentMetaRow{{Value: stored}}); meta.Name != "Release notes" {
		t.Errorf("the list reads the name %q", meta.Name)
	}
}

func TestRename_CursorAgentChatKeepsItsStoreUnlessAFreshLookFindsItStopped(t *testing.T) {
	needSQLite(t)
	for name, tc := range map[string]struct {
		processes fakeProcesses
		live      bool
		want      string
	}{
		"a chat that started after the list was read": {
			fakeProcesses{held: map[string]int{"store.db": 4242}},
			false,
			"Cursor has the conversation open and would write its old name back (in-use): cursor-agent (pid 4242) runs the chat bc-chat",
		},
		"open files that cannot be read": {
			fakeProcesses{readFails: true},
			false,
			"Cursor did not take the name (refused): could not check whether cursor-agent runs the chat bc-chat: process read failed",
		},
		"a chat the list saw running": {
			fakeProcesses{held: map[string]int{"store.db": 4242}},
			true,
			"Cursor has the conversation open and would write its old name back (in-use): cursor-agent (pid 4242) runs the chat bc-chat",
		},
	} {
		t.Run(name, func(t *testing.T) {
			store := cursorAgentStore(t)
			before := sqliteExec(t, store, `SELECT value FROM meta`)
			r := &Renamer{sqlite: RunSQLiteWrite, holders: tc.processes}

			err := r.Rename(context.Background(), Conversation{Tool: ToolCursor, App: AppCLI, ID: "bc-chat", store: store, Live: tc.live}, "Release notes")

			if got := renameError(err); got != tc.want {
				t.Errorf("got  %s\nwant %s", got, tc.want)
			}
			if after := sqliteExec(t, store, `SELECT value FROM meta`); after != before {
				t.Error("the store changed")
			}
		})
	}
}

func TestRename_CursorAgentChatChangesNothingUnlessItsMetaIsAnObject(t *testing.T) {
	needSQLite(t)
	for name, value := range map[string]string{
		"NULL meta":   `NULL`,
		"scalar meta": `'` + hex.EncodeToString([]byte(`3`)) + `'`,
		"array meta":  `'` + hex.EncodeToString([]byte(`["New Agent"]`)) + `'`,
	} {
		t.Run(name, func(t *testing.T) {
			store := cursorAgentStore(t)
			sqliteExec(t, store, `UPDATE meta SET value = `+value+` WHERE key = '0'`)
			before := sqliteExec(t, store, `SELECT quote(value) FROM meta`)
			r := &Renamer{sqlite: RunSQLiteWrite, holders: fakeProcesses{}}

			err := r.Rename(context.Background(), Conversation{Tool: ToolCursor, App: AppCLI, ID: "bc-chat", store: store}, "Release notes")

			if got, want := renameError(err), "Cursor keeps the conversation in a way this version does not know (unexpected): "+store+": changed 0 rows, want 1"; got != want {
				t.Errorf("got  %s\nwant %s", got, want)
			}
			if after := sqliteExec(t, store, `SELECT quote(value) FROM meta`); after != before {
				t.Errorf("the store changed: %s", after)
			}
		})
	}
}

// A name is not a message: a cursor-agent chat marked done and read
// stays so after it is renamed in its store and in the marks.
func TestRename_CursorAgentChatKeepsItsCompletedAndSeenMarks(t *testing.T) {
	needSQLite(t)
	scanner, stores := testScanner(t, RunSQLite, fakeProcesses{})
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	agent := "bc-adbb6bdc-fea5-4d14-aa4b-1adcd8d6c3a0"
	store := filepath.Join(stores.CursorDir, "chats", md5Hex("/tmp/app"), agent, "store.db")
	if err := os.MkdirAll(filepath.Dir(store), 0o755); err != nil {
		t.Fatal(err)
	}
	meta := hex.EncodeToString([]byte(`{"agentId":"` + agent + `","name":"New Agent","createdAt":1777203106986}`))
	sqliteExec(t, store, `PRAGMA journal_mode = WAL; CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT); INSERT INTO meta VALUES ('0', '`+meta+`');`)
	// The last message was an hour ago. sqlite3 on macOS keeps the WAL
	// file after it closes the store.
	written := scanner.Now().Add(-time.Hour)
	if err := keepModified(written, store, store+"-wal"); err != nil {
		t.Fatal(err)
	}
	c, ok := scanner.Find(context.Background(), ToolCursor, agent, ScanOptions{})
	if !ok {
		t.Fatal("the chat is not listed")
	}
	if err := scanner.Mark(ToolCursor, agent, MarkCompleted); err != nil {
		t.Fatal(err)
	}

	if err := (&Renamer{sqlite: RunSQLiteWrite, holders: fakeProcesses{}}).Rename(context.Background(), c, "Release notes"); err != nil {
		t.Fatal(renameError(err))
	}
	if err := scanner.Rename(ToolCursor, agent, "Release notes"); err != nil {
		t.Fatal(err)
	}

	renamed := byID(t, scanner.Scan(context.Background(), ScanOptions{}), agent)
	if renamed.Title != "Release notes" || renamed.State != StateCompleted || renamed.CompletedAt == "" || renamed.SeenAt == "" {
		t.Errorf("after the rename: title %q, state %s, completedAt %q, seenAt %q", renamed.Title, renamed.State, renamed.CompletedAt, renamed.SeenAt)
	}
	if stored := sqliteExec(t, store, `SELECT value FROM meta WHERE key = '0'`); decodeCursorAgentMeta([]cursorAgentMetaRow{{Value: stored}}).Name != "Release notes" {
		t.Errorf("the store holds %s", stored)
	}
}

func TestRename_CursorSaysSQLiteIsMissing(t *testing.T) {
	noWrite := func(context.Context, string, string) ([]json.RawMessage, error) { return nil, errNoSQLite }
	r := &Renamer{CursorDB: "state.vscdb", sqlite: noWrite}

	err := r.Rename(context.Background(), Conversation{Tool: ToolCursor, App: AppIDE, ID: renamedCursorChat}, "New name")

	if got, want := renameError(err), "the program that writes names into Cursor is not installed (no-program): the sqlite3 command is not installed"; got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

const chatGPTChat = "6a1b2c3d-0000-4000-8000-00000000abcd"

func chatGPTServer(t *testing.T, answer func(w http.ResponseWriter, r *http.Request)) *chatGPTTitles {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(answer))
	t.Cleanup(server.Close)
	return &chatGPTTitles{baseURL: server.URL, client: server.Client(), credentials: fakeCodexLogin{found: true}}
}

func TestRename_ChatGPTChangesTheChatTitleOnChatGPT(t *testing.T) {
	var asked string
	titles := chatGPTServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		asked = fmt.Sprintf("%s %s auth=%t account=%s type=%s body=%s", r.Method, r.URL.Path,
			r.Header.Get("Authorization") == "Bearer "+fakeToken, r.Header.Get("ChatGPT-Account-Id"), r.Header.Get("Content-Type"), body)
		_, _ = io.WriteString(w, `{"success":true}`)
	})

	if err := (&Renamer{chatGPT: titles}).Rename(context.Background(), Conversation{Tool: ToolChatGPT, App: AppChat, ID: chatGPTChat}, "Trip plans, day 2"); err != nil {
		t.Fatal(renameError(err))
	}

	if want := "PATCH /backend-api/conversation/" + chatGPTChat + " auth=true account=" + fakeAccount + ` type=application/json body={"title":"Trip plans, day 2"}`; asked != want {
		t.Errorf("asked  %s\nwant   %s", asked, want)
	}
}

func TestRename_ChatGPTSaysWhyTheTitleDidNotChange(t *testing.T) {
	answer := func(status int, body string) func(http.ResponseWriter, *http.Request) {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, body)
		}
	}
	for name, tc := range map[string]struct {
		titles *chatGPTTitles
		want   string
	}{
		"an expired sign-in": {
			chatGPTServer(t, answer(http.StatusUnauthorized, `{"detail":"token expired"}`)),
			`no sign-in that may rename ChatGPT chats was found, or it expired (signed-out): HTTP 401: {"detail":"token expired"}`,
		},
		"a refusal": {
			chatGPTServer(t, answer(http.StatusNotFound, `{"detail":"Can't load conversation"}`)),
			`ChatGPT did not take the name (refused): HTTP 404: {"detail":"Can't load conversation"}`,
		},
		"an answer that says it failed": {
			chatGPTServer(t, answer(http.StatusOK, `{"success":false}`)),
			"ChatGPT did not take the name (refused): the rename answer says the title did not change",
		},
		"an answer that does not say": {
			chatGPTServer(t, answer(http.StatusOK, `{}`)),
			"ChatGPT keeps the conversation in a way this version does not know (unexpected): the rename answer does not say whether the title changed",
		},
		"an answer that is not JSON": {
			chatGPTServer(t, answer(http.StatusOK, `<html>`)),
			"ChatGPT keeps the conversation in a way this version does not know (unexpected): rename answer: invalid character '<' looking for beginning of value",
		},
		"a refusal that echoes the token": {
			chatGPTServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, strings.Repeat("x", 260)+" sent: "+r.Header.Get("Authorization"))
			}),
			"ChatGPT did not take the name (refused): HTTP 400: " + strings.Repeat("x", 260) + " sent: Bearer <redacted>",
		},
		"no Codex login": {
			func() *chatGPTTitles {
				titles := chatGPTServer(t, answer(http.StatusOK, `{}`))
				titles.credentials = fakeCodexLogin{}
				return titles
			}(),
			"no sign-in that may rename ChatGPT chats was found, or it expired (signed-out): no Codex login with ChatGPT on this machine",
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := (&Renamer{chatGPT: tc.titles}).Rename(context.Background(), Conversation{Tool: ToolChatGPT, App: AppChat, ID: chatGPTChat}, "New name")
			if got := renameError(err); got != tc.want {
				t.Errorf("got  %s\nwant %s", got, tc.want)
			}
			if strings.Contains(renameError(err), fakeToken) {
				t.Error("the error holds the token")
			}
		})
	}
}
