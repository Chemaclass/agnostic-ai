//go:build darwin || linux

package approvals

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func tryLockFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
}

func lockHeldElsewhere(err error) bool {
	return errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN)
}
