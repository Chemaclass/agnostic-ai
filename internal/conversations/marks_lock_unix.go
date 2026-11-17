//go:build darwin || linux

package conversations

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// tryLockFile takes the file's lock, or fails at once when another
// open file holds it, in this process or another. Closing the file
// gives the lock up.
func tryLockFile(file *os.File) (release func(), err error) {
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return nil, err
	}
	return func() { _ = file.Close() }, nil
}

func lockHeldElsewhere(err error) bool {
	return errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN)
}
