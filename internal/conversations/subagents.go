package conversations

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		Type        string `json:"type"`
		Timestamp   string `json:"timestamp"`
		IsSidechain bool   `json:"isSidechain"`
		Operation   string `json:"operation"`
		Content     string `json:"content"`
		Message     struct {
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
		Type      string          `json:"type"`
		Name      string          `json:"name"`
		ID        string          `json:"id"`
		ToolUseID string          `json:"tool_use_id"`
		IsError   bool            `json:"is_error"`
		Content   json.RawMessage `json:"content"`
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
		var result nativeTaskResult
		switch {
		case len(event.Result) > 0 && event.Result[0] == '{':
			if err := json.Unmarshal(event.Result, &result); err != nil {
				return err
			}
		case event.IsSidechain:
			sidechain, err := sidechainTaskResult(called, block.Content)
			if err != nil {
				return err
			}
			result = sidechain
		default:
			return errors.New("native task result has no activity metadata")
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

type nativeTaskResult struct {
	AgentID        string `json:"agentId"`
	Status         string `json:"status"`
	TaskID         string `json:"task_id"`
	Success        bool   `json:"success"`
	ResumedAgentID string `json:"resumedAgentId"`
	RevivedAgentID string `json:"revivedAgentId"`
}

// A fork's own transcript opens with its launch. The parent's transcript
// already records that launch with its metadata.
const forkStartedResult = "Fork started — that fork is you."

// An async launch names its agent only in prose meant for the model.
const asyncLaunchedPrefix = "Async agent launched"

// Claude Code writes toolUseResult only in the main transcript. A subagent's
// transcript (isSidechain) keeps just the tool_result content, so the
// activity is read from that content by tool name:
//   - TaskStop and SendMessage: the content is the same JSON object.
//   - Agent and Task: a subagent's call runs in the foreground, so its result
//     means the nested agent has returned and no task keeps working. The
//     fork's record of its own launch adds nothing either. An async launch
//     cannot be read and keeps the count unknown.
func sidechainTaskResult(called string, content json.RawMessage) (nativeTaskResult, error) {
	text, err := toolResultText(content)
	if err != nil {
		return nativeTaskResult{}, err
	}
	switch called {
	case "TaskStop", "SendMessage":
		var result nativeTaskResult
		if err := json.Unmarshal([]byte(text), &result); err != nil {
			return nativeTaskResult{}, fmt.Errorf("subagent %s result is not a JSON object: %w", called, err)
		}
		return result, nil
	case "Agent", "Task":
		if text == forkStartedResult {
			return nativeTaskResult{}, nil
		}
		if strings.HasPrefix(text, asyncLaunchedPrefix) {
			return nativeTaskResult{}, errors.New("subagent async launch has no activity metadata")
		}
		return nativeTaskResult{}, nil
	}
	return nativeTaskResult{}, fmt.Errorf("subagent %s result has no activity metadata", called)
}

// A tool_result's content is either a string or a list of blocks whose first
// text block carries the result.
func toolResultText(content json.RawMessage) (string, error) {
	if len(content) == 0 {
		return "", errors.New("tool result has no content")
	}
	if content[0] == '"' {
		var text string
		err := json.Unmarshal(content, &text)
		return text, err
	}
	if content[0] != '[' {
		return "", errors.New("tool result content is neither text nor blocks")
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(content, &blocks); err != nil {
		return "", err
	}
	for _, block := range blocks {
		if block.Type == "text" {
			return block.Text, nil
		}
	}
	return "", errors.New("tool result content has no text block")
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

// claudeWork is what a live session's task events say about its native
// subagents: which agents work, and which Agent or Task calls wait for
// their result, each a subagent that works.
type claudeWork struct {
	agents  map[string]bool
	pending map[string]bool
}

// count is how many subagents work.
func (work *claudeWork) count() int {
	count := len(work.pending)
	for _, working := range work.agents {
		if working {
			count++
		}
	}
	return count
}

func (s *Scanner) claudeWorkingSubagents(file string, session liveSession) *int {
	work, _ := s.claudeWorkOf(file, session)
	if work == nil {
		return nil
	}
	count := work.count()
	return &count
}

// claudeWorkOf reads a live session's task events, and those of its
// subagents, written since its process started. Nil when they cannot be
// read or the process start is unknown.
func (s *Scanner) claudeWorkOf(file string, session liveSession) (*claudeWork, []string) {
	dir := claudeSubagentsDir(file)
	files := []string{file}
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, []string{dir}
	}
	for _, entry := range entries {
		if _, ok := claudeAgentID(entry); ok {
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
	work := &claudeWork{agents: map[string]bool{}, pending: map[string]bool{}}
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
		for call := range log.value.pending {
			work.pending[call] = true
		}
		for id, event := range log.value.tasks {
			previous, found := tasks[id]
			if !found || event.at.After(previous.at) || (event.at.Equal(previous.at) && !event.working) {
				tasks[id] = event
			}
		}
	}
	for id, event := range tasks {
		work.agents[id] = event.working
	}
	return work, watch
}

// claudeSubagentsDir is the folder that holds a session's subagent
// transcripts, agent-<id>.jsonl, each beside its agent-<id>.meta.json.
func claudeSubagentsDir(file string) string {
	return strings.TrimSuffix(file, ".jsonl") + string(filepath.Separator) + "subagents"
}

// claudeAgentID is the agent id of a subagent transcript in that folder.
func claudeAgentID(entry os.DirEntry) (string, bool) {
	name := entry.Name()
	if entry.IsDir() || !strings.HasPrefix(name, "agent-") || !strings.HasSuffix(name, ".jsonl") {
		return "", false
	}
	return strings.TrimSuffix(strings.TrimPrefix(name, "agent-"), ".jsonl"), true
}

// claudeAgentMeta is what Claude Code writes beside a subagent's
// transcript when it spawns one: the call's description and agent type,
// and the id of the Agent or Task call that spawned it.
type claudeAgentMeta struct {
	AgentType   string `json:"agentType"`
	Description string `json:"description"`
	ToolUseID   string `json:"toolUseId"`
}

// claudeAgentMetaOf reads a subagent's meta file, kept until it changes.
// A subagent without one, as older Claude Code versions write, or with
// one that cannot be read, has no description or type.
func (s *Scanner) claudeAgentMetaOf(path string) claudeAgentMeta {
	stamp := fileStamp(path)
	if kept, ok := s.claudeAgentMetas[path]; ok && kept.stamp == stamp {
		return kept.value
	}
	var meta claudeAgentMeta
	raw, err := os.ReadFile(path)
	if err == nil && json.Unmarshal(raw, &meta) != nil {
		meta = claudeAgentMeta{}
	}
	if s.claudeAgentMetas == nil {
		s.claudeAgentMetas = map[string]cached[claudeAgentMeta]{}
	}
	s.claudeAgentMetas[path] = cached[claudeAgentMeta]{stamp: stamp, value: meta}
	return meta
}

// claudeHelpers lists a live session's native subagents that work, from
// its subagents folder: those work names working, and those whose
// spawning call work names as waiting for its result. A finished one is
// not listed. Nil work is unknown, and lists none.
func (s *Scanner) claudeHelpers(file string, work *claudeWork, seen map[string]bool) []helper {
	if work == nil || work.count() == 0 {
		return nil
	}
	dir := claudeSubagentsDir(file)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var helpers []helper
	for _, entry := range entries {
		// Only an id an open request can name is listed.
		id, ok := claudeAgentID(entry)
		if !ok || !ValidAgentID(id) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue // Removed since the folder was read.
		}
		metaPath := filepath.Join(dir, "agent-"+id+".meta.json")
		seen[metaPath] = true
		meta := s.claudeAgentMetaOf(metaPath)
		working := work.agents[id] || (meta.ToolUseID != "" && work.pending[meta.ToolUseID])
		if !working {
			continue
		}
		helpers = append(helpers, helper{
			Tool: ToolClaude, ID: id, Kind: meta.AgentType, State: helperWorking,
			Title:     s.helperTitle(meta.Description, ""),
			UpdatedAt: stamp(info.ModTime()),
		})
	}
	return sortedHelpers(helpers)
}

type codexWorker struct {
	ID               string `json:"id"`
	Source           string `json:"source"`
	Rollout          string `json:"rollout_path"`
	Cwd              string `json:"cwd"`
	Originator       string `json:"originator"`
	Name             string `json:"name"`
	Title            string `json:"title"`
	FirstUserMessage string `json:"first_user_message"`
	UpdatedAtMs      int64  `json:"updated_at_ms"`
}

const codexWorkersQuery = `SELECT id, source, rollout_path, coalesce(cwd, '') AS cwd, coalesce(originator, '') AS originator,
  coalesce(name, '') AS name, coalesce(title, '') AS title, coalesce(first_user_message, '') AS first_user_message,
  coalesce(updated_at_ms, 0) AS updated_at_ms
FROM threads WHERE thread_source = 'subagent' AND archived = 0`

// codexSpawn is how a Codex native subagent was spawned, from its
// thread's source. Older Codex versions name no nickname or role.
type codexSpawn struct {
	ParentID string `json:"parent_thread_id"`
	Nickname string `json:"agent_nickname"`
	Role     string `json:"agent_role"`
}

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

// codexWorkingSubagents sets, on each Codex row, how many native
// subagents under it work, and lists its direct subagents that work as
// helpers. A subagent works while a process holds its thread and its
// rollout says a turn runs. It returns the rollouts it read, and the
// rows that open each working subagent the way a Codex thread opens:
// one in a terminal switches to the process that holds it.
func (s *Scanner) codexWorkingSubagents(ctx context.Context, statePath string, conversations []Conversation, editor editorApp) ([]string, []Conversation) {
	workers, err := queryRows[codexWorker](ctx, s.SQLite, statePath, codexWorkersQuery)
	if err != nil {
		return nil, nil
	}
	parents := map[string]string{}
	spawns := map[string]codexSpawn{}
	byID := map[string]codexWorker{}
	locks := map[string]string{}
	var paths []string
	for _, worker := range workers {
		var source struct {
			Subagent struct {
				Spawn codexSpawn `json:"thread_spawn"`
			} `json:"subagent"`
		}
		if json.Unmarshal([]byte(worker.Source), &source) != nil || !sessionIDPattern.MatchString(worker.ID) || !sessionIDPattern.MatchString(source.Subagent.Spawn.ParentID) {
			return nil, nil // An unrecognized relationship must not become a zero count.
		}
		parents[worker.ID] = source.Subagent.Spawn.ParentID
		spawns[worker.ID] = source.Subagent.Spawn
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
	// working maps each subagent that works to the pid that holds it.
	working := map[string]int{}
	var watched []string
	if s.codexWorkers == nil {
		s.codexWorkers = map[string]*nativeLog[codexWorkerTurn]{}
	}
	held, err := s.Processes.Holders(paths)
	if err != nil {
		return nil, nil // Running workers that cannot be listed must not become a zero count.
	}
	for lock, pid := range held {
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
		if known && log.value.running {
			working[id] = pid
		}
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
	helpers := map[string][]helper{}
	var rows []Conversation
	for _, worker := range workers {
		pid, isWorking := working[worker.ID]
		if !isWorking {
			continue // A finished subagent is not listed.
		}
		spawn := spawns[worker.ID]
		helpers[spawn.ParentID] = append(helpers[spawn.ParentID], helper{
			Tool: ToolCodex, ID: worker.ID, Kind: spawn.Role, State: helperWorking,
			Title:     s.helperTitle(codexWorkerTitle(worker, spawn), worker.FirstUserMessage),
			UpdatedAt: timeFromMillis(worker.UpdatedAtMs),
		})
		row := Conversation{Tool: ToolCodex, ID: worker.ID, Folder: worker.Cwd, UpdatedAt: timeFromMillis(worker.UpdatedAtMs)}
		codexRoute(&row, codexThread{ID: worker.ID, Cwd: worker.Cwd, Originator: worker.Originator, Source: worker.Source}, editor)
		focusLive(&row, pid)
		rows = append(rows, row)
	}
	for i := range conversations {
		id := conversations[i].ID
		if !unknown[id] {
			count := counts[id]
			conversations[i].WorkingSubagents = &count
			conversations[i].Helpers = sortedHelpers(helpers[id])
		}
	}
	// The watched rollouts join the scan's stamp, which hashes them in
	// order, so they are sorted to keep an unchanged set's stamp stable.
	slices.Sort(watched)
	return watched, rows
}

// codexWorkerTitle is a subagent's own name or title. With neither, nor
// a first prompt to take one from, it is the nickname Codex gave it.
func codexWorkerTitle(worker codexWorker, spawn codexSpawn) string {
	title := firstNonEmpty(worker.Name, worker.Title)
	if title == "" && worker.FirstUserMessage == "" {
		return spawn.Nickname
	}
	return title
}
