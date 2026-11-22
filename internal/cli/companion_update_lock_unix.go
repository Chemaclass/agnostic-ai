//go:build darwin || linux

package cli

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// lockCompanionFolder holds the folder that contains the companion app,
// so a second install, update or uninstall of it stops before it changes
// anything. The folder is locked, not the app, because the app is renamed
// during the swap. Each writer opens the folder itself, so a process that
// holds the lock must not wait on a child that takes it.
func lockCompanionFolder(folder string) (release func(), err error) {
	handle, err := os.Open(folder)
	if err != nil {
		return func() {}, nil
	}
	if err := unix.Flock(int(handle.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = handle.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, fmt.Errorf("another install or update of the companion in %s is running; wait for it to finish", folder)
		}
		// A folder that takes no lock still updates; the steps that follow report real problems.
		return func() {}, nil
	}
	return func() { _ = handle.Close() }, nil
}
