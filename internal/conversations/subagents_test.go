package conversations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

const workerTime = "2026-11-14T09:00:00Z"

func appendWorkerEvent(t *testing.T, path string, event obj) {
	t.Helper()
	if _, set := event["timestamp"]; !set {
		event["timestamp"] = workerTime
	}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func workerDispatch(id string) obj {
	return obj{"type": "assistant", "message": obj{"content": []obj{{"type": "tool_use", "name": "Agent", "id": id}}}}
}

func workerResult(call string, result obj) obj {
	return obj{"type": "user", "toolUseResult": result, "message": obj{"content": []obj{{"type": "tool_result", "tool_use_id": call}}}}
}

func TestClaudeWorkingSubagents_FollowsLaunchCompletionStopAndResume(t *testing.T) {
	scanner, stores := testScanner(t, nil, fakeProcesses{})
	file := filepath.Join(stores.ClaudeDir, "projects", "demo", idSafari+".jsonl")
	write(t, file, "")
	started, _ := time.Parse(time.RFC3339, workerTime)
	session := liveSession{startedAt: started.Add(-time.Minute).UnixMilli()}
	check := func(want int) {
		t.Helper()
		count, _ := scanner.claudeWorkingSubagents(file, session)
		if count == nil || *count != want {
			t.Fatalf("working = %v, want %d", count, want)
		}
	}
	appendWorkerEvent(t, file, workerDispatch("first"))
	check(1)
	appendWorkerEvent(t, file, workerResult("first", obj{"status": "async_launched", "agentId": "agent-a"}))
	check(1)
	// Ordinary tool results are allowed to be strings or arrays and do not
	// participate in the native task lifecycle.
	appendWorkerEvent(t, file, obj{"type": "user", "toolUseResult": "ordinary output", "message": obj{"content": []obj{{"type": "tool_result", "tool_use_id": "other"}}}})
	check(1)
	appendWorkerEvent(t, file, workerDispatch("second"))
	check(2)
	appendWorkerEvent(t, file, workerResult("second", obj{"status": "completed", "agentId": "agent-b"}))
	check(1)
	appendWorkerEvent(t, file, obj{"type": "queue-operation", "operation": "enqueue", "content": "<task-notification><task-id>agent-a</task-id><status>completed</status></task-notification>"})
	check(0)
	appendWorkerEvent(t, file, obj{"type": "assistant", "message": obj{"content": []obj{{"type": "tool_use", "name": "SendMessage", "id": "message"}}}})
	appendWorkerEvent(t, file, workerResult("message", obj{"success": true, "resumedAgentId": "agent-a"}))
	check(1)
	appendWorkerEvent(t, file, obj{"type": "assistant", "message": obj{"content": []obj{{"type": "tool_use", "name": "TaskStop", "id": "stop"}}}})
	appendWorkerEvent(t, file, workerResult("stop", obj{"task_id": "agent-a"}))
	check(0)
	// A new process cannot inherit tasks from the process that ended.
	appendWorkerEvent(t, file, workerDispatch("unfinished-before-restart"))
	session.startedAt = started.Add(time.Minute).UnixMilli()
	check(0)
}

func TestClaudeWorkingSubagents_UnknownAndTruncatedLogsDoNotInventWorkers(t *testing.T) {
	scanner, stores := testScanner(t, nil, fakeProcesses{})
	file := filepath.Join(stores.ClaudeDir, "projects", "demo", idSafari+".jsonl")
	write(t, file, "")
	started, _ := time.Parse(time.RFC3339, workerTime)
	if count, _ := scanner.claudeWorkingSubagents(file, liveSession{}); count != nil {
		t.Fatal("missing process start time became a known count")
	}
	session := liveSession{startedAt: started.Add(-time.Minute).UnixMilli()}
	appendWorkerEvent(t, file, workerDispatch("unfinished"))
	count, _ := scanner.claudeWorkingSubagents(file, session)
	if count == nil || *count != 1 {
		t.Fatal("dispatch was not counted")
	}
	write(t, file, "not-json\n")
	if count, _ := scanner.claudeWorkingSubagents(file, session); count != nil {
		t.Fatal("unreadable events became a known count")
	}
	if count, _ := scanner.claudeWorkingSubagents(file, session); count != nil {
		t.Fatal("retrying unreadable events became a known count")
	}
	write(t, file, "")
	if count, _ := scanner.claudeWorkingSubagents(file, session); count == nil || *count != 0 {
		t.Fatal("truncated history retained a worker")
	}
}

func TestCodexWorkingSubagents_CountsLiveDescendantsAndObservesCompletion(t *testing.T) {
	scanner, stores := testScanner(t, nil, fakeProcesses{})
	parent, child, grandchild := idSafari, idCache, idSide
	first := filepath.Join(stores.CodexDir, "sessions", "child.jsonl")
	second := filepath.Join(stores.CodexDir, "sessions", "grandchild.jsonl")
	write(t, first, "")
	write(t, second, "")
	appendWorkerEvent(t, first, obj{"type": "event_msg", "payload": obj{"type": "task_started"}})
	appendWorkerEvent(t, second, obj{"type": "event_msg", "payload": obj{"type": "task_started"}})
	worker := func(id, parent, path string) json.RawMessage {
		source, _ := json.Marshal(obj{"subagent": obj{"thread_spawn": obj{"parent_thread_id": parent}}})
		row, _ := json.Marshal(obj{"id": id, "source": string(source), "rollout_path": path})
		return row
	}
	scanner.SQLite = func(context.Context, string, string) ([]json.RawMessage, error) {
		return []json.RawMessage{worker(child, parent, first), worker(grandchild, child, second)}, nil
	}
	writeWorkerLocks(t, stores.CodexDir, child, grandchild)
	scanner.Processes = fakeProcesses{held: map[string]int{child + ".lock": 10, grandchild + ".lock": 10}}
	rows := []Conversation{{ID: parent}}
	check := func(want int) {
		t.Helper()
		watched, _ := scanner.codexWorkingSubagents(context.Background(), "state.sqlite", rows, editorApps[0])
		if rows[0].WorkingSubagents == nil || *rows[0].WorkingSubagents != want || len(watched) != 2 {
			t.Fatalf("working = %v, watched = %v, want %d and two rollouts", rows[0].WorkingSubagents, watched, want)
		}
	}
	check(2)
	appendWorkerEvent(t, second, obj{"type": "event_msg", "payload": obj{"type": "task_complete"}})
	check(1)
	appendWorkerEvent(t, first, obj{"type": "event_msg", "payload": obj{"type": "turn_aborted"}})
	check(0)
	scanner.Processes = fakeProcesses{}
	scanner.codexWorkingSubagents(context.Background(), "state.sqlite", rows, editorApps[0])
	if *rows[0].WorkingSubagents != 0 {
		t.Fatal("dead process retained working children")
	}
	rows[0].WorkingSubagents = nil
	scanner.SQLite = func(context.Context, string, string) ([]json.RawMessage, error) {
		return nil, errors.New("unavailable")
	}
	scanner.codexWorkingSubagents(context.Background(), "state.sqlite", rows, editorApps[0])
	if rows[0].WorkingSubagents != nil {
		t.Fatal("an unavailable index became a known count")
	}
}

func writeWorkerLocks(t *testing.T, codexDir string, ids ...string) {
	t.Helper()
	for _, id := range ids {
		write(t, filepath.Join(codexDir, "thread-writer-locks", id+".lock"), "")
	}
}

func codexWorkerRow(id, parent, rollout string) json.RawMessage {
	source, _ := json.Marshal(obj{"subagent": obj{"thread_spawn": obj{"parent_thread_id": parent}}})
	row, _ := json.Marshal(obj{"id": id, "source": string(source), "rollout_path": rollout})
	return row
}

func TestCodexWorkingSubagents_AWorkerWithoutALockFileDoesNotHideTheCount(t *testing.T) {
	scanner, stores := testScanner(t, nil, fakeProcesses{})
	parent, live, historical := idSafari, idCache, idSide
	rollout := filepath.Join(stores.CodexDir, "sessions", "live.jsonl")
	write(t, rollout, "")
	appendWorkerEvent(t, rollout, obj{"type": "event_msg", "payload": obj{"type": "task_started"}})
	scanner.SQLite = func(context.Context, string, string) ([]json.RawMessage, error) {
		return []json.RawMessage{codexWorkerRow(live, parent, rollout), codexWorkerRow(historical, parent, rollout)}, nil
	}
	writeWorkerLocks(t, stores.CodexDir, live)
	scanner.Processes = fakeProcesses{held: map[string]int{live + ".lock": 10}, missingFails: true}
	rows := []Conversation{{ID: parent}}
	scanner.codexWorkingSubagents(context.Background(), "state.sqlite", rows, editorApps[0])
	if rows[0].WorkingSubagents == nil || *rows[0].WorkingSubagents != 1 {
		t.Fatalf("working = %v, want 1 despite a worker whose lock file is gone", rows[0].WorkingSubagents)
	}
	rows[0].WorkingSubagents = nil
	scanner.Processes = fakeProcesses{readFails: true}
	scanner.codexWorkingSubagents(context.Background(), "state.sqlite", rows, editorApps[0])
	if rows[0].WorkingSubagents != nil {
		t.Fatal("a failed process read became a known count")
	}
}

func TestCodexWorkingSubagents_WatchesRolloutsInAStableOrder(t *testing.T) {
	scanner, stores := testScanner(t, nil, fakeProcesses{})
	parent := idSafari
	var workers []json.RawMessage
	held := map[string]int{}
	for i := range 12 {
		id := fmt.Sprintf("aaaaaaaa-aaaa-4aaa-8aaa-%012d", i)
		rollout := filepath.Join(stores.CodexDir, "sessions", id+".jsonl")
		write(t, rollout, "")
		appendWorkerEvent(t, rollout, obj{"type": "event_msg", "payload": obj{"type": "task_started"}})
		writeWorkerLocks(t, stores.CodexDir, id)
		workers = append(workers, codexWorkerRow(id, parent, rollout))
		held[id+".lock"] = 10
	}
	scanner.SQLite = func(context.Context, string, string) ([]json.RawMessage, error) {
		return workers, nil
	}
	scanner.Processes = fakeProcesses{held: held}
	rows := []Conversation{{ID: parent}}
	first, _ := scanner.codexWorkingSubagents(context.Background(), "state.sqlite", rows, editorApps[0])
	for range 30 {
		again, _ := scanner.codexWorkingSubagents(context.Background(), "state.sqlite", rows, editorApps[0])
		if !slices.Equal(first, again) {
			t.Fatalf("watched rollouts changed order between unchanged scans:\n%v\n%v", first, again)
		}
	}
	if len(first) != 12 || *rows[0].WorkingSubagents != 12 {
		t.Fatalf("watched %d rollouts and counted %v, want 12 of each", len(first), rows[0].WorkingSubagents)
	}
}

func TestWorkingSubagents_KeepAnIdleParentWorkingWithoutHidingItsQuestion(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, workerTime)
	count := 2
	row := Conversation{Live: true, Activity: ActivityIdle, UpdatedAt: workerTime, WorkingSubagents: &count}
	if stateOf(row, markEntry{CompletedAt: workerTime}, time.Time{}, now) != StateWorking {
		t.Fatal("working native children were hidden behind the parent's completed mark")
	}
	row.Activity = ActivityWaiting
	if stateOf(row, markEntry{}, time.Time{}, now) != StateBlocked {
		t.Fatal("working native children hid their parent's question")
	}
	count = 0
	row.Activity = ActivityIdle
	if workingNow(row) {
		t.Fatal("zero native workers kept the parent working")
	}
}

