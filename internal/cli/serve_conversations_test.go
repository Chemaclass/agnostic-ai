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

func (stillProcesses) Alive(int, string) bool          { return false }
func (stillProcesses) TTY(int) (string, error)         { return "", errors.New("no terminal") }
func (stillProcesses) TerminalApp(int) (string, error) { return "", nil }
func (stillProcesses) Holders([]string) map[string]int { return nil }
func (stillProcesses) AppRunning(string) bool          { return false }

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
		processes: stillProcesses{},
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
