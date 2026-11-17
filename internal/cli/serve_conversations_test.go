package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/conversations"
)

type stillProcesses struct{}

func (stillProcesses) Alive(int, string) bool { return false }
func (stillProcesses) AliveAmong([]conversations.ProcessRef) (map[int]bool, error) {
	return nil, nil
}
func (stillProcesses) Table() ([]conversations.ProcessInfo, error) { return nil, nil }
func (stillProcesses) Args([]int) map[int]string                   { return nil }
func (stillProcesses) Cwds([]int) map[int]string                   { return nil }
func (stillProcesses) Holders([]string) (map[string]int, error)    { return nil, nil }
func (stillProcesses) AppRunning(string) bool                      { return false }
func (stillProcesses) ClaudeParents([]int) (map[int]string, error) { return nil, nil }
func (stillProcesses) ClaudeJobs(context.Context) ([]conversations.ClaudeJob, error) {
	return nil, nil
}

// runningProcesses has one process running, with every other answer of stillProcesses.
type runningProcesses struct {
	stillProcesses
	pid int
}

func (r runningProcesses) AliveAmong(processes []conversations.ProcessRef) (map[int]bool, error) {
	alive := map[int]bool{}
	for _, process := range processes {
		if process.PID == r.pid {
			alive[process.PID] = true
		}
	}
	return alive, nil
}

type recordingLauncher struct{ calls []string }

func (l *recordingLauncher) OpenLink(_ context.Context, link string) error {
	l.calls = append(l.calls, "link "+link)
	return nil
}

func (l *recordingLauncher) OpenFolder(_ context.Context, app, folder string) error {
	l.calls = append(l.calls, "folder "+app+" "+folder)
	return nil
}

func (l *recordingLauncher) ActivateApp(_ context.Context, app string) error {
	l.calls = append(l.calls, "activate "+app)
	return nil
}

func (l *recordingLauncher) RunInNewTerminal(_ context.Context, command string) error {
	l.calls = append(l.calls, "terminal "+command)
	return nil
}

func (l *recordingLauncher) FocusTab(context.Context, string, string) (bool, error) {
	return false, errors.New("unexpected focus")
}

func (l *recordingLauncher) RunInNewTab(context.Context, string, string) error {
	return errors.New("unexpected tab")
}

func (l *recordingLauncher) TitleTab(_ context.Context, app, tty, title string) (bool, error) {
	l.calls = append(l.calls, "title "+app+" "+tty+" "+title)
	return true, nil
}

func (l *recordingLauncher) TypeInTab(_ context.Context, app, tty, text string, _ bool) (bool, error) {
	l.calls = append(l.calls, "type "+app+" "+tty+" "+text)
	return true, nil
}

func realDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

var projectFolderName = regexp.MustCompile(`[^A-Za-z0-9]`)

func writeClaudeSession(t *testing.T, claudeDir, id, cwd, prompt string) {
	t.Helper()
	line, err := json.Marshal(map[string]any{
		"type": "user", "cwd": cwd, "entrypoint": "cli", "timestamp": "2026-11-07T08:00:00.000Z",
		"message": map[string]any{"role": "user", "content": prompt},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Claude Code names a project folder after its path, each character
	// other than a letter or digit written as "-", so C:\ nests nothing.
	path := filepath.Join(claudeDir, "projects", projectFolderName.ReplaceAllString(cwd, "-"), id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(line, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

const (
	inProject  = "11111111-1111-4111-8111-111111111111"
	inWorktree = "22222222-2222-4222-8222-222222222222"
	elsewhere  = "33333333-3333-4333-8333-333333333333"
)

func conversationsFixture(t *testing.T, global bool) (*conversationsHandler, *recordingLauncher, string) {
	t.Helper()
	home, project, worktree, other := realDir(t), realDir(t), realDir(t), realDir(t)
	claudeDir := filepath.Join(home, ".claude")
	writeClaudeSession(t, claudeDir, inProject, filepath.Join(project, "src"), "Fix the parser")
	writeClaudeSession(t, claudeDir, inWorktree, worktree, "Review the branch")
	writeClaudeSession(t, claudeDir, elsewhere, other, "Unrelated work")
	launcher := &recordingLauncher{}
	scanner := &conversations.Scanner{
		Stores: conversations.Stores{Home: home, ClaudeDir: claudeDir, CodexDir: filepath.Join(home, ".codex"), CursorDB: filepath.Join(home, "state.vscdb"), CursorDir: filepath.Join(home, ".cursor")},
		SQLite: func(context.Context, string, string) ([]json.RawMessage, error) {
			return nil, errors.New("no sqlite3 here")
		},
		Processes: stillProcesses{},
		Now:       func() time.Time { return time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC) },
	}
	h := &conversationsHandler{
		root:      project,
		global:    global,
		scanner:   scanner,
		launcher:  launcher,
		worktrees: func(string) []string { return []string{project, worktree} },
		// No test writes into a real tool.
		renameInTool:  func(context.Context, conversations.Conversation, string) error { return nil },
		logOut:        io.Discard,
		openReplyWait: conversationOpenReplyTimeout,
	}
	return h, launcher, project
}

func listConversations(t *testing.T, h http.Handler) conversations.View {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4310"+conversationsViewPath+"?editor=cursor", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body)
	}
	var view conversations.View
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	return view
}

func ids(view conversations.View) string {
	var list []string
	for _, c := range view.Conversations {
		list = append(list, c.ID)
	}
	return strings.Join(list, ",")
}

func TestConversationsHandler_ListsTheProjectAndItsWorktrees(t *testing.T) {
	h, _, _ := conversationsFixture(t, false)

	view := listConversations(t, h)

	if view.SchemaVersion != conversations.SchemaVersion || ids(view) != inProject+","+inWorktree {
		t.Errorf("project view = %s %s", view.SchemaVersion, ids(view))
	}
}

// patientLauncher runs a terminal command only while its caller still waits, as a real launcher does.
type patientLauncher struct{ *recordingLauncher }

func (l patientLauncher) RunInNewTerminal(ctx context.Context, command string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return l.recordingLauncher.RunInNewTerminal(ctx, command)
}

func TestConversationsHandler_FinishesAnOpenItsCallerStoppedWaitingFor(t *testing.T) {
	h, launcher, _ := conversationsFixture(t, false)
	h.launcher = patientLauncher{launcher}
	gaveUp, cancel := context.WithCancel(context.Background())
	cancel()

	rec := postEdit(t, h, conversationOpenPath, `{"tool":"claude","id":"`+inProject+`","editor":"cursor"}`, func(r *http.Request) { *r = *r.WithContext(gaveUp) })

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"opened":true`) || len(launcher.calls) != 1 {
		t.Errorf("open = %d %s, launcher calls %q; want it opened although the caller left", rec.Code, rec.Body, launcher.calls)
	}
}

// slowLauncher takes delay to open a terminal, as a terminal script waiting for a window does.
type slowLauncher struct {
	*recordingLauncher
	delay time.Duration
}

func (l slowLauncher) RunInNewTerminal(ctx context.Context, command string) error {
	time.Sleep(l.delay)
	return l.recordingLauncher.RunInNewTerminal(ctx, command)
}

func TestConversationsHandler_AnswersAnOpenSlowerThanTheServerWriteTimeout(t *testing.T) {
	const unit = 10 * time.Millisecond
	h, launcher, _ := conversationsFixture(t, false)
	h.launcher = slowLauncher{launcher, 45 * unit}
	h.openReplyWait = 90 * unit
	srv := newServeServer(h)
	srv.ReadTimeout, srv.WriteTimeout = 10*unit, 30*unit
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	client := &http.Client{Timeout: 60 * unit}

	resp, err := client.Post("http://"+ln.Addr().String()+conversationOpenPath, "application/json", strings.NewReader(`{"tool":"claude","id":"`+inProject+`","editor":"cursor"}`))
	if err != nil {
		t.Fatalf("open got no answer: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"opened":true`) {
		t.Errorf("open = %d %s; want it answered", resp.StatusCode, body)
	}
}

func TestConversationOpenReplyTimeout_OutlastsTheCompanionOpenWait(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "..", "companion", "server.go"))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`const openTimeout = (\d+) \* time\.Second`).FindSubmatch(source)
	if match == nil {
		t.Fatal("companion/server.go no longer declares openTimeout in seconds; tie conversationOpenReplyTimeout to it again")
	}
	seconds, _ := strconv.Atoi(string(match[1]))
	companionWait := time.Duration(seconds) * time.Second

	if conversationOpenReplyTimeout <= companionWait {
		t.Errorf("an open answers within %s, but the companion waits %s", conversationOpenReplyTimeout, companionWait)
	}
}

