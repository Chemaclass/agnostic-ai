package conversations

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// claudeWindow is how much of each end of a session file the reader
// parses, as Claude Code's own session list does: titles and the
// latest prompt are rewritten near the end, the first prompt sits at
// the start, and files grow past 100 MB.
const claudeWindow = 64 << 10

// liveSessionFile is sessions/<pid>.json; its *.key siblings hold credentials.
var liveSessionFile = regexp.MustCompile(`^[0-9]+\.json$`)

// claudeJobPattern keeps a job name safe to pass to the shell unquoted.
var claudeJobPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

var sessionIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// claudeActiveJobStates are background job states `claude attach` can open.
var claudeActiveJobStates = map[string]bool{"running": true, "blocked": true, "waiting": true, "idle": true}

type claudeMeta struct {
	title, firstPrompt, lastPrompt string
	cwd, branch, entrypoint        string
	started, updated               string
	sidechain                      bool
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
	Message     *struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// claudeLive is a running Claude Code process, from sessions/<pid>.json.
type claudeLive struct {
	PID       int    `json:"pid"`
	SessionID string `json:"sessionId"`
	ProcStart string `json:"procStart"`
	// Status is busy, shell (running a command), waiting (for the
	// person) or idle.
	Status string `json:"status"`
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
	process  liveProcess
	activity Activity
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
	files, err := filepath.Glob(filepath.Join(projects, "*", "*.jsonl"))
	if err != nil {
		return missing(ToolClaude, "Could not list "+projects)
	}
	live := s.claudeLiveSessions()
	jobs := s.claudeActiveJobs()
	editor := extensionEditor(s.Stores.Home, "anthropic.claude-code", opts.Editor)
	if s.claude == nil {
		s.claude = map[string]claudeCacheEntry{}
	}
	seen := map[string]bool{}
	var conversations []Conversation
	for _, file := range files {
		if ctx.Err() != nil {
			break
		}
		id := strings.TrimSuffix(filepath.Base(file), ".jsonl")
		if !sessionIDPattern.MatchString(id) {
			continue
		}
		seen[file] = true
		meta, ok := s.claudeMeta(file)
		if !ok || meta.sidechain || strings.HasPrefix(meta.entrypoint, "sdk") || (meta.title == "" && meta.firstPrompt == "" && meta.lastPrompt == "") {
			continue
		}
		c := Conversation{Tool: ToolClaude, ID: id, Folder: meta.cwd, Branch: meta.branch, StartedAt: meta.started, UpdatedAt: meta.updated}
		c.fill(texts{title: meta.title, firstPrompt: meta.firstPrompt, lastPrompt: meta.lastPrompt})
		session, isLive := live[id]
		c.Live = isLive
		c.Activity = session.activity
		claudeRoute(&c, meta.entrypoint, jobs[id], session.process, isLive, editor)
		conversations = append(conversations, c)
	}
	for file := range s.claude {
		if ctx.Err() == nil && !seen[file] {
			delete(s.claude, file)
		}
	}
	return found(ToolClaude, within(conversations, opts.Within))
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
		c.Open = Route{Kind: RouteTerminal, Label: "Attach in Terminal", Command: "claude attach " + jobID}
	case entrypoint == "claude-vscode":
		c.App = AppIDE
		c.editor = &editor
		c.Open = Route{Kind: RouteLink, Label: "Open in " + editor.label, Link: editor.id + "://anthropic.claude-code/open?session=" + c.ID}
	case entrypoint == "claude-desktop":
		c.App = AppDesktop
		c.Open = Route{Kind: RouteLink, Label: "Open in Claude app", Link: "claude://resume?session=" + c.ID}
	case isLive:
		c.process = &process
		c.Open = Route{Kind: RouteFocus, Label: "Switch to its terminal", Command: resume}
	default:
		c.Open = Route{Kind: RouteTerminal, Label: "Resume in Terminal", Command: resume}
	}
}

// claudeMeta parses a session file, or returns the cached result while
// the file is unchanged.
func (s *Scanner) claudeMeta(file string) (claudeMeta, bool) {
	info, err := os.Stat(file)
	if err != nil {
		return claudeMeta{}, false
	}
	if cached, ok := s.claude[file]; ok && cached.size == info.Size() && cached.modTime.Equal(info.ModTime()) {
		return cached.meta, true
	}
	head, tail, err := readEnds(file, info.Size(), claudeWindow)
	if err != nil {
		return claudeMeta{}, false
	}
	meta := parseClaudeSession(head, tail)
	if meta.cwd == "" || meta.entrypoint == "" {
		streamClaudeMetadata(file, &meta)
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
	tail = make([]byte, window)
	if _, err := f.ReadAt(tail, size-window); err != nil && err != io.EOF {
		return nil, nil, err
	}
	// The head ends and the tail starts mid-line; drop the partial lines.
	if cut := bytes.LastIndexByte(head, '\n'); cut >= 0 {
		head = head[:cut]
	}
	if cut := bytes.IndexByte(tail, '\n'); cut >= 0 {
		tail = tail[cut+1:]
	}
	return head, tail, nil
}

func eachClaudeLine(data []byte, visit func(claudeLine)) {
	for _, raw := range bytes.Split(data, []byte("\n")) {
		if len(bytes.TrimSpace(raw)) == 0 {
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

func parseClaudeSession(head, tail []byte) claudeMeta {
	var meta claudeMeta
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
		if meta.firstPrompt == "" && line.Type == "user" && !line.IsMeta && line.Message != nil {
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
	meta.entrypoint = firstNonEmpty(tailEntry, meta.entrypoint)
	meta.updated = firstNonEmpty(lastMessage, lastAny)
	return meta
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
// are never read.
func (s *Scanner) claudeLiveSessions() map[string]liveSession {
	live := map[string]liveSession{}
	files, _ := filepath.Glob(filepath.Join(s.Stores.ClaudeDir, "sessions", "*.json"))
	for _, file := range files {
		if !liveSessionFile.MatchString(filepath.Base(file)) {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		var entry claudeLive
		if json.Unmarshal(data, &entry) != nil || entry.SessionID == "" {
			continue
		}
		if s.Processes.Alive(entry.PID, entry.ProcStart) {
			live[entry.SessionID] = liveSession{process: liveProcess{PID: entry.PID, ProcStart: entry.ProcStart}, activity: claudeActivity[entry.Status]}
		}
	}
	return live
}

// claudeActiveJobs maps session ids to the background job that holds
// them, for `claude attach`.
func (s *Scanner) claudeActiveJobs() map[string]string {
	jobs := map[string]string{}
	files, _ := filepath.Glob(filepath.Join(s.Stores.ClaudeDir, "jobs", "*", "state.json"))
	for _, file := range files {
		data, err := os.ReadFile(file)
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
