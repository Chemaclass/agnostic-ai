package conversations

import (
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
