package emit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// txEntry records the pre-write state of one file for transaction rollback.
// content is nil when the file did not exist before the write. link is the
// target of a symlink the transaction removed from path.
type txEntry struct {
	path    string
	content []byte
	mode    os.FileMode
	link    string
}

// StartTransaction begins recording pre-write file state so that Rollback
// can undo all writes if a sync pass fails partway through. Commit clears
// the log on success.
func (s *Session) StartTransaction() {
	s.mu.Lock()
	s.transacting = true
	s.txLog = nil
	s.mu.Unlock()
}

// Commit clears the transaction log and disables transaction mode. Call
// after a successful sync to release the log.
func (s *Session) Commit() {
	s.mu.Lock()
	s.transacting = false
	s.txLog = nil
	s.mu.Unlock()
}

// Rollback undoes all file writes and removals recorded since
// StartTransaction. New files are removed; overwritten and removed files
// are restored from their pre-write content, and removed symlinks are
// recreated, along with any folders pruned above them. All entries are
// attempted; errors are joined and returned.
//
// Entries are undone newest first, except that a removed link waits for
// the entries logged before it: Windows fixes a link's file or directory
// type when it creates the link, so the folder a swept link points at
// must be back first. An entry at, above, or under a waiting link's path
// depends on the link, so every waiting link comes back before it.
func (s *Session) Rollback() error {
	ownershipReads.RLock()
	defer ownershipReads.RUnlock()
	s.mu.Lock()
	log := s.txLog
	s.txLog = nil
	s.transacting = false
	s.mu.Unlock()

	var errs []error
	var links []txEntry
	restoreLinks := func() {
		for _, l := range links {
			errs = append(errs, l.undo()...)
		}
		links = nil
	}
	for _, e := range slices.Backward(log) {
		if e.link != "" {
			links = append(links, e)
			continue
		}
		if slices.ContainsFunc(links, func(l txEntry) bool { return pathsOverlap(l.path, e.path) }) {
			restoreLinks()
		}
		errs = append(errs, e.undo()...)
	}
	restoreLinks()
	return errors.Join(errs...)
}

// undo puts back the state e recorded before a write or removal.
func (e txEntry) undo() []error {
	if e.content == nil && e.link == "" {
		if err := os.Remove(e.path); err != nil && !os.IsNotExist(err) {
			return []error{fmt.Errorf("rollback %s: %w", e.path, err)}
		}
		return nil
	}
	var errs []error
	mode := e.mode
	if mode == 0 {
		mode = filePerm
	}
	// A removed file may have given way to a directory of the
	// same name (Cline's single-file `.clinerules`, #1060). The
	// files written under it are undone by now, so prune the
	// empty tree before the file comes back.
	if info, err := os.Lstat(e.path); err == nil && info.IsDir() {
		if err := removeEmptyDirs(e.path); err != nil {
			errs = append(errs, fmt.Errorf("rollback %s: %w", e.path, err))
		}
	}
	// A sweep that removed the path may have pruned the folders it emptied.
	if err := mkdirAll(filepath.Dir(e.path), dirPerm); err != nil {
		errs = append(errs, fmt.Errorf("rollback %s: %w", e.path, err))
	} else if e.link != "" {
		if err := restoreLink(e.link, e.path); err != nil {
			errs = append(errs, fmt.Errorf("rollback %s: %w", e.path, err))
		}
	} else if err := os.WriteFile(e.path, e.content, mode); err != nil {
		errs = append(errs, fmt.Errorf("rollback %s: %w", e.path, err))
	} else if err := os.Chmod(e.path, mode); err != nil {
		errs = append(errs, fmt.Errorf("rollback %s mode: %w", e.path, err))
	}
	return errs
}

// restoreLink recreates the symlink at path to target, over a symlink
// written there since the removal. Removing a symlink never touches what
// it points at.
func restoreLink(target, path string) error {
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return os.Symlink(target, path)
}

// pathsOverlap reports whether a and b are the same path or one lies
// under the other.
func pathsOverlap(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	sep := string(filepath.Separator)
	return a == b || strings.HasPrefix(a, b+sep) || strings.HasPrefix(b, a+sep)
}
