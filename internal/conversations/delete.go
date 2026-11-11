package conversations

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// DeleteReply is the answer to a delete that removed the conversation.
type DeleteReply struct {
	Deleted    bool  `json:"deleted"`
	FreedBytes int64 `json:"freedBytes,omitempty"`
	// Note says what could not be tidied after the data was removed.
	Note string `json:"note,omitempty"`
}

// DeleteRefusal is why a delete removed nothing, or not everything.
type DeleteRefusal string

const (
	// DeleteNotListed: this server does not list the conversation, or the person hid it.
	DeleteNotListed DeleteRefusal = "not-listed"
	// DeleteNotArchived: the conversation is not completed now.
	DeleteNotArchived DeleteRefusal = "not-archived"
	// DeleteRunning: a process or a background job still holds it.
	DeleteRunning DeleteRefusal = "running"
	// DeleteChanged: one of its files was written after the archive.
	DeleteChanged DeleteRefusal = "changed"
	// DeleteUnsupported: its store cannot be deleted from here.
	DeleteUnsupported DeleteRefusal = "unsupported"
	// DeleteFailed: a removal failed; Removed and Remaining say how far it got.
	DeleteFailed DeleteRefusal = "failed"
)

// DeleteError says why nothing, or not everything, was deleted.
type DeleteError struct {
	Kind DeleteRefusal
	// Message is the sentence the person reads.
	Message   string
	Removed   []string
	Remaining []string
	Cause     error
}

func (e *DeleteError) Error() string { return e.Message }

func (e *DeleteError) Unwrap() error { return e.Cause }

func refuse(kind DeleteRefusal, message string) *DeleteError {
	return &DeleteError{Kind: kind, Message: message}
}

const notListedMessage = "That conversation is not in this list any more. Refresh the page."

