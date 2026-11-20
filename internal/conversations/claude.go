package conversations

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// claudeWindow is how much of each end of a session file the reader
// parses, as Claude Code's own session list does: titles and the
// latest prompt are rewritten near the end, the first prompt sits at
// the start, and files grow past 100 MB.
const claudeWindow = 64 << 10

// claudeReplyWindow is the end of a session file read again when the
// last claudeWindow bytes hold neither a prompt nor an agent message,
// as when a large tool result follows the agent's latest message.
const claudeReplyWindow = 256 << 10

// liveSessionFile is sessions/<pid>.json; its *.key siblings hold credentials.
var liveSessionFile = regexp.MustCompile(`^[0-9]+\.json$`)

// claudeJobPattern keeps a job name safe to pass to the shell unquoted
// and never read as a flag.
var claudeJobPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

var sessionIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// claudeActiveJobStates are background job states `claude attach` can open.
var claudeActiveJobStates = map[string]bool{"running": true, "blocked": true, "waiting": true, "idle": true}

type claudeMeta struct {
	title, firstPrompt, lastPrompt string
	reply, question                string
	cwd, branch, entrypoint        string
	// firstEntrypoint is where the session began: a closed session last
	// resumed in a terminal opens there and follows that app's rule. Only
	// a terminal writes sessions/<pid>.json, so an editor or desktop tail
	// cannot be proven closed and keeps its own app.
	firstEntrypoint  string
	started, updated string
	sidechain        bool
	// pending is the tool call at the end of the transcript that has no
	// result yet, for a session that waits for an approval.
	pending *toolCall
}

type claudeCacheEntry struct {
	size    int64
	modTime time.Time
	meta    claudeMeta
}

