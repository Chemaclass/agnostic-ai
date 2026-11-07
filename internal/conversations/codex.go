package conversations

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Codex indexes its threads in state_<n>.sqlite and keeps each prompt in
// thread_history_<n>.sqlite, so neither the compressed rollout files nor
// a zstd library are needed.

var codexStatePattern = regexp.MustCompile(`^state_([0-9]+)\.sqlite$`)
var codexHistoryPattern = regexp.MustCompile(`^thread_history_([0-9]+)\.sqlite$`)

// codexThread is a row of the threads table. Review and subagent
// threads have another thread_source and are left out.
type codexThread struct {
	ID               string `json:"id"`
	Cwd              string `json:"cwd"`
	CreatedAtMs      int64  `json:"created_at_ms"`
	UpdatedAtMs      int64  `json:"updated_at_ms"`
	Originator       string `json:"originator"`
	Source           string `json:"source"`
	GitBranch        string `json:"git_branch"`
	Name             string `json:"name"`
	Title            string `json:"title"`
	FirstUserMessage string `json:"first_user_message"`
}

const codexThreadsQuery = `SELECT id, cwd, created_at_ms, updated_at_ms, coalesce(originator, '') AS originator,
  coalesce(source, '') AS source, coalesce(git_branch, '') AS git_branch, coalesce(name, '') AS name,
  coalesce(title, '') AS title, coalesce(first_user_message, '') AS first_user_message
FROM threads
WHERE thread_source = 'user' AND archived = 0
ORDER BY updated_at_ms DESC`

type codexLastPrompt struct {
	ThreadID string          `json:"thread_id"`
	Content  json.RawMessage `json:"content"`
}

// The newest user message of each thread, through the index on user messages.
const codexLastPromptsQuery = `SELECT t.thread_id AS thread_id, json_extract(t.item_json, '$.content') AS content
FROM thread_items t
JOIN (SELECT thread_id, max(rollout_ordinal) AS ordinal FROM thread_items WHERE item_type = 'userMessage' GROUP BY thread_id) newest
  ON t.thread_id = newest.thread_id AND t.rollout_ordinal = newest.ordinal`

func (s *Scanner) scanCodex(ctx context.Context, opts ScanOptions) sourceResult {
	statePath := newestNumbered(s.Stores.CodexDir, codexStatePattern)
	if statePath == "" {
		return missing(ToolCodex, "No Codex thread index in "+s.Stores.CodexDir)
	}
	threads, err := queryRows[codexThread](ctx, s.SQLite, statePath, codexThreadsQuery)
	if errors.Is(err, errNoSQLite) {
		return missing(ToolCodex, "Reading Codex threads needs the sqlite3 command")
	}
	if err != nil {
		return missing(ToolCodex, "Could not read Codex's thread index "+filepath.Base(statePath))
	}
	lastPrompts := s.codexLastPrompts(ctx)
	editor := extensionEditor(s.Stores.Home, "openai.chatgpt", opts.Editor)
	conversations := make([]Conversation, 0, len(threads))
	for _, thread := range threads {
		if !sessionIDPattern.MatchString(thread.ID) || codexExec(thread) {
			continue
		}
		c := Conversation{
			Tool: ToolCodex, ID: thread.ID, Folder: thread.Cwd, Branch: thread.GitBranch,
			StartedAt: timeFromMillis(thread.CreatedAtMs), UpdatedAt: timeFromMillis(thread.UpdatedAtMs),
		}
		c.fill(texts{title: firstNonEmpty(thread.Name, thread.Title), firstPrompt: thread.FirstUserMessage, lastPrompt: lastPrompts[thread.ID]})
		codexRoute(&c, thread, editor)
		conversations = append(conversations, c)
	}
	s.markLiveCodexThreads(conversations)
	return found(ToolCodex, within(conversations, opts.Within))
}