func TestClaudeWorkingSubagents_AParentCanStopAndResumeANestedWorker(t *testing.T) {
	scanner, stores := testScanner(t, nil, fakeProcesses{})
	file := filepath.Join(stores.ClaudeDir, "projects", "demo", idSafari+".jsonl")
	child := filepath.Join(stores.ClaudeDir, "projects", "demo", idSafari, "subagents", "agent-child.jsonl")
	write(t, file, "")
	write(t, child, "")
	started, _ := time.Parse(time.RFC3339, workerTime)
	session := liveSession{startedAt: started.Add(-time.Minute).UnixMilli()}
	appendWorkerEvent(t, file, workerDispatch("root-call"))
	appendWorkerEvent(t, file, workerResult("root-call", obj{"status": "async_launched", "agentId": "child"}))
	appendWorkerEvent(t, child, workerDispatch("nested-call"))
	appendWorkerEvent(t, child, workerResult("nested-call", obj{"status": "async_launched", "agentId": "grandchild"}))
	// The fixture clock is independent of the machine running this test.
	if err := os.Chtimes(child, started, started); err != nil {
		t.Fatal(err)
	}
	check := func(want int) {
		t.Helper()
		count, _ := scanner.claudeWorkingSubagents(file, session)
		if count == nil {
			t.Fatal("working count is unknown")
		}
		if *count != want {
			t.Fatalf("working = %d, want %d", *count, want)
		}
	}
	check(2)
	appendWorkerEvent(t, file, obj{"type": "assistant", "message": obj{"content": []obj{{"type": "tool_use", "name": "TaskStop", "id": "stop-nested"}}}})
	stopped := workerResult("stop-nested", obj{"task_id": "grandchild"})
	stopped["timestamp"] = started.Add(time.Second).Format(time.RFC3339)
	appendWorkerEvent(t, file, stopped)
	check(1)
	appendWorkerEvent(t, file, obj{"type": "assistant", "message": obj{"content": []obj{{"type": "tool_use", "name": "SendMessage", "id": "resume-nested"}}}})
	resumed := workerResult("resume-nested", obj{"success": true, "resumedAgentId": "grandchild"})
	resumed["timestamp"] = started.Add(2 * time.Second).Format(time.RFC3339)
	appendWorkerEvent(t, file, resumed)
	check(2)
}

