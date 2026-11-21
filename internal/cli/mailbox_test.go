package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
	"github.com/chemaclass/agnostic-ai/internal/mailbox"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

type reviewRefsReply struct {
	Worktree    string   `json:"worktree"`
	Commit      string   `json:"commit"`
	Base        string   `json:"base"`
	Branch      string   `json:"branch"`
	Paths       []string `json:"paths"`
	Uncommitted int      `json:"uncommitted"`
}

func makeReviewRepo(t *testing.T, dir string) (base string, commit string) {
	t.Helper()
	isolateGit(t)
	gitInit(t, dir)
	git(t, dir, "config", "user.name", "Mailbox Test")
	git(t, dir, "config", "user.email", "mailbox@example.test")
	writeFile(t, filepath.Join(dir, "src", "parser.go"), "original\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "base")
	base = git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "update-ref", "refs/remotes/origin/main", base)
	git(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	writeFile(t, filepath.Join(dir, "src", "parser.go"), "changed\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "update")
	commit = git(t, dir, "rev-parse", "HEAD")
	return base, commit
}

func parseReviewRefsReply(t *testing.T, raw []byte) reviewRefsReply {
	t.Helper()
	var refs reviewRefsReply
	if err := json.Unmarshal(raw, &refs); err != nil {
		t.Fatalf("review refs = %s: %v", raw, err)
	}
	return refs
}

func TestMailboxRefsCommand_UsesTheCurrentWorktreeAndOptionalCommit(t *testing.T) {
	dir := realDir(t)
	base, commit := makeReviewRepo(t, dir)
	testutil.Chdir(t, filepath.Join(dir, "src"))
	out, err := runRoot(t, "mailbox", "refs")
	if err != nil {
		t.Fatal(err)
	}
	refs := parseReviewRefsReply(t, []byte(out))
	if !sameRealDir(dir, refs.Worktree) || refs.Base != base || refs.Commit != commit ||
		len(refs.Paths) != 1 || refs.Paths[0] != "src/parser.go" {
		t.Errorf("review refs = %#v", refs)
	}
	out, err = runRoot(t, "mailbox", "refs", "--base", base, "--commit", base)
	if err != nil {
		t.Fatal(err)
	}
	refs = parseReviewRefsReply(t, []byte(out))
	if refs.Commit != base || refs.Base != base || len(refs.Paths) != 0 {
		t.Errorf("selected earlier commit = %#v", refs)
	}
}

func TestMailboxRefsCommand_ReportsWhenCurrentFolderIsOutsideGit(t *testing.T) {
	dir := realDir(t)
	testutil.Chdir(t, dir)
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	out, err := runRoot(t, "mailbox", "refs")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "worktree") {
		t.Errorf("mailbox refs outside Git = %q, %v", out, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "mailbox")); !os.IsNotExist(err) {
		t.Errorf("read-only command created a mailbox: %v", err)
	}
}

const mailboxCodexID = "0199a2b4-5c1e-7d30-9f2a-3b6c8d1e4f50"

var (
	agentClaude = conversations.Ref{Tool: conversations.ToolClaude, ID: inProject}
	agentCodex  = conversations.Ref{Tool: conversations.ToolCodex, ID: mailboxCodexID}
	agentThird  = conversations.Ref{Tool: conversations.ToolClaude, ID: elsewhere}
)

// mailboxAgentProcesses is a process table where this command runs in
// a shell under one tool, as an agent's Bash tool runs it.
type mailboxAgentProcesses struct {
	stillProcesses
	table []conversations.ProcessInfo
}

func (p *mailboxAgentProcesses) Table() ([]conversations.ProcessInfo, error) { return p.table, nil }

type mailboxAgentFixture struct {
	t         *testing.T
	store     mailbox.Store
	scanner   *conversations.Scanner
	processes *mailboxAgentProcesses
	env       map[string]string
	project   string
}

const mailboxAgentStarted = "Sat Nov  7 08:00:00 2026"

// newMailboxAgentFixture lists two Claude Code sessions and one Codex
// session, keeps the mailbox under a temporary home so the hooks find
// it too, and starts with approval on, the default.
func newMailboxAgentFixture(t *testing.T) *mailboxAgentFixture {
	t.Helper()
	_, store := hookMailboxHome(t)
	home, project := realDir(t), realDir(t)
	claudeDir := filepath.Join(home, ".claude")
	writeClaudeSession(t, claudeDir, inProject, project, "Fix the parser")
	writeClaudeSession(t, claudeDir, elsewhere, project, "Unrelated work")
	codexDir := filepath.Join(home, ".codex")
	writeFile(t, filepath.Join(codexDir, "state_5.sqlite"), "")
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	thread := json.RawMessage(fmt.Sprintf(`{"id":%q,"cwd":%q,"updated_at_ms":%d,"originator":"codex-tui","source":"cli","name":"Peer in Codex"}`,
		mailboxCodexID, project, now.UnixMilli()))
	processes := &mailboxAgentProcesses{}
	f := &mailboxAgentFixture{t: t, store: store, processes: processes, env: map[string]string{}, project: project}
	f.scanner = &conversations.Scanner{
		Stores: conversations.Stores{Home: home, ClaudeDir: claudeDir, CodexDir: codexDir, CursorDB: filepath.Join(home, "state.vscdb"), CursorDir: filepath.Join(home, ".cursor")},
		SQLite: func(_ context.Context, path, query string) ([]json.RawMessage, error) {
			if filepath.Base(path) == "state_5.sqlite" && strings.Contains(query, "FROM threads") {
				return []json.RawMessage{thread}, nil
			}
			return nil, errors.New("no sqlite3 here")
		},
		Processes: processes,
		Now:       func() time.Time { return now },
		MarksPath: filepath.Join(filepath.Dir(store.Dir), "conversation-marks.json"),
	}
	previous := mailboxEnv
	mailboxEnv = func() mailboxSession {
		return mailboxSession{scanner: f.scanner, getenv: func(name string) string { return f.env[name] }, pid: 100}
	}
	t.Cleanup(func() { mailboxEnv = previous })
	testutil.Chdir(t, project)
	return f
}

// as makes the next command run inside session ref.
func (f *mailboxAgentFixture) as(ref conversations.Ref) {
	f.t.Helper()
	tool := "/bin/sh"
	clear(f.env)
	_ = os.Remove(filepath.Join(f.scanner.Stores.ClaudeDir, "sessions", "98.json"))
	switch ref.Tool {
	case conversations.ToolClaude:
		tool = "/Users/me/.local/bin/claude"
		writeFile(f.t, filepath.Join(f.scanner.Stores.ClaudeDir, "sessions", "98.json"),
			fmt.Sprintf(`{"pid":98,"sessionId":%q,"procStart":%q,"status":"busy"}`, ref.ID, mailboxAgentStarted))
	case conversations.ToolCodex:
		tool = "/opt/homebrew/bin/codex"
		f.env["CODEX_THREAD_ID"] = ref.ID
	case conversations.ToolCursor:
		tool = "/Users/me/.local/bin/cursor-agent"
		f.env["CURSOR_CONVERSATION_ID"] = ref.ID
	}
	f.processes.table = []conversations.ProcessInfo{
		{PID: 100, PPID: 99, Started: mailboxAgentStarted, Command: "agnostic-ai"},
		{PID: 99, PPID: 98, Started: mailboxAgentStarted, Command: "/bin/zsh"},
		{PID: 98, PPID: 1, Started: mailboxAgentStarted, Command: tool},
	}
}

func (f *mailboxAgentFixture) outside() {
	clear(f.env)
	f.processes.table = []conversations.ProcessInfo{
		{PID: 100, PPID: 99, Started: mailboxAgentStarted, Command: "agnostic-ai"},
		{PID: 99, PPID: 1, Started: mailboxAgentStarted, Command: "/bin/zsh"},
	}
}

func (f *mailboxAgentFixture) approval(on bool) {
	f.t.Helper()
	if err := f.store.SaveSettings(context.Background(), mailbox.Settings{ApproveAgentSends: on}); err != nil {
		f.t.Fatal(err)
	}
}

func (f *mailboxAgentFixture) send(payload string) (string, error) {
	f.t.Helper()
	return runRootWithInput(f.t, payload, "mailbox", "send")
}

func (f *mailboxAgentFixture) mustSend(payload string) string {
	f.t.Helper()
	out, err := f.send(payload)
	if err != nil {
		f.t.Fatalf("mailbox send %s: %v", payload, err)
	}
	return out
}

func (f *mailboxAgentFixture) thread(id string) mailbox.Thread {
	f.t.Helper()
	list, err := f.store.Threads(context.Background())
	if err != nil {
		f.t.Fatal(err)
	}
	for _, thread := range list.Threads {
		if thread.ID == id {
			return thread
		}
	}
	f.t.Fatalf("thread %s not found in %+v", id, list)
	return mailbox.Thread{}
}

var receiptThread = regexp.MustCompile(`Thread (th_[0-9]{8}_[0-9a-f]{8}), message ([0-9]+)\.`)

func threadOf(t *testing.T, out string) string {
	t.Helper()
	match := receiptThread.FindStringSubmatch(out)
	if match == nil {
		t.Fatalf("no thread in %q", out)
	}
	return match[1]
}

func question(to conversations.Ref) string {
	return fmt.Sprintf(`{"to":"%s:%s","type":"question","body":"Does the lock cover claims?"}`, to.Tool, to.ID)
}

func requireMailboxRefusal(t *testing.T, out string, err error, guard string) {
	t.Helper()
	if err == nil {
		t.Fatalf("output %q, want guard %q", out, guard)
	}
	text := err.Error()
	prefix := "Not sent (guard: " + guard + "). "
	if !strings.HasPrefix(text, prefix) || !strings.Contains(text, "\n") || len(strings.TrimSpace(strings.SplitN(text, "\n", 2)[1])) == 0 {
		t.Errorf("refusal = %q, want guard %q, reason and next step", text, guard)
	}
	if out != "" {
		t.Errorf("a refusal printed %q on stdout", out)
	}
}

func TestMailboxSend_StoresTheDetectedSender(t *testing.T) {
	for _, test := range []struct {
		name     string
		from, to conversations.Ref
	}{
		{"from Claude Code", agentClaude, agentCodex},
		{"from Codex", agentCodex, agentClaude},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newMailboxAgentFixture(t)
			f.as(test.from)
			out := f.mustSend(question(test.to))
			if !strings.HasPrefix(out, "Held for approval in the companion. Thread ") {
				t.Errorf("output = %q", out)
			}
			m := f.thread(threadOf(t, out)).Messages[0]
			if m.From.Ref != test.from || m.From.By != "agent" || m.To != test.to || m.State != mailbox.Held {
				t.Errorf("stored message = %+v", m)
			}
		})
	}
}

