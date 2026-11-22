package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/conversations"
	"github.com/chemaclass/agnostic-ai/internal/mailbox"
)

func mailboxHTTPFixture(t *testing.T) (*conversationsHandler, *recordingLauncher) {
	t.Helper()
	h, launcher, _ := conversationsFixture(t, true)
	h.scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	h.companion = testCompanionSecret
	return h, launcher
}

// mailboxHTTP calls as the companion, with its secret.
func mailboxHTTP(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return mailboxHTTPAs(t, h, string(testCompanionSecret), method, path, body)
}

// mailboxHTTPAs calls with secret, or as another local client when it is empty.
func mailboxHTTPAs(t *testing.T, h http.Handler, secret, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "http://127.0.0.1:4310/"+strings.TrimPrefix(path, "/"), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		req.Header.Set(companionSecretHeader, secret)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func mailboxSendBody(to, typ, body, thread string) string {
	return fmt.Sprintf(`{"from":{"tool":"claude","id":%q},"to":{"id":%q},"type":%q,"body":%q,"thread":%q}`, inProject, to, typ, body, thread)
}

func mailboxSendThread(t *testing.T, h http.Handler, body string) string {
	t.Helper()
	rec := mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/send", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("mailbox send = %d %s", rec.Code, rec.Body)
	}
	var receipt struct {
		Thread string `json:"thread"`
		N      int    `json:"n"`
		State  string `json:"state"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Thread == "" || receipt.N < 1 || receipt.State != "waiting" {
		t.Errorf("mailbox receipt = %#v", receipt)
	}
	return receipt.Thread
}

func mailboxExpectRefusal(t *testing.T, rec *httptest.ResponseRecorder, guard string) {
	t.Helper()
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("mailbox refusal = %d %s, want 422 guard %q", rec.Code, rec.Body, guard)
		return
	}
	var got struct {
		Guard string `json:"guard"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Guard != guard || !strings.Contains(got.Error, "(guard: "+guard+")") || !strings.Contains(got.Error, "\n") {
		t.Errorf("mailbox refusal = %#v, want guard %q, reason and next step", got, guard)
	}
}

func TestMailboxHTTP_ListsTheThreadUnderBothSessionsWithoutChangingMarks(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	before := listConversations(t, h)
	marksBefore, err := os.ReadFile(h.scanner.MarksPath)
	if err != nil {
		t.Fatal(err)
	}
	marked := make(map[string]conversations.Conversation, len(before.Conversations))
	for _, row := range before.Conversations {
		marked[row.ID] = row
	}
	tag := mailboxHTTP(t, h, http.MethodGet, conversationsViewPath, "").Header().Get("ETag")
	id := mailboxSendThread(t, h, mailboxSendBody(inWorktree, "question", "Did the tests pass?", ""))

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4310"+conversationsViewPath, nil)
	req.Header.Set("If-None-Match", tag)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Header().Get("ETag") == tag {
		t.Errorf("an earlier ETag after send = %d %q, want a new list", rec.Code, rec.Header().Get("ETag"))
	}
	view := listConversations(t, h)
	for _, row := range view.Conversations {
		prior := marked[row.ID]
		if row.State != prior.State || row.SeenAt != prior.SeenAt || row.CompletedAt != prior.CompletedAt {
			t.Errorf("send changed marks of %s", row.ID)
		}
		if row.ID == elsewhere {
			if len(row.Threads) != 0 {
				t.Errorf("unrelated row showed threads: %#v", row.Threads)
			}
			continue
		}
		if len(row.Threads) != 1 {
			t.Errorf("row %s showed %d threads, want one", row.ID, len(row.Threads))
			continue
		}
		thread := row.Threads[0]
		if thread.ID != id || thread.Count != 1 || thread.Cap != 8 || thread.Waiting != 1 ||
			thread.LastType != "question" || thread.With.ID == row.ID || thread.WithTitle == "" {
			t.Errorf("thread on %s = %#v", row.ID, thread)
		}
	}
	for _, session := range []string{inProject, inWorktree} {
		rec = mailboxHTTP(t, h, http.MethodGet, "/api/mailbox/threads?tool=claude&id="+session, "")
		body := rec.Body.String()
		if rec.Code != http.StatusOK || !strings.Contains(body, id) || !strings.Contains(body, `"by":"human"`) ||
			!strings.Contains(body, `"queuedAt"`) || !strings.Contains(body, `"state":"waiting"`) {
			t.Errorf("threads of %s = %d %s", session, rec.Code, body)
		}
	}
	marksAfter, err := os.ReadFile(h.scanner.MarksPath)
	if err != nil || !bytes.Equal(marksBefore, marksAfter) {
		t.Errorf("send changed the marks file: %v", err)
	}
}

func TestMailboxHTTP_RefusesUnlistedSessionsAndForgedFields(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	valid := mailboxSendBody(inWorktree, "question", "Did the tests pass?", "")
	for _, test := range []struct{ name, body, guard string }{
		{"not known", mailboxSendBody("44444444-4444-4444-8444-444444444444", "question", "Did the tests pass?", ""), "unknown-session"},
		{"sender identity", strings.Replace(valid, `"id":`, `"by":"agent","id":`, 1), "format"},
		{"message state", `{"state":"delivered",` + strings.TrimPrefix(valid, "{"), "format"},
		{"queue time", `{"queuedAt":"2026-11-21T10:03:40Z",` + strings.TrimPrefix(valid, "{"), "format"},
		{"cap", `{"cap":80,` + strings.TrimPrefix(valid, "{"), "format"},
		{"events", `{"events":[],` + strings.TrimPrefix(valid, "{"), "format"},
		{"review request body", mailboxSendBody(inWorktree, "review_request", "", ""), "echo"},
		{"handoff outside Git", mailboxSendBody(inWorktree, "handoff", "", ""), "refs"},
		{"forged resume", fmt.Sprintf(`{"from":{"tool":"claude","id":%q},"to":{"id":%q},"type":"handoff","resume":{"worktree":"/tmp/unrelated"}}`, inProject, inWorktree), "format"},
		{"credential", mailboxSendBody(inWorktree, "question", "API_KEY=fixture-sensitive-value-123456789", ""), "secret"},
		{"too large", mailboxSendBody(inWorktree, "question", strings.Repeat("x", 4001), ""), "size"},
	} {
		t.Run(test.name, func(t *testing.T) {
			mailboxExpectRefusal(t, mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/send", test.body), test.guard)
		})
	}
}

func TestMailboxHTTP_HandsOverASessionWithoutPaths(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	_, head := makeReviewRepo(t, h.root)
	body := fmt.Sprintf(`{"from":{"tool":"claude","id":%q},"to":{"id":%q},"type":"handoff"}`, inProject, inWorktree)
	rec := mailboxHTTP(t, h, http.MethodPost, mailboxSendPath, body)
	var receipt mailbox.Receipt
	if err := json.Unmarshal(rec.Body.Bytes(), &receipt); rec.Code != http.StatusOK || err != nil {
		t.Fatalf("mailbox send = %d %s (%v)", rec.Code, rec.Body, err)
	}
	branch := git(t, h.root, "branch", "--show-current")
	if receipt.State != mailbox.Waiting || receipt.Resume == nil || receipt.Resume.Worktree != canonicalDir(h.root) ||
		receipt.Resume.Branch != branch || receipt.Resume.Head != head || receipt.Resume.Note != "" {
		t.Errorf("mailbox receipt = %+v, resume = %+v", receipt, receipt.Resume)
	}
	rec = mailboxHTTP(t, h, http.MethodPost, mailboxHandoffPath, mailboxHandoffBody(receipt.Thread, "accept"))
	mailboxExpectRefusal(t, rec, "format")
}

func TestMailboxHTTP_RequiresLocalOriginAndJSON(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	body := mailboxSendBody(inWorktree, "question", "Did the tests pass?", "")
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4310/api/mailbox/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("mailbox send with wrong content type = %d %s", rec.Code, rec.Body)
	}

	mailboxSendThread(t, h, body)
	for _, path := range []string{"/api/mailbox/threads?tool=claude&id=" + inProject, conversationsViewPath} {
		req = httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4310"+path, nil)
		req.Header.Set("Origin", "https://allowed.example")
		rec = httptest.NewRecorder()
		authority.CORS(h, "https://allowed.example").ServeHTTP(rec, req)
		if path == conversationsViewPath {
			if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `"threads"`) {
				t.Errorf("allowed website sees mailbox summaries = %d %s", rec.Code, rec.Body)
			}
		} else if rec.Code != http.StatusForbidden {
			t.Errorf("allowed website reads mailbox details = %d %s", rec.Code, rec.Body)
		}
	}
}

