package approvals

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/applypatch"
	"github.com/chemaclass/agnostic-ai/internal/conversations"
)

// MaxLogBytes is the size at which the decision log rolls over to one
// older file.
const MaxLogBytes = 5 << 20

// Entry is one line of the decision log.
type Entry struct {
	Time     time.Time `json:"time"`
	Tool     string    `json:"tool"`
	Session  string    `json:"session,omitempty"`
	Cwd      string    `json:"cwd,omitempty"`
	Call     string    `json:"call"`
	Detail   string    `json:"detail,omitempty"`
	Mode     string    `json:"mode"`
	Decision Outcome   `json:"decision"`
	Rule     string    `json:"rule,omitempty"`
	// By is ByCompanion for a request held for the companion.
	By string `json:"by,omitempty"`
}

// NewEntry is the log line for a decision on r, with a detail that
// leaves out anything that may hold a credential.
func NewEntry(tool, session, mode string, r Request, d Decision, now time.Time) Entry {
	input, err := json.Marshal(r.Input)
	if err != nil {
		input = nil
	}
	detail := conversations.CallDetail(r.Call, input)
	if detail == "" && patchCalls[r.Call] {
		detail = patchFiles(r.Input)
	}
	return Entry{
		Time:     now.UTC(),
		Tool:     tool,
		Session:  session,
		Cwd:      r.Cwd,
		Call:     r.Call,
		Detail:   detail,
		Mode:     mode,
		Decision: d.Outcome,
		Rule:     d.Rule,
	}
}

// patchFiles lists the files an apply_patch body adds, updates, deletes
// or moves to.
func patchFiles(input map[string]any) string {
	var files []string
	for _, field := range []string{"patch", "input", "command"} {
		body, _ := input[field].(string)
		for _, op := range applypatch.Parse(body) {
			for _, file := range []string{op.Path, op.MoveTo} {
				if file != "" && !slices.Contains(files, file) {
					files = append(files, file)
				}
			}
		}
	}
	return strings.Join(files, ", ")
}

// Log appends decisions to a JSON Lines file, keeping it under
// MaxBytes by moving a full file to Path + ".1".
type Log struct {
	Path     string
	MaxBytes int64
}

// Append writes e as one line.
func (l Log) Append(e Entry) error {
	line, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("encode approval decision: %w", err)
	}
	line = append(line, '\n')
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o700); err != nil {
		return fmt.Errorf("create approvals log folder: %w", err)
	}
	release, err := lockLog(l.Path)
	if err != nil {
		return err
	}
	defer release()
	limit := l.MaxBytes
	if limit <= 0 {
		limit = MaxLogBytes
	}
	if info, err := os.Stat(l.Path); err == nil && info.Size()+int64(len(line)) > limit {
		if err := os.Rename(l.Path, l.Path+".1"); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("roll over approvals log: %w", err)
		}
	}
	f, err := os.OpenFile(l.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open approvals log: %w", err)
	}
	if _, err := f.Write(line); err != nil {
		_ = f.Close()
		return fmt.Errorf("write approvals log: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close approvals log: %w", err)
	}
	return nil
}

// logLockWait bounds how long a decision waits for another hook's
// write; each holds the lock for one stat, rename and append.
const logLockWait = 2 * time.Second

// lockLog holds a lock file beside the log, so two hooks that find the
// log full do not both roll it over and lose the older file.
func lockLog(path string) (release func(), err error) {
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("lock approvals log: %w", err)
	}
	deadline := time.Now().Add(logLockWait)
	for {
		err := tryLockFile(lock)
		if err == nil {
			return func() { _ = lock.Close() }, nil
		}
		if !lockHeldElsewhere(err) || time.Now().After(deadline) {
			_ = lock.Close()
			return nil, fmt.Errorf("lock approvals log: %w", err)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
