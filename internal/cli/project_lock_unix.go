//go:build darwin || linux

package cli

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func tryLockProjectFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
}

func projectLockContended(err error) bool {
	return errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN)
}
