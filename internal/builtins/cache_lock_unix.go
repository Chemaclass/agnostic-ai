//go:build unix

package builtins

import (
	"os"

	"golang.org/x/sys/unix"
)

func lockCache(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_EX)
}
