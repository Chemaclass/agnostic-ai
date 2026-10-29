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
// recorded such a colon output and every one still holds the bytes it
// wrote; removing the host deletes its streams, so an edited one would
// be lost without a backup. Any other file at dir is an error that
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
	if target, statErr := os.Stat(dir); statErr == nil && target.IsDir() {
		return nil
	}
	colonOutputs := s.recordedColonOutputs(dir)
	if !info.Mode().IsRegular() || info.Size() != 0 || len(colonOutputs) == 0 {
		return fmt.Errorf("%s: a file stands where sync must write a folder; delete it and sync again", dir)
	}
	for path, sum := range colonOutputs {
		data, err := os.ReadFile(path)
		if IsAbsent(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		if ContentSum(string(data)) != sum {
			return fmt.Errorf("%s: edited since the last sync; copy its text elsewhere, delete %s, and sync again", path, dir)
		}
	}
	_, err = s.remove(dir, "", nil, false, dryRun)
	return err
}

// recordedColonOutputs returns each `<dir>:<rest>` path the last sync
// recorded, with the sum of the bytes it wrote.
func (s *Session) recordedColonOutputs(dir string) map[string]string {
	prefix := filepath.ToSlash(dir) + ":"
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]string{}
	for _, sums := range []map[string]string{s.backupSums, s.keepSums} {
		for path, sum := range sums {
			if sum != "" && strings.HasPrefix(filepath.ToSlash(path), prefix) {
				out[path] = sum
			}
		}
	}
	return out
}