// A subagent's transcript has no toolUseResult. These lines keep the shapes
// Claude Code 2.1.321 writes there, with their content redacted.
func sidechainDispatch(name, id string) obj {
	return obj{"type": "assistant", "isSidechain": true, "message": obj{"content": []obj{{"type": "tool_use", "name": name, "id": id, "input": obj{}}}}}
}

func sidechainResult(call string, content any) obj {
	return obj{"type": "user", "isSidechain": true, "message": obj{"role": "user", "content": []obj{{"tool_use_id": call, "type": "tool_result", "content": content}}}}
}

func textBlocks(texts ...string) []obj {
	blocks := make([]obj, 0, len(texts))
	for _, text := range texts {
		blocks = append(blocks, obj{"type": "text", "text": text})
	}
	return blocks
}

func TestClaudeWorkingSubagents_ReadsSubagentsThatStartOrStopOtherAgents(t *testing.T) {
	scanner, stores := testScanner(t, nil, fakeProcesses{})
	file := filepath.Join(stores.ClaudeDir, "projects", "demo", idSafari+".jsonl")
	child := filepath.Join(stores.ClaudeDir, "projects", "demo", idSafari, "subagents", "agent-child.jsonl")
	fork := filepath.Join(stores.ClaudeDir, "projects", "demo", idSafari, "subagents", "agent-fork.jsonl")
	write(t, file, "")
	write(t, child, "")
	write(t, fork, "")
	started, _ := time.Parse(time.RFC3339, workerTime)
	session := liveSession{startedAt: started.Add(-time.Minute).UnixMilli()}
	appendWorkerEvent(t, file, workerDispatch("root-call"))
	appendWorkerEvent(t, file, workerResult("root-call", obj{"status": "async_launched", "agentId": "child"}))
	appendWorkerEvent(t, file, workerDispatch("fork-call"))
	appendWorkerEvent(t, file, workerResult("fork-call", obj{"status": "async_launched", "agentId": "fork"}))
	appendWorkerEvent(t, fork, obj{"type": "fork-context-ref"})
	appendWorkerEvent(t, fork, sidechainDispatch("Agent", "fork-call"))
	appendWorkerEvent(t, fork, sidechainResult("fork-call", textBlocks("Fork started — that fork is you.")))
	touch := func(path string) {
		t.Helper()
		// The fixture clock is independent of the machine running this test.
		if err := os.Chtimes(path, started, started); err != nil {
			t.Fatal(err)
		}
	}
	touch(fork)
	check := func(want int) {
		t.Helper()
		touch(child)
		count, _ := scanner.claudeWorkingSubagents(file, session)
		if count == nil {
			t.Fatal("working count is unknown")
		}
		if *count != want {
			t.Fatalf("working = %d, want %d", *count, want)
		}
	}
	check(2)
	appendWorkerEvent(t, child, sidechainDispatch("Agent", "nested-agent"))
	check(3)
	appendWorkerEvent(t, child, sidechainResult("nested-agent", textBlocks(
		"  This agent's report was delivered to you as a message from \"grandchild\" (its SubagentHandback call). Read it there; it is not repeated here.",
		"[Harness note, redacted]",
	)))
	check(2)
	appendWorkerEvent(t, child, sidechainDispatch("SendMessage", "nested-resume"))
	appendWorkerEvent(t, child, sidechainResult("nested-resume", textBlocks(`{"success":true,"message":"Resuming agent grandc","resumedAgentId":"grandchild"}`)))
	check(3)
	appendWorkerEvent(t, child, sidechainDispatch("SendMessage", "nested-report"))
	appendWorkerEvent(t, child, sidechainResult("nested-report", textBlocks(`{"success":true,"message":"Message queued for the main conversation's next turn."}`)))
	check(3)
	appendWorkerEvent(t, child, sidechainDispatch("TaskStop", "nested-stop"))
	stopped := sidechainResult("nested-stop", `{"message":"Successfully stopped task: grandchild (redacted)","task_id":"grandchild","task_type":"local_agent","command":"redacted"}`)
	stopped["timestamp"] = started.Add(time.Second).Format(time.RFC3339)
	appendWorkerEvent(t, child, stopped)
	check(2)
}

