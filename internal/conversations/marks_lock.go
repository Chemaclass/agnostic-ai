package conversations

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// marksLockWait is how long a change to the marks waits for another
// process to finish its own. Each holds the lock for one small file's
// read and write.
const marksLockWait = 5 * time.Second

// lockMarks holds the marks file's lock until the returned function is
// called. Several servers on one machine share the file: the
// companion's, the dashboard's and each project's. Each reads the
// file, changes it and writes all of it back, so without a lock that
// spans the three steps the later write discards the earlier one's
// mark. The lock is a file beside the marks, held through the system's
// own file locking, so it also ends when its holder dies.
func lockMarks(path string) (release func(), err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("lock conversation marks: %w", err)
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("lock conversation marks: %w", err)
	}
	deadline := time.Now().Add(marksLockWait)
	for {
		err := tryLockFile(lock)
		if err == nil {
			// Closing the file gives the lock up.
			return func() { _ = lock.Close() }, nil
		}
		if !lockHeldElsewhere(err) || time.Now().After(deadline) {
			_ = lock.Close()
			return nil, fmt.Errorf("lock conversation marks: %w", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// ErrRenameInProgress says another request, on this server or another
// one that shares the marks file, is renaming the same conversation.
var ErrRenameInProgress = errors.New("another rename of this conversation is still running")

// LockRename holds one conversation's rename lock until the returned
// function is called. A rename writes the name into the tool and then
// into the marks file; holding the lock across both keeps two renames
// of one conversation from ending with one name in the tool and the
// other in the list. The lock is a file beside the marks, one per
// conversation, so every server that shares the marks shares it, and a
// rename of another conversation never waits. A rename that finds the
// lock held fails at once with ErrRenameInProgress.
func (s *Scanner) LockRename(tool Tool, id string) (release func(), err error) {
	if s.MarksPath == "" {
		return nil, errors.New("this server keeps no conversation marks")
	}
	key := sha256.Sum256([]byte(markKey(tool, id)))
	path := filepath.Join(filepath.Dir(s.MarksPath), "rename-locks", hex.EncodeToString(key[:16])+".lock")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("lock the rename: %w", err)
	}
	lock, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("lock the rename: %w", err)
	}
	err = tryLockFile(lock)
	if err != nil {
		_ = lock.Close()
		if lockHeldElsewhere(err) {
			return nil, ErrRenameInProgress
		}
		return nil, fmt.Errorf("lock the rename: %w", err)
	}
	// Closing the file gives the lock up.
	return func() { _ = lock.Close() }, nil
}
