package conversations

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
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
	// ClaudeAppDir is the Claude desktop app's data folder, whose web
	// storage caches the person's chats and Cowork tasks.
	ClaudeAppDir string
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
		Home:         home,
		ClaudeDir:    claude,
		CodexDir:     codex,
		CursorDB:     filepath.Join(cursorUserDir(home), "globalStorage", "state.vscdb"),
		CursorDir:    filepath.Join(home, ".cursor"),
		ClaudeAppDir: claudeAppDir(home),
	}
}

func claudeAppDir(home string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Claude")
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "Claude")
	default:
		return filepath.Join(home, ".config", "Claude")
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
	Stores         Stores
	SQLite         SQLiteQuery
	Processes      Processes
	Now            func() time.Time
	shellStops     atomic.Uint64
	stoppingShells sync.Map
	shellSignal    func(int, bool) error
	shellWait      func(context.Context, time.Duration) error
	// MarksPath is the file that keeps the person's seen and completed
	// marks. Empty keeps none.
	MarksPath string
	// Cloud lists the Claude Code sessions that run at claude.ai/code.
	// Nil reads none, and nothing leaves this machine.
	Cloud CloudSessions
	// CodexCloud lists the Codex threads that run in the Codex cloud.
	// Nil reads none.
	CodexCloud CloudSessions

	mu               sync.Mutex
	claude           map[string]claudeCacheEntry
	claudeWorkers    map[string]*claudeWorkerCache
	claudeAgentMetas map[string]cached[claudeAgentMeta]
	// claudeAgentReplies are subagents' latest messages, by transcript.
	claudeAgentReplies map[string]cached[string]
	codexWorkers       map[string]*nativeLog[codexWorkerTurn]
	cursorAgents       map[string]cached[cursorAgentMeta]
	cursorTranscripts  map[string]cached[texts]
	// cursorSlugs caches the folders each cursor-agent project slug
	// gives, since finding them reads folders from the root down.
	cursorSlugs   map[string][]string
	cursorPrompts map[string]cached[texts]
	claudeApp     cached[claudeAppRead]
	shown         shownCache
	keptSources   map[Tool]keptSource
	keptClouds    map[Tool]*keptCloud
	// sharedClouds maps the mark key of each shared row of the last read
	// to the cloud id it took in, so a mark on it reaches both.
	sharedClouds map[string]string
	// twins holds, by mark key, the cloud twin each local session took
	// in while it ran; see followTwins.
	twins map[string]twin
	// appArchives holds, by mark key, the conversations their own app
	// keeps archived; see stillAppArchived.
	appArchives map[string]appArchive
	// claudeChatsSeen holds, by id, the Claude app chats an earlier read
	// found in the app's cached list; see claudeChatsGone.
	claudeChatsSeen map[string]Conversation
	// search holds the latest search's matches for its later pages.
	search keptSearch

	// lastReadMu guards lastRead apart from mu: an open reads it without
	// waiting for a scan to end.
	lastReadMu sync.Mutex
	lastRead   keptRead
	// readsBegun counts the reads that began, under lastReadMu.
	readsBegun uint64

	// openingMu guards opening, apart from mu and the marks lock: an open
	// takes it only to join or leave the list.
	openingMu sync.Mutex
	// opening holds, by mark key, the open in progress for each chat, so
	// a second open of the same chat waits for it instead of resuming the
	// chat a second time.
	opening map[string]*sharedOpen

	// rolloutsMu guards codexLiveRollouts and cursorLiveTranscripts apart
	// from mu: Stamp reads them without waiting for a scan to end.
	rolloutsMu sync.Mutex
	// codexLiveRollouts are the rollouts of the Codex threads a process
	// had loaded at the last read.
	codexLiveRollouts []string
	// Native Claude worker logs and their folders, including folders that do
	// not exist yet, so a worker launch or completion invalidates the stamp.
	claudeWorkerFiles []string
	// cursorLiveTranscripts are the transcripts of the cursor-agent chats
	// a process held at the last read.
	cursorLiveTranscripts []string

	// betweenReads runs between the two reads Delete makes of a session's
	// files, where a resumed session can write; nil runs nothing.
	betweenReads func()

	// scanningSince is when the holder of mu took it, in Unix
	// nanoseconds; zero while none holds it. It is read without mu.
	scanningSince atomic.Int64
}