// Delete removes one archived Claude Code session from this machine:
// its transcript and the folders Claude Code names after it, then its
// marks. It removes only what it can verify belongs to the session, and
// the transcript last, so after any failure the row is still listed,
// still archived, and the delete can be retried.
func (s *Scanner) Delete(ctx context.Context, tool Tool, id string, opts ScanOptions) (DeleteReply, error) {
	if s.MarksPath == "" {
		return DeleteReply{}, refuse(DeleteNotArchived, "This server keeps no conversation marks, so nothing is archived.")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// A Codex or Cursor list kept from the last minute may miss a write since.
	delete(s.keptSources, tool)
	results, _ := s.readAll(ctx, opts)
	if ctx.Err() != nil {
		return DeleteReply{}, refuse(DeleteNotListed, notListedMessage)
	}
	c, found := findIn(results, tool, id)
	if !found {
		return DeleteReply{}, refuse(DeleteNotListed, notListedMessage)
	}
	// The lock spans the check and the removal, so no other server
	// changes the archive meanwhile.
	release, err := lockMarks(s.MarksPath)
	if err != nil {
		return DeleteReply{}, &DeleteError{Kind: DeleteFailed, Message: "Nothing was deleted: " + err.Error(), Cause: err}
	}
	defer release()
	now := s.Now()
	file, _, err := readMarks(s.MarksPath, now)
	if err != nil {
		return DeleteReply{}, &DeleteError{Kind: DeleteFailed, Message: "Nothing was deleted: " + err.Error(), Cause: err}
	}
	key := markKey(tool, id)
	entry := file.Marks[key]
	if entry.HiddenAt != "" {
		return DeleteReply{}, refuse(DeleteNotListed, notListedMessage)
	}
	// Before the state: an idle open session marked completed is still completed.
	if c.Live || c.App == AppBackground {
		return DeleteReply{}, refuse(DeleteRunning, "This conversation is still running. Close it in Claude Code, then delete it.")
	}
	if entry.CompletedAt == "" || stateOf(c, entry, parseStamp(file.Since), now) != StateCompleted {
		return DeleteReply{}, refuse(DeleteNotArchived, "Only an archived conversation can be deleted. This one is not archived any more: something was written in it, or it was opened. Refresh the list.")
	}
	if reason := s.deleteUnsupported(c); reason != "" {
		return DeleteReply{}, refuse(DeleteUnsupported, reason)
	}
	paths, err := s.claudeSessionPaths(c)
	if err != nil {
		return DeleteReply{}, &DeleteError{Kind: DeleteUnsupported, Message: "agnostic-ai cannot verify which files belong to it, so it deletes none: " + err.Error(), Cause: err}
	}
	// The baseline comes first: a write after it moves the newest time, or differs from it later.
	transcript, err := os.Lstat(c.file)
	if err != nil {
		return DeleteReply{}, &DeleteError{Kind: DeleteFailed, Message: "Nothing was deleted: " + err.Error(), Cause: err}
	}
	if s.betweenReads != nil {
		s.betweenReads()
	}
	freed, newest, err := measure(paths)
	var linked *linkedError
	if errors.As(err, &linked) {
		return DeleteReply{}, &DeleteError{Kind: DeleteUnsupported, Message: "agnostic-ai cannot verify which files belong to it, so it deletes none: " + err.Error(), Cause: err}
	}
	if err != nil {
		return DeleteReply{}, &DeleteError{Kind: DeleteFailed, Message: "Nothing was deleted: " + err.Error(), Cause: err}
	}
	// Marks keep whole seconds, so a write in the mark's own second may have come after it.
	if !newest.Truncate(time.Second).Before(parseStamp(entry.CompletedAt)) {
		return DeleteReply{}, refuse(DeleteChanged, "Something was written in this conversation after you archived it. Archive it again if you still want to delete it.")
	}
	if refusal := s.stillClosed(c, transcript); refusal != nil {
		return DeleteReply{}, refusal
	}
	// The side folders can take a while: the session is checked once more before its transcript goes.
	beforeTranscript := func() *DeleteError { return s.stillClosed(c, transcript) }
	if err := removeInOrder(paths, beforeTranscript); err != nil {
		return DeleteReply{}, err
	}
	reply := DeleteReply{Deleted: true, FreedBytes: freed}
	delete(s.claude, c.file)
	delete(file.Marks, key)
	if err := writeMarks(s.MarksPath, file); err != nil {
		reply.Note = "The session data is gone. Its entry in the marks file could not be cleared: " + err.Error() + "."
		return reply, nil
	}
	if err := s.forgetTab(key); err != nil {
		reply.Note = "The session data is gone. The terminal tab it last ran in could not be forgotten: " + err.Error() + "."
	}
	return reply, nil
}

// findIn is the conversation results list for tool and id.
func findIn(results []sourceResult, tool Tool, id string) (Conversation, bool) {
	for _, result := range results {
		for _, c := range result.conversations {
			if c.Tool == tool && c.ID == id {
				return c, true
			}
		}
	}
	return Conversation{}, false
}

// deleteUnsupported says why a conversation's store cannot be deleted
// from here; empty for a local Claude Code session.
func (s *Scanner) deleteUnsupported(c Conversation) string {
	if _, system := s.Processes.(SystemProcesses); system && runtime.GOOS == "windows" {
		return "agnostic-ai cannot tell on Windows whether a session still runs, so it deletes none."
	}
	switch {
	case c.App == AppCloud:
		return "This conversation is stored in the tool's cloud, not on this computer. Delete it there."
	case c.Tool == ToolCodex:
		return "Deleting Codex threads from here is not supported yet. To delete it, run this in a terminal: codex delete " + c.ID
	case c.Tool == ToolCursor && c.store != "":
		return "Deleting cursor-agent chats is not supported yet."
	case c.Tool == ToolCursor:
		return "Cursor keeps every chat in one shared database and has no command to delete one. Delete it in Cursor."
	case c.Tool != ToolClaude:
		return "agnostic-ai cannot delete this kind of conversation."
	}
	return ""
}

// claudeSessionPaths lists the locations Claude Code names after the
// session, in removal order, the transcript last. Each is a direct child
// of a known folder whose name is exactly the session id; project memory
// and other sessions sit beside them and are never named.
func (s *Scanner) claudeSessionPaths(c Conversation) ([]string, error) {
	if !sessionIDPattern.MatchString(c.ID) {
		return nil, fmt.Errorf("%q is not a Claude Code session id", c.ID)
	}
	projects := filepath.Join(s.Stores.ClaudeDir, "projects")
	if !filepath.IsAbs(c.file) || filepath.Base(c.file) != c.ID+".jsonl" {
		return nil, fmt.Errorf("the transcript %q is not named after the session", c.file)
	}
	rel, err := filepath.Rel(projects, c.file)
	if err != nil || len(strings.Split(rel, string(filepath.Separator))) != 2 || strings.HasPrefix(rel, "..") {
		return nil, fmt.Errorf("the transcript %q is not in a project folder of %s", c.file, projects)
	}
	if err := rejectLinkedParents(s.Stores.ClaudeDir, c.file); err != nil {
		return nil, err
	}
	matches, err := filepath.Glob(filepath.Join(projects, "*", c.ID+".jsonl"))
	if err != nil || len(matches) != 1 || matches[0] != c.file {
		return nil, fmt.Errorf("the session id is in %d project folders", len(matches))
	}
	transcript, err := os.Lstat(c.file)
	if err != nil {
		return nil, err
	}
	if !transcript.Mode().IsRegular() {
		return nil, fmt.Errorf("the transcript %q is not a regular file", c.file)
	}
	var paths []string
	for _, candidate := range []string{
		filepath.Join(s.Stores.ClaudeDir, "session-env", c.ID),
		filepath.Join(s.Stores.ClaudeDir, "file-history", c.ID),
		filepath.Join(s.Stores.ClaudeDir, "uploads", c.ID),
		strings.TrimSuffix(c.file, ".jsonl"),
	} {
		if err := rejectLinkedParents(s.Stores.ClaudeDir, candidate); err != nil {
			return nil, err
		}
		info, err := os.Lstat(candidate)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			continue
		case err != nil:
			return nil, err
		case !info.IsDir():
			// A link would be removed while the data it points to stays.
			return nil, fmt.Errorf("%s is not a folder", candidate)
		}
		paths = append(paths, candidate)
	}
	return append(paths, c.file), nil
}

