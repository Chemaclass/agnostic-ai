package conversations

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	idSafari = "11111111-1111-4111-8111-111111111111"
	idCache  = "22222222-2222-4222-8222-222222222222"
	idSecret = "33333333-3333-4333-8333-333333333333"
	idSide   = "44444444-4444-4444-8444-444444444444"
)

type fakeProcesses struct {
	alive   map[int]bool
	table   []ProcessInfo
	args    map[int]string
	cwds    map[int]string
	held    map[string]int
	running map[string]bool
}

func (f fakeProcesses) Holders(paths []string) map[string]int {
	held := map[string]int{}
	for _, path := range paths {
		if pid, ok := f.held[path]; ok {
			held[path] = pid
		} else if pid, ok := f.held[filepath.Base(path)]; ok {
			held[path] = pid
		}
	}
	return held
}

func (f fakeProcesses) AppRunning(name string) bool { return f.running[name] }

func (f fakeProcesses) Alive(pid int, _ string) bool { return f.alive[pid] }

func (f fakeProcesses) AliveAmong(processes []ProcessRef) map[int]bool {
	alive := map[int]bool{}
	for _, process := range processes {
		if f.alive[process.PID] {
			alive[process.PID] = true
		}
	}
	return alive
}

func (f fakeProcesses) Table() ([]ProcessInfo, error) { return f.table, nil }

func (f fakeProcesses) Args([]int) map[int]string { return f.args }

func (f fakeProcesses) Cwds([]int) map[int]string { return f.cwds }

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func lines(values ...any) string {
	var out []string
	for _, value := range values {
		data, err := json.Marshal(value)
		if err != nil {
			panic(err)
		}
		out = append(out, string(data))
	}
	return strings.Join(out, "\n") + "\n"
}

type obj = map[string]any

func noSQLite(context.Context, string, string) ([]json.RawMessage, error) { return nil, errNoSQLite }

func testScanner(t *testing.T, sqlite SQLiteQuery, processes Processes) (*Scanner, Stores) {
	t.Helper()
	home := t.TempDir()
	stores := Stores{
		Home:      home,
		ClaudeDir: filepath.Join(home, ".claude"),
		CodexDir:  filepath.Join(home, ".codex"),
		CursorDB:  filepath.Join(home, "Cursor", "state.vscdb"),
		CursorDir: filepath.Join(home, ".cursor"),
	}
	now := func() time.Time { return time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC) }
	return &Scanner{Stores: stores, SQLite: sqlite, Processes: processes, Now: now}, stores
}

func asJSON(t *testing.T, value any) string {
	t.Helper()
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// inJSON writes value as asJSON does inside a string, so a Windows
// path's backslashes come out escaped.
func inJSON(t *testing.T, value string) string {
	t.Helper()
	return strings.Trim(asJSON(t, value), `"`)
}

func byID(t *testing.T, view View, id string) Conversation {
	t.Helper()
	for _, c := range view.Conversations {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("no conversation %s in %s", id, asJSON(t, view))
	return Conversation{}
}

func writeClaudeSessions(t *testing.T, stores Stores) {
	project := filepath.Join(stores.ClaudeDir, "projects", "-tmp-app")
	write(t, filepath.Join(project, idSafari+".jsonl"), lines(
		obj{"type": "user", "cwd": "/tmp/app", "gitBranch": "main", "entrypoint": "cli", "timestamp": "2026-11-07T08:00:00.000Z",
			"message": obj{"role": "user", "content": "<ide_opened_file>The user opened x.go</ide_opened_file>\nFix the login bug\nIt fails on Safari\n\nSee the logs\nA fourth line"}},
		obj{"type": "assistant", "timestamp": "2026-11-07T08:00:05.000Z", "message": obj{"role": "assistant", "content": []obj{{"type": "text", "text": "On it."}}}},
		obj{"type": "user", "timestamp": "2026-11-07T08:00:09.000Z", "message": obj{"role": "user", "content": []obj{{"type": "tool_result", "content": "ok"}}}},
		obj{"type": "ai-title", "aiTitle": "Fix Safari login"},
		obj{"type": "custom-title", "customTitle": "Safari login bug"},
		obj{"type": "last-prompt", "lastPrompt": "Now add a test for it"},
		obj{"type": "user", "cwd": "/tmp/app", "gitBranch": "fix/safari", "entrypoint": "cli", "timestamp": "2026-11-07T09:30:00.000Z",
			"message": obj{"role": "user", "content": "Now add a test for it"}},
	))
	write(t, filepath.Join(project, idCache+".jsonl"), lines(
		obj{"type": "user", "cwd": "/tmp/app", "entrypoint": "claude-vscode", "timestamp": "2026-11-06T10:00:00.000Z",
			"message": obj{"role": "user", "content": []obj{{"type": "text", "text": "Explain the cache"}}}},
		obj{"type": "ai-title", "aiTitle": "Cache explainer"},
	))
	write(t, filepath.Join(project, idSecret+".jsonl"), lines(
		obj{"type": "user", "cwd": "/tmp/app", "entrypoint": "cli", "timestamp": "2026-11-05T10:00:00.000Z",
			"message": obj{"role": "user", "content": "Use password hunter2 for the staging login"}},
	))
	write(t, filepath.Join(project, idSide+".jsonl"), lines(
		obj{"type": "user", "isSidechain": true, "cwd": "/tmp/app", "timestamp": "2026-11-07T11:00:00.000Z", "message": obj{"role": "user", "content": "Subagent task"}},
	))
	write(t, filepath.Join(project, "notes.jsonl"), lines(obj{"type": "user", "message": obj{"role": "user", "content": "Not a session"}}))
	write(t, filepath.Join(project, idSafari, "subagents", "agent-1.jsonl"), lines(obj{"type": "user", "isSidechain": true, "message": obj{"role": "user", "content": "Nested"}}))
	write(t, filepath.Join(stores.ClaudeDir, "sessions", "4242.json"), `{"pid":4242,"sessionId":"`+idSafari+`","procStart":"Sat Nov  7 08:00:00 2026"}`)
	write(t, filepath.Join(stores.ClaudeDir, "sessions", "999.json"), `{"pid":999,"sessionId":"`+idCache+`"}`)
	write(t, filepath.Join(stores.ClaudeDir, "sessions", "4242.feedface.key"), "never read")
}

func TestScan_ListsClaudeSessionsWithTitlesPromptsAndRoutes(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{alive: map[int]bool{4242: true}})
	writeClaudeSessions(t, stores)

	view := scanner.Scan(context.Background(), ScanOptions{Editor: "vscode"})

	if actual := asJSON(t, view.Tools); actual != `[
  {
    "tool": "claude",
    "label": "Claude Code",
    "found": true,
    "count": 3,
    "shown": 3
  },
  {
    "tool": "codex",
    "label": "Codex",
    "found": false,
    "count": 0,
    "shown": 0,
    "note": "No Codex thread index in `+inJSON(t, stores.CodexDir)+`"
  },
  {
    "tool": "cursor",
    "label": "Cursor",
    "found": false,
    "count": 0,
    "shown": 0,
    "note": "No Cursor chat index at `+inJSON(t, stores.CursorDB)+`"
  }
]` {
		t.Errorf("tools =\n%s", actual)
	}
	if actual := asJSON(t, byID(t, view, idSafari)); actual != `{
  "tool": "claude",
  "app": "cli",
  "id": "`+idSafari+`",
  "title": "Safari login bug",
  "folder": "/tmp/app",
  "branch": "fix/safari",
  "startedAt": "2026-11-07T08:00:00Z",
  "updatedAt": "2026-11-07T09:30:00Z",
  "opening": [
    "Fix the login bug",
    "It fails on Safari",
    "See the logs"
  ],
  "prompt": "Now add a test for it",
  "live": true,
  "state": "active",
  "open": {
    "kind": "focus",
    "label": "Open in Terminal",
    "command": "cd '/tmp/app' && claude --resume `+idSafari+`"
  }
}` {
		t.Errorf("live CLI session =\n%s", actual)
	}
	if actual := asJSON(t, byID(t, view, idCache).Open); actual != `{
  "kind": "link",
  "label": "Open in VS Code",
  "link": "vscode://anthropic.claude-code/open?session=`+idCache+`"
}` {
		t.Errorf("editor session route =\n%s", actual)
	}
	secret := byID(t, view, idSecret)
	if secret.Title != "" || secret.Opening != nil || strings.Join(secret.Withheld, ",") != "title,opening" {
		t.Errorf("a prompt naming a password must be withheld, actual title %q opening %q withheld %v", secret.Title, secret.Opening, secret.Withheld)
	}
	if want := []string{idSafari, idCache, idSecret}; len(view.Conversations) != 3 || view.Conversations[0].ID != want[0] || view.Conversations[2].ID != want[2] {
		t.Errorf("want newest first %v, actual %s", want, asJSON(t, view.Conversations))
	}
}

func TestScan_ReadsTheTitleAtTheEndOfALargeSession(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	var b strings.Builder
	b.WriteString(lines(obj{"type": "user", "cwd": "/tmp/big", "entrypoint": "cli", "timestamp": "2026-11-01T08:00:00.000Z", "message": obj{"role": "user", "content": "Start the migration"}}))
	filler := lines(obj{"type": "assistant", "message": obj{"role": "assistant", "content": []obj{{"type": "text", "text": strings.Repeat("x", 2000)}}}})
	for b.Len() < 3*claudeWindow {
		b.WriteString(filler)
	}
	b.WriteString(lines(obj{"type": "ai-title", "aiTitle": "Database migration"}, obj{"type": "last-prompt", "lastPrompt": "Ship it"}))
	write(t, filepath.Join(stores.ClaudeDir, "projects", "-tmp-big", idSafari+".jsonl"), b.String())

	c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), idSafari)

	if c.Title != "Database migration" || strings.Join(c.Opening, "|") != "Start the migration" || c.Prompt != "Ship it" {
		t.Errorf("large session = title %q opening %q prompt %q", c.Title, c.Opening, c.Prompt)
	}
}

