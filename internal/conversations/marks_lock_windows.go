package conversations

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// tryLockFile takes the file's lock, or fails at once when another
// open file holds it, in this process or another.
func tryLockFile(file *os.File) error {
	return windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
}

func lockHeldElsewhere(err error) bool {
	return errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}