// NewScanner reads the current user's stores with the system sqlite3
// and process table.
func NewScanner() *Scanner {
	return &Scanner{Stores: DefaultStores(), SQLite: RunSQLite, Processes: SystemProcesses{}, Now: time.Now, MarksPath: DefaultMarksPath()}
}

// ScanOptions narrow and shape one scan.
type ScanOptions struct {
	// StuckShellAfter is the age at which a Claude Code shell needs attention. Nil uses the default and zero disables it.
	StuckShellAfter *time.Duration
	// Editor is the editor chosen in Settings; extension links use its scheme.
	Editor string
	// Within keeps only conversations whose folder is inside one of
	// these folders. Empty keeps every conversation.
	Within []string
	// Before asks for the page of conversations last written before
	// this time. Zero asks for the first page: the open conversations,
	// the ones that need the person, and the past week.
	Before time.Time
	// Query keeps the conversations of every age whose shown text holds
	// each of its words; the pages then hold only those.
	Query string
	// Tool keeps one tool's conversations; empty keeps all three.
	Tool Tool
}

// Scan returns one page of conversations, newest first. The first page
// holds every open conversation, every one that needs the person, and
// the week before now; each later
// page holds the week before its Before time, reaching further back
// past weeks with no conversation, so a page is never empty while older
// ones remain. View.Next asks for the following page.
func (s *Scanner) Scan(ctx context.Context, opts ScanOptions) View {
	return s.scan(ctx, opts, nil)
}

// ScanStamped is Scan for a caller that took stamp just before. A
// search keys its read on stamp instead of stamping the stores again,
// so its rows are as fresh as stamp: a write made after it shows on
// the next request, whose stamp differs.
func (s *Scanner) ScanStamped(ctx context.Context, opts ScanOptions, stamp StoreStamp) View {
	return s.scan(ctx, opts, &stamp)
}

// ScanningSince is when the running scan, or the open, mark or delete
// that reads the stores, began; zero while none runs. Each waits for
// the one before, so one that never ends holds every later scan, and
// this time only grows older.
func (s *Scanner) ScanningSince() time.Time {
	if at := s.scanningSince.Load(); at != 0 {
		return time.Unix(0, at)
	}
	return time.Time{}
}

// lock takes mu and notes when, for ScanningSince.
func (s *Scanner) lock() {
	s.mu.Lock()
	s.held()
}

// held notes that mu was taken now.
func (s *Scanner) held() { s.scanningSince.Store(time.Now().UnixNano()) }

// unlock releases mu taken with lock or noted with held.
func (s *Scanner) unlock() {
	s.scanningSince.Store(0)
	s.mu.Unlock()
}

func (s *Scanner) scan(ctx context.Context, opts ScanOptions, stamp *StoreStamp) View {
	s.lock()
	defer s.unlock()
	words := queryWords(opts.Query)
	first := opts.Before.IsZero()
	read, kept := s.searchRead(ctx, opts, words, first, stamp)
	if !kept {
		defer s.shown.finish()
	}
	all := read.all
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
	view.NeedsYou, view.PermissionWaits = read.needsYou, read.waits
	if read.marksErr != nil {
		view.MarksNote = "Seen and completed marks are unavailable: " + read.marksErr.Error()
	}
	view.CloudNote = read.cloudNote
	if more {
		view.Next = view.From
	}
	if len(words) > 0 {
		matches := len(all)
		view.Matches = &matches
	}
	for _, source := range read.sources {
		for _, c := range page {
			if c.Tool == source.Tool {
				source.Shown++
			}
		}
		view.Tools = append(view.Tools, source)
	}
	if view.Conversations == nil {
		view.Conversations = []Conversation{}
	}
	return view
}

// keptSearch is what one read of every store gave a scan: the rows it
// keeps, with the sources and notes the view reports.
type keptSearch struct {
	// key names the search and the stamp of the files it read; empty
	// for a read that is not kept.
	key       string
	all       []Conversation
	sources   []Source
	needsYou  int
	waits     []Conversation
	marksErr  error
	cloudNote string
}

