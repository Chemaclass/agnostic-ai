package conversations

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// pageSpan is how far back one page of the list reaches.
const pageSpan = 7 * 24 * time.Hour

// Stores locates each tool's conversation store.
type Stores struct {
	Home string
	// ClaudeDir is Claude Code's config folder (CLAUDE_CONFIG_DIR or ~/.claude).
	ClaudeDir string
	// CodexDir is Codex's home (CODEX_HOME or ~/.codex).
	CodexDir string
	// CursorDB is Cursor's global state database, which indexes its chats.
	CursorDB string
	// CursorDir is ~/.cursor, which holds cursor-agent chats and extensions.
	CursorDir string
}

// DefaultStores finds the stores for the current user and OS.
func DefaultStores() Stores {
	home, _ := os.UserHomeDir()
	claude := os.Getenv("CLAUDE_CONFIG_DIR")
	if claude == "" {
		claude = filepath.Join(home, ".claude")
	}
	codex := os.Getenv("CODEX_HOME")
	if codex == "" {
		codex = filepath.Join(home, ".codex")
	}
	return Stores{
		Home:      home,
		ClaudeDir: claude,
		CodexDir:  codex,
		CursorDB:  filepath.Join(cursorUserDir(home), "globalStorage", "state.vscdb"),
		CursorDir: filepath.Join(home, ".cursor"),
	}
}

func cursorUserDir(home string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Cursor", "User")
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "Cursor", "User")
	default:
		return filepath.Join(home, ".config", "Cursor", "User")
	}
}

// Scanner reads the stores. It keeps what it parsed from each Claude
// Code file and cursor-agent chat until the file changes, so a refresh
// rereads only new chats.
type Scanner struct {
	Stores    Stores
	SQLite    SQLiteQuery
	Processes Processes
	Now       func() time.Time
	// MarksPath is the file that keeps the person's seen and completed
	// marks. Empty keeps none.
	MarksPath string
	// Cloud lists the Claude Code sessions that run at claude.ai/code.
	// Nil reads none, and nothing leaves this machine.
	Cloud CloudSessions
	// CodexCloud lists the Codex threads that run in the Codex cloud.
	// Nil reads none.
	CodexCloud CloudSessions

	mu                sync.Mutex
	claude            map[string]claudeCacheEntry
	cursorAgents      map[string]cached[cursorAgentMeta]
	cursorTranscripts map[string]cached[[2]string]
	cursorPrompts     map[string]cached[cursorPrompts]
	shown             shownCache
	keptSources       map[Tool]keptSource
	keptClouds        map[Tool]*keptCloud

	// rolloutsMu guards codexLiveRollouts apart from mu: Stamp reads
	// them without waiting for a scan to end.
	rolloutsMu sync.Mutex
	// codexLiveRollouts are the rollouts of the Codex threads a process
	// had loaded at the last read.
	codexLiveRollouts []string
}

// NewScanner reads the current user's stores with the system sqlite3
// and process table.
func NewScanner() *Scanner {
	return &Scanner{Stores: DefaultStores(), SQLite: RunSQLite, Processes: SystemProcesses{}, Now: time.Now, MarksPath: DefaultMarksPath()}
}

// ScanOptions narrow and shape one scan.
type ScanOptions struct {
	// Editor is the editor chosen in Settings; extension links use its scheme.
	Editor string
	// Within keeps only conversations whose folder is inside one of
	// these folders. Empty keeps every conversation.
	Within []string
	// Before asks for the page of conversations last written before
	// this time. Zero asks for the first page: the open conversations
	// and the past week.
	Before time.Time
	// Query keeps the conversations of every age whose shown text holds
	// each of its words; the pages then hold only those.
	Query string
	// Tool keeps one tool's conversations; empty keeps all three.
	Tool Tool
}

