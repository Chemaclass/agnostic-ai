package conversations

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Codex indexes its threads in state_<n>.sqlite and keeps each prompt in
// thread_history_<n>.sqlite, so neither the compressed rollout files nor
// a zstd library are needed to list them. Only a thread a process has
// loaded has its rollout read, for whether a turn runs; Codex writes
// that one as plain lines.

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
	// RolloutPath is the file Codex appends the thread's events to.
	RolloutPath string `json:"rollout_path"`
}

const codexThreadsQuery = `SELECT id, cwd, created_at_ms, updated_at_ms, coalesce(originator, '') AS originator,
  coalesce(source, '') AS source, coalesce(git_branch, '') AS git_branch, coalesce(name, '') AS name,
  coalesce(title, '') AS title, coalesce(first_user_message, '') AS first_user_message,
  coalesce(rollout_path, '') AS rollout_path
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

type codexLastReply struct {
	ThreadID string `json:"thread_id"`
	Start    string `json:"start"`
	Ending   string `json:"ending"`
	Length   int    `json:"length"`
	Asked    string `json:"asked"`
}

// codexLastRepliesQuery reads, for each listed thread, the last agent
// message after its last prompt, through the index on prompts and the
// one on items by ordinal. A prompt typed while a turn runs is stored in
// that turn, so turns do not mark what came after it. Each row holds
// at most the first and last replyCheckedBytes+1 characters of the
// message, so a long reply costs a fixed amount.
func codexLastRepliesQuery(threads []string) string {
	text := `coalesce(json_extract(a.item_json, '$.text'), '')`
	part := strconv.Itoa(replyCheckedBytes + 1)
	return `SELECT a.thread_id AS thread_id,
  substr(` + text + `, 1, ` + part + `) AS start,
  substr(` + text + `, -` + part + `) AS ending,
  length(` + text + `) AS length,
  substr(coalesce(json_extract(a.item_json, '$.questions[0].title'), ''), 1, ` + part + `) AS asked
FROM (SELECT p.thread_id AS thread_id,
        (SELECT max(i.rollout_ordinal) FROM thread_items i
         WHERE i.thread_id = p.thread_id AND i.rollout_ordinal > p.ordinal AND i.item_type = 'agentMessage') AS ordinal
      FROM (SELECT thread_id, max(rollout_ordinal) AS ordinal FROM thread_items
            WHERE item_type = 'userMessage' AND thread_id IN ('` + strings.Join(threads, "', '") + `')
            GROUP BY thread_id) p) latest