// searchRead reads every store and keeps the rows opts asks for. A
// search's later page takes the rows its first page read while Stamp
// holds, so reading the weeks of one search one after the other reads
// the stores once, as long as nothing changed between them. A kept read
// leaves the shown texts cache alone: one week's texts could replace a
// large store's. Otherwise searchRead starts that cache and the caller
// finishes it.
func (s *Scanner) searchRead(ctx context.Context, opts ScanOptions, words []string, first bool, stamp *StoreStamp) (read keptSearch, kept bool) {
	var key string
	var files []string
	var stores string
	search := shapeOf(opts) + "\x00" + string(opts.Tool) + "\x00" + strings.Join(words, " ")
	if len(words) > 0 {
		// Taken before the read, like the server's list tag: a write made
		// during the read changes it, and the next page reads again.
		if stamp != nil {
			files, stores = stamp.files, stamp.stores
		} else {
			files = s.storeFiles()
			stores = fileStamp(files...)
		}
		key = s.stampOf(files, stores) + "\x00" + search
		if !first && s.search.key == key {
			return s.search, true
		}
	}
	s.shown.start()
	results, cloudNote := s.readAll(ctx, opts)
	needsYou, marksErr := s.applyMarks(results)
	read = keptSearch{needsYou: needsYou, marksErr: marksErr, cloudNote: cloudNote}
	for _, result := range results {
		for _, c := range result.conversations {
			if _, waits := c.PermissionWait(); waits && c.Live && needsPerson(c.State) {
				read.waits = append(read.waits, c)
			}
		}
		if opts.Tool == "" || result.source.Tool == opts.Tool {
			read.all = append(read.all, result.conversations...)
		}
		result.source.Count = len(result.conversations)
		read.sources = append(read.sources, result.source)
	}
	if len(words) > 0 {
		read.all = s.searchAll(ctx, read.all, words)
		read.key = key
		// The read itself writes the marks and finds the files of workers
		// that went live. When no store file changed meanwhile, the stamp
		// after it names what it read, so the next page takes it.
		if fileStamp(files...) == stores {
			read.key = s.Stamp() + "\x00" + search
		}
		// A canceled read may miss rows, so no later page takes it.
		if ctx.Err() != nil {
			read.key = ""
		}
		s.search = read
	}
	return read, false
}

// Find returns one conversation of any age from a fresh read.
func (s *Scanner) Find(ctx context.Context, tool Tool, id string, opts ScanOptions) (Conversation, bool) {
	return s.find(ctx, tool, id, opts, false)
}

func (s *Scanner) find(ctx context.Context, tool Tool, id string, opts ScanOptions, opening bool) (Conversation, bool) {
	s.lock()
	defer s.unlock()
	results, _ := s.readAll(ctx, opts)
	return rowIn(results, tool, id, opening)
}

// busyReuseSpan is how old the last read may be for an open to use it
// while another read runs. A read begins when a file changed or a new
// freshSpan began, so the one before it is at most a span old, and the
// running one may itself take most of a span when a store or a cloud is
// slow to answer.
const busyReuseSpan = 2 * freshSpan

// findToOpen finds the conversation an open names. With no read
// running it reads afresh, as Find does. While one runs it takes the
// row from the read before it, which is the list the person clicked in,
// so the open neither waits for that read nor reads everything again.
// Open checks again that a live session still runs before it switches
// to it. A row that would resume a session waits for the running read
// instead: the session may have started again since, and a resume would
// start a second copy of it. That read may have looked at the running
// processes before the open arrived, so the open takes its rows only
// from a read that began after it arrived, and otherwise reads afresh.
func (s *Scanner) findToOpen(ctx context.Context, tool Tool, id string, opts ScanOptions) (Conversation, bool) {
	if !s.mu.TryLock() {
		if results, ok := s.readSince(opts, busyReuseSpan); ok {
			if c, found := rowIn(results, tool, id, true); found && !s.wouldResume(c) {
				return c, true
			}
		}
		arrived := s.beganReads()
		s.mu.Lock()
		if results, ok := s.readBegunAfter(opts, arrived); ok {
			s.mu.Unlock()
			return rowIn(results, tool, id, true)
		}
	}
	s.held()
	defer s.unlock()
	results, _ := s.readAll(ctx, opts)
	return rowIn(results, tool, id, true)
}

