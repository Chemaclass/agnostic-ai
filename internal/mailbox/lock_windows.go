package mailbox

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// tryLock takes the file's lock, or fails at once when another
// open file holds it, in this process or another. Closing the file
// gives the lock up.
func tryLock(file *os.File) (release func(), err error) {
	if err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{}); err != nil {
		return nil, err
	}
	return func() { _ = file.Close() }, nil
}

func lockHeldElsewhere(err error) bool {
	return errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}
