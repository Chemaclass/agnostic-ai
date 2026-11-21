package conversations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
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

// runningSeen is a process that runs runSeen, started by the Claude Code session idSafari.
func runningSeen() fakeProcesses {
	return fakeProcesses{
		held:    map[string]int{runSeen + ".lock": runPID},
		parents: map[int]string{runPID: idSafari},
		alive:   map[int]bool{runPID: true},
	}
}

func TestScan_ListsARunningCodexRunUnderTheClaudeSessionThatStartedIt(t *testing.T) {
	stores, marksPath := codexRunStores(t)

	view := codexRunScanner(t, stores, marksPath, runningSeen()).Scan(context.Background(), ScanOptions{})

	want := `[{"tool":"codex","id":"` + runSeen + `","title":"Review PR 3513","state":"working","updatedAt":"2026-11-07T11:50:00Z"}]`
	if got := helpersJSON(t, byID(t, view, idSafari).Helpers); got != compactJSON(t, want) {
		t.Errorf("the run its process names the session of belongs under that session; the run no process runs nowhere:\n%s", got)
	}
	for _, c := range view.Conversations {
		if c.Tool == ToolCodex {
			t.Errorf("a Codex run is never a row of its own: %s", asJSON(t, c))
		}
	}
	if marks, _ := os.ReadFile(marksPath); strings.Contains(string(marks), "startedBy") {
		t.Errorf("the link is read from the process on every scan, never saved, actual %s", marks)
	}
}

func TestScan_ARunThatEndedLeavesItsSessionAndCannotBeOpened(t *testing.T) {
	stores, marksPath := codexRunStores(t)
	codexRunScanner(t, stores, marksPath, runningSeen()).Scan(context.Background(), ScanOptions{})

	ended := codexRunScanner(t, stores, marksPath, fakeProcesses{})
	view := ended.Scan(context.Background(), ScanOptions{})

	if helpers := byID(t, view, idSafari).Helpers; helpers != nil {
		t.Errorf("a finished run is not listed, actual %s", asJSON(t, helpers))
	}
	if _, found := ended.find(context.Background(), ToolCodex, runSeen, ScanOptions{}, true); found {
		t.Error("a run no row lists must not open")
	}
}

func TestReopen_FocusesTheProcessThatRunsAHelperInsteadOfResumingIt(t *testing.T) {
	stores, marksPath := codexRunStores(t)
	scanner := codexRunScanner(t, stores, marksPath, runningSeen())
	launcher := &fakeLauncher{}

	reply, found := scanner.Reopen(context.Background(), ToolCodex, runSeen, ScanOptions{}, launcher)

	if !found {
		t.Fatal("a running run its session lists must open")
	}
	for _, call := range launcher.calls {
		if strings.HasPrefix(call, "terminal ") || strings.HasPrefix(call, "new tab ") {
			t.Errorf("a running run must not be resumed a second time, actual calls %v", launcher.calls)
		}
	}
	if reply.Message != "Codex keeps this thread open in the background (process 77), but no Codex terminal shows it." {
		t.Errorf("reply = %q", reply.Message)
	}
	if _, found := scanner.Find(context.Background(), ToolCodex, runSeen, ScanOptions{}); found {
		t.Error("marks and renames must not reach a run: it is no row")
	}
}

func TestReopen_OpensOnlyHelpersOfChatsInTheRequestedScope(t *testing.T) {
	stores, marksPath := codexRunStores(t)
	// runUnseen runs too, but its process names no Claude Code session.
	processes := runningSeen()
	processes.held[runUnseen+".lock"] = runPID + 1
	scanner := codexRunScanner(t, stores, marksPath, processes)
	open := func(id string, within ...string) bool {
		_, found := scanner.find(context.Background(), ToolCodex, id, ScanOptions{Within: within}, true)
		return found
	}

	if open(runSeen, "/tmp/unrelated-project") {
		t.Error("a run of a chat outside the scope must not open")
	}
	if open(runUnseen, "/tmp/reviews") {
		t.Error("a run in the scope's folder that no chat lists must not open")
	}
	if !open(runSeen, "/tmp/app") {
		t.Error("a run whose chat is in scope opens, though the run works in another folder")
	}
}