// wouldResume reports whether opening c would resume its session in a
// new tab: its route says so, or the process that ran it has ended. A
// Claude Code session of the desktop app or an editor opens through a
// link, which resumes it there and names no process to check, so it
// always waits: the session may have moved to a terminal since.
func (s *Scanner) wouldResume(c Conversation) bool {
	switch c.Open.Kind {
	case RouteTerminal:
		return true
	case RouteFocus:
		return c.process == nil || !s.Processes.Alive(c.process.PID, c.process.ProcStart)
	case RouteLink:
		return c.Tool == ToolClaude && (c.App == AppDesktop || c.App == AppIDE)
	default:
		return false
	}
}

// rowIn finds one conversation in a read. With opening it also gives
// the rows only an open names: the cloud row a shared row took in,
// which opens its page, and the threads of the helpers a row in scope
// lists. The mark and rename requests never reach those.
func rowIn(results []sourceResult, tool Tool, id string, opening bool) (Conversation, bool) {
	for _, result := range results {
		for _, c := range result.conversations {
			if c.Tool == tool && c.ID == id {
				return c, true
			}
			if opening && c.Cloud != nil && c.Tool == tool && c.Cloud.ID == id {
				return *c.Cloud.row, true
			}
		}
	}
	if !opening || !listsHelper(results, tool, id) {
		return Conversation{}, false
	}
	for _, result := range results {
		for _, c := range result.unlisted {
			if c.Tool == tool && c.ID == id {
				return c, true
			}
		}
	}
	return Conversation{}, false
}

// keptRead is the scanner's latest read of every store.
type keptRead struct {
	at time.Time
	// number is the read's place in readsBegun.
	number uint64
	// shape is the part of the options the rows depend on.
	shape   string
	results []sourceResult
}

// shapeOf is the part of the options a reader's rows depend on.
func shapeOf(opts ScanOptions) string {
	return opts.Editor + "\x00" + strings.Join(opts.Within, "\x00") + "\x00" + opts.shellAfter().String()
}

// beginRead counts a read that begins and returns its number.
func (s *Scanner) beginRead() uint64 {
	s.lastReadMu.Lock()
	defer s.lastReadMu.Unlock()
	s.readsBegun++
	return s.readsBegun
}

// beganReads is how many reads have begun.
func (s *Scanner) beganReads() uint64 {
	s.lastReadMu.Lock()
	defer s.lastReadMu.Unlock()
	return s.readsBegun
}

// keepRead remembers a finished read for readSince.
func (s *Scanner) keepRead(number uint64, opts ScanOptions, results []sourceResult) {
	kept := make([]sourceResult, len(results))
	for i, result := range results {
		kept[i] = copyOf(result)
		// A later scan places the kept Codex helpers again, in place.
		kept[i].unlisted = slices.Clone(result.unlisted)
	}
	s.lastReadMu.Lock()
	defer s.lastReadMu.Unlock()
	s.lastRead = keptRead{at: s.Now(), number: number, shape: shapeOf(opts), results: kept}
}

// readBegunAfter returns the rows of the latest read when it used the
// same options and began after the given count of reads had begun.
func (s *Scanner) readBegunAfter(opts ScanOptions, reads uint64) ([]sourceResult, bool) {
	s.lastReadMu.Lock()
	defer s.lastReadMu.Unlock()
	last := s.lastRead
	if last.results == nil || last.shape != shapeOf(opts) || last.number <= reads {
		return nil, false
	}
	return last.results, true
}

// readSince returns the rows of the latest read when it used the same
// options and ended no longer than span ago.
func (s *Scanner) readSince(opts ScanOptions, span time.Duration) ([]sourceResult, bool) {
	s.lastReadMu.Lock()
	defer s.lastReadMu.Unlock()
	last := s.lastRead
	if last.results == nil || last.shape != shapeOf(opts) || s.Now().Sub(last.at) > span {
		return nil, false
	}
	return last.results, true
}

