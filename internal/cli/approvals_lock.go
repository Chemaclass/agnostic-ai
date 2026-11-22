package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/approvals"
)

const approvalsLockPause = 40 * time.Millisecond

// approvalsLockTries bounds the wait for the approvals lock to about 2 seconds.
var approvalsLockTries = 50

// lockApprovalsWrites holds a lock beside the decision log while one
// process changes the home approvals block, so the companion's server,
// the web server and the CLI each check and write in turn. Release it by
// calling the returned func.
func lockApprovalsWrites(source string) (func(), error) {
	path := filepath.Join(filepath.Dir(approvals.LogPath(source)), "write.lock")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Dir(path), err)
	}
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	release := func() { _ = file.Close() }
	for try := 1; ; try++ {
		err := tryLockProjectFile(file)
		if err == nil || !projectLockContended(err) {
			// Without file locks the write still goes ahead, as before the lock existed.
			return release, nil
		}
		if try == approvalsLockTries {
			release()
			return nil, fmt.Errorf("lock %s: another agnostic-ai process is changing approvals; try again: %w", path, err)
		}
		time.Sleep(approvalsLockPause)
	}
}

// writeApprovalsAskLocked is writeApprovalsAsk under the approvals lock.
func writeApprovalsAskLocked(source string, proven bool) (string, error) {
	release, err := lockApprovalsWrites(source)
	if err != nil {
		return "", err
	}
	defer release()
	return writeApprovalsAsk(source, proven)
}
