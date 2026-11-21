//go:build !darwin && !linux && !windows

package mailbox

import (
	"errors"
	"os"
	"sync"
)

// heldLocks names the lock files this process holds. These systems,
// such as the browser build, run one process on files nothing else
// shares, so a lock only has to exclude the requests of this process.
var heldLocks = struct {
	sync.Mutex
	names map[string]bool
}{names: map[string]bool{}}

var errLockHeld = errors.New("another request of this process holds the lock")

func tryLock(file *os.File) (release func(), err error) {
	heldLocks.Lock()
	defer heldLocks.Unlock()
	name := file.Name()
	if heldLocks.names[name] {
		return nil, errLockHeld
	}
	heldLocks.names[name] = true
	return func() {
		heldLocks.Lock()
		delete(heldLocks.names, name)
		heldLocks.Unlock()
		_ = file.Close()
	}, nil
}

func lockHeldElsewhere(err error) bool { return errors.Is(err, errLockHeld) }