func TestScan_ReusesParsedSessionsUntilTheFileChanges(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	file := filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", idCache+".jsonl")
	write(t, file, lines(obj{"type": "user", "cwd": "/tmp/app", "timestamp": "2026-11-06T10:00:00.000Z", "message": obj{"role": "user", "content": "First"}}))
	if c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), idCache); c.Title != "First" {
		t.Fatalf("title = %q", c.Title)
	}
	write(t, file, lines(obj{"type": "user", "cwd": "/tmp/app", "timestamp": "2026-11-06T10:00:00.000Z", "message": obj{"role": "user", "content": "Second, longer"}}))
	if c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), idCache); c.Title != "Second, longer" {
		t.Errorf("a changed file must be read again, title = %q", c.Title)
	}
}

func TestScan_RoutesAClaudeBackgroundJobToAttach(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	write(t, filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", idCache+".jsonl"), lines(
		obj{"type": "user", "cwd": "/tmp/app", "entrypoint": "cli", "timestamp": "2026-11-06T10:00:00.000Z", "message": obj{"role": "user", "content": "Run the suite"}}))
	write(t, filepath.Join(stores.ClaudeDir, "jobs", "5f12e206", "state.json"), `{"sessionId":"`+idCache+`","state":"running"}`)
	write(t, filepath.Join(stores.ClaudeDir, "jobs", "0ld0ld00", "state.json"), `{"sessionId":"`+idSafari+`","state":"done"}`)

	c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), idCache)

	if c.App != AppBackground || c.Open.Kind != RouteTerminal || c.Open.Command != "claude attach 5f12e206" {
		t.Errorf("background job route = %s %s", c.App, asJSON(t, c.Open))
	}
}

// fakeStores answers queries by database file name and a word of the query.
func fakeStores(t *testing.T, answers map[string]string) SQLiteQuery {
	return func(_ context.Context, path, query string) ([]json.RawMessage, error) {
		for key, rows := range answers {
			file, word, _ := strings.Cut(key, "|")
			if filepath.Base(path) == file && strings.Contains(query, word) {
				var out []json.RawMessage
				if err := json.Unmarshal([]byte(rows), &out); err != nil {
					t.Fatalf("fixture %s: %v", key, err)
				}
				return out, nil
			}
		}
		return nil, errors.New("unexpected query on " + filepath.Base(path))
	}
}

func TestScan_ListsCodexThreadsFromTheNewestIndex(t *testing.T) {
	desktop, editor, terminal := "55555555-5555-4555-8555-555555555555", "66666666-6666-4666-8666-666666666666", "77777777-7777-4777-8777-777777777777"
	scanner, stores := testScanner(t, fakeStores(t, map[string]string{
		"state_5.sqlite|FROM threads": `[
			{"id":"` + desktop + `","cwd":"/tmp/app","created_at_ms":1762500000000,"updated_at_ms":1762520000000,"originator":"Codex Desktop","source":"vscode","git_branch":"main","name":"Plan the release","title":"","first_user_message":"Plan the release notes"},
			{"id":"` + editor + `","cwd":"/tmp/app","created_at_ms":1762400000000,"updated_at_ms":1762510000000,"originator":"codex_vscode","source":"vscode","git_branch":"","name":"","title":"Fix flaky test","first_user_message":"Fix the flaky test"},
			{"id":"` + terminal + `","cwd":"/tmp/it's","created_at_ms":1762300000000,"updated_at_ms":1762300000000,"originator":"codex-tui","source":"cli","git_branch":"","name":"","title":"","first_user_message":"<environment_context>cwd</environment_context>Refactor the parser"}
		]`,
		"thread_history_1.sqlite|thread_items": `[{"thread_id":"` + editor + `","content":"[{\"type\":\"text\",\"text\":\"Run it again\",\"text_elements\":[]}]"}]`,
	}), fakeProcesses{})
	write(t, filepath.Join(stores.CodexDir, "state_4.sqlite"), "")
	write(t, filepath.Join(stores.CodexDir, "state_5.sqlite"), "")
	write(t, filepath.Join(stores.CodexDir, "thread_history_1.sqlite"), "")
	write(t, filepath.Join(stores.Home, ".cursor", "extensions", "openai.chatgpt-26.1021.22027", "package.json"), "{}")

	view := scanner.Scan(context.Background(), ScanOptions{Editor: "vscode"})

	if actual := asJSON(t, []Route{byID(t, view, desktop).Open, byID(t, view, editor).Open, byID(t, view, terminal).Open}); actual != `[
  {
    "kind": "link",
    "label": "Open in Codex app",
    "link": "codex://threads/`+desktop+`"
  },
  {
    "kind": "link",
    "label": "Open in Cursor",
    "link": "cursor://openai.chatgpt/local/`+editor+`"
  },
  {
    "kind": "terminal",
    "label": "Open in Terminal",
    "command": "cd '/tmp/it'\\''s' && codex resume `+terminal+`"
  }
]` {
		t.Errorf("Codex routes (the extension is only in Cursor, so Cursor opens it) =\n%s", actual)
	}
	if c := byID(t, view, editor); c.Title != "Fix flaky test" || c.Prompt != "Run it again" || c.UpdatedAt != "2025-11-07T10:06:40Z" {
		t.Errorf("editor thread = title %q prompt %q updated %s", c.Title, c.Prompt, c.UpdatedAt)
	}
	if c := byID(t, view, terminal); c.Title != "Refactor the parser" {
		t.Errorf("injected context must not reach the title, actual %q", c.Title)
	}
}