func TestMailboxCommands_OutsideASupportedSessionStoreNothing(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*mailboxAgentFixture)
		want  string
	}{
		{"no session", (*mailboxAgentFixture).outside, "inside a Claude Code or Codex session"},
		{"a Cursor agent", func(f *mailboxAgentFixture) {
			f.as(conversations.Ref{Tool: conversations.ToolCursor, ID: "cursor-chat-1234"})
		}, "Not sent (guard: unknown-session). The mailbox works between Claude Code and Codex sessions."},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newMailboxAgentFixture(t)
			test.setup(f)
			for _, args := range [][]string{
				{"mailbox", "send"}, {"mailbox", "done", "--thread", "th_20261121_7f3a9c10"}, {"mailbox", "list"},
				{"mailbox", "show", "th_20261121_7f3a9c10"}, {"mailbox", "claim", "src"}, {"mailbox", "release", "src"},
				{"mailbox", "accept", "--thread", "th_20261121_7f3a9c10", "--n", "1"},
				{"mailbox", "decline", "--thread", "th_20261121_7f3a9c10", "--n", "1"},
			} {
				out, err := runRootWithInput(t, question(agentCodex), args...)
				if err == nil || !strings.Contains(err.Error(), test.want) || out != "" {
					t.Errorf("%v = %q, %v; want %q", args, out, err, test.want)
				}
			}
			if _, err := os.Lstat(f.store.Dir); !os.IsNotExist(err) {
				t.Errorf("the mailbox folder exists after refused commands: %v", err)
			}
		})
	}
}

