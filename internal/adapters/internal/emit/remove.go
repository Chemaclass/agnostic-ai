package emit

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// IsAbsent reports whether err means a file or directory cannot be
// read because it is not there. The path not existing (fs.ErrNotExist)
// is the obvious case. The other is the js/wasm playground, which has
// no filesystem at all: every os syscall there fails, but with an
// inconsistent grab-bag of errors (ENOSYS on file reads, an
// "O_DIRECTORY is not supported" variant on directory reads, ...) that
// no single sentinel reliably matches. Rather than chase each variant,
// treat any non-nil error as "absent" when GOOS is js: there is
// nothing on disk to read, so optional source inputs (overlays, helper
// files, prior outputs) are simply not present and the adapter should
// proceed. On any real OS this is exactly an fs.ErrNotExist check, so
// sync / check / doctor behavior is unchanged.
func IsAbsent(err error) bool {
	if err == nil {
		return false
	}
	if runtime.GOOS == "js" {
		return true
	}
	return errors.Is(err, fs.ErrNotExist)
}

// RemoveGenerated deletes path when it exists and carries the
// agnostic-ai provenance header. It is the inverse of WriteFile: use it
// when an adapter previously emitted a file but no longer has content
// to write for it (for example a `.codex/config.toml` that lost its
// last MCP, hook, and overlay between syncs). Files without the
// provenance marker are user-authored and left untouched, and so are
// user-owned paths (sync.unmanaged).
//
// dryRun prints the intended removal instead of touching disk so
// `sync --dry-run` previews stay side-effect-free. Capture mode leaves
// disk alone for the same reason WriteFile diverts there, and records
// the removal (see CapturedRemovals). Detailed
// recording logs the removal as a "delete" action so `sync` accounting
// includes the cleaned-up file. Transaction logging captures the
// pre-removal bytes so Rollback can restore the file.
func (s *Session) RemoveGenerated(path string, dryRun bool) error {
	_, err := s.RemoveOwned(path, "", dryRun)
	return err
}

// RemoveOwned is RemoveGenerated that also accepts proof of ownership
// for a file without the provenance header: sum is the ContentSum
// recorded when agnostic-ai wrote the file, and a file whose bytes
// still match it is removed. A header-less file edited since (or with
// no recorded sum) is kept. Used by the sync orphan sweep, where the
// prior ledger is the record of what sync wrote (#785).
//
// removed reports whether the file was deleted, or would be under
// dryRun, so callers can tell a refusal from a removal without a
// second stat.
func (s *Session) RemoveOwned(path, sum string, dryRun bool) (removed bool, err error) {
	return s.removeOwned(path, sum, dryRun, false)
}

// RemoveOwnedOrphan protects recorded hand edits before removing owned files.
func (s *Session) RemoveOwnedOrphan(path, sum string) (bool, error) {
	return s.removeOwned(path, sum, false, true)
}