func TestScan_ListsCursorChatsInTheProjectWithTheirPrompts(t *testing.T) {
	inApp, elsewhere, agent := "88888888-8888-4888-8888-888888888888", "99999999-9999-4999-8999-999999999999", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	meta := hex.EncodeToString([]byte(`{"agentId":"` + agent + `","name":"New Agent","createdAt":1762500000000}`))
	scanner, stores := testScanner(t, fakeStores(t, map[string]string{
		"state.vscdb|FROM composerHeaders": `[
			{"id":"` + inApp + `","created_at":1762500000000,"recency":1762600000000,"name":"Parser refactor","folder":"/tmp/app/src","branch":"main"},
			{"id":"` + elsewhere + `","created_at":1762500000000,"recency":1762700000000,"name":"Other repo","folder":"/tmp/other","branch":""}
		]`,
		"state.vscdb|chosen(composerId)": `[{"id":"` + inApp + `","first_text":"Split the parser\nKeep the API","last_text":"<timestamp>Saturday, Nov 8, 2025</timestamp><system_notification><task>build done</task></system_notification><user_query>Add benchmarks</user_query>"}]`,
		"store.db|FROM meta":             `[{"value":"` + meta + `"}]`,
	}), fakeProcesses{})
	write(t, stores.CursorDB, "")
	write(t, filepath.Join(stores.CursorDir, "chats", md5Hex("/tmp/app"), agent, "store.db"), "")
	write(t, filepath.Join(stores.CursorDir, "projects", "p1", "agent-transcripts", agent, agent+".jsonl"), lines(
		obj{"role": "user", "message": obj{"content": []obj{{"type": "text", "text": "<user_info>os</user_info>"}}}},
		obj{"role": "user", "message": obj{"content": []obj{{"type": "text", "text": "<manually_attached_skills>\nreview\n</manually_attached_skills>\n<timestamp>Friday, Nov 7, 2025, 8:00 AM</timestamp>\n<user_query>Fix the CI job</user_query>"}}}},
		obj{"role": "user", "message": obj{"content": []obj{{"type": "text", "text": "<timestamp>Friday, Nov 7, 2025, 8:05 AM</timestamp>\n<user_query>Run it again</user_query>"}}}},
	))

	scanner.Now = func() time.Time { return time.UnixMilli(1762800000000) }

	view := scanner.Scan(context.Background(), ScanOptions{Within: []string{"/tmp/app"}})

	cursor := view.Tools[2]
	if cursor.Count != 2 || cursor.Shown != 2 {
		t.Errorf("Cursor source = %s", asJSON(t, cursor))
	}
	if actual := asJSON(t, byID(t, view, inApp)); actual != `{
  "tool": "cursor",
  "app": "ide",
  "id": "`+inApp+`",
  "title": "Parser refactor",
  "folder": "/tmp/app/src",
  "branch": "main",
  "startedAt": "2025-11-07T07:20:00Z",
  "updatedAt": "2025-11-08T11:06:40Z",
  "opening": [
    "Split the parser",
    "Keep the API"
  ],
  "prompt": "Add benchmarks",
  "state": "dormant",
  "deleteUnsupported": "Cursor keeps every chat in one shared database and has no command to delete one. Delete it in Cursor.",
  "open": {
    "kind": "link",
    "label": "Open in Cursor",
    "link": "cursor://anysphere.cursor-deeplink/agent?id=`+inApp+`"
  }
}` {
		t.Errorf("editor chat =\n%s", actual)
	}
	if c := byID(t, view, agent); c.App != AppCLI || c.Title != "Fix the CI job" || c.Prompt != "Run it again" || c.Open.Command != "cd '/tmp/app' && cursor-agent --resume "+agent {
		t.Errorf("cursor-agent chat = %s", asJSON(t, c))
	}
	for query, want := range map[string]int{"ci job": 1, "friday": 0, "build done": 0} {
		found := scanner.Scan(context.Background(), ScanOptions{Within: []string{"/tmp/app"}, Query: query})
		if found.Matches == nil || *found.Matches != want {
			t.Errorf("search %q matched %s, want %d", query, asJSON(t, found.Matches), want)
		}
	}
}

func TestCursorTyped_KeepsOnlyWhatWasTyped(t *testing.T) {
	for in, want := range map[string]string{
		"<timestamp>Mon</timestamp>\n<user_query>Fix it</user_query>":                                "Fix it",
		"<external_links>x</external_links><user_query>One</user_query><user_query>Two</user_query>": "One\nTwo",
		"Plain text with no query": "Plain text with no query",
	} {
		if actual := cursorTyped(in); actual != want {
			t.Errorf("cursorTyped(%q) = %q, want %q", in, actual, want)
		}
	}
}

func TestScan_SaysWhenSQLiteIsMissing(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	write(t, filepath.Join(stores.CodexDir, "state_5.sqlite"), "")
	write(t, stores.CursorDB, "")

	view := scanner.Scan(context.Background(), ScanOptions{})

	if view.Tools[1].Note != "Reading Codex threads needs the sqlite3 command" || view.Tools[2].Note != "Reading Cursor chats needs the sqlite3 command" {
		t.Errorf("tools = %s", asJSON(t, view.Tools))
	}
}

type fakeLauncher struct {
	calls     []string
	err       error
	folderErr error
	newTabErr error
	focusOK   bool
}

func (f *fakeLauncher) OpenFolder(_ context.Context, app, folder string) error {
	f.calls = append(f.calls, "folder "+app+" "+folder)
	return f.folderErr
}

func (f *fakeLauncher) OpenLink(_ context.Context, link string) error {
	f.calls = append(f.calls, "link "+link)
	return f.err
}

func (f *fakeLauncher) ActivateApp(_ context.Context, app string) error {
	f.calls = append(f.calls, "activate "+app)
	return f.err
}

func (f *fakeLauncher) RunInNewTerminal(_ context.Context, command string) error {
	f.calls = append(f.calls, "terminal "+command)
	return f.err
}

func (f *fakeLauncher) FocusTab(_ context.Context, app, tty string) (bool, error) {
	f.calls = append(f.calls, "focus "+app+" "+tty)
	return f.focusOK, f.err
}

func (f *fakeLauncher) TitleTab(_ context.Context, app, tty, title string) (bool, error) {
	f.calls = append(f.calls, "title "+app+" "+tty+" "+title)
	return f.focusOK, f.err
}

func (f *fakeLauncher) RunInNewTab(_ context.Context, app, command string) error {
	f.calls = append(f.calls, "new tab "+app+" "+command)
	if f.newTabErr != nil {
		return f.newTabErr
	}
	return f.err
}

