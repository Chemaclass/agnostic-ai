package conversations

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Native task events are read only for live parents or children. Keep a cursor
// so a long-running session does not reread its history every five seconds.
type nativeLog[T any] struct {
	info   os.FileInfo
	offset int64
	value  T
}

func readNativeLog[T any](path string, kept *nativeLog[T], fresh func() T, visit func(*T, []byte) error) (*nativeLog[T], error) {
	file, err := os.Open(path)
	if err != nil {
		return kept, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return kept, err
	}
	if kept == nil || !os.SameFile(kept.info, info) || info.Size() < kept.offset || (info.Size() == kept.offset && !info.ModTime().Equal(kept.info.ModTime())) {
		kept = &nativeLog[T]{info: info, value: fresh()}
	}
	reader := bufio.NewScanner(io.NewSectionReader(file, kept.offset, info.Size()-kept.offset))
	reader.Buffer(make([]byte, 64<<10), 8<<20)
	reader.Split(func(data []byte, _ bool) (int, []byte, error) {
		if at := bytes.IndexByte(data, '\n'); at >= 0 {
			return at + 1, data[:at+1], nil
		}
		return 0, nil, nil // A partial final line waits for its next write.
	})
	for reader.Scan() {
		line := reader.Bytes()
		if err := visit(&kept.value, line); err != nil {
			return nil, err
		}
		kept.offset += int64(len(line))
	}
	if err := reader.Err(); err != nil {
		return nil, err
	}
	kept.info = info
	return kept, nil
}

type claudeWorkerEvents struct {
	tasks   map[string]nativeTaskEvent
	pending map[string]bool
	calls   map[string]string
}

type nativeTaskEvent struct {
	working bool
	at      time.Time
}

func newClaudeWorkerEvents() claudeWorkerEvents {
	return claudeWorkerEvents{tasks: map[string]nativeTaskEvent{}, pending: map[string]bool{}, calls: map[string]string{}}
}

// Claude Code records Agent/Task dispatches, async launches, task notifications,
// successful stops and resumed agents in its own transcript. No prompt text or
// worker output is returned to the client.
func (events *claudeWorkerEvents) visit(line []byte, startedAt int64) error {
	var event struct {
		Type      string `json:"type"`
		Timestamp string `json:"timestamp"`
		Operation string `json:"operation"`
		Content   string `json:"content"`
		Message   struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
		Result json.RawMessage `json:"toolUseResult"`
	}
	if err := json.Unmarshal(line, &event); err != nil {
		return err
	}
	if event.Type != "assistant" && event.Type != "user" && event.Type != "queue-operation" {
		return nil
	}
	when, err := time.Parse(time.RFC3339Nano, event.Timestamp)
	if err != nil || when.UnixMilli() < startedAt {
		return nil
	}
	if event.Type == "queue-operation" && event.Operation == "enqueue" && strings.HasPrefix(event.Content, "<task-notification>") {
		id := notificationField(event.Content, "task-id")
		switch notificationField(event.Content, "status") {
		case "completed", "failed", "killed", "stopped":
			events.tasks[id] = nativeTaskEvent{at: when}
			delete(events.pending, notificationField(event.Content, "tool-use-id"))
		}
	}
	var blocks []struct {
		Type      string `json:"type"`
		Name      string `json:"name"`
		ID        string `json:"id"`
		ToolUseID string `json:"tool_use_id"`
		IsError   bool   `json:"is_error"`
	}
	if len(event.Message.Content) == 0 || event.Message.Content[0] != '[' {
		return nil
	}
	if err := json.Unmarshal(event.Message.Content, &blocks); err != nil {
		return err
	}
	for _, block := range blocks {
		if event.Type == "assistant" && block.Type == "tool_use" {
			switch block.Name {
			case "Agent", "Task":
				events.pending[block.ID] = true
				events.calls[block.ID] = block.Name
			case "TaskStop", "SendMessage":
				events.calls[block.ID] = block.Name
			}
		}
		if event.Type != "user" || block.Type != "tool_result" {
			continue
		}
		wasAgent := events.pending[block.ToolUseID]
		called := events.calls[block.ToolUseID]
		delete(events.pending, block.ToolUseID)
		delete(events.calls, block.ToolUseID)
		if block.IsError {
			continue
		}
		if called == "" {
			continue
		}
		var result struct {
			AgentID        string `json:"agentId"`
			Status         string `json:"status"`
			TaskID         string `json:"task_id"`
			Success        bool   `json:"success"`
			ResumedAgentID string `json:"resumedAgentId"`
			RevivedAgentID string `json:"revivedAgentId"`
		}
		if len(event.Result) == 0 || event.Result[0] != '{' {
			return errors.New("native task result has no activity metadata")
		}
		if err := json.Unmarshal(event.Result, &result); err != nil {
			return err
		}
		if wasAgent && result.Status == "async_launched" && result.AgentID != "" {
			events.tasks[result.AgentID] = nativeTaskEvent{working: true, at: when}
		} else if wasAgent && result.AgentID != "" {
			events.tasks[result.AgentID] = nativeTaskEvent{at: when}
		}
		if called == "TaskStop" && result.TaskID != "" {
			events.tasks[result.TaskID] = nativeTaskEvent{at: when}
		}
		if called == "SendMessage" && result.Success {
			for _, id := range []string{result.ResumedAgentID, result.RevivedAgentID} {
				if id != "" {
					events.tasks[id] = nativeTaskEvent{working: true, at: when}
				}
			}
		}
	}
	return nil
}

func notificationField(content, name string) string {
	_, after, found := strings.Cut(content, "<"+name+">")
	if !found {
		return ""
	}
	value, _, found := strings.Cut(after, "</"+name+">")
	if !found {
		return ""
	}
	return value
}

