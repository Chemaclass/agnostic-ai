package emit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ClearStaleStreamHost makes room for a folder at dir. On Windows an
// older sync that wrote `<dir>:<rest>` left an empty file named dir, the
// host of that hidden stream, and every later write under dir fails
// with "not a directory". An empty file is removed when the last sync
// recorded such a colon output; any other file at dir is an error that
// names it. A capture plans writes without touching the disk, so it
// skips the check.
func (s *Session) ClearStaleStreamHost(dir string, dryRun bool) error {
	if s.IsCapturing() {
		return nil
	}
	info, err := os.Lstat(dir)
	if IsAbsent(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lstat %s: %w", dir, err)
	}
	if info.IsDir() {
		return nil
	}
	if info.Mode().IsRegular() && info.Size() == 0 && s.recordedColonOutput(dir) {
		_, err := s.remove(dir, "", nil, false, dryRun)
		return err
	}
	return fmt.Errorf("%s: a file stands where sync must write a folder; delete it and sync again", dir)
}

func (s *Session) recordedColonOutput(dir string) bool {
	prefix := filepath.ToSlash(dir) + ":"
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sums := range []map[string]string{s.backupSums, s.keepSums} {
		for path := range sums {
			if strings.HasPrefix(filepath.ToSlash(path), prefix) {
				return true
			}
		}
	}
	return false
}