func TestOpen_FollowsEachRoute(t *testing.T) {
	for name, tc := range map[string]struct {
		conversation Conversation
		processes    fakeProcesses
		launcher     fakeLauncher
		reply        OpenReply
		calls        string
	}{
		"link": {
			conversation: Conversation{Open: Route{Kind: RouteLink, Label: "Open in Cursor", Link: "cursor://x"}},
			reply:        OpenReply{Opened: true, Message: "Opened in Cursor."},
			calls:        "link cursor://x",
		},
		"editor extension, folder first": {
			conversation: Conversation{Folder: "/tmp/app", editor: &editorApps[0], Open: Route{Kind: RouteLink, Label: "Open in Cursor", Link: "cursor://anthropic.claude-code/open?session=x"}},
			reply:        OpenReply{Opened: true, Message: "Opened in Cursor."},
			calls:        "folder Cursor /tmp/app; link cursor://anthropic.claude-code/open?session=x",
		},
		"editor extension, folder gone": {
			conversation: Conversation{Folder: "/tmp/gone", editor: &editorApps[0], Open: Route{Kind: RouteLink, Label: "Open in Cursor", Link: "cursor://openai.chatgpt/local/x"}},
			launcher:     fakeLauncher{folderErr: errors.New("no such folder")},
			reply:        OpenReply{Opened: true, Message: "Opened in Cursor, but its folder /tmp/gone could not be opened, so it may show in another window."},
			calls:        "folder Cursor /tmp/gone; link cursor://openai.chatgpt/local/x",
		},
		"new terminal": {
			conversation: Conversation{Open: Route{Kind: RouteTerminal, Command: "codex resume x"}},
			reply:        OpenReply{Opened: true, Message: "Resumed in a new Terminal window."},
			calls:        "terminal codex resume x",
		},
		"Automation not allowed": {
			conversation: Conversation{Open: Route{Kind: RouteTerminal, Command: "codex resume x"}},
			launcher:     fakeLauncher{err: errAutomationDenied},
			reply:        OpenReply{Message: "macOS blocked opening Terminal. Allow agnostic-ai companion, or the terminal that runs agnostic-ai serve, to control Terminal in System Settings › Privacy & Security › Automation, or run this command yourself.", Command: "codex resume x"},
			calls:        "terminal codex resume x",
		},
		"no terminal on this system": {
			conversation: Conversation{Open: Route{Kind: RouteTerminal, Command: "codex resume x"}},
			launcher:     fakeLauncher{err: errNotHere},
			reply:        OpenReply{Message: "This system cannot open a terminal from the page. Run this command in a terminal.", Command: "codex resume x"},
			calls:        "terminal codex resume x",
		},
	} {
		t.Run(name, func(t *testing.T) {
			launcher := tc.launcher
			reply := Open(context.Background(), tc.conversation, tc.processes, &launcher)
			if reply != tc.reply {
				t.Errorf("reply = %+v, want %+v", reply, tc.reply)
			}
			if actual := strings.Join(launcher.calls, "; "); actual != tc.calls {
				t.Errorf("calls = %q, want %q", actual, tc.calls)
			}
		})
	}
}

func TestCheckedLink_OpensOnlyToolSchemes(t *testing.T) {
	for _, link := range []string{"cursor://anysphere.cursor-deeplink/agent?id=x", "codex://threads/x", "claude://resume?session=x", "vscode://anthropic.claude-code/open?session=x"} {
		if err := checkedLink(link); err != nil {
			t.Errorf("%s: %v", link, err)
		}
	}
	for _, link := range []string{"https://example.com", "file:///etc/passwd", "javascript:alert(1)", "x-apple.systempreferences:"} {
		if err := checkedLink(link); err == nil {
			t.Errorf("%s must be refused", link)
		}
	}
}

func TestTypedText_DropsInjectedBlocksAndKeepsWhatWasTyped(t *testing.T) {
	for in, want := range map[string]string{
		"<system-reminder>be brief</system-reminder>\nHello   there":                  "Hello there",
		"<user_query>\nFix the CI job\n</user_query>":                                 "Fix the CI job",
		"<timestamp>2026-01-01</timestamp> is rejected by the parser":                 "<timestamp>2026-01-01</timestamp> is rejected by the parser",
		"<command-name>/review</command-name>\n<command-args>PR 12</command-args>":    "/review\nPR 12",
		"<ide_selection lines=\"1-3\">secret code</ide_selection>Why does this fail?": "Why does this fail?",
	} {
		if actual := typedText(in); actual != want {
			t.Errorf("typedText(%q) = %q, want %q", in, actual, want)
		}
	}
}

func TestSQLiteURI_ReadsTheWALOnlyWhenOneExists(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "Application Support", "state.vscdb")
	write(t, db, "")
	if actual := sqliteURI(db); actual != "file://"+uriPath(dir)+"/Application%20Support/state.vscdb?immutable=1" {
		t.Errorf("without a WAL = %s", actual)
	}
	write(t, db+"-wal", "")
	if actual := sqliteURI(db); !strings.HasSuffix(actual, "?mode=ro") {
		t.Errorf("with a WAL = %s", actual)
	}
}

func TestRunSQLite_ReadsRowsAsJSON(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 is not installed")
	}
	db := filepath.Join(t.TempDir(), "threads.sqlite")
	if out, err := exec.Command("sqlite3", db, "CREATE TABLE threads(id TEXT, n INTEGER); INSERT INTO threads VALUES ('a', 1), ('b', NULL);").CombinedOutput(); err != nil {
		t.Fatalf("create: %v %s", err, out)
	}
	type row struct {
		ID string `json:"id"`
		N  int    `json:"n"`
	}
	rows, err := queryRows[row](context.Background(), RunSQLite, db, "SELECT id, n FROM threads ORDER BY id")
	if err != nil || asJSON(t, rows) != "[\n  {\n    \"id\": \"a\",\n    \"n\": 1\n  },\n  {\n    \"id\": \"b\",\n    \"n\": 0\n  }\n]" {
		t.Errorf("rows = %s, %v", asJSON(t, rows), err)
	}
	if _, err := RunSQLite(context.Background(), db, "SELECT * FROM missing"); err == nil || !strings.Contains(err.Error(), "no such table") {
		t.Errorf("a bad query must fail with sqlite3's reason, actual %v", err)
	}
}

// folderOpenIn reports whether the editor state at storagePath lists a
// window on folder itself.
func folderOpenIn(storagePath, folder string) bool {
	return openWindowFolders(storagePath)[folderKey(folder)]
}

func TestFolderOpenIn_ReadsTheEditorsOpenWindows(t *testing.T) {
	storage := filepath.Join(t.TempDir(), "storage.json")
	write(t, storage, `{"windowsState":{"lastActiveWindow":{"folder":"file:///Users/me/Products/service"},"openedWindows":[{"folder":"file:///Users/me/Products/agnostic-ai"},{"folder":"file:///Users/me/My%20Code/app"}]}}`)
	for folder, want := range map[string]bool{
		"/Users/me/Products/agnostic-ai": true,
		"/Users/me/Products/service":     true,
		"/Users/me/My Code/app":          true,
		"/Users/me/Products/agnostic":    false,
		"/Users/me/Products":             false,
	} {
		if actual := folderOpenIn(storage, folder); actual != want {
			t.Errorf("folderOpenIn(%q) = %v, want %v", folder, actual, want)
		}
	}
	if folderOpenIn(filepath.Join(t.TempDir(), "missing.json"), "/Users/me/Products/agnostic-ai") {
		t.Error("an editor with no saved state has no open window")
	}
}

func TestScan_DatesByTheLastMessageAndKeepsTheStartFolder(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{alive: map[int]bool{77: true}})
	write(t, filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", idCache+".jsonl"), lines(
		obj{"type": "user", "cwd": "/tmp/app", "entrypoint": "cli", "timestamp": "2026-11-07T08:00:00.000Z", "message": obj{"role": "user", "content": "Start here"}},
		obj{"type": "assistant", "cwd": "/tmp/app/ui", "entrypoint": "cli", "timestamp": "2026-11-07T08:05:00.000Z", "message": obj{"role": "assistant", "content": []obj{{"type": "text", "text": "Done"}}}},
		obj{"type": "file-history-snapshot", "timestamp": "2026-11-07T11:00:00.000Z"},
	))
	write(t, filepath.Join(stores.ClaudeDir, "sessions", "77.json"), `{"pid":77,"sessionId":"`+idCache+`","status":"waiting"}`)

	c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), idCache)

	if c.Folder != "/tmp/app" || c.UpdatedAt != "2026-11-07T08:05:00Z" || c.Open.Command != "cd '/tmp/app' && claude --resume "+idCache {
		t.Errorf("want the start folder and the last message time, got folder %q updated %s command %q", c.Folder, c.UpdatedAt, c.Open.Command)
	}
	if !c.Live || c.Activity != ActivityWaiting || c.QuietSince != "" {
		t.Errorf("a live session that reports it waits for the person = live %v activity %q quiet since %q", c.Live, c.Activity, c.QuietSince)
	}
}

