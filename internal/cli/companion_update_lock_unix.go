//go:build darwin || linux

package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// lockCompanionUpdate holds the folder that contains app, so a second
// update-app of the same bundle stops before it downloads anything. The
// folder is locked, not the bundle, because the bundle is renamed during the swap.
func lockCompanionUpdate(app string) (release func(), err error) {
	folder, err := os.Open(filepath.Dir(app))
	if err != nil {
		return nil, fmt.Errorf("lock %s for the update: %w", app, err)
	}
	if err := unix.Flock(int(folder.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = folder.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, fmt.Errorf("another update of %s is running; wait for it to finish", app)
		}
		return nil, fmt.Errorf("lock %s for the update: %w", app, err)
	}
	return func() { _ = folder.Close() }, nil
}