func rejectLinkedParents(root, path string) error {
	root = filepath.Clean(root)
	for parent := filepath.Dir(path); parent != root; parent = filepath.Dir(parent) {
		if parent == filepath.Dir(parent) {
			return fmt.Errorf("%s is outside %s", path, root)
		}
		info, err := os.Lstat(parent)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return &linkedError{path: parent}
		}
	}
	return nil
}

// linkedError names a link inside a session's folders: what it points
// to would stay on disk after the link is removed.
type linkedError struct{ path string }

func (e *linkedError) Error() string { return e.path + " is a link" }

// measure walks paths without following links: the bytes their files
// hold and the newest time one of them was written. Folder times are
// left out: removing files moves them, and a retry would read that as a write.
func measure(paths []string) (bytes int64, newest time.Time, err error) {
	for _, path := range paths {
		err := filepath.WalkDir(path, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.Type()&fs.ModeSymlink != 0 {
				return &linkedError{path: path}
			}
			if !entry.Type().IsRegular() {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			bytes += info.Size()
			if info.ModTime().After(newest) {
				newest = info.ModTime()
			}
			return nil
		})
		if err != nil {
			return 0, time.Time{}, err
		}
	}
	return bytes, newest, nil
}

// stillClosed asks again, right before the removal, whether a process
// or a job took the session, or its transcript changed since it was
// measured. A process that starts in the moment after is not seen.
func (s *Scanner) stillClosed(c Conversation, measured fs.FileInfo) *DeleteError {
	// A lookup that fails answers that nothing runs, so the answer comes from a table that was read.
	table, err := s.Processes.Table()
	if err != nil {
		return &DeleteError{Kind: DeleteFailed, Message: "agnostic-ai could not check whether this session still runs, so it deleted nothing.", Cause: err}
	}
	started := map[int]string{}
	for _, process := range table {
		started[process.PID] = process.Started
	}
	live := false
	for _, entry := range s.claudeLiveEntries() {
		start, runs := started[entry.PID]
		if entry.SessionID == c.ID && runs && (entry.ProcStart == "" || start == strings.TrimSpace(entry.ProcStart)) {
			live = true
		}
	}
	_, job := s.claudeActiveJobs()[c.ID]
	if live || job {
		return refuse(DeleteRunning, "This conversation is still running. Close it in Claude Code, then delete it.")
	}
	now, err := os.Lstat(c.file)
	if err != nil || now.Size() != measured.Size() || !now.ModTime().Equal(measured.ModTime()) {
		return refuse(DeleteChanged, "Something was written in this conversation after you archived it. Archive it again if you still want to delete it.")
	}
	return nil
}

// removeInOrder removes each path and its contents, stopping at the
// first failure, and asks beforeLast before it removes the last one.
func removeInOrder(paths []string, beforeLast func() *DeleteError) *DeleteError {
	for at, path := range paths {
		if at == len(paths)-1 {
			if stop := beforeLast(); stop != nil {
				return &DeleteError{
					Kind:      DeleteFailed,
					Message:   fmt.Sprintf("%s Deleted %d of %d locations. Still on disk: %s. Its marks were kept.", stoppedBefore(stop), at, len(paths), path),
					Removed:   paths[:at],
					Remaining: paths[at:],
					Cause:     stop,
				}
			}
		}
		if err := os.RemoveAll(path); err != nil {
			return &DeleteError{
				Kind: DeleteFailed,
				Message: fmt.Sprintf("Deleted %d of %d locations, then stopped: %v. Still on disk: %s. Its marks were kept.",
					at, len(paths), err, strings.Join(paths[at:], ", ")),
				Removed:   paths[:at],
				Remaining: paths[at:],
				Cause:     err,
			}
		}
	}
	return nil
}

// forgetTab drops the terminal tab remembered for key, under the lock
// every server shares.
func (s *Scanner) forgetTab(key string) error {
	path := s.tabsPath()
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	release, err := lockMarks(path)
	if err != nil {
		return err
	}
	defer release()
	tabs := readTabs(path)
	if _, ok := tabs.Tabs[key]; !ok {
		return nil
	}
	delete(tabs.Tabs, key)
	return writeJSONFile(path, tabs)
}

// stoppedBefore says why the removal stopped before the transcript.
func stoppedBefore(stop *DeleteError) string {
	if stop.Kind == DeleteRunning {
		return "This conversation started running while it was being deleted, so agnostic-ai stopped before its transcript."
	}
	if stop.Kind == DeleteChanged {
		return "Something was written in this conversation while it was being deleted, so agnostic-ai stopped before its transcript."
	}
	return "agnostic-ai could not check again whether this session still runs, so it stopped before its transcript."
}