func TestScan_FindsTheFolderOfASessionWithHugeLinesAndLeavesSDKRunsOut(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	huge := strings.Repeat("diff line\n", 3*claudeWindow/10)
	// SDK sessions write cwd and entrypoint after the message, on lines
	// longer than the window the reader parses at each end.
	sessionWith := func(entrypoint string) string {
		return `{"type":"custom-title","customTitle":"Review chapters"}` + "\n" +
			`{"type":"user","message":{"role":"user","content":"` + huge + `"},"timestamp":"2026-11-07T08:00:00.000Z","entrypoint":"` + entrypoint + `","cwd":"/tmp/review","gitBranch":"feat/x"}` + "\n" +
			`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"` + huge + `"}]},"timestamp":"2026-11-07T08:01:00.000Z","entrypoint":"` + entrypoint + `","cwd":"/tmp/review"}` + "\n" +
			`{"type":"last-prompt","lastPrompt":"Group this diff"}` + "\n"
	}
	write(t, filepath.Join(stores.ClaudeDir, "projects", "-tmp-review", idSafari+".jsonl"), sessionWith("sdk-cli"))
	write(t, filepath.Join(stores.ClaudeDir, "projects", "-tmp-review", idCache+".jsonl"), sessionWith("cli"))

	view := scanner.Scan(context.Background(), ScanOptions{})

	if len(view.Conversations) != 1 {
		t.Fatalf("the SDK run must be left out, got %s", asJSON(t, view.Conversations))
	}
	if c := view.Conversations[0]; c.ID != idCache || c.Folder != "/tmp/review" || c.Branch != "feat/x" || c.App != AppCLI {
		t.Errorf("huge-line session = %s", asJSON(t, c))
	}
}

func TestScan_LeavesCodexExecRunsOutAndMarksLoadedThreadsLive(t *testing.T) {
	loaded, idle, exec := "55555555-5555-4555-8555-555555555555", "66666666-6666-4666-8666-666666666666", "77777777-7777-4777-8777-777777777777"
	scanner, stores := testScanner(t, fakeStores(t, map[string]string{
		"state_5.sqlite|FROM threads": `[
			{"id":"` + loaded + `","cwd":"/tmp/app","created_at_ms":1762500000000,"updated_at_ms":1762510000000,"originator":"codex-tui","source":"cli","name":"Loaded"},
			{"id":"` + idle + `","cwd":"/tmp/app","created_at_ms":1762500000000,"updated_at_ms":1762510000000,"originator":"codex-tui","source":"cli","name":"Closed"},
			{"id":"` + exec + `","cwd":"/tmp/app","created_at_ms":1762500000000,"updated_at_ms":1762510000000,"originator":"codex_exec","source":"exec","name":"Exec run"}
		]`,
	}), fakeProcesses{held: map[string]int{loaded + ".lock": 4242}, alive: map[int]bool{4242: true}, table: []ProcessInfo{
		{PID: 400, PPID: 1, Command: "/System/Applications/Utilities/Terminal.app/Contents/MacOS/Terminal"},
		{PID: 401, PPID: 400, TTY: "/dev/ttys004", Command: "/usr/bin/login"},
		{PID: 4242, PPID: 401, TTY: "/dev/ttys004", Command: "codex"},
	}})
	scanner.Now = func() time.Time { return time.UnixMilli(1762520000000) }
	write(t, filepath.Join(stores.CodexDir, "state_5.sqlite"), "")
	for _, id := range []string{loaded, idle} {
		write(t, filepath.Join(stores.CodexDir, "thread-writer-locks", id+".lock"), "")
	}

	view := scanner.Scan(context.Background(), ScanOptions{})

	if len(view.Conversations) != 2 || !byID(t, view, loaded).Live || byID(t, view, idle).Live {
		t.Errorf("want two threads, only the loaded one live: %s", asJSON(t, view.Conversations))
	}
	if open := byID(t, view, loaded).Open; open.Kind != RouteFocus || open.Label != "Switch to Terminal" {
		t.Errorf("an open CLI thread switches to its terminal, actual %s", asJSON(t, open))
	}
	launcher := &fakeLauncher{focusOK: true}
	reply := Open(context.Background(), byID(t, view, loaded), scanner.Processes, launcher)
	if strings.Join(launcher.calls, ";") != "focus Terminal /dev/ttys004" || !reply.Opened {
		t.Errorf("opening it must switch to its tab, not resume it again: calls %v, reply %s", launcher.calls, asJSON(t, reply))
	}
}

func TestScan_MarksTheCursorChatsOpenInAWindow(t *testing.T) {
	open, closed := "88888888-8888-4888-8888-888888888888", "99999999-9999-4999-8999-999999999999"
	scanner, stores := testScanner(t, fakeStores(t, map[string]string{
		"state.vscdb|FROM composerHeaders": `[
			{"id":"` + open + `","workspace_id":"ws1","created_at":1762500000000,"recency":1762600000000,"name":"Open chat","folder":"/tmp/my app","branch":""},
			{"id":"` + closed + `","workspace_id":"ws1","created_at":1762500000000,"recency":1762500000000,"name":"Older chat","folder":"/tmp/my app","branch":""}
		]`,
		"state.vscdb|chosen(composerId)": `[]`,
		"state.vscdb|ItemTable":          `[{"value":"{\"selectedComposerIds\":[\"` + open + `\"],\"hasMigratedComposerData\":true}"}]`,
	}), fakeProcesses{running: map[string]bool{"Cursor": true}})
	write(t, stores.CursorDB, "")
	write(t, filepath.Join(filepath.Dir(stores.CursorDB), "storage.json"), `{"windowsState":{"openedWindows":[{"folder":"file:///tmp/my%20app"}]}}`)

	view := scanner.Scan(context.Background(), ScanOptions{})

	if !byID(t, view, open).Live || byID(t, view, closed).Live {
		t.Errorf("only the selected chat in an open window is live: %s", asJSON(t, view.Conversations))
	}
}

// terminalProcesses has pid running in Terminal on /dev/ttys004.
func terminalProcesses(pid int, held map[string]int, args map[int]string) fakeProcesses {
	return fakeProcesses{held: held, args: args, alive: map[int]bool{pid: true}, table: []ProcessInfo{
		{PID: 400, PPID: 1, Command: "/System/Applications/Utilities/Terminal.app/Contents/MacOS/Terminal"},
		{PID: 401, PPID: 400, TTY: "/dev/ttys004", Command: "/usr/bin/login"},
		{PID: pid, PPID: 401, TTY: "/dev/ttys004", Command: "node"},
	}}
}

// cursorAgentScanner lists a cursor-agent chat in /tmp/app for each of
// ids; store gives each chat's store.db.
func cursorAgentScanner(t *testing.T, processes Processes, ids ...string) (scanner *Scanner, store func(id string) string) {
	t.Helper()
	metaOf := func(id string) string {
		return hex.EncodeToString([]byte(`{"agentId":"` + id + `","name":"Chat ` + id[:1] + `","createdAt":1762500000000}`))
	}
	sqlite := func(_ context.Context, path, query string) ([]json.RawMessage, error) {
		switch {
		case filepath.Base(path) == "store.db" && strings.Contains(query, "FROM meta"):
			return []json.RawMessage{json.RawMessage(`{"value":"` + metaOf(filepath.Base(filepath.Dir(path))) + `"}`)}, nil
		case filepath.Base(path) == "state.vscdb":
			return nil, nil
		}
		return nil, errors.New("unexpected query on " + filepath.Base(path))
	}
	dir := t.TempDir()
	store = func(id string) string {
		return filepath.Join(dir, "cursor", "chats", md5Hex("/tmp/app"), id, "store.db")
	}
	scanner, stores := testScanner(t, sqlite, processes)
	stores.CursorDir = filepath.Join(dir, "cursor")
	scanner.Stores = stores
	scanner.Now = time.Now
	write(t, stores.CursorDB, "")
	for _, id := range ids {
		write(t, store(id), "")
	}
	return scanner, store
}