JOIN thread_items a ON a.thread_id = latest.thread_id AND a.rollout_ordinal = latest.ordinal`
}

// codexSaid is the part of an agent message a row keeps.
type codexSaid struct {
	reply, question string
}

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
	rollouts := map[string]string{}
	textsOf := map[string]texts{}
	for _, thread := range threads {
		if !sessionIDPattern.MatchString(thread.ID) || codexExec(thread) {
			continue
		}
		c := Conversation{
			Tool: ToolCodex, ID: thread.ID, Folder: thread.Cwd, Branch: thread.GitBranch,
			StartedAt: timeFromMillis(thread.CreatedAtMs), UpdatedAt: timeFromMillis(thread.UpdatedAtMs),
		}
		textsOf[thread.ID] = texts{title: firstNonEmpty(thread.Name, thread.Title), firstPrompt: thread.FirstUserMessage, lastPrompt: lastPrompts[thread.ID]}
		codexRoute(&c, thread, editor)
		conversations = append(conversations, c)
		rollouts[thread.ID] = thread.RolloutPath
	}
	s.markLiveCodexThreads(conversations, rollouts)
	conversations = within(conversations, opts.Within)
	ids := make([]string, 0, len(conversations))
	for _, c := range conversations {
		ids = append(ids, c.ID)
	}
	lastReplies := s.codexLastReplies(ctx, ids)
	for i := range conversations {
		t, said := textsOf[conversations[i].ID], lastReplies[conversations[i].ID]
		t.reply, t.question = said.reply, said.question
		s.fill(&conversations[i], t)
	}
	return found(ToolCodex, conversations)
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

// liveWindow bounds which chats are checked for a live process: one
// untouched for a week is not in use.
const liveWindow = 7 * 24 * time.Hour

// markLiveCodexThreads marks the threads a running Codex process has
// loaded. Codex keeps thread-writer-locks/<id>.lock open while a process
// holds the thread; the files stay after it exits, so the test is
// whether a process still has one open. An open CLI thread switches to
// the terminal that runs it, since a second resume would compete with it.
// A loaded thread whose rollout says a turn runs is working.
func (s *Scanner) markLiveCodexThreads(conversations []Conversation, rollouts map[string]string) {
	var liveRollouts []string
	cutoff := s.Now().Add(-liveWindow).UTC().Format(time.RFC3339)
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
		if rollout, ok := s.codexRollout(rollouts[c.ID]); ok {
			liveRollouts = append(liveRollouts, rollout)
			var quiet time.Time
			c.Activity, quiet = codexActivity(rollout, s.Now())
			if !quiet.IsZero() {
				c.QuietSince = quiet.UTC().Format(time.RFC3339)
			}
		}
		if c.App == AppCLI {
			c.process = &liveProcess{PID: pid}
			c.Open = Route{Kind: RouteFocus, Label: "Open in Terminal", Command: c.Open.Command}
		}
	}
	// A turn that begins or ends writes only to the rollout, so the
	// rollouts of loaded threads are among the files a Codex read depends on.
	s.rolloutsMu.Lock()
	s.codexLiveRollouts = liveRollouts
	s.rolloutsMu.Unlock()
}

// codexRollout checks a rollout path from the thread index before it
// is opened: a plain-lines file under Codex's own folder.
func (s *Scanner) codexRollout(path string) (string, bool) {
	if path == "" || filepath.Ext(path) != ".jsonl" {
		return "", false
	}
	inside, err := filepath.Rel(s.Stores.CodexDir, path)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) || filepath.IsAbs(inside) {
		return "", false
	}
	return path, true
}

// codexQuietSpan is how long a turn may write nothing before it is
// taken to wait for the person. Codex does not write down that it asks
// for an approval or for input, and a turn that hangs or lost its end
// never says so either. All three look the same: a turn that began and
// a rollout that holds still. A turn that works writes as it goes, so
// one silent for this long needs a look, at the price of a command that
// runs longer reading as a wait.
const codexQuietSpan = 5 * time.Minute

// codexActivity is what a loaded thread does, read from its rollout:
// working while a turn runs and writes, waiting once that turn has
// written nothing for codexQuietSpan, and nothing the rollout can say
// between turns. A wait comes with the rollout's last write, since the
// wait is only read from that silence.
func codexActivity(rollout string, now time.Time) (Activity, time.Time) {
	if !codexTurnRuns(rollout) {
		return "", time.Time{}
	}
	info, err := os.Stat(rollout)
	if err != nil {
		return "", time.Time{}
	}
	if now.Sub(info.ModTime()) >= codexQuietSpan {
		return ActivityWaiting, info.ModTime()
	}
	return ActivityWorking, time.Time{}
}

// codexRolloutTail is how much of a rollout's end is read for its last
// turn event. Events between two of them run to megabytes in a long
// turn; see codexTurnRuns for a tail that holds none.
const codexRolloutTail = 256 << 10

// codexTurnEvents are the events that begin and end a turn, and
// whether a turn runs after each.
var codexTurnEvents = map[string]bool{
	"task_started":  true,
	"task_complete": false,
	"turn_aborted":  false,
}

// codexTurnRuns reports whether the rollout's last turn event began a
// turn. Codex writes task_started when a turn begins and task_complete
// or turn_aborted when it ends, and nothing after an end until the next
// begin. So a tail that holds no turn event, cut from a longer file, is
// the middle of a turn that runs; a whole file with none has no turn.
// A file that cannot be read says nothing, and the thread shows as open.
func codexTurnRuns(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return false
	}
	from := max(info.Size()-codexRolloutTail, 0)
	tail := make([]byte, info.Size()-from)
	if _, err := file.ReadAt(tail, from); err != nil && !errors.Is(err, io.EOF) {
		return false
	}
	lines := strings.Split(string(tail), "\n")
	if from > 0 {
		// The first line of a cut tail is the end of a longer one.
		lines = lines[1:]
	}
	for i := len(lines) - 1; i >= 0; i-- {
		var event struct {
			Type    string `json:"type"`
			Payload struct {
				Type string `json:"type"`
			} `json:"payload"`
		}
		if json.Unmarshal([]byte(lines[i]), &event) != nil || event.Type != "event_msg" {
			continue
		}
		if runs, known := codexTurnEvents[event.Payload.Type]; known {
			return runs
		}
	}
	return from > 0
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

// codexLastReplies maps thread ids to their agent's latest message. It
// is a query of its own, so a history store it cannot read still shows
// the prompts. threads are ids that passed sessionIDPattern, so they
// are safe inside the query.
func (s *Scanner) codexLastReplies(ctx context.Context, threads []string) map[string]codexSaid {
	replies := map[string]codexSaid{}
	historyPath := newestNumbered(s.Stores.CodexDir, codexHistoryPattern)
	if historyPath == "" || len(threads) == 0 {
		return replies
	}
	rows, err := queryRows[codexLastReply](ctx, s.SQLite, historyPath, codexLastRepliesQuery(threads))
	if err != nil {
		return replies
	}
	for _, row := range rows {
		ending := row.Ending
		// A cut ending that holds no line break may start mid-line, so
		// its last line is not known whole.
		if utf8.RuneCountInString(ending) < row.Length && !strings.Contains(ending, "\n") {
			ending = ""
		}
		reply, question := agentSaid(row.Start, ending, row.Asked)
		if reply != "" || question != "" {
			replies[row.ThreadID] = codexSaid{reply: reply, question: question}
		}
	}
	return replies
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