// claudeLine holds the fields of a session line the list needs.
type claudeLine struct {
	Type        string `json:"type"`
	AITitle     string `json:"aiTitle"`
	CustomTitle string `json:"customTitle"`
	LastPrompt  string `json:"lastPrompt"`
	Cwd         string `json:"cwd"`
	GitBranch   string `json:"gitBranch"`
	Entrypoint  string `json:"entrypoint"`
	Timestamp   string `json:"timestamp"`
	IsSidechain bool   `json:"isSidechain"`
	IsMeta      bool   `json:"isMeta"`
	IsAPIError  bool   `json:"isApiErrorMessage"`
	// Origin says who wrote a user line; Claude Code leaves it out on
	// tool results and on lines from older versions.
	Origin *struct {
		Kind string `json:"kind"`
	} `json:"origin"`
	Message *struct {
		Role    string          `json:"role"`
		Model   string          `json:"model"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// fromPerson reports whether a user line is the person's own: a
// background task's report or a peer's message is not.
func (line claudeLine) fromPerson() bool {
	return !line.IsMeta && (line.Origin == nil || line.Origin.Kind == "human")
}

// claudeReplies follows the lines of a session's end for the agent's
// latest message and a question still open.
type claudeReplies struct {
	message, asked string
	// known is set once the lines held a message or a typed prompt:
	// what came before it cannot change the reply.
	known bool
	// pending are the tool calls with no result yet, oldest first.
	pending []toolCall
	// subagent reads a subagent's own transcript, whose lines are all
	// sidechain lines. Only its own messages count there: a later message
	// to it, such as a teammate's, keeps what it said last shown.
	subagent bool
}

// waitingCall is the oldest tool call that has no result yet: Claude
// Code asks about the calls of one message in order, so a later one
// waits behind it. Nil when every call has its result.
func (r *claudeReplies) waitingCall() *toolCall {
	if len(r.pending) == 0 {
		return nil
	}
	call := r.pending[0]
	return &call
}

// claudeBlock is one part of a message's content.
type claudeBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
	Name string `json:"name"`
	// ID names a tool call; ToolUseID names the call a result answers.
	ID        string `json:"id"`
	ToolUseID string `json:"tool_use_id"`
	// Input is a tool call's input, read only for the tools whose shape
	// is known: any tool, such as an MCP server's, may send any shape.
	Input json.RawMessage `json:"input"`
}

// askedQuestions is the input of an AskUserQuestion call.
type askedQuestions struct {
	Questions []struct {
		Question string `json:"question"`
	} `json:"questions"`
}

// firstQuestion is the first question an AskUserQuestion call asks, or
// "" when its input holds none.
func (block claudeBlock) firstQuestion() string {
	var input askedQuestions
	if json.Unmarshal(block.Input, &input) != nil || len(input.Questions) == 0 {
		return ""
	}
	return input.Questions[0].Question
}

// visit reads one line. Only the agent's own text counts: tool calls,
// API errors, placeholder messages and subagent lines are skipped, and a
// line of any other type is bookkeeping.
func (r *claudeReplies) visit(line claudeLine) {
	if (line.IsSidechain && !r.subagent) || line.Message == nil {
		return
	}
	switch line.Type {
	case "assistant":
		if line.IsAPIError || line.Message.Model == "<synthetic>" {
			return
		}
		var blocks []claudeBlock
		if json.Unmarshal(line.Message.Content, &blocks) != nil {
			return
		}
		var parts []string
		for _, block := range blocks {
			if block.Type == "tool_use" && block.ID != "" {
				r.pending = append(r.pending, toolCall{id: block.ID, name: block.Name, input: callInput(block.Name, block.Input)})
			}
			switch {
			case block.Type == "text" && strings.TrimSpace(block.Text) != "":
				parts = append(parts, block.Text)
			case block.Type == "tool_use" && block.Name == "AskUserQuestion" && block.firstQuestion() != "":
				r.asked = block.firstQuestion()
			}
		}
		if len(parts) > 0 {
			r.message, r.known = strings.Join(parts, "\n"), true
		}
	case "user":
		if r.subagent {
			return
		}
		r.answered(line.Message.Content)
		if line.fromPerson() && len(r.pending) > 0 {
			// A new prompt starts a new turn: no call of an older one waits.
			if text, typed := claudeContentText(line.Message.Content); typed && typedText(text) != "" {
				r.pending = nil
			}
		}
		if !line.fromPerson() || (r.asked == "" && r.known && r.message == "") {
			return
		}
		text, typed := claudeContentText(line.Message.Content)
		if !typed {
			// A tool result answers an open question.
			r.asked = ""
			return
		}
		if typedText(text) != "" {
			r.message, r.asked, r.known = "", "", true
		}
	}
}

// answered drops the pending calls a user line's tool results answer.
func (r *claudeReplies) answered(content json.RawMessage) {
	if len(r.pending) == 0 {
		return
	}
	var blocks []claudeBlock
	if json.Unmarshal(content, &blocks) != nil {
		return
	}
	for _, block := range blocks {
		if block.Type != "tool_result" {
			continue
		}
		r.pending = slices.DeleteFunc(r.pending, func(call toolCall) bool { return call.id == block.ToolUseID })
	}
}

// claudeLive is a running Claude Code process, from sessions/<pid>.json.
type claudeLive struct {
	PID       int             `json:"pid"`
	SessionID string          `json:"sessionId"`
	ProcStart string          `json:"procStart"`
	StartedAt json.RawMessage `json:"startedAt"`
	// Status is busy, shell (running a command), waiting (for the
	// person) or idle.
	Status string `json:"status"`
	// Entrypoint is where this process runs it, which can differ from
	// where the session began: a plugin session resumed in a terminal.
	Entrypoint string `json:"entrypoint"`
	// BridgeSessionID is session_<id> while Remote Control shares it.
	BridgeSessionID string `json:"bridgeSessionId"`
	// WaitingFor says what a waiting session waits for; see claudeWaitKinds.
	WaitingFor string `json:"waitingFor"`
	// StatusUpdatedAt is when Status last changed, in Unix milliseconds.
	StatusUpdatedAt json.RawMessage `json:"statusUpdatedAt"`
}

// claudeActivity maps a live session's status to what the page shows.
var claudeActivity = map[string]Activity{
	"busy":    ActivityWorking,
	"shell":   ActivityWorking,
	"waiting": ActivityWaiting,
	"idle":    ActivityIdle,
}

// liveSession is a running process that holds a session, and what it does.
type liveSession struct {
	startedAt  int64
	process    liveProcess
	activity   Activity
	status     string
	entrypoint string
	bridge     string
	waitingFor string
	// statusSince is when the status last changed; zero when unknown.
	statusSince time.Time
}

type claudeJob struct {
	SessionID       string `json:"sessionId"`
	ResumeSessionID string `json:"resumeSessionId"`
	State           string `json:"state"`
}

func (s *Scanner) scanClaude(ctx context.Context, opts ScanOptions) sourceResult {
	projects := filepath.Join(s.Stores.ClaudeDir, "projects")
	if _, err := os.Stat(projects); err != nil {
		return missing(ToolClaude, "No Claude Code sessions in "+projects)
	}
	// Subagent transcripts live one level deeper, under <session>/subagents.
	files, listedAll := listTree(projects, 1, "*.jsonl")
	live, liveRead, liveNote := s.claudeLiveSessions()
	jobs := s.claudeActiveJobs()
	editor := extensionEditor(s.Stores.Home, "anthropic.claude-code", opts.Editor)
	if s.claude == nil {
		s.claude = map[string]claudeCacheEntry{}
	}
	seen := map[string]bool{}
	seenMetas := map[string]bool{}
	ids := make(map[string]string, len(files))
	var conversations []Conversation
	var workerFiles []string
	for _, file := range files {
		if ctx.Err() != nil {
			break
		}
		id := strings.TrimSuffix(filepath.Base(file), ".jsonl")
		if !sessionIDPattern.MatchString(id) {
			continue
		}
		seen[file] = true
		ids[id] = strings.TrimPrefix(file, projects+string(filepath.Separator))
		meta, ok := s.claudeMeta(file)
		if !ok || meta.sidechain || strings.HasPrefix(meta.entrypoint, "sdk") || (meta.title == "" && meta.firstPrompt == "" && meta.lastPrompt == "") {
			continue
		}
		c := Conversation{Tool: ToolClaude, ID: id, Folder: meta.cwd, Branch: meta.branch, StartedAt: meta.started, UpdatedAt: meta.updated, file: file}
		s.fill(&c, texts{title: meta.title, firstPrompt: meta.firstPrompt, lastPrompt: meta.lastPrompt, reply: meta.reply, question: meta.question})
		session, isLive := live[id]
		c.Live = isLive
		c.Activity = session.activity
		if canStopShells() && (session.status == "shell" || session.status == "busy") && opts.shellAfter() > 0 {
			c.shellSession = &session.process
			c.mainWorking = meta.pending != nil || session.status == "busy"
		}
		if isLive && session.activity == ActivityWaiting {
			c.Waiting = claudeWaiting(session.waitingFor, meta.pending)
			if c.Waiting != nil {
				c.Waiting.since = session.statusSince
			}
		}
		// A subagent runs inside its session's process, so only a live
		// session lists working ones. Work that cannot be read lists none.
		if isLive {
			work, watched := s.claudeWorkOf(file, session)
			workerFiles = append(workerFiles, watched...)
			if work != nil {
				count := work.count()
				c.WorkingSubagents = &count
				c.mainWorking = c.mainWorking || work.mainWorking
			}
			c.Helpers = s.claudeHelpers(file, work, seenMetas)
		}
		c.bridge = session.bridge
		entrypoint := meta.entrypoint
		switch {
		case isLive:
			entrypoint = firstNonEmpty(session.entrypoint, meta.entrypoint)
		case entrypoint == "cli":
			entrypoint = meta.firstEntrypoint
		}
		claudeRoute(&c, entrypoint, jobs[id], session.process, isLive, editor)
		// Only a terminal session is known to write sessions/<pid>.json while it runs.
		c.liveRead = liveRead && c.App == AppCLI
		conversations = append(conversations, c)
	}
	for file := range s.claude {
		if ctx.Err() == nil && !seen[file] {
			delete(s.claude, file)
		}
	}
	if ctx.Err() == nil {
		forget(s.claudeAgentMetas, seenMetas)
		forget(s.claudeAgentReplies, seenMetas)
	}
	s.rolloutsMu.Lock()
	s.claudeWorkerFiles = workerFiles
	s.rolloutsMu.Unlock()
	for file := range s.claudeWorkers {
		id := strings.TrimSuffix(filepath.Base(file), ".jsonl")
		if _, running := live[id]; !running {
			delete(s.claudeWorkers, file)
		}
	}
	result := found(ToolClaude, within(conversations, opts.Within))
	result.source.Note = liveNote
	if listedAll && ctx.Err() == nil {
		result.inventories = provenBy(sourceClaudeCode, ToolClaude, projects, ids)
	}
	return result
}

// claudeRoute picks where a session reopens: its background job, the
// editor panel or desktop app it came from, the terminal tab that runs
// it, or a new terminal.
func claudeRoute(c *Conversation, entrypoint, jobID string, process liveProcess, isLive bool, editor editorApp) {
	resume := resumeIn(c.Folder, "claude --resume "+c.ID)
	c.App = AppCLI
	switch {
	case jobID != "":
		c.App = AppBackground
		c.Open = Route{Kind: RouteTerminal, Label: "Open in Terminal", Command: "claude attach " + jobID}
	case entrypoint == "claude-vscode":
		c.App = AppIDE
		c.editor = &editor
		c.Open = Route{Kind: RouteLink, Label: "Open in " + editor.label, Link: editor.id + "://anthropic.claude-code/open?session=" + c.ID}
	case entrypoint == "claude-desktop":
		c.App = AppDesktop
		c.Open = Route{Kind: RouteLink, Label: "Open in Claude app", Link: "claude://resume?session=" + c.ID}
	case isLive:
		c.process = &process
		// Only a terminal session is its own process: a background job's
		// daemon, an editor or the Claude app would start it again or lose track.
		c.Stoppable = canStopShells() && process.PID > 1 && process.ProcStart != ""
		c.Open = Route{Kind: RouteFocus, Label: "Open in Terminal", Command: resume}
	default:
		c.Open = Route{Kind: RouteTerminal, Label: "Resume in Terminal", Command: resume}
	}
}

// claudeMeta parses a session file, or returns the cached result while
// the file is unchanged.
func (s *Scanner) claudeMeta(file string) (claudeMeta, bool) {
	info, err := os.Stat(file)
	if err != nil || !info.Mode().IsRegular() {
		return claudeMeta{}, false
	}
	if cached, ok := s.claude[file]; ok && cached.size == info.Size() && cached.modTime.Equal(info.ModTime()) {
		return cached.meta, true
	}
	head, tail, err := readEnds(file, info.Size(), claudeWindow)
	if err != nil {
		return claudeMeta{}, false
	}
	meta, replyKnown := parseClaudeSession(head, tail)
	if !replyKnown {
		if lines, ok := lookBack(file, info.Size()); ok {
			var replies claudeReplies
			eachClaudeLine(lines, replies.visit)
			meta.reply, meta.question = agentSaid(replies.message, replies.message, replies.asked)
			meta.pending = replies.waitingCall()
		}
	}
	if meta.cwd == "" || meta.entrypoint == "" {
		streamClaudeMetadata(file, &meta)
	}
	if meta.firstEntrypoint == "" {
		meta.firstEntrypoint = meta.entrypoint
	}
	if meta.updated == "" {
		meta.updated = info.ModTime().UTC().Format(time.RFC3339)
	}
	s.claude[file] = claudeCacheEntry{size: info.Size(), modTime: info.ModTime(), meta: meta}
	return meta, true
}

// readEnds returns the complete lines in the first and last window
// bytes of a file. A short file is read once and serves as both.
func readEnds(path string, size int64, window int64) (head, tail []byte, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = f.Close() }()
	if size <= 2*window {
		whole, err := io.ReadAll(f)
		return whole, whole, err
	}
	head = make([]byte, window)
	if _, err := io.ReadFull(f, head); err != nil {
		return nil, nil, err
	}
	// The head ends mid-line; drop the partial line.
	if cut := bytes.LastIndexByte(head, '\n'); cut >= 0 {
		head = head[:cut]
	}
	tail, err = readTail(f, size, window)
	if err != nil {
		return nil, nil, err
	}
	return head, tail, nil
}

// readTail returns the complete lines in the last window bytes of f.
func readTail(f *os.File, size, window int64) ([]byte, error) {
	window = min(window, size)
	tail := make([]byte, window)
	if _, err := f.ReadAt(tail, size-window); err != nil && err != io.EOF {
		return nil, err
	}
	if window == size {
		return tail, nil
	}
	// The tail starts mid-line; drop the partial line.
	if cut := bytes.IndexByte(tail, '\n'); cut >= 0 {
		tail = tail[cut+1:]
	}
	return tail, nil
}

// lookBack returns the complete lines in the last claudeReplyWindow
// bytes of a file larger than the two ends readEnds read; ok is false
// for a file readEnds read whole. A file that cannot be read gives none.
func lookBack(path string, size int64) (lines []byte, ok bool) {
	if size <= 2*claudeWindow {
		return nil, false
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, true
	}
	defer func() { _ = f.Close() }()
	lines, _ = readTail(f, size, claudeReplyWindow)
	return lines, true
}

func eachClaudeLine(data []byte, visit func(claudeLine)) {
	eachClaudeLineWithout(data, nil, visit)
}

// eachClaudeLineWithout visits the lines that do not hold every token
// of skip, which are never decoded.
func eachClaudeLineWithout(data []byte, skip [][]byte, visit func(claudeLine)) {
	for _, raw := range bytes.Split(data, []byte("\n")) {
		if len(bytes.TrimSpace(raw)) == 0 || holdsEvery(raw, skip) {
			continue
		}
		var line claudeLine
		// A line that does not decode carries nothing the list needs.
		if json.Unmarshal(raw, &line) != nil {
			continue
		}
		visit(line)
	}
}

// holdsEvery reports whether raw holds each of tokens; false for none.
func holdsEvery(raw []byte, tokens [][]byte) bool {
	for _, token := range tokens {
		if !bytes.Contains(raw, token) {
			return false
		}
	}
	return len(tokens) > 0
}

// parseClaudeSession reads a session's head and tail. replyKnown is
// false when the tail held neither a prompt nor an agent message.
func parseClaudeSession(head, tail []byte) (meta claudeMeta, replyKnown bool) {
	var replies claudeReplies
	var headCustom, headAI, tailCustom, tailAI string
	eachClaudeLine(head, func(line claudeLine) {
		if line.IsSidechain {
			meta.sidechain = true
		}
		if meta.started == "" {
			meta.started = isoTime(line.Timestamp)
		}
		setOnce(&meta.cwd, line.Cwd)
		setOnce(&meta.branch, line.GitBranch)
		setOnce(&meta.entrypoint, line.Entrypoint)
		setLast(&headCustom, line.CustomTitle)
		setLast(&headAI, line.AITitle)
		if meta.firstPrompt == "" && line.Type == "user" && line.fromPerson() && line.Message != nil {
			if text, typed := claudeContentText(line.Message.Content); typed && typedText(text) != "" {
				meta.firstPrompt = text
			}
		}
	})
	// The folder is where the session started, where it resumes; the
	// branch is the latest one. The time is the last message from the
	// person or the agent, so bookkeeping lines do not reorder the list.
	var tailCwd, tailBranch, tailEntry, lastMessage, lastAny string
	eachClaudeLine(tail, func(line claudeLine) {
		replies.visit(line)
		setLast(&tailCustom, line.CustomTitle)
		setLast(&tailAI, line.AITitle)
		setLast(&meta.lastPrompt, line.LastPrompt)
		setLast(&tailCwd, line.Cwd)
		setLast(&tailBranch, line.GitBranch)
		setLast(&tailEntry, line.Entrypoint)
		stamp := isoTime(line.Timestamp)
		setLast(&lastAny, stamp)
		if line.Type == "user" || line.Type == "assistant" {
			setLast(&lastMessage, stamp)
		}
	})
	meta.title = firstNonEmpty(tailCustom, headCustom, tailAI, headAI)
	meta.cwd = firstNonEmpty(meta.cwd, tailCwd)
	meta.branch = firstNonEmpty(tailBranch, meta.branch)
	meta.firstEntrypoint = meta.entrypoint
	meta.entrypoint = firstNonEmpty(tailEntry, meta.entrypoint)
	meta.updated = firstNonEmpty(lastMessage, lastAny)
	meta.reply, meta.question = agentSaid(replies.message, replies.message, replies.asked)
	meta.pending = replies.waitingCall()
	return meta, replies.known
}

// claudeContentText joins a message's text. typed is false for a tool
// result, which Claude Code also stores as a user message.
func claudeContentText(raw json.RawMessage) (text string, typed bool) {
	var plain string
	if json.Unmarshal(raw, &plain) == nil {
		return plain, true
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return "", false
	}
	var parts []string
	for _, block := range blocks {
		switch block.Type {
		case "tool_result":
			return "", false
		case "text":
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n"), len(parts) > 0
}

// claudeLiveSessions maps each session id that a running Claude Code
// process holds to that process. Each sessions/<pid>.json belongs to
// one process; a crash leaves it behind, so the pid must still be the
// process that wrote it. The sibling *.key files hold credentials and
// are never read. read is false when the sessions folder or the process
// table could not be read, or a session file could not be read while the
// process its name gives still runs; note then says which.
func (s *Scanner) claudeLiveSessions() (sessions map[string]liveSession, read bool, note string) {
	entries, unreadable, entriesErr := s.claudeLiveEntries()
	var processes []ProcessRef
	for _, entry := range entries {
		processes = append(processes, ProcessRef{PID: entry.PID, ProcStart: entry.ProcStart})
	}
	for pid := range unreadable {
		processes = append(processes, ProcessRef{PID: pid})
	}
	// One question for every session: asking about each in turn starts
	// a ps per open session on every scan.
	alive, err := s.Processes.AliveAmong(processes)
	if err == nil {
		if blocking := unreadableHeld(unreadable, func(pid int) bool { return alive[pid] }); blocking != nil {
			entriesErr = errors.Join(entriesErr, blocking)
			note = "A Claude Code session file could not be read while its process runs, so no session is shown as closed: " + blocking.Error()
		}
	}
	live := map[string]liveSession{}
	for _, entry := range entries {
		if alive[entry.PID] {
			var startedAt int64
			if json.Unmarshal(entry.StartedAt, &startedAt) != nil {
				startedAt = 0 // An unsupported start format leaves only the worker count unknown.
			}
			var statusSince time.Time
			var statusAt int64
			if json.Unmarshal(entry.StatusUpdatedAt, &statusAt) == nil && statusAt > 0 {
				statusSince = time.UnixMilli(statusAt)
			}
			live[entry.SessionID] = liveSession{process: liveProcess{PID: entry.PID, ProcStart: entry.ProcStart}, activity: claudeActivity[entry.Status], status: entry.Status, entrypoint: entry.Entrypoint, bridge: entry.BridgeSessionID, startedAt: startedAt, waitingFor: entry.WaitingFor, statusSince: statusSince}
		}
	}
	return live, err == nil && entriesErr == nil, note
}

// unreadableHeld is the error for the unreadable session files whose
// process, named by the file, still runs: one of them may hold any
// session. A file whose process is gone, as a crash leaves it, holds none.
func unreadableHeld(unreadable map[int]error, running func(pid int) bool) error {
	var held error
	for pid, err := range unreadable {
		if running(pid) {
			held = errors.Join(held, err)
		}
	}
	return held
}

// claudeLiveEntries reads sessions/<pid>.json, which a Claude Code
// process writes while it runs and may leave behind when it dies.
// unreadable maps the pid in the name of each file that could not be
// read or parsed to why; err says the folder could not be listed.
func (s *Scanner) claudeLiveEntries() (entries []claudeLive, unreadable map[int]error, err error) {
	dir := filepath.Join(s.Stores.ClaudeDir, "sessions")
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", dir, err)
	}
	unreadable = map[int]error{}
	for _, file := range files {
		if !liveSessionFile.MatchString(file.Name()) {
			continue
		}
		path := filepath.Join(dir, file.Name())
		pid, _ := strconv.Atoi(strings.TrimSuffix(file.Name(), ".json"))
		data, err := readClaudeMetadata(path)
		if err != nil {
			unreadable[pid] = err
			continue
		}
		var entry claudeLive
		if err := json.Unmarshal(data, &entry); err != nil {
			unreadable[pid] = fmt.Errorf("parse %s: %w", path, err)
			continue
		}
		if entry.SessionID == "" || entry.PID <= 0 {
			unreadable[pid] = fmt.Errorf("parse %s: missing process or session id", path)
			continue
		}
		entries = append(entries, entry)
	}
	return entries, unreadable, nil
}

// claudeActiveJobs maps session ids to the background job that holds
// them, for `claude attach`.
func (s *Scanner) claudeActiveJobs() map[string]string {
	jobs := map[string]string{}
	files, _ := filepath.Glob(filepath.Join(s.Stores.ClaudeDir, "jobs", "*", "state.json"))
	for _, file := range files {
		data, err := readClaudeMetadata(file)
		if err != nil {
			continue
		}
		var job claudeJob
		if json.Unmarshal(data, &job) != nil || !claudeActiveJobStates[job.State] {
			continue
		}
		short := filepath.Base(filepath.Dir(file))
		if !claudeJobPattern.MatchString(short) {
			continue
		}
		for _, id := range []string{job.SessionID, job.ResumeSessionID} {
			if id != "" {
				jobs[id] = short
			}
		}
	}
	return jobs
}

func readClaudeMetadata(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: expected a regular file, got %s", path, info.Mode().Type())
	}
	return os.ReadFile(path)
}

func isoTime(value string) string {
	if value == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return ""
	}
	return parsed.UTC().Format(time.RFC3339)
}

func setOnce(target *string, value string) {
	if *target == "" && value != "" {
		*target = value
	}
}

func setLast(target *string, value string) {
	if value != "" {
		*target = value
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// When neither end of a session file names its folder and entry point,
// streamClaudeMetadata reads on from the start. SDK sessions write these
// fields last on lines that run to megabytes, so only the end of each
// line is kept.
const (
	claudeStreamLimit = 64 << 20
	claudeLineEnd     = 4 << 10
)

// The fields sit outside any string, so the quote before them is not escaped.
var (
	cwdField        = regexp.MustCompile(`(?:^|[^\\])"cwd":"((?:[^"\\]|\\.)*)"`)
	branchField     = regexp.MustCompile(`(?:^|[^\\])"gitBranch":"((?:[^"\\]|\\.)*)"`)
	entrypointField = regexp.MustCompile(`(?:^|[^\\])"entrypoint":"([^"\\]*)"`)
)