func TestScan_MarksRunningCursorAgentChatsLive(t *testing.T) {
	running, ended := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	held := map[string]int{}
	scanner, store := cursorAgentScanner(t, terminalProcesses(4242, held, nil), running, ended)
	held[store(running)] = 4242

	view := scanner.Scan(context.Background(), ScanOptions{Within: []string{"/tmp/app"}})

	if !byID(t, view, running).Live || byID(t, view, ended).Live {
		t.Errorf("only the chat a process holds is live: %s", asJSON(t, view.Conversations))
	}
	if open := byID(t, view, running).Open; open.Kind != RouteFocus || open.Label != "Switch to Terminal" {
		t.Errorf("a running chat switches to its terminal, actual %s", asJSON(t, open))
	}
	if open := byID(t, view, ended).Open; open.Kind != RouteTerminal || open.Command != "cd '/tmp/app' && cursor-agent --resume "+ended {
		t.Errorf("an ended chat resumes, actual %s", asJSON(t, open))
	}
	launcher := &fakeLauncher{focusOK: true}
	reply := Open(context.Background(), byID(t, view, running), scanner.Processes, launcher)
	if strings.Join(launcher.calls, ";") != "focus Terminal /dev/ttys004" || !reply.Opened || reply.Command != "" {
		t.Errorf("opening it must switch to its tab, not resume it again: calls %v, reply %s", launcher.calls, asJSON(t, reply))
	}
}

func paged(id, updated string, live bool) Conversation {
	return Conversation{Tool: ToolClaude, ID: id, UpdatedAt: updated, Live: live}
}

func TestWeekPage_ShowsOpenConversationsThenAWeekAtATime(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	all := []Conversation{
		paged("today", "2026-11-07T10:00:00Z", false),
		paged("open-but-old", "2026-09-01T10:00:00Z", true),
		paged("six-days", "2026-11-01T13:00:00Z", false),
		paged("eight-days", "2026-10-30T10:00:00Z", false),
		paged("after-a-gap", "2026-10-01T10:00:00Z", false),
		paged("gap-week", "2026-09-26T10:00:00Z", false),
		paged("undated", "", false),
	}
	ids := func(list []Conversation) string {
		var out []string
		for _, c := range list {
			out = append(out, c.ID)
		}
		return strings.Join(out, ",")
	}

	page, from, more := weekPage(all, now, true)
	if ids(page) != "open-but-old,today,six-days" || !from.Equal(now.Add(-pageSpan)) || !more {
		t.Errorf("first page = %s from %s more %v", ids(page), from, more)
	}
	page, from, more = weekPage(all, from, false)
	if ids(page) != "eight-days" || !more {
		t.Errorf("second page = %s from %s more %v", ids(page), from, more)
	}
	page, from, more = weekPage(all, from, false)
	if ids(page) != "after-a-gap,gap-week" || !from.Equal(time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)) || !more {
		t.Errorf("a page past empty weeks starts a week before its newest conversation, got %s from %s more %v", ids(page), from, more)
	}
	page, _, more = weekPage(all, from, false)
	if ids(page) != "undated" || more {
		t.Errorf("last page = %s more %v", ids(page), more)
	}
}

func TestScan_PagesAndFindsAnyAge(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	session := func(id, stamp string) {
		write(t, filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", id+".jsonl"), lines(
			obj{"type": "user", "cwd": "/tmp/app", "entrypoint": "cli", "timestamp": stamp, "message": obj{"role": "user", "content": "Chat " + id[:4]}}))
	}
	session(idSafari, "2026-11-06T10:00:00.000Z")
	session(idCache, "2026-10-20T10:00:00.000Z")

	first := scanner.Scan(context.Background(), ScanOptions{})
	if len(first.Conversations) != 1 || first.Next != "2026-10-31T12:00:00Z" || first.Tools[0].Count != 2 || first.Tools[0].Shown != 1 {
		t.Fatalf("first page = %s", asJSON(t, first))
	}
	before, err := time.Parse(time.RFC3339, first.Next)
	if err != nil {
		t.Fatal(err)
	}
	second := scanner.Scan(context.Background(), ScanOptions{Before: before})
	if len(second.Conversations) != 1 || second.Conversations[0].ID != idCache || second.Next != "" {
		t.Errorf("second page = %s", asJSON(t, second))
	}
	if _, ok := scanner.Find(context.Background(), ToolClaude, idCache, ScanOptions{}); !ok {
		t.Error("Find must reach a conversation older than the first page")
	}
}

func TestScan_KeepsAChatWrittenInTheSecondOfAPageBoundary(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	scanner.Now = func() time.Time { return time.Date(2026, 11, 7, 12, 0, 0, 500_000_000, time.UTC) }
	write(t, filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", idCache+".jsonl"), lines(
		obj{"type": "user", "cwd": "/tmp/app", "entrypoint": "cli", "timestamp": "2026-10-31T12:00:00.000Z", "message": obj{"role": "user", "content": "At the boundary"}}))
	write(t, filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", idSafari+".jsonl"), lines(
		obj{"type": "user", "cwd": "/tmp/app", "entrypoint": "cli", "timestamp": "2026-11-06T10:00:00.000Z", "message": obj{"role": "user", "content": "This week"}}))

	shown := 0
	before := time.Time{}
	for range 3 {
		page := scanner.Scan(context.Background(), ScanOptions{Before: before})
		shown += len(page.Conversations)
		if page.Next == "" {
			break
		}
		next, err := time.Parse(time.RFC3339, page.Next)
		if err != nil {
			t.Fatal(err)
		}
		before = next
	}
	if shown != 2 {
		t.Errorf("pages showed %d chats, want 2", shown)
	}
}

func TestScan_LeavesOutAJobWhoseNameIsNotAPlainID(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	write(t, filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", idCache+".jsonl"), lines(
		obj{"type": "user", "cwd": "/tmp/app", "entrypoint": "cli", "timestamp": "2026-11-06T10:00:00.000Z", "message": obj{"role": "user", "content": "Run the suite"}}))
	write(t, filepath.Join(stores.ClaudeDir, "jobs", "-x;touch pwned", "state.json"), `{"sessionId":"`+idCache+`","state":"running"}`)

	c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), idCache)

	if c.App == AppBackground || strings.Contains(c.Open.Command, "pwned") {
		t.Errorf("unsafe job name reached the route: %s %s", c.App, asJSON(t, c.Open))
	}
}

func TestScan_KeepsCachedSessionsWhenTheScanIsCanceled(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	write(t, filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", idCache+".jsonl"), lines(
		obj{"type": "user", "cwd": "/tmp/app", "entrypoint": "cli", "timestamp": "2026-11-06T10:00:00.000Z", "message": obj{"role": "user", "content": "Run the suite"}}))
	scanner.Scan(context.Background(), ScanOptions{})
	cached := len(scanner.claude)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	scanner.Scan(ctx, ScanOptions{})

	if cached == 0 || len(scanner.claude) != cached {
		t.Errorf("cache held %d sessions, then %d after a canceled scan", cached, len(scanner.claude))
	}
}

func TestScan_ReusesShownTextsAndKeepsThoseAScanDidNotAskFor(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{alive: map[int]bool{4242: true}})
	writeClaudeSessions(t, stores)
	first := asJSON(t, scanner.Scan(context.Background(), ScanOptions{}))
	if again := asJSON(t, scanner.Scan(context.Background(), ScanOptions{})); again != first {
		t.Errorf("a second scan of unchanged sessions must show the same rows, withheld texts included\nfirst: %s\nagain: %s", first, again)
	}
	markKept(t, scanner, "Safari login bug")
	if c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), idSafari); c.Title != "Kept" {
		t.Errorf("an unchanged session must reuse its checked texts, title = %q", c.Title)
	}

	held := len(scanner.shown.last)
	if err := os.Remove(filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", idSecret+".jsonl")); err != nil {
		t.Fatal(err)
	}
	scanner.Scan(context.Background(), ScanOptions{})
	// A list asks for one week and a search for every match; what one
	// leaves out, the other would pay for again.
	if len(scanner.shown.last) != held {
		t.Errorf("held the texts of %d rows, then %d after a scan that asked for fewer", held, len(scanner.shown.last))
	}
}

