//go:build darwin || linux

package conversations

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// tryLockFile takes the file's lock, or fails at once when another
// open file holds it, in this process or another.
func tryLockFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
}

func lockHeldElsewhere(err error) bool {
	return errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN)
}
