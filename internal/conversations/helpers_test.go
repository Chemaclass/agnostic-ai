package conversations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	runSeen   = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	runUnseen = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	runPID    = 77
)

// codexRunScanner lists one Claude Code session and two `codex exec`
// runs, each with a writer lock on disk, and keeps marks in marksPath.
func codexRunScanner(t *testing.T, stores Stores, marksPath string, processes fakeProcesses) *Scanner {
	t.Helper()
	written := codexWrittenAt(time.Date(2026, 11, 7, 11, 50, 0, 0, time.UTC))
	run := func(id, title string) string {
		return `{"id":"` + id + `","cwd":"/tmp/reviews","created_at_ms":` + written + `,"updated_at_ms":` + written +
			`,"originator":"codex_exec","source":"exec","name":"","title":"` + title + `","first_user_message":"` + title + `\nThe rest of the brief"}`
	}
	// Codex's own guardian review thread of the same process has thread_source
	// guardian_review, so the user-thread query never returns it.
	sqlite := fakeStores(t, map[string]string{
		"state_5.sqlite|thread_source = 'user'":     "[" + run(runSeen, "Review PR 3513") + "," + run(runUnseen, "Review PR 3512") + "]",
		"state_5.sqlite|thread_source = 'subagent'": "[]",
	})
	scanner := &Scanner{Stores: stores, SQLite: sqlite, Processes: processes, MarksPath: marksPath,
		Now: func() time.Time { return time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC) }}
	return scanner
}

func codexRunStores(t *testing.T) (Stores, string) {
	t.Helper()
	_, stores := testScanner(t, nil, fakeProcesses{})
	write(t, filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", idSafari+".jsonl"), lines(
		obj{"type": "user", "cwd": "/tmp/app", "entrypoint": "cli", "timestamp": "2026-11-07T11:00:00.000Z", "message": obj{"role": "user", "content": "Review the stack"}},
	))
	write(t, filepath.Join(stores.CodexDir, "state_5.sqlite"), "")
	for _, id := range []string{runSeen, runUnseen} {
		write(t, filepath.Join(stores.CodexDir, "thread-writer-locks", id+".lock"), "")
	}
	return stores, filepath.Join(t.TempDir(), "conversation-marks.json")
}

func TestScan_ListsACodexRunUnderTheClaudeSessionThatStartedIt(t *testing.T) {
	stores, marksPath := codexRunStores(t)
	running := fakeProcesses{
		held:    map[string]int{runSeen + ".lock": runPID},
		parents: map[int]string{runPID: idSafari},
	}

	view := codexRunScanner(t, stores, marksPath, running).Scan(context.Background(), ScanOptions{})

	want := `[{"tool":"codex","id":"` + runSeen + `","title":"Review PR 3513","state":"working","updatedAt":"2026-11-07T11:50:00Z"}]`
	if got := helpersJSON(t, byID(t, view, idSafari).Helpers); got != compactJSON(t, want) {
		t.Errorf("the run its process names the session of belongs under that session; the run never seen running nowhere:\n%s", got)
	}
	for _, c := range view.Conversations {
		if c.Tool == ToolCodex {
			t.Errorf("a Codex run is never a row of its own: %s", asJSON(t, c))
		}
	}

	// The run ends: its process and environment are gone. The saved link keeps it under its session.
	ended := codexRunScanner(t, stores, marksPath, fakeProcesses{})
	view = ended.Scan(context.Background(), ScanOptions{})

	helpers := byID(t, view, idSafari).Helpers
	if len(helpers) != 1 || helpers[0].ID != runSeen || helpers[0].State != helperFinished {
		t.Errorf("a run that ended stays under its session as finished, actual %s", asJSON(t, helpers))
	}
	marks, _, err := readMarks(marksPath, time.Time{})
	if saved := marks.Marks["codex:"+runSeen].StartedBy; err != nil || saved == nil || *saved != (Ref{Tool: ToolClaude, ID: idSafari}) {
		t.Errorf("the marks file must keep the run's parent, actual %s (%v)", asJSON(t, marks), err)
	}

	opened, found := ended.find(context.Background(), ToolCodex, runSeen, ScanOptions{}, true)
	if !found || opened.Open.Kind != RouteTerminal || opened.Open.Command != "cd '/tmp/reviews' && codex resume "+runSeen {
		t.Errorf("a run opens the way a Codex thread opens, actual %v %s", found, asJSON(t, opened.Open))
	}
	if _, found := ended.Find(context.Background(), ToolCodex, runSeen, ScanOptions{}); found {
		t.Error("marks and renames must not reach a run: it is no row")
	}
}