func TestMailboxSend_RefusesFieldsTheMailboxSets(t *testing.T) {
	f := newMailboxAgentFixture(t)
	f.as(agentClaude)
	to := `"to":"codex:` + mailboxCodexID + `"`
	for _, payload := range []string{
		`{` + to + `,"type":"question","body":"Hi","from":{"tool":"claude","id":"` + inProject + `"}}`,
		`{` + to + `,"type":"question","body":"Hi","by":"human"}`,
		`{` + to + `,"type":"question","body":"Hi","state":"delivered"}`,
		`{` + to + `,"type":"question","body":"Hi","n":1}`,
		`{` + to + `,"type":"question","body":"Hi","at":"2026-11-21T10:02:11Z"}`,
		`{` + to + `,"type":"question","body":"Hi","queuedAt":"2026-11-21T10:02:11Z"}`,
		`{` + to + `,"type":"question","body":"Hi","deliveredAt":"2026-11-21T10:02:11Z"}`,
		`{` + to + `,"type":"question","body":"Hi","cap":16}`,
		`{` + to + `,"type":"question","body":"Hi","events":[]}`,
		`{` + to + `,"type":"question","body":"Hi","id":"th_20261121_7f3a9c10"}`,
		`{` + to + `,"type":"question","body":"Hi","models":{"from":"gpt-5.5"}}`,
		`{` + to + `,"type":"question","body":"Hi","body":"Again"}`,
		`{` + to + `,"thread":"th_20261121_7f3a9c10","type":"question","body":"Hi"}`,
		`{"type":"question","body":"Hi"}`,
		`{"to":"` + mailboxCodexID + `","type":"question","body":"Hi"}`,
		`["not an object"]`,
		`{` + to + `,"type":"handoff","body":"Take it","gives":{"worktree":"/tmp","paths":["a"],"owner":"x"}}`,
	} {
		out, err := f.send(payload)
		requireMailboxRefusal(t, out, err, "format")
	}
}