func TestClaudeWorkingSubagents_UnreadableSubagentResultsKeepTheCountUnknown(t *testing.T) {
	started, _ := time.Parse(time.RFC3339, workerTime)
	session := liveSession{startedAt: started.Add(-time.Minute).UnixMilli()}
	for name, lines := range map[string][]obj{
		"async launch without metadata": {
			sidechainDispatch("Agent", "call"),
			sidechainResult("call", textBlocks("Async agent launched successfully. (redacted)")),
		},
		"stop result that is not JSON": {
			sidechainDispatch("TaskStop", "call"),
			sidechainResult("call", "Stopped (redacted)"),
		},
		"message result without a text block": {
			sidechainDispatch("SendMessage", "call"),
			sidechainResult("call", []obj{{"type": "image"}}),
		},
		"main transcript result without metadata": {
			workerDispatch("call"),
			{"type": "user", "message": obj{"content": []obj{{"type": "tool_result", "tool_use_id": "call", "content": "redacted"}}}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			scanner, stores := testScanner(t, nil, fakeProcesses{})
			file := filepath.Join(stores.ClaudeDir, "projects", "demo", idSafari+".jsonl")
			child := filepath.Join(stores.ClaudeDir, "projects", "demo", idSafari, "subagents", "agent-child.jsonl")
			write(t, file, "")
			write(t, child, "")
			target := child
			if name == "main transcript result without metadata" {
				target = file
			}
			for _, line := range lines {
				appendWorkerEvent(t, target, line)
			}
			if err := os.Chtimes(child, started, started); err != nil {
				t.Fatal(err)
			}
			if count, _ := scanner.claudeWorkingSubagents(file, session); count != nil {
				t.Fatalf("working = %d, want unknown", *count)
			}
		})
	}
}