func (s *Session) removeOwned(path, sum string, dryRun, protectEdit bool) (removed bool, err error) {
	defer lockPath(path)()
	existing, err := os.ReadFile(path)
	if IsAbsent(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	owned := header.Has(string(existing)) || (sum != "" && ContentSum(string(existing)) == sum)
	// A user-authored file is never removed anyway, so only a generated
	// file that is user-owned counts as a refused removal.
	if !owned || s.skipUnmanaged(path) {
		return false, nil
	}
	s.mu.Lock()
	capturing := s.capturing
	backupSum := s.backupSums[path]
	s.mu.Unlock()
	if capturing {
		return s.remove(path, sum, existing, false, dryRun)
	}
	if keepSum, keep := s.keepsEdit(path, ""); keep {
		s.noteKeptEdit(&s.kept, WrittenFile{Path: path, Sum: keepSum})
		return false, nil
	}
	editSum := backupSum
	if protectEdit {
		editSum = sum
	}
	if !dryRun && editSum != "" && ContentSum(string(existing)) != editSum {
		cur := &onDisk{path: path}
		info, statErr := cur.lstat()
		if statErr != nil {
			return false, fmt.Errorf("lstat %s: %w", path, statErr)
		}
		if !info.Mode().IsRegular() || (!filepath.IsAbs(path) && config.EscapesProject(path)) || symlinkedParent(path) {
			s.noteKeptEdit(&s.kept, WrittenFile{Path: path, Sum: editSum})
			return false, nil
		}
		backup := path + ".bak"
		switch err := backUpEdit(cur, backup); {
		case errors.Is(err, fs.ErrExist):
			s.noteKeptEdit(&s.backedUp, WrittenFile{Path: path, Sum: editSum})
			return false, nil
		case err != nil:
			return false, fmt.Errorf("backup %s: %w", path, err)
		}
		s.noteBackup(path)
		s.mu.Lock()
		if s.transacting {
			s.txLog = append(s.txLog, txEntry{path: backup})
		}
		s.mu.Unlock()
		removed, err := s.removeWithBackup(path, sum, existing, false, dryRun, backup)
		if removed && err == nil {
			s.mu.Lock()
			s.removedEdits = append(s.removedEdits, path)
			s.mu.Unlock()
		}
		return removed, err
	}
	return s.remove(path, sum, existing, false, dryRun)
}

// RemoveCopy deletes a hand-written file whose bytes still hash to sum,
// the proof that a spec holds its text. A file edited since is kept.
// Under backup mode the file is kept as `<path>.bak` so revert can
// restore it.
func (s *Session) RemoveCopy(path, sum string, dryRun bool) (removed bool, err error) {
	existing, err := os.ReadFile(path)
	if IsAbsent(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	if sum == "" || ContentSum(string(existing)) != sum || s.skipUnmanaged(path) {
		return false, nil
	}
	return s.remove(path, sum, existing, true, dryRun)
}

// RemoveLink deletes the symlink at path itself, never what it points to.
// A user-owned path (sync.unmanaged) and anything but a symlink are left
// alone, and so is disk in capture mode. A transaction logs the link's
// target so Rollback can recreate it.
func (s *Session) RemoveLink(path string, dryRun bool) (removed bool, err error) {
	info, err := os.Lstat(path)
	if IsAbsent(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lstat %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 || s.skipUnmanaged(path) || s.IsCapturing() {
		return false, nil
	}
	return s.remove(path, "", nil, false, dryRun)
}

// ReplaceFolderWithLink runs replace, which swaps the folder at path for
// a symlink. An open transaction logs the folder's files, then the link,
// so Rollback puts the folder back before it undoes an earlier write
// inside it, which would otherwise reach through the link. Nothing is
// logged when replace fails.
func (s *Session) ReplaceFolderWithLink(path string, replace func() error) error {
	s.mu.Lock()
	transacting := s.transacting
	s.mu.Unlock()
	if !transacting {
		return replace()
	}
	var files []txEntry
	err := filepath.WalkDir(path, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		files = append(files, txEntry{path: p, content: data, mode: info.Mode().Perm()})
		return nil
	})
	if err != nil && !IsAbsent(err) {
		return err
	}
	if err := replace(); err != nil {
		return err
	}
	s.mu.Lock()
	s.txLog = append(append(s.txLog, files...), txEntry{path: path})
	s.mu.Unlock()
	return nil
}

// remove deletes path, whose bytes are existing, once the caller has
// decided sync may. It honors capture, dry-run, transaction, and
// detailed recording modes; a transaction logs a symlink's target, not
// the bytes it resolves to, so Rollback restores the link itself.
// handWritten marks a file sync did not write, which backup mode keeps as
// `<path>.bak` so revert can restore it.
func (s *Session) remove(path, sum string, existing []byte, handWritten, dryRun bool) (bool, error) {
	return s.removeWithBackup(path, sum, existing, handWritten, dryRun, "")
}

func (s *Session) removeWithBackup(path, sum string, existing []byte, handWritten, dryRun bool, backupPath string) (removed bool, err error) {
	s.mu.Lock()
	capturing := s.capturing
	detailing := s.detailing
	transacting := s.transacting
	backup := s.backup && handWritten
	s.mu.Unlock()

	if capturing {
		s.mu.Lock()
		s.removals = append(s.removals, CapturedRemoval{Path: path, Sum: sum})
		s.mu.Unlock()
		return false, nil
	}
	if dryRun {
		fmt.Printf("--- rm %s ---\n", path)
		return true, nil
	}
	if err := s.checkProjectPath(path, "remove", true); err != nil {
		return false, err
	}

	ownershipReads.RLock()
	defer ownershipReads.RUnlock()

	if transacting {
		entry := txEntry{path: path, content: existing, mode: filePerm}
		if info, statErr := os.Lstat(path); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return false, fmt.Errorf("readlink %s: %w", path, err)
			}
			entry = txEntry{path: path, link: link}
		} else if statErr == nil {
			entry.mode = info.Mode().Perm()
		}
		s.mu.Lock()
		s.txLog = append(s.txLog, entry)
		s.mu.Unlock()
	}

	if backup {
		if err := writeBackupLocked(path+".bak", existing, sourcePerm(os.Stat(path))); err != nil {
			return false, fmt.Errorf("backup %s: %w", path, err)
		}
		s.noteBackup(path)
	}
	if err := os.Remove(path); err != nil && !IsAbsent(err) {
		return false, fmt.Errorf("remove %s: %w", path, err)
	}

	if detailing {
		s.mu.Lock()
		s.detailed = append(s.detailed, WrittenFile{Path: path, Bytes: 0, Action: "delete", Backup: backupPath})
		s.mu.Unlock()
	}
	return true, nil
}