func TestMailboxSend_PrintsEachGuardsRefusal(t *testing.T) {
	f := newMailboxAgentFixture(t)
	f.approval(false)
	f.as(agentClaude)
	to := `"to":"codex:` + mailboxCodexID + `"`
	full := threadOf(t, f.mustSend(question(agentCodex)))
	for range 7 {
		f.mustSend(`{"thread":"` + full + `","type":"finding","body":"More."}`)
	}
	paused := threadOf(t, f.mustSend(question(agentCodex)))
	if err := f.store.Change(context.Background(), paused, "pause"); err != nil {
		t.Fatal(err)
	}
	closed := threadOf(t, f.mustSend(question(agentCodex)))
	if _, err := runRoot(t, "mailbox", "done", "--thread", closed); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ guard, payload string }{
		{"secret", `{` + to + `,"type":"question","body":"api_key=example-secret-sentinel"}`},
		{"size", `{` + to + `,"type":"question","body":"` + strings.Repeat("a", 4001) + `"}`},
		{"size", `{` + to + `,"type":"question","body":"` + strings.Repeat("a", 70000) + `"}`},
		{"cap", `{"thread":"` + full + `","type":"finding","body":"One more."}`},
		{"paused", `{"thread":"` + paused + `","type":"finding","body":"Still there?"}`},
		{"closed", `{"thread":"` + closed + `","type":"finding","body":"Late."}`},
		{"echo", `{` + to + `,"type":"review_request","body":"Looks good to me.","refs":{"worktree":"/tmp","base":"HEAD"}}`},
		{"echo", `{` + to + `,"type":"review_request","refs":{"worktree":"/tmp","base":"HEAD","verdict":"approve"}}`},
		{"refs", `{` + to + `,"type":"review_request"}`},
		{"ownership", `{` + to + `,"type":"handoff","body":"Take it."}`},
		{"unknown-session", `{"to":"claude:` + inWorktree + `","type":"question","body":"Hi"}`},
		{"unknown-session", `{"to":"cursor:` + inWorktree + `","type":"question","body":"Hi"}`},
		{"not-in-thread", `{"thread":"th_20261121_7f3a9c10","type":"finding","body":"Hi"}`},
		{"format", `{` + to + `,"type":"question","body":"Hi","model":"gpt<5>"}`},
	} {
		out, err := f.send(test.payload)
		requireMailboxRefusal(t, out, err, test.guard)
	}
}

