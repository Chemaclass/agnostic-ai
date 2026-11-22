package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/approvals"
)

const approvalsLockPause = 40 * time.Millisecond

// approvalsLockTries bounds the wait for the approvals lock to about 2 seconds.
var approvalsLockTries = 50

// lockApprovalsWrites holds a lock beside the decision log while one
// process changes the home config: the approvals block, and the targets
// and builtins edits of the web page. The companion's server, the web
// server and the CLI then each read, check and write in turn. Release it
// by calling the returned func.
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
	snap, err := readHomeSnapshot(source)
	if err != nil {
		return "", err
	}
	return writeApprovalsAsk(source, snap, proven)
}

// homeSnapshot is the shared and the local home config as one read
// under the approvals lock found them. An approvals change checks and
// builds from it, and writes only while both files still match it.
type homeSnapshot struct {
	paths [2]string
	data  [2][]byte
}

func readHomeSnapshot(source string) (homeSnapshot, error) {
	var snap homeSnapshot
	for i, path := range userConfigPaths(source) {
		data, err := readHomeFile(path)
		if err != nil {
			return homeSnapshot{}, err
		}
		snap.paths[i], snap.data[i] = path, data
	}
	return snap, nil
}

// readHomeFile is path's bytes, or nil when it does not exist.
func readHomeFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return data, nil
}

// files maps each home file to its snapshot bytes, as
// config.LoadHomeApprovalsFiles reads them.
func (s homeSnapshot) files() map[string][]byte {
	return map[string][]byte{filepath.Clean(s.paths[0]): s.data[0], filepath.Clean(s.paths[1]): s.data[1]}
}

// with is the files of s with path holding data.
func (s homeSnapshot) with(path string, data []byte) map[string][]byte {
	files := s.files()
	files[filepath.Clean(path)] = data
	return files
}

// of is the snapshot bytes of path, one of the two home files.
func (s homeSnapshot) of(path string) []byte {
	for i := range s.paths {
		if filepath.Clean(s.paths[i]) == filepath.Clean(path) {
			return s.data[i]
		}
	}
	return nil
}

// unchanged refuses when either home file differs from the snapshot.
func (s homeSnapshot) unchanged() error {
	for i, path := range s.paths {
		now, err := readHomeFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(now, s.data[i]) {
			return fmt.Errorf("%s changed while this change was checked, so it was not made; check the current value and try again", path)
		}
	}
	return nil
}