// Scan returns one page of conversations, newest first. The first page
// holds every open conversation and the week before now; each later
// page holds the week before its Before time, reaching further back
// past weeks with no conversation, so a page is never empty while older
// ones remain. View.Next asks for the following page.
func (s *Scanner) Scan(ctx context.Context, opts ScanOptions) View {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.shown.start()
	defer s.shown.finish()
	results, cloudNote := s.readAll(ctx, opts)
	needsYou, marksErr := s.applyMarks(results)
	var all []Conversation
	for _, result := range results {
		if opts.Tool == "" || result.source.Tool == opts.Tool {
			all = append(all, result.conversations...)
		}
	}
	words := queryWords(opts.Query)
	if len(words) > 0 {
		all = s.searchAll(ctx, all, words)
	}
	first := opts.Before.IsZero()
	to := opts.Before
	if first {
		// Next is written in whole seconds, so the page boundary must be too.
		to = s.Now().Truncate(time.Second)
	}
	page, from, more := weekPage(all, to, first)
	s.fillPending(ctx, page)
	sortNewestFirst(page)

	view := View{
		SchemaVersion: SchemaVersion, GeneratedAt: s.Now().UTC().Format(time.RFC3339),
		From: from.UTC().Format(time.RFC3339), To: to.UTC().Format(time.RFC3339),
		Tools: []Source{}, Conversations: page,
	}
	view.NeedsYou = needsYou
	if marksErr != nil {
		view.MarksNote = "Seen and completed marks are unavailable: " + marksErr.Error()
	}
	view.CloudNote = cloudNote
	if more {
		view.Next = view.From
	}
	if len(words) > 0 {
		matches := len(all)
		view.Matches = &matches
	}
	for _, result := range results {
		result.source.Count = len(result.conversations)
		for _, c := range page {
			if c.Tool == result.source.Tool {
				result.source.Shown++
			}
		}
		view.Tools = append(view.Tools, result.source)
	}
	if view.Conversations == nil {
		view.Conversations = []Conversation{}
	}
	return view
}

// Find returns one conversation of any age from a fresh read.
func (s *Scanner) Find(ctx context.Context, tool Tool, id string, opts ScanOptions) (Conversation, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	results, _ := s.readAll(ctx, opts)
	for _, result := range results {
		for _, c := range result.conversations {
			if c.Tool == tool && c.ID == id {
				return c, true
			}
		}
	}
	return Conversation{}, false
}

// readAll reads every store. A tool's cloud conversations count among
// its own; cloudNote says why they are missing, when they are.
func (s *Scanner) readAll(ctx context.Context, opts ScanOptions) (results []sourceResult, cloudNote string) {
	claude := s.scanClaude(ctx, opts)
	claudeCloud, claudeNote := s.cloudRows(ctx, opts, ToolClaude, s.Cloud)
	claude.conversations = append(claude.conversations, claudeCloud...)
	codex := s.kept(ctx, ToolCodex, opts, s.codexFiles(), func() sourceResult { return s.scanCodex(ctx, opts) })
	codexCloud, codexNote := s.cloudRows(ctx, opts, ToolCodex, s.CodexCloud)
	codex.conversations = append(codex.conversations, codexCloud...)
	var notes []string
	for _, note := range []string{claudeNote, codexNote} {
		if note != "" {
			notes = append(notes, note)
		}
	}
	return []sourceResult{
		claude,
		codex,
		s.kept(ctx, ToolCursor, opts, s.cursorFiles(), func() sourceResult { return s.scanCursor(ctx, opts) }),
	}, strings.Join(notes, " ")
}