func TestMailboxSend_ReviewRequestFromRefsOutput(t *testing.T) {
	f := newMailboxAgentFixture(t)
	dir := realDir(t)
	base, commit := makeReviewRepo(t, dir)
	testutil.Chdir(t, dir)
	f.as(agentClaude)
	refs, err := runRoot(t, "mailbox", "refs")
	if err != nil {
		t.Fatal(err)
	}
	out := f.mustSend(`{"to":"codex:` + mailboxCodexID + `","type":"review_request","model":"claude-opus-5-5","refs":` + strings.TrimSpace(refs) + `}`)
	thread := f.thread(threadOf(t, out))
	m := thread.Messages[0]
	if m.Refs == nil || m.Refs.Base != base || m.Refs.Commit != commit || m.Body != "" {
		t.Errorf("review message = %+v", m)
	}
	if model := thread.Models["claude:"+inProject]; model.Name != "claude-opus-5-5" || model.StatedBy != "agent" {
		t.Errorf("stated model = %+v", thread.Models)
	}
}

func TestMailboxSend_IsHeldUntilApproved(t *testing.T) {
	f := newMailboxAgentFixture(t)
	f.as(agentClaude)
	out := f.mustSend(question(agentCodex))
	id := threadOf(t, out)
	prompt := hookPayload(mailboxCodexID, "UserPromptSubmit", "")
	if got := runHookMailbox(t, "codex", prompt); got != "" {
		t.Fatalf("a held message was delivered: %q", got)
	}
	if err := f.store.Approve(context.Background(), id, 1, "approve"); err != nil {
		t.Fatal(err)
	}
	if got := runHookMailbox(t, "codex", prompt); !strings.Contains(got, "Does the lock cover claims?") {
		t.Errorf("approved message delivery = %q", got)
	}
}

func TestMailboxCommands_RefuseAThirdSession(t *testing.T) {
	f := newMailboxAgentFixture(t)
	f.approval(false)
	root := claimFixtureRepo(t, f)
	f.as(agentClaude)
	if _, err := runRoot(t, "mailbox", "claim", "src"); err != nil {
		t.Fatal(err)
	}
	id := threadOf(t, f.mustSend(fmt.Sprintf(`{"to":"codex:%s","type":"handoff","body":"Take the parser.","gives":{"worktree":%q,"paths":["src"]}}`, mailboxCodexID, root)))
	f.as(agentThird)
	cases := map[string][]string{
		"send":    {"mailbox", "send"},
		"done":    {"mailbox", "done", "--thread", id},
		"show":    {"mailbox", "show", id},
		"accept":  {"mailbox", "accept", "--thread", id, "--n", "1"},
		"decline": {"mailbox", "decline", "--thread", id, "--n", "1"},
	}
	for name, args := range cases {
		out, err := runRootWithInput(t, `{"thread":"`+id+`","type":"finding","body":"I know."}`, args...)
		if err == nil || !strings.Contains(err.Error(), "(guard: not-in-thread)") {
			t.Errorf("%s by a third session = %q, %v", name, out, err)
		}
	}
	if got := f.thread(id); got.Messages[0].Handoff != mailbox.Offered || len(got.Messages) != 1 {
		t.Errorf("a third session changed the thread: %+v", got)
	}
}

func claimFixtureRepo(t *testing.T, f *mailboxAgentFixture) string {
	t.Helper()
	makeReviewRepo(t, f.project)
	return f.project
}