type claudeWorkerCache struct {
	startedAt int64
	logs      map[string]*nativeLog[claudeWorkerEvents]
}

func (s *Scanner) claudeWorkingSubagents(file string, session liveSession) (*int, []string) {
	dir := strings.TrimSuffix(file, ".jsonl") + string(filepath.Separator) + "subagents"
	files := []string{file}
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, []string{dir}
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "agent-") && strings.HasSuffix(entry.Name(), ".jsonl") {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	watch := append([]string{dir}, files...)
	// Without a process start time, old interrupted tasks cannot be separated
	// from this process's tasks. Keep the count unknown.
	if session.startedAt == 0 {
		return nil, watch
	}
	if s.claudeWorkers == nil {
		s.claudeWorkers = map[string]*claudeWorkerCache{}
	}
	kept := s.claudeWorkers[file]
	if kept == nil || kept.startedAt != session.startedAt {
		kept = &claudeWorkerCache{startedAt: session.startedAt, logs: map[string]*nativeLog[claudeWorkerEvents]{}}
		s.claudeWorkers[file] = kept
	}
	tasks := map[string]nativeTaskEvent{}
	pending := 0
	for _, path := range files {
		if path != file {
			info, err := os.Stat(path)
			if err != nil {
				return nil, watch
			}
			if info.ModTime().UnixMilli() < session.startedAt {
				continue
			}
		}
		log, err := readNativeLog(path, kept.logs[path], newClaudeWorkerEvents, func(events *claudeWorkerEvents, line []byte) error {
			return events.visit(line, session.startedAt)
		})
		kept.logs[path] = log
		if err != nil {
			return nil, watch
		}
		pending += len(log.value.pending)
		for id, event := range log.value.tasks {
			previous, found := tasks[id]
			if !found || event.at.After(previous.at) || (event.at.Equal(previous.at) && !event.working) {
				tasks[id] = event
			}
		}
	}
	count := pending
	for _, event := range tasks {
		if event.working {
			count++
		}
	}
	return &count, watch
}

type codexWorker struct {
	ID      string `json:"id"`
	Source  string `json:"source"`
	Rollout string `json:"rollout_path"`
}

const codexWorkersQuery = `SELECT id, source, rollout_path FROM threads WHERE thread_source = 'subagent' AND archived = 0`

type codexWorkerTurn struct {
	running bool
	known   bool
}

func codexWorkerEvent(turn *codexWorkerTurn, line []byte) error {
	var event struct {
		Type    string `json:"type"`
		Payload struct {
			Type string `json:"type"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(line, &event); err != nil {
		return err
	}
	if event.Type == "event_msg" {
		if running, known := codexTurnEvents[event.Payload.Type]; known {
			turn.running, turn.known = running, true
		}
	}
	return nil
}

func (s *Scanner) codexWorkingSubagents(ctx context.Context, statePath string, conversations []Conversation) []string {
	workers, err := queryRows[codexWorker](ctx, s.SQLite, statePath, codexWorkersQuery)
	if err != nil {
		return nil
	}
	parents := map[string]string{}
	byID := map[string]codexWorker{}
	locks := map[string]string{}
	var paths []string
	for _, worker := range workers {
		var source struct {
			Subagent struct {
				Spawn struct {
					ParentID string `json:"parent_thread_id"`
				} `json:"thread_spawn"`
			} `json:"subagent"`
		}
		if json.Unmarshal([]byte(worker.Source), &source) != nil || !sessionIDPattern.MatchString(worker.ID) || !sessionIDPattern.MatchString(source.Subagent.Spawn.ParentID) {
			return nil // An unrecognized relationship must not become a zero count.
		}
		parents[worker.ID] = source.Subagent.Spawn.ParentID
		byID[worker.ID] = worker
		// A worker whose lock file is gone has no process holding it, and
		// lsof fails the whole call on a missing file, so it is not asked.
		lock := filepath.Join(s.Stores.CodexDir, "thread-writer-locks", worker.ID+".lock")
		if _, err := os.Stat(lock); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		locks[lock] = worker.ID
		paths = append(paths, lock)
	}
	counts := map[string]int{}
	unknown := map[string]bool{}
	var watched []string
	if s.codexWorkers == nil {
		s.codexWorkers = map[string]*nativeLog[codexWorkerTurn]{}
	}
	held, err := s.Processes.Holders(paths)
	if err != nil {
		return nil // Running workers that cannot be listed must not become a zero count.
	}
	for lock := range held {
		id := locks[lock]
		worker := byID[id]
		path, valid := s.codexRollout(worker.Rollout)
		var log *nativeLog[codexWorkerTurn]
		var readErr error
		if valid {
			watched = append(watched, path)
			log, readErr = readNativeLog(path, s.codexWorkers[id], func() codexWorkerTurn { return codexWorkerTurn{} }, codexWorkerEvent)
			s.codexWorkers[id] = log
		}
		known := valid && readErr == nil && log.value.known
		seen := map[string]bool{id: true}
		for parent := parents[id]; parent != "" && !seen[parent]; parent = parents[parent] {
			seen[parent] = true
			if !known {
				unknown[parent] = true
			} else if log.value.running {
				counts[parent]++
			}
		}
	}
	for i := range conversations {
		if !unknown[conversations[i].ID] {
			count := counts[conversations[i].ID]
			conversations[i].WorkingSubagents = &count
		}
	}
	// The watched rollouts join the scan's stamp, which hashes them in
	// order, so they are sorted to keep an unchanged set's stamp stable.
	slices.Sort(watched)
	return watched
}