func TestConversationsHandler_ListsEveryConversationOnAGlobalServer(t *testing.T) {
	h, _, _ := conversationsFixture(t, true)

	if actual := ids(listConversations(t, h)); actual != inProject+","+inWorktree+","+elsewhere {
		t.Errorf("global view = %s", actual)
	}
}

func TestConversationsHandler_OpensWithTheRouteItScanned(t *testing.T) {
	h, launcher, project := conversationsFixture(t, false)

	rec := postEdit(t, h, conversationOpenPath, `{"tool":"claude","id":"`+inProject+`","editor":"cursor"}`, nil)

	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"opened":true,"message":"Resumed in a new Terminal window."}` {
		t.Fatalf("open = %d %s", rec.Code, rec.Body)
	}
	if actual := strings.Join(launcher.calls, "; "); actual != "terminal cd '"+filepath.Join(project, "src")+"' && claude --resume "+inProject {
		t.Errorf("launcher calls = %q", actual)
	}
}

func TestConversationsHandler_RefusesWhatItDidNotList(t *testing.T) {
	h, launcher, _ := conversationsFixture(t, false)
	for name, tc := range map[string]struct {
		body, contentType string
		status            int
		reply             string
	}{
		"outside the project": {`{"tool":"claude","id":"` + elsewhere + `"}`, "application/json", http.StatusNotFound, `{"error":"That conversation is not in this list any more. Refresh the page."}`},
		"unknown tool":        {`{"tool":"gemini","id":"` + inProject + `"}`, "application/json", http.StatusBadRequest, `{"error":"tool must be claude, codex or cursor"}`},
		"a command field":     {`{"tool":"claude","id":"` + inProject + `","command":"rm -rf ~"}`, "application/json", http.StatusBadRequest, `{"error":"send {tool, id, editor}"}`},
		"two objects":         {`{"tool":"claude","id":"x"}{}`, "application/json", http.StatusBadRequest, `{"error":"send one JSON object"}`},
		"form post":           {`tool=claude`, "application/x-www-form-urlencoded", http.StatusUnsupportedMediaType, `{"error":"send Content-Type: application/json"}`},
	} {
		t.Run(name, func(t *testing.T) {
			rec := postEdit(t, h, conversationOpenPath, tc.body, func(r *http.Request) { r.Header.Set("Content-Type", tc.contentType) })
			if rec.Code != tc.status || strings.TrimSpace(rec.Body.String()) != tc.reply {
				t.Errorf("= %d %s", rec.Code, rec.Body)
			}
		})
	}
	foreign := postEdit(t, h, conversationOpenPath, `{"tool":"claude","id":"`+inProject+`"}`, func(r *http.Request) { r.Header.Set("Origin", "https://attacker.example") })
	if foreign.Code != http.StatusForbidden {
		t.Errorf("a foreign page = %d %s", foreign.Code, foreign.Body)
	}
	if len(launcher.calls) != 0 {
		t.Errorf("nothing may open, got %v", launcher.calls)
	}
}

func TestServeMux_ServesConversations(t *testing.T) {
	home := realDir(t)
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	srv := httptest.NewServer(newServeMux(nil, authority.HandlerOptions{Root: home}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + conversationsViewPath)
	if err != nil {
		t.Fatal(err)
	}
	var view conversations.View
	err = json.NewDecoder(resp.Body).Decode(&view)
	_ = resp.Body.Close()
	if err != nil || resp.StatusCode != http.StatusOK || view.SchemaVersion != conversations.SchemaVersion || len(view.Tools) != 3 {
		t.Errorf("GET %s = %d %+v (%v)", conversationsViewPath, resp.StatusCode, view, err)
	}

	rename, err := http.NewRequest(http.MethodPost, srv.URL+conversationRenamePath, strings.NewReader(`{"tool":"gemini","id":"x","title":"Parser fix"}`))
	if err != nil {
		t.Fatal(err)
	}
	rename.Header.Set("Content-Type", "application/json")
	rename.Header.Set("Origin", srv.URL)
	resp, err = http.DefaultClient.Do(rename)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("POST %s = %d, want the handler's own refusal", conversationRenamePath, resp.StatusCode)
	}

	remove, err := http.NewRequest(http.MethodPost, srv.URL+conversationDeletePath, strings.NewReader(`{"tool":"gemini","id":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	remove.Header.Set("Content-Type", "application/json")
	remove.Header.Set("Origin", srv.URL)
	resp, err = http.DefaultClient.Do(remove)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("POST %s = %d, want the handler's own refusal", conversationDeletePath, resp.StatusCode)
	}
}