// weekPage picks one page from all: below to, from a week earlier, or
// from a week before the newest conversation when that week is empty.
// The first page also holds every open conversation, whatever its age,
// and takes rows dated after to, which a skewed clock can write.
func weekPage(all []Conversation, to time.Time, first bool) (page []Conversation, from time.Time, more bool) {
	var older []Conversation
	for _, c := range all {
		switch {
		case c.Live && first:
			page = append(page, c)
		case c.Live:
			// The first page already showed it.
		case first || lastWritten(c).Before(to):
			older = append(older, c)
		}
	}
	from = to.Add(-pageSpan)
	if newest, ok := newestBefore(older, to, first); ok && newest.Before(from) {
		from = newest.Add(-pageSpan)
	}
	for _, c := range older {
		if lastWritten(c).Before(from) {
			more = true
		} else {
			page = append(page, c)
		}
	}
	return page, from, more
}

// newestBefore is the latest last-written time among list before to, or
// any time at all on the first page; an undated row counts as the zero time.
func newestBefore(list []Conversation, to time.Time, first bool) (time.Time, bool) {
	var newest time.Time
	found := false
	for _, c := range list {
		written := lastWritten(c)
		if (first || written.Before(to)) && (!found || written.After(newest)) {
			newest, found = written, true
		}
	}
	return newest, found
}

func lastWritten(c Conversation) time.Time {
	written, err := time.Parse(time.RFC3339, c.UpdatedAt)
	if err != nil {
		return time.Time{}
	}
	return written
}

// sourceResult is one reader's conversations, narrowed to the scan's folders.
type sourceResult struct {
	source        Source
	conversations []Conversation
}

func missing(tool Tool, note string) sourceResult {
	return sourceResult{source: Source{Tool: tool, Label: toolLabels[tool], Note: note}}
}

func found(tool Tool, conversations []Conversation) sourceResult {
	return sourceResult{source: Source{Tool: tool, Label: toolLabels[tool], Found: true}, conversations: conversations}
}

func sortNewestFirst(list []Conversation) {
	sort.SliceStable(list, func(i, j int) bool { return list[i].UpdatedAt > list[j].UpdatedAt })
}

func within(list []Conversation, roots []string) []Conversation {
	if len(roots) == 0 {
		return list
	}
	var kept []Conversation
	for _, c := range list {
		if inAnyRoot(c.Folder, roots) {
			kept = append(kept, c)
		}
	}
	return kept
}

func inAnyRoot(folder string, roots []string) bool {
	if folder == "" {
		return false
	}
	for _, root := range roots {
		rel, err := filepath.Rel(root, folder)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func timeFromMillis(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}

// editorApp is an editor whose URL scheme opens extension links.
type editorApp struct {
	// id is the editor id the Settings page uses, and its URL scheme.
	id, label string
	// app is the macOS application name, for opening a folder in it.
	app string
	// extensions is the folder under home that holds its extensions.
	extensions string
}

var editorApps = []editorApp{
	{"cursor", "Cursor", "Cursor", ".cursor/extensions"},
	{"vscode", "VS Code", "Visual Studio Code", ".vscode/extensions"},
	{"vscode-insiders", "VS Code Insiders", "Visual Studio Code - Insiders", ".vscode-insiders/extensions"},
	{"vscodium", "VSCodium", "VSCodium", ".vscode-oss/extensions"},
}

// extensionEditor picks the editor that opens an extension's link: the
// one chosen in Settings when the extension is installed there,
// otherwise the first editor that has it, otherwise the chosen one.
func extensionEditor(home, extension, chosen string) editorApp {
	installed := func(dir string) bool {
		matches, _ := filepath.Glob(filepath.Join(home, dir, extension+"-*"))
		return len(matches) > 0
	}
	for _, app := range editorApps {
		if app.id == chosen && installed(app.extensions) {
			return app
		}
	}
	for _, app := range editorApps {
		if installed(app.extensions) {
			return app
		}
	}
	for _, app := range editorApps {
		if app.id == chosen {
			return app
		}
	}
	return editorApps[0]
}

// shellQuote quotes a path for a POSIX shell.
func shellQuote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

// resumeIn runs command in folder, or as is when the folder is unknown.
func resumeIn(folder, command string) string {
	if folder == "" {
		return command
	}
	return "cd " + shellQuote(folder) + " && " + command
}