// RemoveGeneratedTree walks dir and removes every file that carries
// the agnostic-ai provenance header via RemoveGenerated. Empty
// subdirectories left behind are then removed bottom-up so a legacy
// output tree disappears cleanly when an adapter's default path
// changes (for example `.agents/agents/` after codex moved to
// `.codex/agents/`). User-authored files (no marker) are preserved
// and any directory containing them stays in place.
//
// A missing dir is a no-op. Honors dryRun / capture / detailing /
// transaction modes via RemoveGenerated; directory removals only
// happen on the real path (no transaction logging) since an empty
// directory has no content to restore.
func (s *Session) RemoveGeneratedTree(dir string, dryRun bool) error {
	return s.removeGeneratedTree(dir, "", dryRun)
}

// RemoveGeneratedTreeExt is RemoveGeneratedTree restricted to files with
// the given extension (e.g. ".toml"). Use it when the legacy directory
// is shared with another target that writes a different file type there:
// a whole-tree sweep would delete that target's current output, since
// both carry the provenance header and neither adapter can see the
// other. Codex's pre-v0.26 `.agents/agents/*.toml` is the one such
// sweep today, sharing the directory with Antigravity's nested profiles
// subagents (#638).
func (s *Session) RemoveGeneratedTreeExt(dir, ext string, dryRun bool) error {
	return s.removeGeneratedTree(dir, ext, dryRun)
}

// removeGeneratedTree walks dir and removes generated files, optionally
// limited to one extension. An empty ext matches every file.
func (s *Session) removeGeneratedTree(dir, ext string, dryRun bool) error {
	s.mu.Lock()
	writesOnly := s.writesOnly
	s.mu.Unlock()
	if writesOnly {
		return nil
	}
	info, err := os.Stat(dir)
	if IsAbsent(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil
	}
	var filePaths []string
	if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if ext != "" && filepath.Ext(path) != ext {
			return nil
		}
		filePaths = append(filePaths, path)
		return nil
	}); err != nil {
		return fmt.Errorf("walk %s: %w", dir, err)
	}
	for _, p := range filePaths {
		if err := s.RemoveGenerated(p, dryRun); err != nil {
			return err
		}
	}
	if dryRun {
		return nil
	}
	s.mu.Lock()
	capturing := s.capturing
	s.mu.Unlock()
	if capturing {
		return nil
	}
	// Prune only the directories that held a file this sweep removed.
	// Under `sync --jobs`, another target may have just created an
	// empty directory under dir to write into; removing it fails that
	// write, as "Access is denied" on Windows (#1548).
	return pruneEmptiedDirs(dir, filePaths)
}