func TestConversationsHandler_PagesByWeek(t *testing.T) {
	h, _, _ := conversationsFixture(t, true)
	get := func(query string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4310"+conversationsViewPath+query, nil))
		return rec
	}
	if rec := get("?before=2026-11-08T00:00:00Z"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"to":"2026-11-08T00:00:00Z"`) {
		t.Errorf("a later page = %d %s", rec.Code, rec.Body)
	}
	if rec := get("?before=last-week"); rec.Code != http.StatusBadRequest || strings.TrimSpace(rec.Body.String()) != `{"error":"before must be a time such as 2026-11-01T12:00:00Z, the next value of the previous page"}` {
		t.Errorf("a bad before = %d %s", rec.Code, rec.Body)
	}
}

func TestGitWorktrees_KeepsAProjectInASubfolderToItsOwnFolder(t *testing.T) {
	repo := realDir(t)
	project := filepath.Join(repo, "pkg-a")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git init: %v %s", err, out)
	}

	got := gitWorktrees(project)

	if len(got) != 1 || got[0] != project {
		t.Errorf("worktrees of %s = %v, want only the project folder", project, got)
	}
}

func TestConversationsHandler_SearchesAndFiltersByTool(t *testing.T) {
	h, _, _ := conversationsFixture(t, true)
	get := func(query string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4310"+conversationsViewPath+query, nil))
		return rec
	}
	var view conversations.View
	rec := get("?q=parser&tool=claude")
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil || rec.Code != http.StatusOK || view.Matches == nil || *view.Matches != 1 || ids(view) != inProject {
		t.Errorf("search = %d %s", rec.Code, rec.Body)
	}
	for query, reply := range map[string]string{
		"?tool=gemini":                   `{"error":"tool must be claude, codex or cursor"}`,
		"?q=" + strings.Repeat("a", 201): `{"error":"q is longer than 200 characters"}`,
	} {
		if rec := get(query); rec.Code != http.StatusBadRequest || strings.TrimSpace(rec.Body.String()) != reply {
			t.Errorf("%.20s = %d %s", query, rec.Code, rec.Body)
		}
	}
}

func stateOfListed(t *testing.T, h http.Handler, id string) (conversations.Conversation, int) {
	t.Helper()
	view := listConversations(t, h)
	for _, c := range view.Conversations {
		if c.ID == id {
			return c, view.NeedsYou
		}
	}
	t.Fatalf("no conversation %s listed", id)
	return conversations.Conversation{}, 0
}

func TestConversationsHandler_MarksAConversationCompletedThenOpenAgain(t *testing.T) {
	h, _, _ := conversationsFixture(t, false)
	h.scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")

	if rec := postEdit(t, h, conversationMarkPath, `{"tool":"claude","id":"`+inProject+`","as":"completed"}`, nil); rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("mark completed = %d %s", rec.Code, rec.Body)
	}
	if c, _ := stateOfListed(t, h, inProject); c.State != conversations.StateCompleted || c.CompletedAt != "2026-11-07T12:00:00Z" || c.SeenAt != "2026-11-07T12:00:00Z" {
		t.Errorf("after completing: state %s, completedAt %q, seenAt %q", c.State, c.CompletedAt, c.SeenAt)
	}
	if c, _ := stateOfListed(t, h, inWorktree); c.State != conversations.StateActive || c.CompletedAt != "" {
		t.Errorf("an unmarked conversation: state %s, completedAt %q", c.State, c.CompletedAt)
	}

	if rec := postEdit(t, h, conversationMarkPath, `{"tool":"claude","id":"`+inProject+`","as":"open"}`, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("mark open = %d %s", rec.Code, rec.Body)
	}
	if c, _ := stateOfListed(t, h, inProject); c.State != conversations.StateActive || c.CompletedAt != "" {
		t.Errorf("after reopening: state %s, completedAt %q", c.State, c.CompletedAt)
	}
}

func TestConversationsHandler_OpeningAConversationMarksItSeen(t *testing.T) {
	h, _, _ := conversationsFixture(t, false)
	h.scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")

	if rec := postEdit(t, h, conversationOpenPath, `{"tool":"claude","id":"`+inProject+`","editor":"cursor"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("open = %d %s", rec.Code, rec.Body)
	}
	if c, _ := stateOfListed(t, h, inProject); c.SeenAt != "2026-11-07T12:00:00Z" {
		t.Errorf("seenAt after opening = %q", c.SeenAt)
	}
	if c, _ := stateOfListed(t, h, inWorktree); c.SeenAt != "" {
		t.Errorf("seenAt of a conversation nobody opened = %q", c.SeenAt)
	}
}

func TestConversationsHandler_RefusesAMarkItCannotKeep(t *testing.T) {
	h, _, _ := conversationsFixture(t, false)
	h.scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	for name, tc := range map[string]struct {
		body   string
		status int
		reply  string
	}{
		"outside the project": {`{"tool":"claude","id":"` + elsewhere + `","as":"seen"}`, http.StatusNotFound, `{"error":"That conversation is not in this list any more. Refresh the page."}`},
		"unknown tool":        {`{"tool":"gemini","id":"` + inProject + `","as":"seen"}`, http.StatusBadRequest, `{"error":"tool must be claude, codex or cursor"}`},
		"unknown mark":        {`{"tool":"claude","id":"` + inProject + `","as":"deleted"}`, http.StatusBadRequest, `{"error":"as must be seen, completed, open or hidden"}`},
		"no mark":             {`{"tool":"claude","id":"` + inProject + `"}`, http.StatusBadRequest, `{"error":"as must be seen, completed, open or hidden"}`},
		"an extra field":      {`{"tool":"claude","id":"` + inProject + `","as":"seen","at":"2030-01-01T00:00:00Z"}`, http.StatusBadRequest, `{"error":"send {tool, id, as}"}`},
	} {
		t.Run(name, func(t *testing.T) {
			rec := postEdit(t, h, conversationMarkPath, tc.body, nil)
			if rec.Code != tc.status || strings.TrimSpace(rec.Body.String()) != tc.reply {
				t.Errorf("= %d %s", rec.Code, rec.Body)
			}
		})
	}
	foreign := postEdit(t, h, conversationMarkPath, `{"tool":"claude","id":"`+inProject+`","as":"completed"}`, func(r *http.Request) { r.Header.Set("Origin", "https://attacker.example") })
	if foreign.Code != http.StatusForbidden {
		t.Errorf("a foreign page = %d %s", foreign.Code, foreign.Body)
	}
	if c, _ := stateOfListed(t, h, inProject); c.State != conversations.StateActive || c.SeenAt != "" {
		t.Errorf("a refused mark was kept: state %s, seenAt %q", c.State, c.SeenAt)
	}
}

func TestConversationsHandler_SaysAServerWithoutAMarksFileKeepsNone(t *testing.T) {
	h, _, _ := conversationsFixture(t, false)

	rec := postEdit(t, h, conversationMarkPath, `{"tool":"claude","id":"`+inProject+`","as":"seen"}`, nil)

	if rec.Code != http.StatusInternalServerError || strings.TrimSpace(rec.Body.String()) != `{"error":"this server keeps no conversation marks"}` {
		t.Errorf("= %d %s", rec.Code, rec.Body)
	}
}

func TestConversationsHandler_AnswersNotModifiedWhileTheListHolds(t *testing.T) {
	h, _, _ := conversationsFixture(t, false)
	h.scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	get := func(query, tag string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4310"+conversationsViewPath+query, nil)
		if tag != "" {
			req.Header.Set("If-None-Match", tag)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	// The first scan begins the marks file, which is itself a change.
	get("", "")
	first := get("", "")
	tag := first.Header().Get("ETag")
	if first.Code != http.StatusOK || tag == "" {
		t.Fatalf("first read = %d, ETag %q", first.Code, tag)
	}

	if rec := get("", tag); rec.Code != http.StatusNotModified || rec.Body.Len() != 0 || rec.Header().Get("ETag") != tag {
		t.Errorf("an unchanged list = %d %s, ETag %q", rec.Code, rec.Body, rec.Header().Get("ETag"))
	}
	if rec := get("?q=fix", tag); rec.Code != http.StatusOK {
		t.Errorf("another question with the same tag = %d", rec.Code)
	}

	changes := map[string]func(){
		"a mark": func() {
			postEdit(t, h, conversationMarkPath, `{"tool":"claude","id":"`+inProject+`","as":"completed"}`, nil)
		},
		"a rename": func() {
			postEdit(t, h, conversationRenamePath, `{"tool":"claude","id":"`+inProject+`","title":"Parser fix"}`, nil)
		},
		"a write to a store": func() {
			sessions := filepath.Join(h.scanner.Stores.ClaudeDir, "sessions")
			if err := os.MkdirAll(sessions, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(sessions, "999.json"), []byte(`{}`), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"the next minute": func() {
			now := h.scanner.Now().Add(time.Minute)
			h.scanner.Now = func() time.Time { return now }
		},
	}
	for name, change := range changes {
		change()
		rec := get("", tag)
		if rec.Code != http.StatusOK || rec.Header().Get("ETag") == tag {
			t.Errorf("after %s = %d, ETag unchanged: %t", name, rec.Code, rec.Header().Get("ETag") == tag)
		}
		tag = rec.Header().Get("ETag")
	}
}

func TestConversationsHandler_RenamesAConversation(t *testing.T) {
	h, _, _ := conversationsFixture(t, false)
	h.scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")

	if rec := postEdit(t, h, conversationRenamePath, `{"tool":"claude","id":"`+inProject+`","title":"  Parser   fix "}`, nil); rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("rename = %d %s", rec.Code, rec.Body)
	}
	if c, _ := stateOfListed(t, h, inProject); c.Title != "Parser fix" || c.State != conversations.StateActive || c.SeenAt != "" {
		t.Errorf("after renaming: title %q, state %s, seenAt %q", c.Title, c.State, c.SeenAt)
	}
	if c, _ := stateOfListed(t, h, inWorktree); c.Title != "Review the branch" {
		t.Errorf("another conversation's title = %q", c.Title)
	}
}

func TestConversationsHandler_SavesTheNameOnlyOnceTheToolHasIt(t *testing.T) {
	h, _, _ := conversationsFixture(t, false)
	h.scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	type asked struct {
		id, title, savedBefore string
	}
	var got []asked
	h.renameInTool = func(_ context.Context, c conversations.Conversation, title string) error {
		saved, _ := stateOfListed(t, h, c.ID)
		got = append(got, asked{c.ID, title, saved.Title})
		return nil
	}

	rec := postEdit(t, h, conversationRenamePath, `{"tool":"claude","id":"`+inProject+`","title":"  Parser   fix "}`, nil)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("rename = %d %s", rec.Code, rec.Body)
	}
	// The tool is asked before the reply, while the list still shows the old name.
	if want := []asked{{inProject, "Parser fix", "Fix the parser"}}; !slices.Equal(got, want) {
		t.Errorf("the tool was asked for %+v\nwant %+v", got, want)
	}
	if c, _ := stateOfListed(t, h, inProject); c.Title != "Parser fix" {
		t.Errorf("the saved name = %q", c.Title)
	}
}

func TestConversationsHandler_KeepsTheOldNameAndSaysWhyWhenTheToolRefuses(t *testing.T) {
	for name, tc := range map[string]struct {
		err        error
		wantStatus int
		wantBody   string
		wantLog    string
	}{
		"not supported": {
			&conversations.RenameError{Tool: conversations.ToolClaude, Reason: conversations.RenameNotSupported, Kind: "Claude app chats"},
			http.StatusNotImplemented,
			`{"error":"Renaming Claude app chats is not supported yet. The name was not changed."}`,
			"! could not rename claude conversation " + inProject + " in the tool: renaming Claude app chats is not supported\n",
		},
		"no program": {
			&conversations.RenameError{Tool: conversations.ToolCodex, Reason: conversations.RenameNoProgram, Cause: errors.New(`exec: "codex": executable file not found in $PATH`)},
			http.StatusServiceUnavailable,
			`{"error":"Codex is not installed, and renaming a Codex chat needs its codex command. Install Codex and try again."}`,
			"! could not rename claude conversation " + inProject + " in the tool: the program that writes names into Codex is not installed: exec: \"codex\": executable file not found in $PATH\n",
		},
		"signed out": {
			&conversations.RenameError{Tool: conversations.ToolChatGPT, Reason: conversations.RenameSignedOut, Cause: errors.New(`HTTP 401: {"detail":"token expired"}`)},
			http.StatusForbidden,
			`{"error":"Sign in to ChatGPT in the Codex app, or run codex login, and try again."}`,
			"! could not rename claude conversation " + inProject + ` in the tool: no sign-in that may rename ChatGPT chats was found, or it expired: HTTP 401: {"detail":"token expired"}` + "\n",
		},
		"in use": {
			&conversations.RenameError{Tool: conversations.ToolClaude, Reason: conversations.RenameInUse, Cause: errors.New("the last line is unfinished")},
			http.StatusConflict,
			`{"error":"Claude Code has this conversation open and would put the old name back. Stop or close it there, then try again."}`,
			"! could not rename claude conversation " + inProject + " in the tool: Claude Code has the conversation open and would write its old name back: the last line is unfinished\n",
		},
		"refused": {
			&conversations.RenameError{Tool: conversations.ToolChatGPT, Reason: conversations.RenameRefused, Cause: errors.New(`HTTP 404: {"detail":"Can't load conversation"}`)},
			http.StatusBadGateway,
			`{"error":"ChatGPT did not take the new name. Try again, or rename it in ChatGPT."}`,
			"! could not rename claude conversation " + inProject + ` in the tool: ChatGPT did not take the name: HTTP 404: {"detail":"Can't load conversation"}` + "\n",
		},
		"unreachable": {
			&conversations.RenameError{Tool: conversations.ToolCodex, Reason: conversations.RenameUnreachable, Cause: context.DeadlineExceeded},
			http.StatusBadGateway,
			`{"error":"Codex did not answer in time. Try again."}`,
			"! could not rename claude conversation " + inProject + " in the tool: Codex did not answer in time: context deadline exceeded\n",
		},
		"unexpected": {
			&conversations.RenameError{Tool: conversations.ToolCursor, Reason: conversations.RenameUnexpected, Cause: errors.New("changed 1 rows, want 2")},
			http.StatusBadGateway,
			`{"error":"This version cannot rename this Cursor conversation. Rename it in Cursor."}`,
			"! could not rename claude conversation " + inProject + " in the tool: Cursor keeps the conversation in a way this version does not know: changed 1 rows, want 2\n",
		},
		"not a rename error": {
			errors.New("disk on fire"),
			http.StatusInternalServerError,
			`{"error":"The name could not be saved in the tool. The server log says why."}`,
			"! could not rename claude conversation " + inProject + " in the tool: disk on fire\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			h, _, _ := conversationsFixture(t, false)
			h.scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
			h.renameInTool = func(context.Context, conversations.Conversation, string) error { return tc.err }
			var logged strings.Builder
			h.logOut = &logged

			rec := postEdit(t, h, conversationRenamePath, `{"tool":"claude","id":"`+inProject+`","title":"Parser fix"}`, nil)

			if rec.Code != tc.wantStatus || strings.TrimSpace(rec.Body.String()) != tc.wantBody {
				t.Errorf("reply = %d %s\nwant    %d %s", rec.Code, rec.Body, tc.wantStatus, tc.wantBody)
			}
			if got := logged.String(); got != tc.wantLog {
				t.Errorf("logged %q\nwant   %q", got, tc.wantLog)
			}
			if c, _ := stateOfListed(t, h, inProject); c.Title != "Fix the parser" {
				t.Errorf("a name the tool did not take was saved: %q", c.Title)
			}
		})
	}
}

func TestConversationsHandler_RefusesARenameItCannotKeep(t *testing.T) {
	h, _, _ := conversationsFixture(t, false)
	h.scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	for name, tc := range map[string]struct {
		body   string
		status int
		reply  string
	}{
		"outside the project": {`{"tool":"claude","id":"` + elsewhere + `","title":"Parser fix"}`, http.StatusNotFound, `{"error":"That conversation is not in this list any more. Refresh the page."}`},
		"unknown tool":        {`{"tool":"gemini","id":"` + inProject + `","title":"Parser fix"}`, http.StatusBadRequest, `{"error":"tool must be claude, codex or cursor"}`},
		"an empty name":       {`{"tool":"claude","id":"` + inProject + `","title":"   "}`, http.StatusBadRequest, `{"error":"the name is empty"}`},
		"no name":             {`{"tool":"claude","id":"` + inProject + `"}`, http.StatusBadRequest, `{"error":"the name is empty"}`},
		"a long name":         {`{"tool":"claude","id":"` + inProject + `","title":"` + strings.Repeat("a", 121) + `"}`, http.StatusBadRequest, `{"error":"the name is longer than 120 characters"}`},
		"a password":          {`{"tool":"claude","id":"` + inProject + `","title":"Use password hunter2 here"}`, http.StatusBadRequest, `{"error":"the name may hold a password or key, so it is not saved"}`},
		"an extra field":      {`{"tool":"claude","id":"` + inProject + `","title":"Parser fix","as":"seen"}`, http.StatusBadRequest, `{"error":"send {tool, id, title}"}`},
	} {
		t.Run(name, func(t *testing.T) {
			rec := postEdit(t, h, conversationRenamePath, tc.body, nil)
			if rec.Code != tc.status || strings.TrimSpace(rec.Body.String()) != tc.reply {
				t.Errorf("= %d %s", rec.Code, rec.Body)
			}
		})
	}
	foreign := postEdit(t, h, conversationRenamePath, `{"tool":"claude","id":"`+inProject+`","title":"Parser fix"}`, func(r *http.Request) { r.Header.Set("Origin", "https://attacker.example") })
	if foreign.Code != http.StatusForbidden {
		t.Errorf("a foreign page = %d %s", foreign.Code, foreign.Body)
	}
	if c, _ := stateOfListed(t, h, inProject); c.Title != "Fix the parser" {
		t.Errorf("a refused rename was kept: title %q", c.Title)
	}
}

func TestConversationsHandler_ARenameOnOneServerShowsOnAnother(t *testing.T) {
	companion, _, _ := conversationsFixture(t, true)
	companion.scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	// The companion's global server and a project dashboard: two scanners, one marks file.
	shared := companion.scanner
	dashboard := &conversationsHandler{root: companion.root, scanner: &conversations.Scanner{
		Stores: shared.Stores, SQLite: shared.SQLite, Processes: shared.Processes, Now: shared.Now, MarksPath: shared.MarksPath,
	}, worktrees: companion.worktrees}
	if c, _ := stateOfListed(t, dashboard, inProject); c.Title != "Fix the parser" {
		t.Fatalf("before the rename the dashboard shows %q", c.Title)
	}

	if rec := postEdit(t, companion, conversationRenamePath, `{"tool":"claude","id":"`+inProject+`","title":"Release notes"}`, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("rename = %d %s", rec.Code, rec.Body)
	}

	if c, _ := stateOfListed(t, dashboard, inProject); c.Title != "Release notes" {
		t.Errorf("the dashboard shows %q", c.Title)
	}
	rec := httptest.NewRecorder()
	dashboard.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4310"+conversationsViewPath+"?q=release+notes", nil))
	var view conversations.View
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil || ids(view) != inProject {
		t.Errorf("the dashboard's search = %d %s", rec.Code, rec.Body)
	}
}

func TestConversationsHandler_RefusesASecondRenameOfAConversationWhileTheFirstRuns(t *testing.T) {
	companion, _, _ := conversationsFixture(t, true)
	companion.scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	shared := companion.scanner
	dashboard := &conversationsHandler{root: companion.root, scanner: &conversations.Scanner{
		Stores: shared.Stores, SQLite: shared.SQLite, Processes: shared.Processes, Now: shared.Now, MarksPath: shared.MarksPath,
	}, worktrees: companion.worktrees, logOut: io.Discard}
	var inTool []string
	var toolMu sync.Mutex
	takeName := func(title string) {
		toolMu.Lock()
		defer toolMu.Unlock()
		inTool = append(inTool, title)
	}
	firstAsked, firstGoesOn := make(chan struct{}), make(chan struct{})
	companion.renameInTool = func(_ context.Context, _ conversations.Conversation, title string) error {
		close(firstAsked)
		<-firstGoesOn
		takeName(title)
		return nil
	}
	dashboard.renameInTool = func(_ context.Context, _ conversations.Conversation, title string) error {
		takeName(title)
		return nil
	}
	first := make(chan *httptest.ResponseRecorder)
	go func() {
		first <- postEdit(t, companion, conversationRenamePath, `{"tool":"claude","id":"`+inProject+`","title":"First"}`, nil)
	}()
	<-firstAsked

	second := postEdit(t, dashboard, conversationRenamePath, `{"tool":"claude","id":"`+inProject+`","title":"Second"}`, nil)
	other := postEdit(t, dashboard, conversationRenamePath, `{"tool":"claude","id":"`+inWorktree+`","title":"Other"}`, nil)
	close(firstGoesOn)
	firstReply := <-first

	if second.Code != http.StatusConflict || strings.TrimSpace(second.Body.String()) != `{"error":"This conversation is being renamed already. Wait for that to finish, then try again."}` {
		t.Errorf("the second rename = %d %s", second.Code, second.Body)
	}
	if other.Code != http.StatusNoContent {
		t.Errorf("a rename of another conversation = %d %s", other.Code, other.Body)
	}
	if firstReply.Code != http.StatusNoContent {
		t.Errorf("the first rename = %d %s", firstReply.Code, firstReply.Body)
	}
	if want := []string{"Other", "First"}; !slices.Equal(inTool, want) {
		t.Errorf("the tool took %q, want %q", inTool, want)
	}
	if c, _ := stateOfListed(t, dashboard, inProject); c.Title != "First" {
		t.Errorf("the list shows %q, while the tool has First", c.Title)
	}
	if third := postEdit(t, dashboard, conversationRenamePath, `{"tool":"claude","id":"`+inProject+`","title":"Third"}`, nil); third.Code != http.StatusNoContent {
		t.Errorf("a rename after the first ended = %d %s", third.Code, third.Body)
	}
}

func TestConversationsHandler_AnArchivedConversationThatWorksShowsWorkingOnEveryServer(t *testing.T) {
	companion, _, _ := conversationsFixture(t, true)
	companion.scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	companion.scanner.Processes = runningProcesses{pid: 4242}
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	companion.scanner.Now = func() time.Time { return now }
	shared := companion.scanner
	dashboard := &conversationsHandler{root: companion.root, scanner: &conversations.Scanner{
		Stores: shared.Stores, SQLite: shared.SQLite, Processes: shared.Processes, Now: shared.Now, MarksPath: shared.MarksPath,
	}, worktrees: companion.worktrees}
	for _, id := range []string{inProject, inWorktree} {
		if rec := postEdit(t, companion, conversationMarkPath, `{"tool":"claude","id":"`+id+`","as":"completed"}`, nil); rec.Code != http.StatusNoContent {
			t.Fatalf("archive %s = %d %s", id, rec.Code, rec.Body)
		}
	}
	get := func(h http.Handler, tag string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4310"+conversationsViewPath, nil)
		if tag != "" {
			req.Header.Set("If-None-Match", tag)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	tag := get(companion, "").Header().Get("ETag")
	if rec := get(companion, tag); rec.Code != http.StatusNotModified {
		t.Fatalf("the archived list = %d", rec.Code)
	}

	session := filepath.Join(shared.Stores.ClaudeDir, "sessions", "4242.json")
	writeSession := func(status string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(session), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(session, []byte(`{"pid":4242,"sessionId":"`+inProject+`","status":"`+status+`"}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	now = now.Add(time.Second)
	writeSession("busy")
	rec := get(companion, tag)
	var view conversations.View
	if err := json.Unmarshal(rec.Body.Bytes(), &view); rec.Code != http.StatusOK || err != nil {
		t.Fatalf("the companion's next poll = %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(ids(view), inProject) {
		t.Fatalf("the companion lists %s", ids(view))
	}
	for _, c := range view.Conversations {
		if c.ID == inProject && (c.State != conversations.StateWorking || c.CompletedAt != "") {
			t.Errorf("the resumed conversation on the companion: state %s, completedAt %q", c.State, c.CompletedAt)
		}
	}

	writeSession("idle")
	if c, _ := stateOfListed(t, dashboard, inProject); c.State != conversations.StateActive || c.CompletedAt != "" {
		t.Errorf("after the work ends the dashboard shows: state %s, completedAt %q", c.State, c.CompletedAt)
	}
	if c, _ := stateOfListed(t, dashboard, inWorktree); c.State != conversations.StateCompleted {
		t.Errorf("the other archived conversation on the dashboard: state %s", c.State)
	}
}

// holdingProcesses has a Codex process hold the thread locks it names, with every other answer of stillProcesses.
type holdingProcesses struct {
	stillProcesses
	held map[string]int
}

func (h holdingProcesses) Holders(paths []string) (map[string]int, error) {
	holders := map[string]int{}
	for _, path := range paths {
		if pid, ok := h.held[filepath.Base(path)]; ok {
			holders[path] = pid
		}
	}
	return holders, nil
}

func TestConversationsHandler_EveryServerSaysWhenACodexWaitIsReadFromSilence(t *testing.T) {
	const thread = "44444444-4444-4444-8444-444444444444"
	companion, _, project := conversationsFixture(t, true)
	shared := companion.scanner
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	rollout := filepath.Join(shared.Stores.CodexDir, "sessions", "2026", "11", "07", "rollout-"+thread+".jsonl")
	for path, content := range map[string]string{
		filepath.Join(shared.Stores.CodexDir, "state_5.sqlite"):                      "",
		filepath.Join(shared.Stores.CodexDir, "thread-writer-locks", thread+".lock"): "",
		rollout: `{"type":"event_msg","payload":{"type":"task_started"}}` + "\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	silent := now.Add(-7 * time.Minute)
	if err := os.Chtimes(rollout, silent, silent); err != nil {
		t.Fatal(err)
	}
	threads, err := json.Marshal([]map[string]any{{
		"id": thread, "cwd": project, "created_at_ms": silent.Add(-time.Hour).UnixMilli(), "updated_at_ms": silent.UnixMilli(),
		"originator": "codex-tui", "source": "cli", "name": "Quiet turn", "rollout_path": rollout,
	}})
	if err != nil {
		t.Fatal(err)
	}
	shared.SQLite = func(_ context.Context, path, query string) ([]json.RawMessage, error) {
		if filepath.Base(path) != "state_5.sqlite" || !strings.Contains(query, "FROM threads") {
			return nil, errors.New("no sqlite3 here")
		}
		var rows []json.RawMessage
		return rows, json.Unmarshal(threads, &rows)
	}
	shared.Processes = holdingProcesses{held: map[string]int{thread + ".lock": 4242}}
	shared.Now = func() time.Time { return now }
	dashboard := &conversationsHandler{root: companion.root, global: true, scanner: &conversations.Scanner{
		Stores: shared.Stores, SQLite: shared.SQLite, Processes: shared.Processes, Now: shared.Now,
	}, worktrees: companion.worktrees}

	want := silent.UTC().Format(time.RFC3339)
	for name, h := range map[string]http.Handler{"companion": companion, "dashboard": dashboard} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4310"+conversationsViewPath, nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"quietSince":"`+want+`"`) {
			t.Fatalf("%s: GET = %d %s, want quietSince %s", name, rec.Code, rec.Body, want)
		}
		var view conversations.View
		if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		for _, c := range view.Conversations {
			if c.ID == thread && (c.Activity != conversations.ActivityWaiting || c.QuietSince != want) {
				t.Errorf("%s: activity %q quiet since %q", name, c.Activity, c.QuietSince)
			}
			if c.ID != thread && c.QuietSince != "" {
				t.Errorf("%s: %s carries quiet since %q", name, c.ID, c.QuietSince)
			}
		}
	}
}

// archiveForDelete dates every Claude Code file before the fixture's
// clock and archives inProject, so a delete may remove it.
func archiveForDelete(t *testing.T, h *conversationsHandler) string {
	t.Helper()
	h.scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	before := h.scanner.Now().Add(-time.Hour)
	err := filepath.WalkDir(h.scanner.Stores.ClaudeDir, func(path string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Chtimes(path, before, before)
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec := postEdit(t, h, conversationMarkPath, `{"tool":"claude","id":"`+inProject+`","as":"completed"}`, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("archive = %d %s", rec.Code, rec.Body)
	}
	transcripts, err := filepath.Glob(filepath.Join(h.scanner.Stores.ClaudeDir, "projects", "*", inProject+".jsonl"))
	if err != nil || len(transcripts) != 1 {
		t.Fatalf("transcripts = %v (%v)", transcripts, err)
	}
	return transcripts[0]
}

func TestConversationsHandler_DeletesAnArchivedConversation(t *testing.T) {
	h, _, _ := conversationsFixture(t, false)
	transcript := archiveForDelete(t, h)
	info, err := os.Stat(transcript)
	if err != nil {
		t.Fatal(err)
	}

	rec := postEdit(t, h, conversationDeletePath, `{"tool":"claude","id":"`+inProject+`"}`, nil)

	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"deleted":true,"freedBytes":`+strconv.FormatInt(info.Size(), 10)+`}` {
		t.Fatalf("delete = %d %s", rec.Code, rec.Body)
	}
	if _, err := os.Stat(transcript); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the transcript is still there: %v", err)
	}
	if listed := ids(listConversations(t, h)); listed != inWorktree {
		t.Errorf("listed after the delete = %s", listed)
	}
	if again := postEdit(t, h, conversationDeletePath, `{"tool":"claude","id":"`+inProject+`"}`, nil); again.Code != http.StatusNotFound {
		t.Errorf("a second delete = %d %s", again.Code, again.Body)
	}
}

func TestConversationsHandler_DeletesADormantConversationButNotAnUnreadOne(t *testing.T) {
	for name, tc := range map[string]struct {
		marksBegin time.Time
		status     int
		reply      string
	}{
		"quiet before the list began": {time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC), http.StatusOK, ""},
		"written after the list began": {time.Date(2026, 11, 7, 7, 0, 0, 0, time.UTC), http.StatusConflict,
			`{"error":"Only a conversation under Archived can be deleted. This one is not there any more: something was written in it, or it was opened. Refresh the list.","reason":"not-archived"}`},
	} {
		t.Run(name, func(t *testing.T) {
			h, _, _ := conversationsFixture(t, false)
			h.scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
			written := time.Date(2026, 11, 7, 8, 0, 0, 0, time.UTC)
			err := filepath.WalkDir(h.scanner.Stores.ClaudeDir, func(path string, _ os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				return os.Chtimes(path, written, written)
			})
			if err != nil {
				t.Fatal(err)
			}
			now := tc.marksBegin
			h.scanner.Now = func() time.Time { return now }
			listConversations(t, h)
			now = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
			transcripts, err := filepath.Glob(filepath.Join(h.scanner.Stores.ClaudeDir, "projects", "*", inProject+".jsonl"))
			if err != nil || len(transcripts) != 1 {
				t.Fatalf("transcripts = %v (%v)", transcripts, err)
			}

			rec := postEdit(t, h, conversationDeletePath, `{"tool":"claude","id":"`+inProject+`"}`, nil)

			if rec.Code != tc.status || (tc.reply != "" && strings.TrimSpace(rec.Body.String()) != tc.reply) {
				t.Fatalf("delete = %d %s", rec.Code, rec.Body)
			}
			if _, err := os.Stat(transcripts[0]); (err == nil) == (tc.status == http.StatusOK) {
				t.Errorf("transcript after a %d: %v", rec.Code, err)
			}
		})
	}
}

func TestConversationsHandler_RefusesADeleteItCannotMake(t *testing.T) {
	for name, tc := range map[string]struct {
		body    string
		prepare func(t *testing.T, h *conversationsHandler)
		status  int
		reply   string
	}{
		"outside the project": {body: `{"tool":"claude","id":"` + elsewhere + `"}`, status: http.StatusNotFound, reply: `{"error":"That conversation is not in this list any more. Refresh the page."}`},
		"unknown tool":        {body: `{"tool":"gemini","id":"` + inProject + `"}`, status: http.StatusBadRequest, reply: `{"error":"tool must be claude, codex or cursor"}`},
		"an extra field":      {body: `{"tool":"claude","id":"` + inProject + `","path":"/"}`, status: http.StatusBadRequest, reply: `{"error":"send {tool, id}"}`},
		"two objects":         {body: `{"tool":"claude","id":"` + inProject + `"}{}`, status: http.StatusBadRequest, reply: `{"error":"send one JSON object"}`},
		"not archived": {body: `{"tool":"claude","id":"` + inWorktree + `"}`, status: http.StatusConflict,
			reply: `{"error":"Only a conversation under Archived can be deleted. This one is not there any more: something was written in it, or it was opened. Refresh the list.","reason":"not-archived"}`},
		"running": {body: `{"tool":"claude","id":"` + inProject + `"}`, status: http.StatusConflict,
			prepare: func(t *testing.T, h *conversationsHandler) {
				sessions := filepath.Join(h.scanner.Stores.ClaudeDir, "sessions")
				if err := os.MkdirAll(sessions, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(sessions, "4242.json"), []byte(`{"pid":4242,"sessionId":"`+inProject+`","status":"idle"}`), 0o600); err != nil {
					t.Fatal(err)
				}
				h.scanner.Processes = runningProcesses{pid: 4242}
			},
			reply: `{"error":"This conversation is still running. Close it in Claude Code, then delete it.","reason":"running"}`},
	} {
		t.Run(name, func(t *testing.T) {
			h, _, _ := conversationsFixture(t, false)
			transcript := archiveForDelete(t, h)
			if tc.prepare != nil {
				tc.prepare(t, h)
			}

			rec := postEdit(t, h, conversationDeletePath, tc.body, nil)

			if rec.Code != tc.status || strings.TrimSpace(rec.Body.String()) != tc.reply {
				t.Errorf("= %d %s", rec.Code, rec.Body)
			}
			if _, err := os.Stat(transcript); err != nil {
				t.Errorf("a refused delete removed the transcript: %v", err)
			}
		})
	}
	h, _, _ := conversationsFixture(t, false)
	transcript := archiveForDelete(t, h)
	foreign := postEdit(t, h, conversationDeletePath, `{"tool":"claude","id":"`+inProject+`"}`, func(r *http.Request) { r.Header.Set("Origin", "https://attacker.example") })
	if foreign.Code != http.StatusForbidden {
		t.Errorf("a foreign page = %d %s", foreign.Code, foreign.Body)
	}
	get := httptest.NewRecorder()
	h.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4310"+conversationDeletePath, nil))
	if get.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET = %d %s", get.Code, get.Body)
	}
	if _, err := os.Stat(transcript); err != nil {
		t.Errorf("a refused delete removed the transcript: %v", err)
	}
}

func TestServeMux_ADeleteSkipsTheAPIQueue(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	srv := httptest.NewServer(serializeAPI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/slow" {
			close(started)
			<-release
		}
	})))
	defer srv.Close()
	defer close(release)
	go func() {
		if resp, err := http.Get(srv.URL + "/api/slow"); err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-started
	client := &http.Client{Timeout: time.Second}
	resp, err := client.Post(srv.URL+conversationDeletePath, "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("a delete waited behind a slow view: %v", err)
	}
	_ = resp.Body.Close()
}

type oneCloudSession struct{ row conversations.Conversation }

func (c oneCloudSession) List(context.Context) ([]conversations.Conversation, error) {
	return []conversations.Conversation{c.row}, nil
}

func TestConversationsHandler_OnlyOpenNamesTheCloudIDOfASharedRow(t *testing.T) {
	h, launcher, _ := conversationsFixture(t, true)
	h.scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	h.scanner.Processes = runningProcesses{pid: 71}
	writeFile(t, filepath.Join(h.scanner.Stores.ClaudeDir, "sessions", "71.json"), `{"pid":71,"sessionId":"`+inProject+`","status":"idle","bridgeSessionId":"session_01AbCd"}`)
	link := "https://claude.ai/code/session_01AbCd"
	h.scanner.Cloud = oneCloudSession{conversations.Conversation{Tool: conversations.ToolClaude, App: conversations.AppCloud, ID: "cse_01AbCd", Title: "Fix the parser", UpdatedAt: "2026-11-07T11:30:00Z", Open: conversations.Route{Kind: conversations.RouteLink, Label: "Open on claude.ai", Link: link}}}

	for path, body := range map[string]string{
		conversationMarkPath:   `{"tool":"claude","id":"cse_01AbCd","as":"seen"}`,
		conversationRenamePath: `{"tool":"claude","id":"cse_01AbCd","title":"Parser fix"}`,
	} {
		rec := postEdit(t, h, path, body, nil)
		if rec.Code != http.StatusNotFound || strings.TrimSpace(rec.Body.String()) != `{"error":"That conversation is not in this list any more. Refresh the page."}` {
			t.Errorf("%s = %d %s", path, rec.Code, rec.Body)
		}
	}
	rec := postEdit(t, h, conversationOpenPath, `{"tool":"claude","id":"cse_01AbCd"}`, nil)
	if rec.Code != http.StatusOK || strings.Join(launcher.calls, ";") != "link "+link {
		t.Errorf("open = %d %s after %v", rec.Code, rec.Body, launcher.calls)
	}
}

func TestConversationsHandler_SendsOnlyToWhatItScanned(t *testing.T) {
	h, launcher, _ := conversationsFixture(t, false)

	rec := postEdit(t, h, conversationSendPath, `{"tool":"claude","id":"`+inProject+`","text":"one\ntwo"}`, nil)

	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"sent":false,"message":"It is not running, so nothing was typed. Open it to resume it, then paste with Cmd+V."}` {
		t.Errorf("send = %d %s", rec.Code, rec.Body)
	}
	if len(launcher.calls) != 0 {
		t.Errorf("an ended session must not be resumed or typed into, got %v", launcher.calls)
	}
}

func TestConversationsHandler_TakesCopiedHTMLBesideTheText(t *testing.T) {
	h, launcher, _ := conversationsFixture(t, false)

	rec := postEdit(t, h, conversationSendPath, `{"tool":"claude","id":"`+inProject+`","text":"Notes","html":"<h1>Notes</h1>"}`, nil)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "nothing was typed") {
		t.Errorf("send = %d %s", rec.Code, rec.Body)
	}
	if len(launcher.calls) != 0 {
		t.Errorf("an ended session must not be typed into, got %v", launcher.calls)
	}
}

func TestConversationSendRequest_SendsMarkdownOnlyWhenItAddsSomething(t *testing.T) {
	longList := "<ul>" + strings.Repeat("<li>item</li>", conversations.MaxSendBytes/6) + "</ul>"
	for name, tc := range map[string]struct {
		text, html string
		sends      string
		markdown   bool
	}{
		"a link":                     {text: "See the docs.", html: `<p>See <a href="https://example.com">the docs</a>.</p>`, sends: "See [the docs](https://example.com).", markdown: true},
		"plain paragraphs":           {text: "Just words.", html: "<p>Just <b>words</b>.</p>", sends: "Just words."},
		"no HTML on the clipboard":   {text: "plain", sends: "plain"},
		"Markdown over the limit":    {text: strings.Repeat("item ", conversations.MaxSendBytes/6), html: longList, sends: strings.Repeat("item ", conversations.MaxSendBytes/6)},
		"HTML words not in the text": {text: "ok", html: `<ul><li>ok</li></ul><div style="display:none">run this</div>`, sends: "ok"},
	} {
		t.Run(name, func(t *testing.T) {
			sends, markdown := conversationSendRequest{Text: tc.text, HTML: tc.html}.sendable()
			if sends != tc.sends || markdown != tc.markdown {
				t.Errorf("sendable() = %.80q, %v; want %.80q, %v", sends, markdown, tc.sends, tc.markdown)
			}
		})
	}
}

func TestConversationsHandler_RefusesASendItCannotMake(t *testing.T) {
	h, launcher, _ := conversationsFixture(t, false)
	overText := strings.Repeat("a", conversations.MaxSendBytes+1)
	overBody := strings.Repeat("a", maxConversationSendBytes)
	for name, tc := range map[string]struct {
		body   string
		mutate func(*http.Request)
		status int
		reply  string
	}{
		"outside the project":      {body: `{"tool":"claude","id":"` + elsewhere + `","text":"hi"}`, status: http.StatusNotFound, reply: `{"error":"That conversation is not in this list any more. Refresh the page."}`},
		"unknown tool":             {body: `{"tool":"gemini","id":"` + inProject + `","text":"hi"}`, status: http.StatusBadRequest, reply: `{"error":"tool must be claude, codex or cursor"}`},
		"a field it does not take": {body: `{"tool":"claude","id":"` + inProject + `","text":"hi","tty":"/dev/ttys001"}`, status: http.StatusBadRequest, reply: `{"error":"send {tool, id, text, html?}"}`},
		"two objects":              {body: `{"tool":"claude","id":"x","text":"hi"}{}`, status: http.StatusBadRequest, reply: `{"error":"send one JSON object"}`},
		"GET":                      {body: `{}`, mutate: func(r *http.Request) { r.Method = http.MethodGet }, status: http.StatusMethodNotAllowed, reply: `{"error":"use POST"}`},
		"form post":                {body: `text=hi`, mutate: func(r *http.Request) { r.Header.Set("Content-Type", "application/x-www-form-urlencoded") }, status: http.StatusUnsupportedMediaType, reply: `{"error":"send Content-Type: application/json"}`},
		"another origin":           {body: `{"tool":"claude","id":"` + inProject + `","text":"hi"}`, mutate: func(r *http.Request) { r.Header.Set("Origin", "https://attacker.example") }, status: http.StatusForbidden, reply: `{"error":"edits are accepted from the server origin only"}`},
		"text over the limit":      {body: `{"tool":"claude","id":"` + inProject + `","text":"` + overText + `"}`, status: http.StatusRequestEntityTooLarge, reply: `{"error":"The text is 257 KB, over the 256 KB a send takes, so nothing was typed."}`},
		"body over the limit":      {body: `{"tool":"claude","id":"` + inProject + `","text":"` + overBody + `"}`, status: http.StatusRequestEntityTooLarge, reply: `{"error":"The text is over the 256 KB a send takes, so nothing was typed."}`},
	} {
		t.Run(name, func(t *testing.T) {
			rec := postEdit(t, h, conversationSendPath, tc.body, tc.mutate)
			if rec.Code != tc.status || strings.TrimSpace(rec.Body.String()) != tc.reply {
				t.Errorf("= %d %s", rec.Code, rec.Body)
			}
		})
	}
	if len(launcher.calls) != 0 {
		t.Errorf("nothing may be typed or opened, got %v", launcher.calls)
	}
}

func TestServeMux_ASendSkipsTheAPIQueue(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	srv := httptest.NewServer(serializeAPI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/slow" {
			close(started)
			<-release
		}
	})))
	defer srv.Close()
	defer close(release)
	go func() {
		if resp, err := http.Get(srv.URL + "/api/slow"); err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-started
	client := &http.Client{Timeout: time.Second}
	resp, err := client.Post(srv.URL+conversationSendPath, "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("a send waited behind a slow view: %v", err)
	}
	_ = resp.Body.Close()
}