func TestScan_LinksNoRunWhoseProcessNamesNoClaudeSession(t *testing.T) {
	stores, marksPath := codexRunStores(t)
	started := fakeProcesses{held: map[string]int{runSeen + ".lock": runPID}, parents: map[int]string{}}

	view := codexRunScanner(t, stores, marksPath, started).Scan(context.Background(), ScanOptions{})

	if helpers := byID(t, view, idSafari).Helpers; helpers != nil {
		t.Errorf("a run another program started must not be guessed into a session, actual %s", asJSON(t, helpers))
	}
}

func TestScan_SaysWhyRunsCannotBeLinkedAndSavesNoLink(t *testing.T) {
	stores, marksPath := codexRunStores(t)
	unreadable := fakeProcesses{held: map[string]int{runSeen + ".lock": runPID}, parents: map[int]string{runPID: idSafari}, environmentFails: true}

	view := codexRunScanner(t, stores, marksPath, unreadable).Scan(context.Background(), ScanOptions{})

	if helpers := byID(t, view, idSafari).Helpers; helpers != nil {
		t.Errorf("no environment, no link, actual %s", asJSON(t, helpers))
	}
	if note := view.Tools[1].Note; note != "Codex runs started from Claude Code could not be linked to their session: this system cannot read another process's environment" {
		t.Errorf("the Codex source must say why runs are not linked, actual %q", note)
	}
	if marks, _ := os.ReadFile(marksPath); strings.Contains(string(marks), "startedBy") {
		t.Errorf("no link may be saved, actual %s", marks)
	}
}

func TestApplyMarks_KeepsTheParentFirstSavedForARunAndReadsOldFiles(t *testing.T) {
	scanner, _ := testScanner(t, nil, fakeProcesses{})
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	// A file from before runs were linked, beside one run already linked.
	write(t, scanner.MarksPath, `{"schemaVersion":"conversation-marks/v1","since":"2026-10-01T00:00:00Z","marks":{`+
		`"claude:`+idSafari+`":{"seenAt":"2026-11-01T00:00:00Z"},`+
		`"codex:`+runSeen+`":{"startedBy":{"tool":"claude","id":"`+idSafari+`"}}}}`)

	_, parents, err := scanner.applyMarks(nil, map[string]Ref{runSeen: {Tool: ToolClaude, ID: idCache}, runUnseen: {Tool: ToolClaude, ID: idCache}})

	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Ref{runSeen: {Tool: ToolClaude, ID: idSafari}, runUnseen: {Tool: ToolClaude, ID: idCache}}
	if asJSON(t, parents) != asJSON(t, want) {
		t.Errorf("a saved parent stands and a new one is added, actual %s", asJSON(t, parents))
	}
	file, _, err := readMarks(scanner.MarksPath, scanner.Now())
	if err != nil || file.Marks["claude:"+idSafari].SeenAt == "" || file.Marks["codex:"+runUnseen].StartedBy == nil {
		t.Errorf("the file keeps its old marks and gains the new link, actual %s (%v)", asJSON(t, file), err)
	}
}