func streamClaudeMetadata(file string, meta *claudeMeta) {
	f, err := os.Open(file)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	reader := bufio.NewReaderSize(f, claudeWindow)
	var lineEnd []byte
	for read := 0; read < claudeStreamLimit; {
		chunk, more, err := reader.ReadLine()
		read += len(chunk)
		lineEnd = append(lineEnd, chunk...)
		if len(lineEnd) > claudeLineEnd {
			lineEnd = append(lineEnd[:0], lineEnd[len(lineEnd)-claudeLineEnd:]...)
		}
		if !more {
			if fillFromLineEnd(lineEnd, meta) {
				return
			}
			lineEnd = lineEnd[:0]
		}
		if err != nil {
			return
		}
	}
}

// fillFromLineEnd sets the fields still empty from the end of a line,
// and reports whether the folder and entry point are now both known.
func fillFromLineEnd(lineEnd []byte, meta *claudeMeta) bool {
	for _, field := range []struct {
		pattern *regexp.Regexp
		target  *string
	}{{cwdField, &meta.cwd}, {branchField, &meta.branch}, {entrypointField, &meta.entrypoint}} {
		if *field.target != "" {
			continue
		}
		if match := field.pattern.FindSubmatch(lineEnd); match != nil {
			var value string
			if json.Unmarshal(append(append([]byte{'"'}, match[1]...), '"'), &value) == nil {
				*field.target = value
			}
		}
	}
	return meta.cwd != "" && meta.entrypoint != ""
}