func TestMailboxDone_ClosesAfterAcceptedMessagesDrain(t *testing.T) {
	f := newMailboxAgentFixture(t)
	f.approval(false)
	f.as(agentClaude)
	id := threadOf(t, f.mustSend(question(agentCodex)))
	out, err := runRoot(t, "mailbox", "done", "--thread", id)
	if err != nil || !strings.HasPrefix(out, "The thread is closing.") {
		t.Fatalf("done = %q, %v", out, err)
	}
	if got := f.thread(id).Status; got != mailbox.Closing {
		t.Errorf("status after done = %q", got)
	}
	runHookMailbox(t, "codex", hookPayload(mailboxCodexID, "UserPromptSubmit", ""))
	if got := f.thread(id).Status; got != mailbox.Closed {
		t.Errorf("status after delivery = %q", got)
	}

	f.approval(true)
	held := threadOf(t, f.mustSend(question(agentCodex)))
	out, err = runRoot(t, "mailbox", "done", "--thread", held)
	if err != nil || !strings.Contains(out, "The thread closes after it is approved.") || f.thread(held).Status != mailbox.Open {
		t.Errorf("held done = %q, %v, status %q", out, err, f.thread(held).Status)
	}
	out, err = runRoot(t, "mailbox", "done", "--thread", held)
	requireMailboxRefusal(t, out, err, "closed")
}

func TestMailboxAccept_MovesPathsOnlyWithoutAConflict(t *testing.T) {
	f := newMailboxAgentFixture(t)
	f.approval(false)
	root := claimFixtureRepo(t, f)
	f.as(agentClaude)
	if out, err := runRoot(t, "mailbox", "claim", "src"); err != nil || out != "Claimed 1 path in "+root+".\n" {
		t.Fatalf("claim = %q, %v", out, err)
	}
	offer := func() string {
		return threadOf(t, f.mustSend(fmt.Sprintf(`{"to":"codex:%s","type":"handoff","body":"Take the parser.","gives":{"worktree":%q,"paths":["src/parser.go"]}}`, mailboxCodexID, root)))
	}
	first := offer()

	f.as(agentClaude)
	out, err := runRoot(t, "mailbox", "accept", "--thread", first, "--n", "1")
	requireMailboxRefusal(t, out, err, "ownership")

	// Between the offer and the answer, a third session takes the file.
	if out, err := runRoot(t, "mailbox", "release", "src/parser.go"); err != nil || out != "Released.\n" {
		t.Fatalf("release = %q, %v", out, err)
	}
	f.as(agentThird)
	if _, err := runRoot(t, "mailbox", "claim", "src/parser.go"); err != nil {
		t.Fatal(err)
	}
	f.as(agentClaude)
	out, err = runRoot(t, "mailbox", "claim", "src")
	requireMailboxRefusal(t, out, err, "ownership")
	f.as(agentCodex)
	out, err = runRoot(t, "mailbox", "accept", "--thread", first, "--n", "1")
	requireMailboxRefusal(t, out, err, "ownership")
	if got := f.thread(first).Messages[0].Handoff; got != mailbox.Offered {
		t.Errorf("handoff after a conflict = %q", got)
	}

	f.as(agentThird)
	if _, err := runRoot(t, "mailbox", "release", "src/parser.go"); err != nil {
		t.Fatal(err)
	}
	f.as(agentClaude)
	if _, err := runRoot(t, "mailbox", "claim", "--worktree", root, "src/parser.go"); err != nil {
		t.Fatal(err)
	}
	f.as(agentCodex)
	out, err = runRoot(t, "mailbox", "accept", "--thread", first, "--n", "1")
	if err != nil || out != "Handoff accepted. Work only in "+root+": src/parser.go.\n" {
		t.Fatalf("accept = %q, %v", out, err)
	}
	thread := f.thread(first)
	last := thread.Events[len(thread.Events)-1]
	if thread.Messages[0].Handoff != mailbox.Accepted || last.Action != "accept" || last.By != "agent" {
		t.Errorf("accepted thread = %+v", thread)
	}
	view, err := f.store.ClaimsOf(context.Background(), agentCodex)
	if err != nil || len(view.Claims) != 1 {
		t.Errorf("codex claims = %+v, %v", view, err)
	}

	f.as(agentClaude)
	out, err = f.send(fmt.Sprintf(`{"to":"codex:%s","type":"handoff","body":"Back to me.","gives":{"worktree":%q,"paths":["src/parser.go"]}}`, mailboxCodexID, root))
	requireMailboxRefusal(t, out, err, "ownership")
}