func TestClaudeHelpers_ListsSubagentsWithTypeTitleAndState(t *testing.T) {
	scanner, stores := testScanner(t, nil, fakeProcesses{})
	file := filepath.Join(stores.ClaudeDir, "projects", "demo", idSafari+".jsonl")
	dir := claudeSubagentsDir(file)
	write(t, file, "")
	started, _ := time.Parse(time.RFC3339, workerTime)
	agent := func(id, meta string, at time.Time) {
		transcript := filepath.Join(dir, "agent-"+id+".jsonl")
		write(t, transcript, "")
		if meta != "" {
			write(t, filepath.Join(dir, "agent-"+id+".meta.json"), meta)
		}
		if err := os.Chtimes(transcript, at, at); err != nil {
			t.Fatal(err)
		}
	}
	agent("background", `{"agentType":"Explore","description":"Find rename call sites","toolUseId":"call-background"}`, started.Add(3*time.Minute))
	agent("inline", `{"agentType":"general-purpose","description":"Size budget check","toolUseId":"call-inline"}`, started.Add(2*time.Minute))
	agent("done", `{"agentType":"Plan","description":"Helper rows design","toolUseId":"call-done"}`, started.Add(time.Minute))
	agent("secret", `{"agentType":"general-purpose","description":"Use password hunter2 here","toolUseId":"call-secret"}`, started)
	agent("old", "", started.Add(-time.Hour))
	appendWorkerEvent(t, file, workerDispatch("call-background"))
	appendWorkerEvent(t, file, workerResult("call-background", obj{"status": "async_launched", "agentId": "background"}))
	appendWorkerEvent(t, file, workerDispatch("call-done"))
	appendWorkerEvent(t, file, workerResult("call-done", obj{"status": "completed", "agentId": "done"}))
	// A call waiting for its result is a subagent that works, named by its meta file.
	appendWorkerEvent(t, file, workerDispatch("call-inline"))
	session := liveSession{startedAt: started.Add(-time.Minute).UnixMilli()}

	work, _ := scanner.claudeWorkOf(file, session)
	helpers := scanner.claudeHelpers(file, work, map[string]bool{})

	want := `[
  {"tool":"claude","id":"background","title":"Find rename call sites","kind":"Explore","state":"working","updatedAt":"2026-11-14T09:03:00Z"},
  {"tool":"claude","id":"inline","title":"Size budget check","kind":"general-purpose","state":"working","updatedAt":"2026-11-14T09:02:00Z"},
  {"tool":"claude","id":"done","title":"Helper rows design","kind":"Plan","state":"finished","updatedAt":"2026-11-14T09:01:00Z"},
  {"tool":"claude","id":"secret","title":"","kind":"general-purpose","state":"finished","updatedAt":"2026-11-14T09:00:00Z"},
  {"tool":"claude","id":"old","title":"","state":"finished","updatedAt":"2026-11-14T08:00:00Z"}
]`
	if got := helpersJSON(t, helpers); got != compactJSON(t, want) {
		t.Errorf("subagents, newest first, with a title only when it holds no credential:\n%s", got)
	}
	if count := work.count(); count != 2 {
		t.Errorf("the working count must agree with the working helpers, actual %d", count)
	}
	for _, h := range scanner.claudeHelpers(file, noClaudeWork, map[string]bool{}) {
		if h.State != helperFinished {
			t.Errorf("no subagent of a session no process runs works, actual %s", asJSON(t, h))
		}
	}
	if scanner.claudeHelpers(file, nil, map[string]bool{}) != nil {
		t.Error("unknown work must list no subagent: a finished one cannot be told from one that works")
	}
}

func TestCodexWorkingSubagents_ListsDirectSubagentsAndTheRowsThatOpenThem(t *testing.T) {
	scanner, stores := testScanner(t, nil, fakeProcesses{})
	parent, child, grandchild := idSafari, idCache, idSide
	rollout := filepath.Join(stores.CodexDir, "sessions", "child.jsonl")
	write(t, rollout, "")
	appendWorkerEvent(t, rollout, obj{"type": "event_msg", "payload": obj{"type": "task_started"}})
	worker := func(id, parent, nickname, role, firstPrompt string) json.RawMessage {
		source, _ := json.Marshal(obj{"subagent": obj{"thread_spawn": obj{"parent_thread_id": parent, "agent_nickname": nickname, "agent_role": role}}})
		row, _ := json.Marshal(obj{"id": id, "source": string(source), "rollout_path": rollout, "cwd": "/tmp/app", "originator": "codex-tui",
			"first_user_message": firstPrompt, "updated_at_ms": 1794049200000})
		return row
	}
	scanner.SQLite = func(context.Context, string, string) ([]json.RawMessage, error) {
		return []json.RawMessage{worker(child, parent, "Aquinas", "explorer", ""), worker(grandchild, child, "Leibniz", "", "Check the timer\nthen report")}, nil
	}
	scanner.Processes = fakeProcesses{held: map[string]int{child + ".lock": 10}}
	rows := []Conversation{{ID: parent}, {ID: child}}

	_, opens := scanner.codexWorkingSubagents(context.Background(), "state.sqlite", rows, editorApps[0])

	want := `[{"tool":"codex","id":"` + child + `","title":"Aquinas","kind":"explorer","state":"working","updatedAt":"2026-11-07T11:00:00Z"}]`
	if got := helpersJSON(t, rows[0].Helpers); got != compactJSON(t, want) {
		t.Errorf("a parent lists its direct subagent, named by its nickname when it has no title:\n%s", got)
	}
	if h := rows[1].Helpers; len(h) != 1 || h[0].Title != "Check the timer" || h[0].State != helperFinished {
		t.Errorf("a subagent no process holds is finished and takes its first prompt's line, actual %s", asJSON(t, h))
	}
	if len(opens) != 2 || opens[0].Open.Command != "cd '/tmp/app' && codex resume "+child {
		t.Errorf("each subagent opens the way a Codex thread opens, actual %s", asJSON(t, opens))
	}

	scanner.Processes = fakeProcesses{readFails: true}
	rows = []Conversation{{ID: parent}}
	scanner.codexWorkingSubagents(context.Background(), "state.sqlite", rows, editorApps[0])
	if rows[0].Helpers != nil {
		t.Error("subagents whose locks cannot be read must not be listed as finished")
	}
}