func TestMailboxHTTP_PauseResumeAndStopKeepTheirChangesInTheThread(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	base := mailboxSendBody(inWorktree, "question", "Did the tests pass?", "")
	id := mailboxSendThread(t, h, base)
	change := func(action string) {
		t.Helper()
		rec := mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/thread", fmt.Sprintf(`{"thread":%q,"action":%q}`, id, action))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("thread %s = %d %s", action, rec.Code, rec.Body)
		}
	}
	change("pause")
	mailboxExpectRefusal(t, mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/send", mailboxSendBody(inWorktree, "finding", "Yes.", id)), "paused")
	change("resume")
	for range 7 {
		mailboxSendThread(t, h, mailboxSendBody(inWorktree, "finding", "Yes.", id))
	}
	change("raise-cap")
	mailboxSendThread(t, h, mailboxSendBody(inWorktree, "finding", "Yes.", id))
	change("stop")
	mailboxExpectRefusal(t, mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/send", mailboxSendBody(inWorktree, "finding", "Yes.", id)), "closed")

	rec := mailboxHTTP(t, h, http.MethodGet, "/api/mailbox/threads?tool=claude&id="+inProject, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"closed"`) ||
		!strings.Contains(rec.Body.String(), `"cap":16`) || strings.Count(rec.Body.String(), `"state":"cancelled"`) != 9 { //nolint:misspell // Match the mailbox/v1 wire spelling.
		t.Errorf("stopped thread = %d %s", rec.Code, rec.Body)
	}
}

func TestMailboxHTTP_EnforcesTheCapAndLetsDoneDrainWaitingMessages(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	id := mailboxSendThread(t, h, mailboxSendBody(inWorktree, "question", "Did the tests pass?", ""))
	for range 7 {
		mailboxSendThread(t, h, mailboxSendBody(inWorktree, "finding", "Yes.", id))
	}
	mailboxExpectRefusal(t, mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/send", mailboxSendBody(inWorktree, "finding", "Yes.", id)), "cap")
	mailboxSendThread(t, h, mailboxSendBody(inWorktree, "done", "", id))
	mailboxExpectRefusal(t, mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/send", mailboxSendBody(inWorktree, "finding", "Yes.", id)), "closed")
	rec := mailboxHTTP(t, h, http.MethodGet, "/api/mailbox/threads?tool=claude&id="+inProject, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"closing"`) {
		t.Errorf("closing thread = %d %s", rec.Code, rec.Body)
	}
}

func TestMailboxHTTP_CopyAcknowledgesSuccessAndUnavailableTypingLeavesItWaiting(t *testing.T) {
	h, launcher := mailboxHTTPFixture(t)
	id := mailboxSendThread(t, h, mailboxSendBody(inWorktree, "question", "Did the tests pass?", ""))
	stage := func(how string, confirm bool) *httptest.ResponseRecorder {
		t.Helper()
		body := fmt.Sprintf(`{"thread":%q,"n":1,"how":%q,"confirm":%t}`, id, how, confirm)
		return mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/stage", body)
	}
	rec := stage("type", false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"staged":false`) || len(launcher.calls) != 0 {
		t.Errorf("unavailable terminal typing = %d %s, launcher calls %v", rec.Code, rec.Body, launcher.calls)
	}
	rec = stage("copy", false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"staged":false`) || !strings.Contains(rec.Body.String(), `"text":`) {
		t.Errorf("prepare clipboard copy = %d %s", rec.Code, rec.Body)
	}
	rec = mailboxHTTP(t, h, http.MethodGet, "/api/mailbox/threads?tool=claude&id="+inProject, "")
	if !strings.Contains(rec.Body.String(), `"state":"waiting"`) {
		t.Errorf("prepare clipboard copy changed the message: %d %s", rec.Code, rec.Body)
	}
	rec = stage("copy", true)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"staged":true`) {
		t.Errorf("confirm clipboard copy = %d %s", rec.Code, rec.Body)
	}
	rec = stage("copy", true)
	if rec.Code != http.StatusOK {
		t.Errorf("retrying copy acknowledgement = %d %s", rec.Code, rec.Body)
	}
	rec = mailboxHTTP(t, h, http.MethodGet, "/api/mailbox/threads?tool=claude&id="+inProject, "")
	if !strings.Contains(rec.Body.String(), `"state":"staged"`) || strings.Count(rec.Body.String(), `"action":"copy"`) != 1 {
		t.Errorf("confirm clipboard copy did not update the message: %d %s", rec.Code, rec.Body)
	}
}

func TestMailboxHTTP_AmbiguousPastedIDNeedsASelectedTool(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	path := filepath.Join(h.scanner.Stores.CodexDir, "state_5.sqlite")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	thread := json.RawMessage(fmt.Sprintf(`{"id":%q,"cwd":%q,"updated_at_ms":%d,"originator":"codex-tui","source":"cli","name":"Peer in Codex"}`,
		inWorktree, h.root, h.scanner.Now().UnixMilli()))
	h.scanner.SQLite = func(_ context.Context, path, query string) ([]json.RawMessage, error) {
		if filepath.Base(path) == "state_5.sqlite" && strings.Contains(query, "FROM threads") {
			return []json.RawMessage{thread}, nil
		}
		return nil, errors.New("no sqlite3 here")
	}

	body := mailboxSendBody(inWorktree, "question", "Did the tests pass?", "")
	rec := mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/send", body)
	mailboxExpectRefusal(t, rec, "unknown-session")
	if !strings.Contains(rec.Body.String(), "Two local sessions use that ID.") {
		t.Errorf("ambiguous destination = %d %s", rec.Code, rec.Body)
	}
	body = strings.Replace(body, `"to":{`, `"to":{"tool":"codex",`, 1)
	mailboxSendThread(t, h, body)
}

func TestMailboxHTTP_NestedTargetCanReplyToAnExistingThread(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	cwd := h.worktrees(h.root)[1]
	writeClaudeSession(t, h.scanner.Stores.ClaudeDir, inWorktree, cwd, "Continues claude:"+inProject+"\nReview the branch")
	id := mailboxSendThread(t, h, mailboxSendBody(inWorktree, "question", "Did the tests pass?", ""))
	body := fmt.Sprintf(`{"from":{"tool":"claude","id":%q},"to":{"tool":"claude","id":%q},"type":"finding","body":"Yes."}`, inWorktree, inProject)
	mailboxExpectRefusal(t, mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/send", body), "unknown-session")
	reply := fmt.Sprintf(`{"thread":%q,%s`, id, strings.TrimPrefix(body, "{"))
	if got := mailboxSendThread(t, h, reply); got != id {
		t.Errorf("nested session replied to %q, want %q", got, id)
	}
}

type mailboxTerminalProcesses struct {
	runningProcesses
}

func (p mailboxTerminalProcesses) Alive(pid int, _ string) bool { return pid == p.pid }

func (p mailboxTerminalProcesses) Table() ([]conversations.ProcessInfo, error) {
	return []conversations.ProcessInfo{
		{PID: 76, PPID: 1, Command: "/System/Applications/Utilities/Terminal.app/Contents/MacOS/Terminal"},
		{PID: p.pid, PPID: 76, Stat: "S+", TTY: "/dev/ttys001", Command: "claude"},
	}, nil
}

type mailboxTerminalLauncher struct {
	recordingLauncher
	typeInTab func(context.Context, string, string, string, bool) (bool, error)
}

func (l *mailboxTerminalLauncher) TypeInTab(ctx context.Context, app, tty, text string, setClipboard bool) (bool, error) {
	return l.typeInTab(ctx, app, tty, text, setClipboard)
}

func TestMailboxHTTP_TypesOnceAcrossServerInstances(t *testing.T) {
	h, _, project := conversationsFixture(t, true)
	h.scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	h.companion = testCompanionSecret
	writeFile(t, filepath.Join(h.scanner.Stores.ClaudeDir, "sessions", "77.json"), `{"pid":77,"sessionId":"`+inWorktree+`","status":"idle"}`)
	h.scanner.Processes = mailboxTerminalProcesses{runningProcesses: runningProcesses{pid: 77}}
	id := mailboxSendThread(t, h, mailboxSendBody(inWorktree, "question", "Did the tests pass?", ""))
	body := fmt.Sprintf(`{"thread":%q,"n":1,"how":"type"}`, id)

	var calls atomic.Int32
	typing := make(chan struct{}, 2)
	continueTyping := make(chan struct{})
	release := sync.OnceFunc(func() { close(continueTyping) })
	defer release()
	launcher := &mailboxTerminalLauncher{typeInTab: func(ctx context.Context, app, tty, text string, setClipboard bool) (bool, error) {
		calls.Add(1)
		if app != "Terminal" || tty != "/dev/ttys001" || !setClipboard ||
			!strings.HasPrefix(text, "<agnostic-ai-mailbox>\n") || !strings.Contains(text, "Did the tests pass?") {
			t.Errorf("type into %s %s with clipboard=%v and text %q", app, tty, setClipboard, text)
		}
		typing <- struct{}{}
		select {
		case <-continueTyping:
			return true, nil
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}}
	h.launcher = launcher

	another := &conversationsHandler{userLevel: true, root: project, openReplyWait: h.openReplyWait, launcher: launcher, companion: testCompanionSecret,
		scanner: &conversations.Scanner{
			Stores: h.scanner.Stores, SQLite: h.scanner.SQLite, MarksPath: h.scanner.MarksPath, Now: h.scanner.Now,
			Processes: mailboxTerminalProcesses{runningProcesses: runningProcesses{pid: 77}},
		},
	}

	first := make(chan *httptest.ResponseRecorder, 1)
	go func() { first <- mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/stage", body) }()
	select {
	case <-typing:
	case result := <-first:
		t.Fatalf("first type returned before typing: %d %s", result.Code, result.Body)
	case <-time.After(3 * time.Second):
		t.Fatal("first request did not reach the terminal")
	}

	secondReply := mailboxHTTP(t, another, http.MethodPost, "/api/mailbox/stage", body)
	if secondReply.Code != http.StatusConflict || !strings.Contains(secondReply.Body.String(), "mailbox is busy") {
		t.Errorf("second type while the first holds the lock = %d %s", secondReply.Code, secondReply.Body)
	}
	release()
	firstReply := <-first
	if firstReply.Code != http.StatusOK || !strings.Contains(firstReply.Body.String(), `"staged":true`) {
		t.Errorf("first type = %d %s", firstReply.Code, firstReply.Body)
	}
	mailboxExpectRefusal(t, mailboxHTTP(t, another, http.MethodPost, "/api/mailbox/stage", body), "format")
	if got := calls.Load(); got != 1 {
		t.Errorf("both requests typed %d times, want once", got)
	}
}

func TestMailboxHTTP_ReportsTypingThatCouldNotBeRecorded(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	writeFile(t, filepath.Join(h.scanner.Stores.ClaudeDir, "sessions", "77.json"), `{"pid":77,"sessionId":"`+inWorktree+`","status":"idle"}`)
	h.scanner.Processes = mailboxTerminalProcesses{runningProcesses: runningProcesses{pid: 77}}
	id := mailboxSendThread(t, h, mailboxSendBody(inWorktree, "question", "Did the tests pass?", ""))
	thread := filepath.Join(filepath.Dir(h.scanner.MarksPath), "mailbox", "threads", id+".json")
	h.launcher = &mailboxTerminalLauncher{typeInTab: func(context.Context, string, string, string, bool) (bool, error) {
		// The dispatch moved the text. Simulate storage becoming unavailable before it can save.
		if err := os.Rename(thread, thread+".held"); err != nil {
			return false, err
		}
		if err := os.Mkdir(thread, 0o700); err != nil {
			return false, err
		}
		return true, nil
	}}

	rec := mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/stage", fmt.Sprintf(`{"thread":%q,"n":1,"how":"type"}`, id))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"typed":true`) ||
		!strings.Contains(rec.Body.String(), `"staged":false`) || !strings.Contains(rec.Body.String(), "Check that session before trying again") {
		t.Errorf("typed message whose receipt could not be saved = %d %s", rec.Code, rec.Body)
	}
}

func (*mailboxTerminalLauncher) ShowTmuxPane(context.Context, int, string) (int, error) {
	return 0, nil
}
