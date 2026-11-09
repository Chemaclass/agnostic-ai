package conversations

import (
	"context"
	"path/filepath"
	"strings"
	"time"
)

// keptMaxAge is how long a reader's result is reused while its files
// hold still. The files say when a chat changed, but not when the app
// or process that had one open went away, so a kept result is read
// again at this age and an open mark can outlive its process by it.
const keptMaxAge = time.Minute

// keptSource is one reader's last result and what it was read from.
type keptSource struct {
	// stamp is the fileStamp of the reader's files when it ran.
	stamp string
	// shape is the part of the options the result depends on.
	shape  string
	at     time.Time
	result sourceResult
}

// kept returns the reader's last result while its files and the options
// it depends on are unchanged and the result is younger than
// keptMaxAge; otherwise it reads again. Codex and Cursor keep their
// chats in SQLite, read through the sqlite3 command, which costs far
// more than checking whether the files changed.
func (s *Scanner) kept(ctx context.Context, tool Tool, opts ScanOptions, files []string, read func() sourceResult) sourceResult {
	stamp := fileStamp(files...)
	shape := opts.Editor + "\x00" + strings.Join(opts.Within, "\x00")
	now := s.Now()
	if last, ok := s.keptSources[tool]; ok && last.stamp == stamp && last.shape == shape && now.Sub(last.at) < keptMaxAge && !now.Before(last.at) {
		return copyOf(last.result)
	}
	result := read()
	// A canceled read stopped early; keeping it would show its gaps for keptMaxAge.
	if ctx.Err() != nil {
		return result
	}
	if s.keptSources == nil {
		s.keptSources = map[Tool]keptSource{}
	}
	s.keptSources[tool] = keptSource{stamp: stamp, shape: shape, at: now, result: copyOf(result)}
	return result
}

// copyOf gives the caller its own rows: a scan writes each row's state
// and marks in place.
func copyOf(result sourceResult) sourceResult {
	result.conversations = append([]Conversation(nil), result.conversations...)
	return result
}

// withWAL lists each SQLite file with its write-ahead log, where the
// newest writes sit until SQLite folds them in.
func withWAL(databases ...string) []string {
	files := make([]string, 0, 2*len(databases))
	for _, database := range databases {
		files = append(files, database, database+"-wal")
	}
	return files
}

// codexFiles are the files a Codex read depends on: the thread index,
// the prompt history and the folder of thread locks, whose own time
// changes when a lock comes or goes.
func (s *Scanner) codexFiles() []string {
	files := withWAL(newestNumbered(s.Stores.CodexDir, codexStatePattern), newestNumbered(s.Stores.CodexDir, codexHistoryPattern))
	return append(files, filepath.Join(s.Stores.CodexDir, "thread-writer-locks"))
}

// cursorFiles are the files a Cursor read depends on: the chat index,
// the open windows, each workspace's chat selection and each
// cursor-agent chat.
func (s *Scanner) cursorFiles() []string {
	globalStorage := filepath.Dir(s.Stores.CursorDB)
	workspaces, _ := filepath.Glob(filepath.Join(filepath.Dir(globalStorage), "workspaceStorage", "*", "state.vscdb"))
	agents, _ := filepath.Glob(filepath.Join(s.Stores.CursorDir, "chats", "*", "*", "store.db"))
	files := withWAL(s.Stores.CursorDB)
	files = append(files, filepath.Join(globalStorage, "storage.json"))
	files = append(files, withWAL(workspaces...)...)
	return append(files, withWAL(agents...)...)
}

// claudeFiles are the files a Claude Code read depends on: each
// session's transcript, each running process's state file and each
// background job's state.
func (s *Scanner) claudeFiles() []string {
	transcripts, _ := filepath.Glob(filepath.Join(s.Stores.ClaudeDir, "projects", "*", "*.jsonl"))
	running, _ := filepath.Glob(filepath.Join(s.Stores.ClaudeDir, "sessions", "*.json"))
	jobs, _ := filepath.Glob(filepath.Join(s.Stores.ClaudeDir, "jobs", "*", "state.json"))
	return append(append(transcripts, running...), jobs...)
}

// Stamp changes whenever a file that a scan reads changed: the three
// tools' stores and the marks. It starts no program and reads no file's
// contents, so a caller that scans on a timer can ask it first and skip
// the scan while it holds. It cannot see a process end or a day pass,
// so such a caller still scans at least every StampMaxAge.
func (s *Scanner) Stamp() string {
	files := append(s.claudeFiles(), s.codexFiles()...)
	files = append(files, s.cursorFiles()...)
	if s.MarksPath != "" {
		files = append(files, s.MarksPath)
	}
	// The names are part of the stamp: a file that replaces another of
	// the same size and time is still a change.
	return strings.Join(files, "\x00") + "\x00" + fileStamp(files...)
}

// StampMaxAge is how long a caller may skip scans on an unchanged
// Stamp: what the files cannot show, such as a process that ended or a
// conversation going dormant, is at most this late.
const StampMaxAge = keptMaxAge