func TestShownCache_KeepsOnlyTheLatestScanOnceItIsFull(t *testing.T) {
	var cache shownCache
	ask := func(from, to int) {
		cache.start()
		for i := from; i < to; i++ {
			cache.get(texts{title: strconv.Itoa(i)})
		}
		cache.finish()
	}
	ask(0, maxShownKept-10)
	ask(maxShownKept-10, maxShownKept)
	if len(cache.last) != maxShownKept {
		t.Fatalf("held %d texts below the bound, want %d", len(cache.last), maxShownKept)
	}

	ask(maxShownKept, maxShownKept+5)

	if len(cache.last) != 5 {
		t.Errorf("held %d texts past the bound, want the 5 of the latest scan", len(cache.last))
	}
}

func TestScan_KeepsShownTextsWhenTheScanIsCanceled(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	writeClaudeSessions(t, stores)
	scanner.Scan(context.Background(), ScanOptions{})
	held := len(scanner.shown.last)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	scanner.Scan(ctx, ScanOptions{})

	if held == 0 || len(scanner.shown.last) != held {
		t.Errorf("held the texts of %d rows, then %d after a canceled scan", held, len(scanner.shown.last))
	}
}

// markKept changes the cached title of the row titled title, so a scan
// that shows "Kept" reused the cache instead of checking again.
func markKept(t *testing.T, scanner *Scanner, title string) {
	t.Helper()
	for key, shown := range scanner.shown.last {
		if shown.title == title {
			shown.title = "Kept"
			scanner.shown.last[key] = shown
			return
		}
	}
	t.Fatalf("no cached texts titled %q", title)
}

func TestFill_WithholdsAnIndentedSecretInAPrompt(t *testing.T) {
	for _, prompts := range [][2]string{
		{"Please set:\nenv:\n  FOO: abcd1234", "```yaml\nheaders:\n  X-Custom: abcd1234\n```"},
		{"Config:\n\tenv: {FOO: abcd1234}", "We have:\n  a: 1\n env: {FOO: abcd1234}"},
	} {
		shown := showTexts(texts{title: "Set up the client", firstPrompt: prompts[0], lastPrompt: prompts[1]})

		if shown.opening != nil || shown.prompt != "" || len(shown.withheld) != 2 {
			t.Errorf("%q: opening %q, prompt %q, withheld %v", prompts, shown.opening, shown.prompt, shown.withheld)
		}
	}
}

func TestSQLiteURI_WritesAWindowsPathAsAFileURIPath(t *testing.T) {
	if actual := sqliteURI(`C:\Users\me\.codex\state_5.sqlite`); actual != "file:///C:/Users/me/.codex/state_5.sqlite?immutable=1" {
		t.Errorf("SQLite reads file://C:... as a host named C, actual %s", actual)
	}
}

func TestFolderOpenIn_DecodesTheEditorsFolderURIs(t *testing.T) {
	storage := filepath.Join(t.TempDir(), "storage.json")
	write(t, storage, `{"windowsState":{"openedWindows":[{"folder":"file:///Users/me/%40scope/pkg%2Bv2"},{"folder":"file:///c%3A/Users/me/app"}]}}`)
	for folder, want := range map[string]bool{
		"/Users/me/@scope/pkg+v2": true,
		`C:\Users\me\app`:         true,
		"/Users/me/@scope/pkg":    false,
	} {
		if actual := folderOpenIn(storage, folder); actual != want {
			t.Errorf("folderOpenIn(%q) = %v, want %v", folder, actual, want)
		}
	}
}

func TestParseHolders_SkipsASqliteReader(t *testing.T) {
	store := "/home/me/.cursor/chats/abc/id/store.db"
	out := "p4242\ncnode\nf11\nn" + store + "\np5151\ncsqlite3\nf3\nn" + store + "\n"

	held := parseHolders(out, map[string]string{store: store})

	if held[store] != 4242 {
		t.Errorf("the chat's own process holds it, not the sqlite3 reader: %v", held)
	}
}

func TestParseHolders_PrefersTheAgentOverAnotherHolder(t *testing.T) {
	store := "/home/me/.cursor/chats/abc/id/store.db"
	out := "p4242\ncnode\nf11\nn" + store + "\np6161\ncrg\nf9\nn" + store + "\n"

	held := parseHolders(out, map[string]string{store: store})

	if held[store] != 4242 {
		t.Errorf("the agent holds it, not another reader: %v", held)
	}
}

func TestReopen_SwitchesToACursorAgentChatThatStartedAfterTheScan(t *testing.T) {
	started, ended, viewed := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	held := map[string]int{}
	args := map[int]string{4242: "/Users/me/.local/share/cursor-agent/node --use-system-ca index.js", 5151: "/Applications/DB Browser.app/Contents/MacOS/DB"}
	scanner, store := cursorAgentScanner(t, terminalProcesses(4242, held, args), started, ended, viewed)
	opts := ScanOptions{Within: []string{"/tmp/app"}}
	scanner.Scan(context.Background(), opts)
	held[store(started)] = 4242
	held[store(viewed)] = 5151

	launcher := &fakeLauncher{focusOK: true}
	reply, _ := scanner.Reopen(context.Background(), ToolCursor, started, opts, launcher)
	if strings.Join(launcher.calls, ";") != "focus Terminal /dev/ttys004" || !reply.Opened || reply.Command != "" {
		t.Errorf("a chat that runs now must switch to its tab, not resume again: calls %v, reply %s", launcher.calls, asJSON(t, reply))
	}
	for _, id := range []string{ended, viewed} {
		launcher := &fakeLauncher{}
		scanner.Reopen(context.Background(), ToolCursor, id, opts, launcher)
		if want := "terminal cd '/tmp/app' && cursor-agent --resume " + id; strings.Join(launcher.calls, ";") != want {
			t.Errorf("a chat no agent holds resumes: calls %v, want %q", launcher.calls, want)
		}
	}
}

func TestSystemProcesses_HoldersFindsALockUnderASymlinkedFolder(t *testing.T) {
	if _, err := exec.LookPath("lsof"); err != nil {
		t.Skip("lsof is not installed")
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, "codex", "thread-writer-locks", "a.lock"), "")
	if err := os.Symlink(filepath.Join(dir, "codex"), filepath.Join(dir, ".codex")); err != nil {
		t.Skip("cannot create a symlink:", err)
	}
	lock := filepath.Join(dir, ".codex", "thread-writer-locks", "a.lock")
	f, err := os.Open(lock)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	if held := (SystemProcesses{}).Holders([]string{lock}); held[lock] != os.Getpid() {
		t.Errorf("a lock this process holds through a symlink must be held, actual %v", held)
	}
}