// codexRoute opens a thread where it started: the editor extension, the
// desktop app, or a terminal.
func codexRoute(c *Conversation, thread codexThread, editor editorApp) {
	switch {
	case thread.Originator == "Codex Desktop":
		c.App = AppDesktop
		c.Open = Route{Kind: RouteLink, Label: "Open in Codex app", Link: "codex://threads/" + c.ID}
	case thread.Originator == "codex_vscode" || (thread.Originator == "" && thread.Source == "vscode"):
		c.App = AppIDE
		c.editor = &editor
		c.Open = Route{Kind: RouteLink, Label: "Open in " + editor.label, Link: editor.id + "://openai.chatgpt/local/" + c.ID}
	default:
		c.App = AppCLI
		c.Open = Route{Kind: RouteTerminal, Label: "Open in Terminal", Command: resumeIn(c.Folder, "codex resume "+c.ID)}
	}
}

// codexExec is a `codex exec` run: a job a program started, not a
// conversation a person resumes.
func codexExec(thread codexThread) bool {
	return thread.Originator == "codex_exec" || thread.Source == "exec"
}

// codexLiveWindow bounds which threads are checked for a live process:
// one untouched for a week is not in use.
const codexLiveWindow = 7 * 24 * time.Hour

// markLiveCodexThreads marks the threads a running Codex process has
// loaded. Codex keeps thread-writer-locks/<id>.lock open while a process
// holds the thread; the files stay after it exits, so the test is
// whether a process still has one open. An open CLI thread switches to
// the terminal that runs it, since a second resume would compete with it.
func (s *Scanner) markLiveCodexThreads(conversations []Conversation) {
	cutoff := s.Now().Add(-codexLiveWindow).UTC().Format(time.RFC3339)
	lockOf := map[string]int{}
	var locks []string
	for i, c := range conversations {
		if c.UpdatedAt < cutoff {
			continue
		}
		lock := filepath.Join(s.Stores.CodexDir, "thread-writer-locks", c.ID+".lock")
		if _, err := os.Stat(lock); err == nil {
			lockOf[lock] = i
			locks = append(locks, lock)
		}
	}
	for lock, pid := range s.Processes.Holders(locks) {
		i, ok := lockOf[lock]
		if !ok {
			continue
		}
		c := &conversations[i]
		c.Live = true
		if c.App == AppCLI {
			c.process = &liveProcess{PID: pid}
			c.Open = Route{Kind: RouteFocus, Label: "Open in Terminal", Command: c.Open.Command}
		}
	}
}

// codexLastPrompts maps thread ids to their newest prompt. Threads the
// history store does not cover show no latest prompt.
func (s *Scanner) codexLastPrompts(ctx context.Context) map[string]string {
	prompts := map[string]string{}
	historyPath := newestNumbered(s.Stores.CodexDir, codexHistoryPattern)
	if historyPath == "" {
		return prompts
	}
	rows, err := queryRows[codexLastPrompt](ctx, s.SQLite, historyPath, codexLastPromptsQuery)
	if err != nil {
		return prompts
	}
	for _, row := range rows {
		prompts[row.ThreadID] = codexContentText(row.Content)
	}
	return prompts
}

// codexContentText joins the text parts of a user message. sqlite3
// returns the json_extract result as a JSON string holding the array.
func codexContentText(raw json.RawMessage) string {
	var encoded string
	if json.Unmarshal(raw, &encoded) == nil {
		raw = json.RawMessage(encoded)
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	var texts []string
	for _, part := range parts {
		if part.Type == "text" {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// newestNumbered returns the file in dir matching pattern with the
// highest number, as Codex bumps the number on a schema change.
func newestNumbered(dir string, pattern *regexp.Regexp) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	type numbered struct {
		n    int
		name string
	}
	var candidates []numbered
	for _, entry := range entries {
		match := pattern.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		n, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		candidates = append(candidates, numbered{n, entry.Name()})
	}
	if len(candidates) == 0 {
		return ""
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].n > candidates[j].n })
	return filepath.Join(dir, candidates[0].name)
}