// pruneEmptiedDirs removes each file's parent directory, and its
// ancestors up to and including root, while they are empty. Non-empty
// directories (user-authored files survived) stay put.
// Candidates are tried deepest first, so a parent is checked only after
// every emptied child below it is gone.
func pruneEmptiedDirs(root string, files []string) error {
	root = filepath.Clean(root)
	seen := map[string]bool{}
	var dirs []string
	for _, f := range files {
		for d := filepath.Dir(f); !seen[d]; d = filepath.Dir(d) {
			seen[d] = true
			dirs = append(dirs, d)
			if d == root || !strings.HasPrefix(d, root+string(filepath.Separator)) {
				break
			}
		}
	}
	sort.SliceStable(dirs, func(i, j int) bool {
		return strings.Count(dirs[i], string(filepath.Separator)) > strings.Count(dirs[j], string(filepath.Separator))
	})
	for _, d := range dirs {
		if err := removeIfEmpty(d); err != nil {
			return err
		}
	}
	return nil
}

// removeIfEmpty removes dir when it has no entries.
func removeIfEmpty(dir string) error {
	entries, err := os.ReadDir(dir)
	if IsAbsent(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("readdir %s: %w", dir, err)
	}
	if len(entries) > 0 {
		return nil
	}
	// A concurrent write from another target can land between the
	// ReadDir above and this Remove, and then the directory is no
	// longer ours to prune.
	if err := os.Remove(dir); err != nil && !isDirNotEmpty(err) && !IsAbsent(err) {
		return fmt.Errorf("remove %s: %w", dir, err)
	}
	return nil
}

// removeEmptyDirs walks dir bottom-up and removes every directory
// whose contents are now empty. Stops at the first non-empty directory
// since the parent above it is necessarily non-empty too.
func removeEmptyDirs(dir string) error {
	var dirs []string
	if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			dirs = append(dirs, path)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("walk %s: %w", dir, err)
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		entries, err := os.ReadDir(dirs[i])
		if err != nil {
			if IsAbsent(err) {
				continue
			}
			return fmt.Errorf("readdir %s: %w", dirs[i], err)
		}
		if len(entries) > 0 {
			continue
		}
		// A concurrent write from another target can land between the
		// ReadDir above and this Remove, and then the directory is no
		// longer ours to prune. Treat that as the success it
		// effectively is.
		if err := os.Remove(dirs[i]); err != nil && !IsAbsent(err) && !isDirNotEmpty(err) {
			return fmt.Errorf("remove %s: %w", dirs[i], err)
		}
	}
	return nil
}

// isDirNotEmpty reports whether err is the platform's "directory not
// empty" refusal from removing a directory that gained an entry.
//
// It tests fs.ErrExist rather than syscall.ENOTEMPTY because that is
// the only spelling both platforms answer to. Unix maps ENOTEMPTY onto
// fs.ErrExist, and Windows maps ERROR_DIR_NOT_EMPTY (145) onto it too,
// while Windows' own syscall.ENOTEMPTY is a synthetic APPLICATION_ERROR
// value that nothing returns. Testing ENOTEMPTY directly therefore
// worked on Unix and was dead code on Windows, which is where the race
// it guards actually surfaced (#918).
//
// os.Remove has no path that reports an existing file, so matching
// fs.ErrExist here cannot mask an unrelated failure.
func isDirNotEmpty(err error) bool {
	return err != nil && errors.Is(err, fs.ErrExist)
}

// CopyTree mirrors the regular files under srcDir into dstDir,
// preserving file mode bits so executable scripts keep their +x bit.
// Empty source dir is a no-op. A linked root is followed; nested
// links and other irregular entries are skipped silently.
//
// Each copied file flows through the same mode pipeline as WriteFile,
// so dryRun, capture, recording, detailed recording, and backup all
// behave identically. skip is an optional predicate keyed on the path
// relative to srcDir (forward slashes). Returning true skips the file.
// Adapters use this to exclude SKILL.md when they re-render the
// frontmatter themselves and only want sibling assets propagated.
func (s *Session) CopyTree(srcDir, dstDir string, skip func(rel string) bool, dryRun bool) error {
	return spec.WalkSourceRoot(srcDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if path == srcDir && IsAbsent(walkErr) {
				return nil
			}
			return walkErr
		}
		if path == srcDir && !d.IsDir() {
			return filepath.SkipAll
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if skip != nil && skip(filepath.ToSlash(rel)) {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		dst := filepath.Join(dstDir, rel)
		return s.writeFileWithMode(dst, string(data), fi.Mode().Perm(), true, dryRun)
	})
}
