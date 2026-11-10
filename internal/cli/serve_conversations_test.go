package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/conversations"
)

type stillProcesses struct{}

func (stillProcesses) Alive(int, string) bool { return false }
func (stillProcesses) AliveAmong([]conversations.ProcessRef) map[int]bool {
	return nil
}
func (stillProcesses) TTY(int) (string, error)         { return "", errors.New("no terminal") }
func (stillProcesses) TerminalApp(int) (string, error) { return "", nil }
func (stillProcesses) Holders([]string) map[string]int { return nil }
func (stillProcesses) AppRunning(string) bool          { return false }

// runningProcesses has one process running, with every other answer of stillProcesses.
type runningProcesses struct {
	stillProcesses
	pid int
}

func (r runningProcesses) AliveAmong(processes []conversations.ProcessRef) map[int]bool {
	alive := map[int]bool{}
	for _, process := range processes {
		if process.PID == r.pid {
			alive[process.PID] = true
		}
	}
	return alive
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

func (l *recordingLauncher) FocusTerminalTab(context.Context, string) (bool, error) {
	return false, errors.New("unexpected focus")
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

func TestConversationsHandler_AnArchivedConversationThatWorksShowsWorkingOnEveryServer(t *testing.T) {
	companion, _, _ := conversationsFixture(t, true)
	companion.scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	companion.scanner.Processes = runningProcesses{pid: 4242}
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