// readAll reads every store. A tool's cloud conversations count among
// its own; cloudNote says why they are missing, when they are.
func (s *Scanner) readAll(ctx context.Context, opts ScanOptions) (results []sourceResult, cloudNote string) {
	readTable := sync.OnceValues(s.Processes.Table)
	number := s.beginRead()
	claude := readAt(s.Now(), s.scanClaude(ctx, opts))
	claudeCloud, claudeNote := s.cloudRows(ctx, opts, ToolClaude, s.Cloud)
	claude.conversations = shareCloud(append(claude.conversations, claudeCloud...))
	whole := ctx.Err() == nil && len(opts.Within) == 0 && claude.source.Found && s.cloudWhole(ToolClaude, s.Cloud)
	claude.conversations = s.followTwins(claude.conversations, whole)
	s.sharedClouds = nil
	for _, c := range claude.conversations {
		if c.Cloud != nil {
			if s.sharedClouds == nil {
				s.sharedClouds = map[string]string{}
			}
			s.sharedClouds[markKey(c.Tool, c.ID)] = c.Cloud.ID
		}
	}
	appRows, appNote := s.claudeAppRows(opts)
	if len(appRows) > 0 {
		claude.source.Found = true
	}
	claude.conversations = append(claude.conversations, readAt(s.Now(), sourceResult{conversations: appRows}).conversations...)
	claude.source.Note = joinNotes(claude.source.Note, appNote)
	codex := s.kept(ctx, ToolCodex, opts, s.codexFiles(), func() sourceResult { return s.scanCodex(ctx, opts) })
	codexCloud, codexNote := s.cloudRows(ctx, opts, ToolCodex, s.CodexCloud)
	codex.conversations = append(codex.conversations, codexCloud...)
	var notes []string
	for _, note := range []string{claudeNote, codexNote} {
		if note != "" {
			notes = append(notes, note)
		}
	}
	results = []sourceResult{
		claude,
		codex,
		s.kept(ctx, ToolCursor, opts, s.cursorFiles(), func() sourceResult { return s.scanCursor(ctx, opts) }),
	}
	if s.hasChatGPTApp() {
		results = append(results, s.kept(ctx, ToolChatGPT, opts, s.chatGPTFiles(), func() sourceResult { return s.scanChatGPT(ctx, opts) }))
	}
	attachRuns(results)
	s.showStuckShells(results[0].conversations, opts.shellAfter(), readTable)
	s.placeLive(results, readTable)
	s.followAppArchives(results)
	// A canceled read stopped early, so its rows have gaps.
	if ctx.Err() == nil {
		s.keepRead(number, opts, results)
	}
	return results, strings.Join(notes, " ")
}

// joinNotes puts two notes about one source together, leaving out an
// empty one.
func joinNotes(first, second string) string {
	switch {
	case first == "":
		return second
	case second == "":
		return first
	default:
		return first + " " + second
	}
}

// weekPage picks one page from all: below to, from a week earlier, or
// from a week before the newest conversation when that week is empty.
// The first page also holds every open conversation, every one that
// needs the person, whatever its age, so the
// rows under Needs you are the ones View.NeedsYou counts. It takes rows dated after to, which a skewed
// clock can write.
func weekPage(all []Conversation, to time.Time, first bool) (page []Conversation, from time.Time, more bool) {
	var older []Conversation
	for _, c := range all {
		pinned := c.Live || needsPerson(c.State)
		switch {
		case pinned && first:
			page = append(page, c)
		case pinned:
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
	// runs are the running Codex exec runs, listed under the session that started them.
	runs []codexExecRun
	// unlisted are threads that are no row but open from a row's helpers:
	// running Codex exec runs and working Codex native subagents.
	unlisted []Conversation
	// inventories are the sources this read listed completely.
	inventories []inventory
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
// A folder with control characters gives no line: a person who copies
// it would paste them into a terminal.
func resumeIn(folder, command string) string {
	if hasControl(folder) {
		return ""
	}
	if folder == "" {
		return command
	}
	return "cd " + shellQuote(folder) + " && " + command
}