func TestScan_ListsNoRunWhenItsLocksCannotBeRead(t *testing.T) {
	stores, marksPath := codexRunStores(t)
	unreadable := runningSeen()
	unreadable.readFails = true

	view := codexRunScanner(t, stores, marksPath, unreadable).Scan(context.Background(), ScanOptions{})

	if helpers := byID(t, view, idSafari).Helpers; helpers != nil {
		t.Errorf("a run that may or may not work is not listed, actual %s", asJSON(t, helpers))
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

func TestScan_SaysWhyRunsCannotBeLinked(t *testing.T) {
	stores, marksPath := codexRunStores(t)
	unreadable := runningSeen()
	unreadable.environmentFails = true

	view := codexRunScanner(t, stores, marksPath, unreadable).Scan(context.Background(), ScanOptions{})

	if helpers := byID(t, view, idSafari).Helpers; helpers != nil {
		t.Errorf("no environment, no link, actual %s", asJSON(t, helpers))
	}
	if note := view.Tools[1].Note; note != "Codex runs started from Claude Code could not be linked to their session: this system cannot read another process's environment" {
		t.Errorf("the Codex source must say why runs are not linked, actual %q", note)
	}
}

func TestScan_ReadsAMarksFileWithALinkAnOlderBuildSavedAndDropsIt(t *testing.T) {
	stores, marksPath := codexRunStores(t)
	// A build of this branch saved the runs it saw; this one ignores the field.
	write(t, marksPath, `{"schemaVersion":"conversation-marks/v1","since":"2026-10-01T00:00:00Z","marks":{`+
		`"claude:`+idSafari+`":{"seenAt":"2026-11-07T11:30:00Z","title":"Kept name"},`+
		`"codex:`+runSeen+`":{"startedBy":{"tool":"claude","id":"`+idSafari+`"}}}}`)
	scanner := codexRunScanner(t, stores, marksPath, fakeProcesses{})

	view := scanner.Scan(context.Background(), ScanOptions{})

	safari := byID(t, view, idSafari)
	if safari.Title != "Kept name" || safari.Helpers != nil {
		t.Errorf("the file reads as before and a saved link lists no run, actual %s", asJSON(t, safari))
	}
	if err := scanner.Rename(ToolClaude, idSafari, "New name"); err != nil {
		t.Fatal(err)
	}
	if marks, _ := os.ReadFile(marksPath); strings.Contains(string(marks), "startedBy") {
		t.Errorf("a write leaves the old link out, actual %s", marks)
	}
}

func TestScan_ListsNoClaudeSubagentWhenLiveSessionsCannotBeRead(t *testing.T) {
	scanner, stores := testScanner(t, nil, fakeProcesses{alive: map[int]bool{runPID: true}})
	file := filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", idSafari+".jsonl")
	write(t, file, lines(
		obj{"type": "user", "cwd": "/tmp/app", "entrypoint": "cli", "timestamp": workerTime, "message": obj{"role": "user", "content": "Review the stack"}},
	))
	write(t, filepath.Join(claudeSubagentsDir(file), "agent-explore.jsonl"), "")
	write(t, filepath.Join(claudeSubagentsDir(file), "agent-explore.meta.json"), `{"agentType":"Explore","description":"Find call sites","toolUseId":"call-explore"}`)
	appendWorkerEvent(t, file, workerDispatch("call-explore"))
	started, _ := time.Parse(time.RFC3339, workerTime)
	write(t, filepath.Join(stores.ClaudeDir, "sessions", "77.json"),
		`{"pid":77,"sessionId":"`+idSafari+`","procStart":"","status":"busy","startedAt":`+asJSON(t, started.Add(-time.Minute).UnixMilli())+`}`)

	live, _ := scanner.Find(context.Background(), ToolClaude, idSafari, ScanOptions{})
	if len(live.Helpers) != 1 || live.Helpers[0].ID != "explore" {
		t.Fatalf("a live session lists its working subagent, actual %s", asJSON(t, live.Helpers))
	}

	scanner.Processes = fakeProcesses{alive: map[int]bool{runPID: true}, readFails: true}
	unread, _ := scanner.Find(context.Background(), ToolClaude, idSafari, ScanOptions{})
	if unread.Helpers != nil {
		t.Errorf("subagents of a session whose process cannot be read are not listed, actual %s", asJSON(t, unread.Helpers))
	}
}

func TestClaudeHelpers_ListsWorkingSubagentsWithTypeAndTitle(t *testing.T) {
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
	appendWorkerEvent(t, file, workerDispatch("call-secret"))
	session := liveSession{startedAt: started.Add(-time.Minute).UnixMilli()}

	work, _ := scanner.claudeWorkOf(file, session)
	helpers := scanner.claudeHelpers(file, work, map[string]bool{})

	want := `[
  {"tool":"claude","id":"background","title":"Find rename call sites","kind":"Explore","state":"working","updatedAt":"2026-11-14T09:03:00Z"},
  {"tool":"claude","id":"inline","title":"Size budget check","kind":"general-purpose","state":"working","updatedAt":"2026-11-14T09:02:00Z"},
  {"tool":"claude","id":"secret","title":"","kind":"general-purpose","state":"working","updatedAt":"2026-11-14T09:00:00Z"}
]`
	if got := helpersJSON(t, helpers); got != compactJSON(t, want) {
		t.Errorf("working subagents only, newest first, with a title only when it holds no credential:\n%s", got)
	}
	if count := work.count(); count != len(helpers) {
		t.Errorf("the working count must agree with the listed helpers, actual %d", count)
	}
	if scanner.claudeHelpers(file, nil, map[string]bool{}) != nil {
		t.Error("unknown work must list no subagent")
	}
	if scanner.claudeHelpers(file, &claudeWork{}, map[string]bool{}) != nil {
		t.Error("a session where nothing works lists no subagent")
	}
}

func TestCodexWorkingSubagents_ListsWorkingDirectSubagentsAndFocusesTheirProcess(t *testing.T) {
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
	writeWorkerLocks(t, stores.CodexDir, child, grandchild)
	scanner.Processes = fakeProcesses{held: map[string]int{child + ".lock": 10}}
	rows := []Conversation{{ID: parent}, {ID: child}}

	_, opens := scanner.codexWorkingSubagents(context.Background(), "state.sqlite", rows, editorApps[0])

	want := `[{"tool":"codex","id":"` + child + `","title":"Aquinas","kind":"explorer","state":"working","updatedAt":"2026-11-07T11:00:00Z"}]`
	if got := helpersJSON(t, rows[0].Helpers); got != compactJSON(t, want) {
		t.Errorf("a parent lists its direct subagent, named by its nickname when it has no title:\n%s", got)
	}
	if rows[1].Helpers != nil {
		t.Errorf("a subagent no process holds has finished and is not listed, actual %s", asJSON(t, rows[1].Helpers))
	}
	if len(opens) != 1 || opens[0].Open.Kind != RouteFocus || opens[0].process == nil || opens[0].process.PID != 10 ||
		opens[0].Open.Command != "cd '/tmp/app' && codex resume "+child {
		t.Errorf("a working subagent in a terminal opens by switching to its process, actual %s", asJSON(t, opens))
	}

	scanner.Processes = fakeProcesses{readFails: true}
	rows = []Conversation{{ID: parent}}
	scanner.codexWorkingSubagents(context.Background(), "state.sqlite", rows, editorApps[0])
	if rows[0].Helpers != nil {
		t.Error("subagents whose locks cannot be read must not be listed")
	}
}

func TestClaudeParentIn_NeedsOneChildSessionAndOneSessionID(t *testing.T) {
	cases := map[string]struct {
		entries []string
		want    string
	}{
		"a command Claude Code ran":   {[]string{"PATH=/bin", "CLAUDE_CODE_SESSION_ID=" + idSafari, "CLAUDE_CODE_CHILD_SESSION=1"}, idSafari},
		"a name set twice":            {[]string{"CLAUDE_CODE_SESSION_ID=" + idCache, "CLAUDE_CODE_CHILD_SESSION=1", "CLAUDE_CODE_SESSION_ID=" + idSafari}, ""},
		"no child session":            {[]string{"CLAUDE_CODE_SESSION_ID=" + idSafari}, ""},
		"a child session set to zero": {[]string{"CLAUDE_CODE_SESSION_ID=" + idSafari, "CLAUDE_CODE_CHILD_SESSION=0"}, ""},
		"an id that is no session id": {[]string{"CLAUDE_CODE_SESSION_ID=../../etc", "CLAUDE_CODE_CHILD_SESSION=1"}, ""},
		"the names inside one value":  {[]string{"REVIEW_TEXT=example CLAUDE_CODE_SESSION_ID=" + idSafari + " CLAUDE_CODE_CHILD_SESSION=1"}, ""},
	}
	for name, tc := range cases {
		got, _ := claudeParentIn(tc.entries)
		if got != tc.want {
			t.Errorf("%s: got %q, want %q", name, got, tc.want)
		}
	}
}

func TestClaudeEntries_KeepsOnlyTheSessionNames(t *testing.T) {
	kept := keptEntries([]string{"API_TOKEN=secret", "CLAUDE_CODE_SESSION_ID=" + idSafari, "CLAUDE_CODE_CHILD_SESSION=1", "X=CLAUDE_CODE_CHILD_SESSION=1"})
	if strings.Join(kept, " ") != "CLAUDE_CODE_SESSION_ID="+idSafari+" CLAUDE_CODE_CHILD_SESSION=1" {
		t.Errorf("kept = %v", kept)
	}
}

func TestEnvironmentError_KeepsItsReasonAndCause(t *testing.T) {
	cause := errors.New("kern.procargs2 of process 77: invalid argument")
	err := error(&environmentError{reason: environmentUnreadable, cause: cause})
	var unread *environmentError
	if !errors.As(err, &unread) || unread.reason != environmentUnreadable || !errors.Is(err, cause) {
		t.Fatalf("callers branch on the reason and logs keep the cause: %v", err)
	}
	if err.Error() != "the environment of running processes could not be read: kern.procargs2 of process 77: invalid argument" {
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

// liveSubagentScanner lists the live Claude Code session idSafari, run
// by process 77 in a Terminal tab, with one working Explore subagent.
func liveSubagentScanner(t *testing.T) *Scanner {
	t.Helper()
	table := append([]ProcessInfo{{PID: 22657, PPID: 1, Stat: "S", Started: "Mon Nov 16 09:00:00 2026", Command: terminalAppPath}},
		terminalAttach("/dev/ttys004", runPID, "Mon Nov 16 16:43:20 2026")...)
	scanner, stores := testScanner(t, nil, fakeProcesses{alive: map[int]bool{runPID: true}, table: table})
	file := filepath.Join(stores.ClaudeDir, "projects", "-tmp-app", idSafari+".jsonl")
	write(t, file, lines(
		obj{"type": "user", "cwd": "/tmp/app", "entrypoint": "cli", "timestamp": workerTime, "message": obj{"role": "user", "content": "Review the stack"}},
	))
	write(t, filepath.Join(claudeSubagentsDir(file), "agent-explore.jsonl"), "")
	write(t, filepath.Join(claudeSubagentsDir(file), "agent-explore.meta.json"), `{"agentType":"Explore","description":"Find call sites","toolUseId":"call-explore"}`)
	appendWorkerEvent(t, file, workerDispatch("call-explore"))
	started, _ := time.Parse(time.RFC3339, workerTime)
	write(t, filepath.Join(stores.ClaudeDir, "sessions", "77.json"),
		`{"pid":77,"sessionId":"`+idSafari+`","procStart":"","status":"busy","startedAt":`+asJSON(t, started.Add(-time.Minute).UnixMilli())+`}`)
	return scanner
}

func TestReopenHelper_FocusesTheParentAndNamesTheSubagent(t *testing.T) {
	scanner := liveSubagentScanner(t)
	launcher := &fakeLauncher{focusOK: true}

	reply, found := scanner.ReopenHelper(context.Background(), ToolClaude, idSafari, "explore", ScanOptions{}, launcher)

	if !found || !reply.Opened {
		t.Fatalf("a subagent its chat lists opens that chat, actual %+v found %v, calls %v", reply, found, launcher.calls)
	}
	if !slices.ContainsFunc(launcher.calls, func(call string) bool { return strings.HasPrefix(call, "focus ") }) {
		t.Errorf("the parent's tab must come to the front, actual calls %v", launcher.calls)
	}
	want := "Switched to the Terminal tab that runs it. Its subagent is 'Find call sites'. Use the agent rows below the prompt to switch to it."
	if reply.Message != want {
		t.Errorf("reply = %q", reply.Message)
	}
	if reply.Helper == nil || reply.Helper.ID != "explore" || reply.Helper.Title != "Find call sites" {
		t.Errorf("the reply names the subagent that was meant, actual %s", asJSON(t, reply.Helper))
	}
}

func TestReopenHelper_RefusesASubagentTheParentDoesNotList(t *testing.T) {
	scanner := liveSubagentScanner(t)
	launcher := &fakeLauncher{}

	reply, found := scanner.ReopenHelper(context.Background(), ToolClaude, idSafari, "finished", ScanOptions{}, launcher)

	if !found || reply.Opened || reply.Helper != nil {
		t.Errorf("an unknown subagent is refused, actual %+v found %v", reply, found)
	}
	if len(launcher.calls) != 0 {
		t.Errorf("a refused subagent focuses nothing, actual calls %v", launcher.calls)
	}
}

func TestValidAgentID(t *testing.T) {
	for _, id := range []string{"explore", "a00045583c3432f5c", "aside_question-1"} {
		if !ValidAgentID(id) {
			t.Errorf("%q is an agent id", id)
		}
	}
	for _, id := range []string{"", "../x", "a b", "x/y", strings.Repeat("a", 129)} {
		if ValidAgentID(id) {
			t.Errorf("%q is not an agent id", id)
		}
	}
}

// heldFocus holds every tab switch until release closes, and counts them.
type heldFocus struct {
	fakeLauncher
	focuses atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (l *heldFocus) FocusTab(context.Context, string, string) (bool, error) {
	l.focuses.Add(1)
	l.started <- struct{}{}
	<-l.release
	return true, nil
}

func TestReopenHelper_OpensOfOneChatShareOneOpenAndEachGetsItsOwnReply(t *testing.T) {
	scanner := liveSubagentScanner(t)
	launcher := &heldFocus{started: make(chan struct{}, 4), release: make(chan struct{})}
	open := func(subagent string) <-chan openAnswer {
		answered := make(chan openAnswer, 1)
		go func() {
			reply, found := scanner.ReopenHelper(context.Background(), ToolClaude, idSafari, subagent, ScanOptions{}, launcher)
			answered <- openAnswer{reply, found}
		}()
		return answered
	}

	first := open("")
	<-launcher.started
	picked, gone := open("explore"), open("finished")
	time.Sleep(sharedWait)
	close(launcher.release)

	if got := answerOf(t, first); !got.reply.Opened || got.reply.Helper != nil {
		t.Errorf("the chat's own open names no subagent: %+v", got.reply)
	}
	if got := answerOf(t, picked); got.reply.Helper == nil || !strings.Contains(got.reply.Message, "'Find call sites'") {
		t.Errorf("an open that joined names its subagent: %+v", got.reply)
	}
	if got := answerOf(t, gone); got.reply.Opened || got.reply.Message != subagentGone {
		t.Errorf("an unknown subagent is refused even when it joins an open: %+v", got.reply)
	}
	if n := launcher.focuses.Load(); n != 1 {
		t.Errorf("opens of one chat share one tab switch, actual %d", n)
	}
}

func TestSubagentReply_KeepsTheOpenMessageAndPointsAtTheAgentRowsOnlyInATerminal(t *testing.T) {
	picked := helper{Tool: ToolClaude, ID: "explore", Title: "Find call sites", Kind: "Explore"}
	inTerminal := Conversation{App: AppCLI, Open: Route{Kind: RouteFocus}}
	for name, tc := range map[string]struct {
		conversation Conversation
		picked       helper
		want         string
	}{
		"a terminal tab": {inTerminal, picked,
			"Opened. Its subagent is 'Find call sites'. Use the agent rows below the prompt to switch to it."},
		"an editor extension": {Conversation{App: AppIDE, Open: Route{Kind: RouteLink}}, picked,
			"Opened. Its subagent is 'Find call sites'."},
		"a background session": {Conversation{App: AppCLI, Open: Route{Kind: RouteFocus}, placed: &placement{claudeBackground: true}}, picked,
			"Opened. Its subagent is 'Find call sites'."},
		"a subagent with only a type": {inTerminal, helper{Tool: ToolClaude, ID: "x", Kind: "Explore"},
			"Opened. Its subagent is 'Explore'. Use the agent rows below the prompt to switch to it."},
		"a subagent with no name": {inTerminal, helper{Tool: ToolClaude, ID: "x"},
			"Opened. Use the agent rows below the prompt to switch to its subagent."},
	} {
		t.Run(name, func(t *testing.T) {
			reply := subagentReply(OpenReply{Opened: true, Message: "Opened."}, tc.conversation, tc.picked)
			if reply.Message != tc.want || reply.Helper == nil || reply.Helper.ID != tc.picked.ID {
				t.Errorf("reply = %q, helper %v", reply.Message, reply.Helper)
			}
		})
	}
}

func TestClaudeHelpers_CarryTheSubagentsLatestMessageAsItsReply(t *testing.T) {
	scanner, stores := testScanner(t, nil, fakeProcesses{})
	file := filepath.Join(stores.ClaudeDir, "projects", "demo", idSafari+".jsonl")
	dir := claudeSubagentsDir(file)
	write(t, file, "")
	own := func(line obj) obj { line["isSidechain"] = true; return line }
	transcripts := map[string][]any{
		"talks":  {own(typedPrompt("Find the call sites")), own(agentLine(textPart("Looking in internal/cli first."))), own(agentLine(obj{"type": "tool_use", "id": "t1", "name": "Grep"})), own(toolResultLine("12 matches"))},
		"secret": {own(typedPrompt("Log in")), own(agentLine(textPart("I logged in with password hunter2.")))},
		"silent": {own(typedPrompt("Size the budget")), own(agentLine(obj{"type": "tool_use", "id": "t2", "name": "Read"}))},
	}
	for id, transcript := range transcripts {
		write(t, filepath.Join(dir, "agent-"+id+".jsonl"), lines(transcript...))
		write(t, filepath.Join(dir, "agent-"+id+".meta.json"), `{"agentType":"Explore","description":"`+id+`","toolUseId":"call-`+id+`"}`)
		appendWorkerEvent(t, file, workerDispatch("call-"+id))
	}
	started, _ := time.Parse(time.RFC3339, workerTime)
	work, _ := scanner.claudeWorkOf(file, liveSession{startedAt: started.Add(-time.Minute).UnixMilli()})

	replies := map[string]string{}
	for _, h := range scanner.claudeHelpers(file, work, map[string]bool{}) {
		replies[h.ID] = h.Reply
	}

	want := map[string]string{"talks": "Looking in internal/cli first.", "secret": "", "silent": ""}
	if !maps.Equal(replies, want) {
		t.Errorf("each subagent's latest message, withheld when it may hold a credential, none before it wrote one: %v", replies)
	}
}

func TestClaudeAgentReply_KeepsItsLineThroughALaterMessageAndRereadsOnlyAChangedFile(t *testing.T) {
	scanner, _ := testScanner(t, nil, fakeProcesses{})
	path := filepath.Join(t.TempDir(), "agent-a.jsonl")
	own := func(line obj) obj { line["isSidechain"] = true; return line }
	teammate := own(typedPrompt("<teammate-message teammate_id=\"lead\">Also check the docs.</teammate-message>"))
	write(t, path, lines(own(typedPrompt("Find the call sites")), own(agentLine(textPart("Twelve so far."))), teammate))

	if got := scanner.claudeAgentReplyOf(path); got != "Twelve so far." {
		t.Errorf("a message to the subagent keeps its latest line, actual %q", got)
	}
	scanner.claudeAgentReplies[path] = cached[string]{stamp: fileStamp(path), value: "kept"}
	if got := scanner.claudeAgentReplyOf(path); got != "kept" {
		t.Errorf("an unchanged transcript is not read again, actual %q", got)
	}
	write(t, path, lines(own(agentLine(textPart("Done: fourteen.")))))
	if got := scanner.claudeAgentReplyOf(path); got != "Done: fourteen." {
		t.Errorf("a changed transcript is read again, actual %q", got)
	}
}

func TestReadClaudeAgentReply_ReadsBackPastALargeToolResultWithinItsBound(t *testing.T) {
	own := func(line obj) obj { line["isSidechain"] = true; return line }
	big := func(kb int) obj {
		return own(obj{"type": "user", "message": obj{"role": "user", "content": []obj{{"type": "tool_result", "tool_use_id": "t", "content": strings.Repeat("x", kb<<10)}}}})
	}
	for name, tc := range map[string]struct {
		transcript []any
		want       string
	}{
		"a tool result past the first window": {[]any{own(agentLine(textPart("Twelve so far."))), big(100)}, "Twelve so far."},
		"a message past the bound":            {[]any{own(agentLine(textPart("Twelve so far."))), big(300)}, ""},
		"a tool call whose input holds a tool result shape": {[]any{
			own(agentLine(textPart("old"))),
			own(agentLine(textPart("new"), obj{"type": "tool_use", "id": "t", "name": "Write", "input": obj{"block": obj{"type": "tool_result"}}})),
		}, "new"},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "agent-a.jsonl")
			write(t, path, lines(tc.transcript...))
			if got := readClaudeAgentReply(path); got != tc.want {
				t.Errorf("reply = %q, want %q", got, tc.want)
			}
		})
	}
}