func TestMailboxListAndShow_LeaveOutContentNotYetShown(t *testing.T) {
	f := newMailboxAgentFixture(t)
	f.approval(false)
	f.as(agentClaude)
	id := threadOf(t, f.mustSend(question(agentCodex)))
	f.as(agentCodex)
	out, err := runRoot(t, "mailbox", "show", id)
	if err != nil || strings.Contains(out, "Does the lock cover claims?") || !strings.Contains(out, `"state": "waiting"`) {
		t.Errorf("show before delivery = %q, %v", out, err)
	}
	runHookMailbox(t, "codex", hookPayload(mailboxCodexID, "UserPromptSubmit", ""))
	for _, args := range [][]string{{"mailbox", "show", id}, {"mailbox", "list"}} {
		out, err := runRoot(t, args...)
		if err != nil || !strings.Contains(out, "Does the lock cover claims?") {
			t.Errorf("%v after delivery = %q, %v", args, out, err)
		}
	}
}

func TestMailboxCommands_HaveOnlyTheDocumentedFlags(t *testing.T) {
	want := map[string]string{
		"send": "", "done": "thread", "list": "", "show": "", "claim": "worktree", "release": "worktree",
		"accept": "n,thread", "decline": "n,thread", "refs": "base,commit",
	}
	for _, sub := range newMailboxCmd().Commands() {
		var names []string
		sub.Flags().VisitAll(func(flag *pflag.Flag) { names = append(names, flag.Name) })
		slices.Sort(names)
		if got, ok := want[sub.Name()]; !ok || strings.Join(names, ",") != got {
			t.Errorf("mailbox %s flags = %v, want %q", sub.Name(), names, got)
		}
	}
}

// TestMailboxAgentFlow_IsTheSameForEachTool runs one exchange with the
// tools swapped and compares what each side printed and stored.
func TestMailboxAgentFlow_IsTheSameForEachTool(t *testing.T) {
	run := func(first, second conversations.Ref) ([]string, mailbox.Thread) {
		f := newMailboxAgentFixture(t)
		var lines []string
		f.as(first)
		out := f.mustSend(question(second) + "\n")
		id := threadOf(t, out)
		lines = append(lines, out)
		if err := f.store.Approve(context.Background(), id, 1, "approve"); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, runHookMailbox(t, string(second.Tool), hookPayload(second.ID, "UserPromptSubmit", "")))
		f.as(second)
		lines = append(lines, f.mustSend(`{"thread":"`+id+`","type":"finding","body":"Yes, one lock."}`))
		out, err := runRoot(t, "mailbox", "done", "--thread", id)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, out)
		thread := f.thread(id)
		for i, line := range lines {
			line = strings.ReplaceAll(line, id, "<thread>")
			line = strings.ReplaceAll(line, first.ID, "<first>")
			lines[i] = strings.ReplaceAll(line, second.ID, "<second>")
		}
		return lines, thread
	}
	claudeFirst, a := run(agentClaude, agentCodex)
	codexFirst, b := run(agentCodex, agentClaude)
	for i := range claudeFirst {
		if i == 1 {
			continue // each tool's hook prints its own reply format
		}
		if claudeFirst[i] != codexFirst[i] {
			t.Errorf("step %d: %q vs %q", i, claudeFirst[i], codexFirst[i])
		}
	}
	if !strings.Contains(claudeFirst[1], "Does the lock cover claims?") || !strings.Contains(codexFirst[1], "Does the lock cover claims?") {
		t.Errorf("deliveries = %q, %q", claudeFirst[1], codexFirst[1])
	}
	for i := range a.Messages {
		ma, mb := a.Messages[i], b.Messages[i]
		if ma.Type != mb.Type || ma.State != mb.State || ma.From.By != mb.From.By || ma.Body != mb.Body ||
			ma.From.Ref != b.Messages[i].To || ma.To != mb.From.Ref {
			t.Errorf("message %d: %+v vs %+v", i+1, ma, mb)
		}
	}
	if a.Status != b.Status || len(a.Events) != len(b.Events) {
		t.Errorf("threads differ: %+v vs %+v", a, b)
	}
}