func TestScan_ReadsACursorAgentChatAgainOnlyWhenItChanges(t *testing.T) {
	agent := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	meta := hex.EncodeToString([]byte(`{"agentId":"` + agent + `","name":"Release","createdAt":1762500000000}`))
	answer := fakeStores(t, map[string]string{
		"state.vscdb|FROM composerHeaders": `[]`,
		"store.db|FROM meta":               `[{"value":"` + meta + `"}]`,
	})
	storeReads, emptyReads := 0, 0
	scanner, stores := testScanner(t, func(ctx context.Context, path, query string) ([]json.RawMessage, error) {
		if strings.Contains(path, "empty") {
			emptyReads++
			return nil, nil
		}
		if filepath.Base(path) == "store.db" {
			storeReads++
		}
		return answer(ctx, path, query)
	}, fakeProcesses{})
	write(t, filepath.Join(stores.CursorDir, "chats", md5Hex("/tmp/app"), "empty", "store.db"), "")
	store := filepath.Join(stores.CursorDir, "chats", md5Hex("/tmp/app"), agent, "store.db")
	transcript := filepath.Join(stores.CursorDir, "projects", "p1", "agent-transcripts", agent, agent+".jsonl")
	prompt := func(text string) string {
		return lines(obj{"role": "user", "message": obj{"content": []obj{{"type": "text", "text": "<user_query>" + text + "</user_query>"}}}})
	}
	write(t, stores.CursorDB, "")
	write(t, store, "")
	write(t, transcript, prompt("Ship it"))
	scanner.Now = func() time.Time { return time.Now() }
	scan := func() Conversation {
		return byID(t, scanner.Scan(context.Background(), ScanOptions{Within: []string{"/tmp/app"}}), agent)
	}

	scan()
	info, err := os.Stat(transcript)
	if err != nil {
		t.Fatal(err)
	}
	write(t, transcript, prompt("Skip it"))
	if err := os.Chtimes(transcript, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	scan()
	if c := scan(); storeReads != 1 || emptyReads != 1 || c.Prompt != "Ship it" {
		t.Errorf("an unchanged chat must come from the cache: %d store reads, %d of a store without metadata, prompt %q", storeReads, emptyReads, c.Prompt)
	}

	write(t, store, "changed")
	write(t, transcript, prompt("Fix the build"))
	if c := scan(); storeReads != 2 || c.Prompt != "Fix the build" {
		t.Errorf("a changed chat must be read again: %d store reads, prompt %q", storeReads, c.Prompt)
	}
}

func TestScan_SearchesConversationsOfEveryAge(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	session := func(id, stamp, prompt string) {
		write(t, filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", id+".jsonl"), lines(
			obj{"type": "user", "cwd": "/tmp/app", "entrypoint": "cli", "timestamp": stamp, "message": obj{"role": "user", "content": prompt}}))
	}
	session(idSafari, "2026-11-06T10:00:00.000Z", "Fix the Safari login")
	session(idCache, "2026-08-01T10:00:00.000Z", "Tune the cache eviction for Safari")
	session(idSecret, "2026-07-01T10:00:00.000Z", "Rotate the staging password for Safari")

	view := scanner.Scan(context.Background(), ScanOptions{Query: "safari"})
	if view.Matches == nil || *view.Matches != 2 {
		t.Fatalf("a credential prompt is withheld, so only two rows can match: %s", asJSON(t, view))
	}
	if len(view.Conversations) != 1 || view.Conversations[0].ID != idSafari || view.Next == "" {
		t.Errorf("first page = %s", asJSON(t, view))
	}
	before, err := time.Parse(time.RFC3339, view.Next)
	if err != nil {
		t.Fatal(err)
	}
	older := scanner.Scan(context.Background(), ScanOptions{Query: "safari", Before: before})
	if len(older.Conversations) != 1 || older.Conversations[0].ID != idCache || older.Next != "" {
		t.Errorf("a month-old match must come on the next page, got %s", asJSON(t, older))
	}

	both := scanner.Scan(context.Background(), ScanOptions{Query: "CACHE  safari"})
	if *both.Matches != 1 || both.Conversations[0].ID != idCache {
		t.Errorf("every word must match, in any case: %s", asJSON(t, both))
	}
	if none := scanner.Scan(context.Background(), ScanOptions{Query: "safari", Tool: ToolCodex}); *none.Matches != 0 || len(none.Conversations) != 0 {
		t.Errorf("the tool filter applies to the search: %s", asJSON(t, none))
	}
	if plain := scanner.Scan(context.Background(), ScanOptions{}); plain.Matches != nil {
		t.Errorf("a scan without a search reports no match count: %s", asJSON(t, plain))
	}
}

func TestScan_SearchReadsEveryCursorChatOnceUntilItChanges(t *testing.T) {
	queries := 0
	stores := fakeStores(t, map[string]string{
		"state.vscdb|FROM composerHeaders": `[{"id":"88888888-8888-4888-8888-888888888888","workspace_id":"ws1","created_at":1700000000000,"recency":1700000000000,"name":"Old chat","folder":"/tmp/app","branch":""}]`,
		"state.vscdb|chosen(composerId)":   `[{"id":"88888888-8888-4888-8888-888888888888","first_text":"Start here","last_text":"Migrate the billing webhooks"}]`,
	})
	counting := func(ctx context.Context, path, query string) ([]json.RawMessage, error) {
		if strings.Contains(query, "chosen(composerId)") {
			queries++
		}
		return stores(ctx, path, query)
	}
	scanner, paths := testScanner(t, counting, fakeProcesses{})
	write(t, paths.CursorDB, "")

	for range 2 {
		view := scanner.Scan(context.Background(), ScanOptions{Query: "billing webhooks"})
		if *view.Matches != 1 || view.Conversations[0].Title != "Old chat" {
			t.Fatalf("search must reach a two-year-old Cursor chat's prompt: %s", asJSON(t, view))
		}
	}
	if queries != 1 {
		t.Errorf("an unchanged chat's prompts are read once, read %d times", queries)
	}
}

func TestEnclosingWindowFolder_PrefersTheEditorWindowAroundTheFolder(t *testing.T) {
	storage := filepath.Join(t.TempDir(), "storage.json")
	write(t, storage, `{"windowsState":{"openedWindows":[{"folder":"file:///repo"},{"folder":"file:///repo/pkg/api"},{"folder":"file:///"},{"folder":"file:///home/me"}]}}`)
	for folder, want := range map[string]string{
		"/repo/pkg":         "/repo",
		"/repo/pkg/api/v1":  "/repo/pkg/api",
		"/repo":             "/repo",
		"/elsewhere/repo":   "",
		"/repository/thing": "",
		"/home/me/app/src":  "",
		"/home/me":          "/home/me",
	} {
		if actual, _ := enclosingWindowFolder(storage, folder, "/home/me"); filepath.ToSlash(actual) != want {
			t.Errorf("enclosingWindowFolder(%q) = %q, want %q", folder, actual, want)
		}
	}
}

func TestMatching_SearchesTheTextAPersonSees(t *testing.T) {
	rows := []Conversation{{Tool: ToolClaude, App: AppCLI, ID: "a", Title: "<bash-input>gl</bash-input>"}}
	if len(matching(rows, queryWords("! gl"))) != 1 {
		t.Error("a typed shell command matches as the page shows it")
	}
	if len(matching(rows, queryWords("bash-input"))) != 0 {
		t.Error("the markup around a typed shell command never matches")
	}
}

func TestPsStartTimes_ReadsAPidAndItsStartTimeFromEachLine(t *testing.T) {
	started := psStartTimes("  412 Mon Nov  9 08:00:01 2026\n61234 Sun Nov  8 23:59:59 2026\n\nnot a pid line")
	if len(started) != 2 || started[412] != "Mon Nov  9 08:00:01 2026" || started[61234] != "Sun Nov  8 23:59:59 2026" {
		t.Errorf("started = %v", started)
	}
}

func TestSystemProcesses_AliveAmongKeepsThisProcessAndDropsAWrongStartTime(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ps is not on Windows")
	}
	self := os.Getpid()
	alive := SystemProcesses{}.AliveAmong([]ProcessRef{{PID: self}, {PID: 0}})
	if len(alive) != 1 || !alive[self] {
		t.Errorf("alive = %v, want only this process %d", alive, self)
	}
	if alive := (SystemProcesses{}).AliveAmong([]ProcessRef{{PID: self, ProcStart: "Thu Jan  1 00:00:00 1970"}}); len(alive) != 0 {
		t.Errorf("a pid that started at another time counted as alive: %v", alive)
	}
	if alive := (SystemProcesses{}).AliveAmong(nil); len(alive) != 0 {
		t.Errorf("no processes asked about, got %v", alive)
	}
}