func TestClaudeParentIn_NeedsAChildSessionAndASessionID(t *testing.T) {
	cases := map[string]struct {
		entries []string
		want    string
	}{
		"a command Claude Code ran":   {[]string{"PATH=/bin", "CLAUDE_CODE_SESSION_ID=" + idSafari, "CLAUDE_CODE_CHILD_SESSION=1"}, idSafari},
		"the last entry of a name":    {[]string{"CLAUDE_CODE_SESSION_ID=" + idCache, "CLAUDE_CODE_CHILD_SESSION=1", "CLAUDE_CODE_SESSION_ID=" + idSafari}, idSafari},
		"no child session":            {[]string{"CLAUDE_CODE_SESSION_ID=" + idSafari}, ""},
		"a child session set to zero": {[]string{"CLAUDE_CODE_SESSION_ID=" + idSafari, "CLAUDE_CODE_CHILD_SESSION=0"}, ""},
		"an id that is no session id": {[]string{"CLAUDE_CODE_SESSION_ID=../../etc", "CLAUDE_CODE_CHILD_SESSION=1"}, ""},
	}
	for name, tc := range cases {
		got, _ := claudeParentIn(tc.entries)
		if got != tc.want {
			t.Errorf("%s: got %q, want %q", name, got, tc.want)
		}
	}
}

func TestParsePSEnvironments_SplitsEachProcessLine(t *testing.T) {
	out := "  77 /opt/homebrew/bin/codex exec --json CLAUDE_CODE_SESSION_ID=" + idSafari + " CLAUDE_CODE_CHILD_SESSION=1\n 78 /bin/zsh HOME=/Users/me\nnot a pid line"
	environments := parsePSEnvironments(out)
	if parent, ok := claudeParentIn(environments[77]); !ok || parent != idSafari {
		t.Errorf("pid 77 runs inside the session, actual %q %v", parent, ok)
	}
	if _, ok := claudeParentIn(environments[78]); ok || len(environments) != 2 {
		t.Errorf("pid 78 runs in no session and the odd line is skipped, actual %v", environments)
	}
}

func TestEnvironmentError_KeepsItsReasonAndCause(t *testing.T) {
	cause := errors.New("ps: exit status 2")
	err := error(&environmentError{reason: environmentUnreadable, cause: cause})
	var unread *environmentError
	if !errors.As(err, &unread) || unread.reason != environmentUnreadable || !errors.Is(err, cause) {
		t.Fatalf("callers branch on the reason and logs keep the cause: %v", err)
	}
	if err.Error() != "the environment of running processes could not be read: ps: exit status 2" {
		t.Errorf("message = %q", err.Error())
	}
}

func compactJSON(t *testing.T, value string) string {
	t.Helper()
	var out bytes.Buffer
	if err := json.Compact(&out, []byte(value)); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func helpersJSON(t *testing.T, helpers []helper) string {
	t.Helper()
	return compactJSON(t, asJSON(t, helpers))
}
