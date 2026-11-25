package emit

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/preview"
)

// WriteFile creates parent directories as needed and writes content to path.
// When dryRun is true the file is not written; the path and content are
// printed to stdout instead. When capture mode is active, the call is
// recorded and no IO occurs. When recording mode is active, the path is
// appended to the recorder. When detailed recording mode is active, the
// action (create/update/skip) is determined by comparing against existing
// content; unchanged files are skipped and not rewritten.
func (s *Session) WriteFile(path, content string, dryRun bool) error {
	return s.writeFileWithMode(path, normalizeTrailingNewline(content), filePerm, false, dryRun)
}

// WriteGeneratedFile refuses to replace different bytes without proof of ownership.
func (s *Session) WriteGeneratedFile(path, content string, dryRun bool) error {
	return s.writeFile(path, normalizeTrailingNewline(content), filePerm, false, dryRun, true)
}

// WriteExecutableFile writes a generated script with executable permissions.
// It follows the same capture, unmanaged, backup, transaction, and detailed
// recording behavior as WriteFile.
func (s *Session) WriteExecutableFile(path, content string, dryRun bool) error {
	return s.writeFileWithMode(path, normalizeTrailingNewline(content), executablePerm, true, dryRun)
}

// normalizeTrailingNewline collapses any run of trailing newlines into
// exactly one. Empty content stays empty. Applied to emitted text
// artifacts so a body that ends with one `\n` does not gain a spurious
// blank line just because an upstream concatenation appended an extra
// separator. CopyTree intentionally bypasses this so propagated assets
// stay byte-identical.
func normalizeTrailingNewline(content string) string {
	if content == "" {
		return content
	}
	return strings.TrimRight(content, "\n") + "\n"
}

func symlinkedParent(path string) bool {
	for dir := filepath.Dir(path); dir != "." && dir != ""; {
		if info, err := os.Lstat(dir); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return false
}

// pathLocks serializes concurrent access to the same output path across
// sessions. Parallel sync (`--jobs > 1`) emits every target on its own
// Session, but several targets legitimately write the SAME shared path
// (e.g. the AGENTS.md family of pointer files). Without coordination two
// targets race the read-classify-write critical section below: both observe
// the path missing, both classify a "create", both log a rollback entry,
// and both call os.WriteFile. A per-path mutex makes exactly one target
// create the file and the rest observe it present with identical content
// and skip — matching serial emission, keeping the transaction log free of
// duplicate entries, and never tearing a half-written file.
//
// This is file coordination, not shared business state: the map holds
// one mutex per distinct path for the process lifetime (bounded by the
// output tree, freed on exit) and carries no data of its own.
var pathLocks sync.Map // map[string]*sync.Mutex

// ownershipReads excludes reads while emitted files are being mutated.
var ownershipReads sync.RWMutex

func lockPath(path string) func() {
	m, _ := pathLocks.LoadOrStore(filepath.Clean(path), &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// ReadFile waits for in-process output mutations before reading path.
func ReadFile(path string) ([]byte, error) {
	ownershipReads.Lock()
	defer ownershipReads.Unlock()
	return os.ReadFile(path)
}

// mkdirAll creates dir and its parents, absorbing the transient failures
// that concurrent creation of a shared parent produces.
//
// `sync --jobs` emits targets in parallel and pathLocks keys on a file
// path, so two targets writing different files under one not-yet-existing
// parent reach os.MkdirAll at the same time. When both try to create that
// parent, one can come back with EINVAL or ENOENT instead of the EEXIST
// that MkdirAll knows how to absorb: MkdirAll only swallows a failed
// Mkdir when its follow-up Lstat finds a directory, and that Lstat can
// also fail while the winning thread is still mid-create.
//
// This is not inference. Instrumenting the failure showed the parent
// absent, the working directory valid, no removal of any kind anywhere in
// the emit layer, and an immediate os.Mkdir of that same parent
// succeeding every single time (#526). Nothing is deleting the directory;
// the create is just transiently rejected.
//
// The window opened when antigravity moved to `.agents/rules`, putting a
// second child under the `.agents/` parent that codex, amp, zed, crush,
// openhands, windsurf, augment, and kilo already write skills into. It is
// not antigravity-specific: any target adding a second child of a shared
// root reaches the same edge.
//
// A bounded retry is the right shape here rather than a mutex. The
// contention is between the OS and two threads, not over shared program
// state, and serializing every directory creation would cost parallelism
// on every sync to paper over a failure that resolves in microseconds.
func mkdirAll(dir string, perm os.FileMode) error { //nolint:unparam // perm names the mode at each call, like os.MkdirAll
	var err error
	delay := time.Millisecond
	for attempt := range 5 {
		if err = os.MkdirAll(dir, perm); err == nil {
			return nil
		}
		// A racing thread may have finished between the failure and now.
		if fi, statErr := os.Stat(dir); statErr == nil && fi.IsDir() {
			return nil
		}
		if attempt < 4 {
			time.Sleep(delay)
			delay *= 2
		}
	}
	return err
}

// parentGone reports whether err is the failure a concurrent prune of the
// parent directory produces.
//
// Both errnos count. macOS raises EINVAL as readily as ENOENT when a path
// component is being created or removed underneath the call, which is the
// same pairing mkdirAll above already absorbs. Checking only ENOENT left
// the EINVAL half of one race unhandled and surfaced as an intermittent
// `open .agents/agents/reviewer.md: invalid argument` from sync (#701).
//
// Widening this cannot swallow a genuinely invalid path: writeFileAt
// retries the write once and a real EINVAL comes straight back.
func parentGone(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.EINVAL)
}

// writeFileAt writes content to path, recreating the parent directory
// when a concurrent prune removed it between mkdirAll and the write.
//
// `sync --jobs` runs one target's empty-directory prune alongside
// another target's write into the same shared directory. Codex sweeps
// its legacy `.agents/agents/*.toml` and prunes the directory once the
// last one is gone, while Antigravity writes nested
// `.agents/agents/<name>/agent.md`
// into it. Observed as both `no such file or directory` and `invalid
// argument` on macOS. The prune is correct and the write is correct;
// only their interleaving is wrong, and recreating the parent is the
// cheap half of that fix. See pruneEmptiedDirs for the other half.
func writeFileAt(path, content string, mode os.FileMode, enforceMode bool) error {
	ownershipReads.RLock()
	defer ownershipReads.RUnlock()
	err := os.WriteFile(path, []byte(content), mode)
	if err == nil {
		if enforceMode {
			return os.Chmod(path, mode)
		}
		return nil
	}
	if !parentGone(err) {
		return err
	}
	if mkErr := mkdirAll(filepath.Dir(path), dirPerm); mkErr != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		return err
	}
	if enforceMode {
		return os.Chmod(path, mode)
	}
	return nil
}

func (s *Session) writeFileWithMode(path, content string, mode os.FileMode, enforceMode, dryRun bool) error {
	return s.writeFile(path, content, mode, enforceMode, dryRun, false)
}

func (s *Session) writeFile(path, content string, mode os.FileMode, enforceMode, dryRun, generatedOnly bool) error {
	if s.skipUnmanaged(path) {
		return nil
	}
	// Refused before capture and dry-run so sync --check and --dry-run
	// report what sync would.
	if config.NamesDataStream(path) {
		return dataStreamRefusal(path)
	}
	content = s.collisionWinner(path, content)
	s.mu.Lock()
	capturing := s.capturing
	backup := s.backup
	recording := s.recording
	detailing := s.detailing
	transacting := s.transacting
	if capturing {
		file := CapturedFile{Path: path, Content: content, GeneratedOnly: generatedOnly}
		if enforceMode {
			file.Mode = mode
		}
		s.captured = append(s.captured, file)
	}
	if recording {
		s.recorded = append(s.recorded, path)
	}
	if _, watched := s.planned[path]; watched {
		s.planned[path] = &content
	}
	s.mu.Unlock()

	if capturing {
		return nil
	}
	// Serialize the read-classify-write below against any other session
	// targeting the same path: a target reading a shared file while another
	// rewrites it sees partial bytes and would keep them as a hand edit.
	// Ordering is always pathLock (outer) then s.mu (inner); the early s.mu
	// section above has already been released, so the two never deadlock.
	defer lockPath(path)()
	if err := s.resolvePriorOutput(path); err != nil {
		return err
	}
	cur := &onDisk{path: path}
	if generatedOnly {
		if err := s.refuseHandWrittenFile(cur, content); err != nil {
			return err
		}
	}
	if sum, keep := s.keepsEdit(path, content); keep {
		s.mu.Lock()
		if _, watched := s.planned[path]; watched {
			s.planned[path] = nil
		}
		s.noteKeptEditLocked(&s.kept, WrittenFile{Path: path, Bytes: len(content), Sum: sum}, detailing)
		s.mu.Unlock()
		return nil
	}
	if dryRun {
		fmt.Printf("--- %s ---\n%s\n", path, preview.Display(path, content).Text)
		return nil
	}
	if generatedOnly && !detailing {
		if existing, err := cur.bytes(); err == nil && string(existing) == content {
			return nil
		}
	}
	if err := s.checkProjectPath(path, "write", false); err != nil {
		// Matching bytes need no write, so a linked spec folder that
		// already holds the scaffold still syncs.
		if _, ok := errors.AsType[*config.LinkRefusal](err); !ok {
			return err
		}
		existing, readErr := cur.bytes()
		if readErr != nil || string(existing) != content || enforceMode && !cur.hasPerm(mode) {
			return err
		}
		if detailing {
			s.mu.Lock()
			s.detailed = append(s.detailed, WrittenFile{Path: path, Bytes: len(content), Action: "skip", Sum: ContentSum(content)})
			s.mu.Unlock()
		}
		return nil
	}
	// A file already found there proves its folder exists, so an
	// unchanged output costs no extra call.
	ensureParent := func() error {
		if cur.found() {
			return nil
		}
		return makeParent(path)
	}
	editBackup := s.backsUpEdit(cur, content)
	if editBackup && cur.isLink() {
		if _, err := projectLinkTarget(path); err != nil {
			// Never copy a link target from outside the project into it.
			return fmt.Errorf("refusing to write %s: it links outside the project and holds a hand edit", path)
		}
	}
	var backupPath string
	if editBackup {
		backupPath = path + ".bak"
		err := backUpEdit(cur, backupPath)
		switch {
		case errors.Is(err, fs.ErrExist) && s.backupHolds(cur, backupPath):
		case errors.Is(err, fs.ErrExist):
			s.mu.Lock()
			s.noteKeptEditLocked(&s.backedUp, WrittenFile{Path: path, Bytes: len(content), Sum: s.priorChecksum(path, s.backupSums)}, detailing)
			s.mu.Unlock()
			return nil
		case err != nil:
			return fmt.Errorf("backup %s: %w", path, err)
		}
		s.noteBackup(path)
		if transacting && err == nil {
			// A rollback removes it, so a retry does not take it for an
			// earlier backup and leave the edit stuck in place.
			s.mu.Lock()
			s.txLog = append(s.txLog, txEntry{path: backupPath})
			s.mu.Unlock()
		}
	}

	// Detailed recording: inspect existing content to classify the action.
	if detailing {
		existing, err := cur.bytes()
		var action string
		switch {
		case os.IsNotExist(err):
			action = "create"
		case err == nil && string(existing) == content && (!enforceMode || cur.hasPerm(mode)):
			// File is already up to date; skip the write.
			s.mu.Lock()
			s.detailed = append(s.detailed, WrittenFile{Path: path, Bytes: len(content), Action: "skip", Sum: ContentSum(content)})
			s.mu.Unlock()
			return nil
		default:
			action = "update"
		}
		if err := ensureParent(); err != nil {
			return err
		}
		// Log pre-write state for rollback (only for actual writes, not skips).
		if transacting {
			var pre []byte
			var preMode os.FileMode
			if action == "update" {
				pre = existing
				if info, statErr := cur.stat(); statErr == nil {
					preMode = info.Mode().Perm()
				}
			}
			s.mu.Lock()
			s.txLog = append(s.txLog, txEntry{path: path, content: pre, mode: preMode})
			s.mu.Unlock()
		}
		if backup && action == "update" {
			if err := writeBackup(path+".bak", existing, sourcePerm(cur.stat())); err != nil {
				return fmt.Errorf("backup %s: %w", path, err)
			}
			s.noteBackup(path)
		}
		if err := writeFileAt(path, content, mode, enforceMode); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		s.mu.Lock()
		s.detailed = append(s.detailed, WrittenFile{Path: path, Bytes: len(content), Action: action, Sum: ContentSum(content), Backup: backupPath})
		if editBackup {
			s.overwrote = append(s.overwrote, path)
		}
		s.mu.Unlock()
		return nil
	}

	if err := ensureParent(); err != nil {
		return err
	}
	// Log pre-write state for rollback.
	if transacting {
		pre, readErr := cur.bytes()
		info, statErr := cur.stat()
		s.mu.Lock()
		switch {
		case readErr == nil:
			preMode := filePerm
			if statErr == nil {
				preMode = info.Mode().Perm()
			}
			s.txLog = append(s.txLog, txEntry{path: path, content: pre, mode: preMode})
		case os.IsNotExist(readErr):
			s.txLog = append(s.txLog, txEntry{path: path, content: nil})
		}
		// Other read errors: skip logging; the write below will also fail.
		s.mu.Unlock()
	}

	if backup {
		if existing, err := cur.bytes(); err == nil && string(existing) != content {
			if err := writeBackup(path+".bak", existing, sourcePerm(cur.stat())); err != nil {
				return fmt.Errorf("backup %s: %w", path, err)
			}
			s.noteBackup(path)
		}
	}
	if err := writeFileAt(path, content, mode, enforceMode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if editBackup {
		s.mu.Lock()
		s.overwrote = append(s.overwrote, path)
		s.mu.Unlock()
	}
	return nil
}

func (s *Session) refuseHandWrittenFile(cur *onDisk, content string) error {
	if err := s.checkProjectPath(cur.path, "write", false); err != nil {
		return err
	}
	info, err := cur.lstat()
	if IsAbsent(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect %s: %w", cur.path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to write %s: expected a regular file", cur.path)
	}
	existing, err := cur.bytes()
	if err != nil {
		return fmt.Errorf("read %s: %w", cur.path, err)
	}
	if string(existing) == content || header.Leads(cur.path, string(existing)) {
		return nil
	}
	s.mu.Lock()
	backupSum := s.priorChecksum(cur.path, s.backupSums)
	keepSum := s.priorChecksum(cur.path, s.keepSums)
	s.mu.Unlock()
	if MatchesSum(existing, backupSum) || MatchesSum(existing, keepSum) {
		return nil
	}
	return fmt.Errorf("refusing to overwrite hand-written file %s; keep it with sync.unmanaged or move it before syncing", cur.path)
}

// writeBackup keeps content at path with perm, the source file's mode.
// It replaces an earlier backup with a new file, so the copy never takes
// the old file's wider mode, and it refuses a link or other non-file
// there rather than write through it.
func writeBackup(path string, content []byte, perm os.FileMode) error {
	ownershipReads.RLock()
	defer ownershipReads.RUnlock()
	return writeBackupLocked(path, content, perm)
}

func writeBackupLocked(path string, content []byte, perm os.FileMode) error {
	info, err := os.Lstat(path)
	switch {
	case err == nil && !info.Mode().IsRegular():
		return fmt.Errorf("%s exists and is not a regular file", path)
	case err == nil:
		if err := os.Remove(path); err != nil {
			return err
		}
	case !IsAbsent(err):
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	_, err = f.Write(content)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(path)
	}
	return err
}

// sourcePerm is the permission bits of a stat result, filePerm when the
// stat failed.
func sourcePerm(info fs.FileInfo, err error) os.FileMode {
	if err != nil {
		return filePerm
	}
	return info.Mode().Perm()
}

// backUpEdit copies path to backup, which must not exist yet in any form,
// so an earlier backup is never lost and a planted link is never followed.
func backUpEdit(cur *onDisk, backup string) error {
	data, err := cur.bytes()
	if err != nil {
		return err
	}
	info, err := cur.stat()
	if err != nil {
		return err
	}
	ownershipReads.RLock()
	defer ownershipReads.RUnlock()
	f, err := os.OpenFile(backup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		// A partial copy would read as an earlier backup on the next sync.
		_ = os.Remove(backup)
	}
	return err
}

// backupHolds reports whether backup is a separate regular file that
// the last sync saved, still unchanged, and holding cur's bytes, so
// writing over cur loses nothing. A copy the user made proves nothing:
// it may be another output this run writes over.
func (s *Session) backupHolds(cur *onDisk, backup string) bool {
	s.mu.Lock()
	sum := s.savedBackups[filepath.ToSlash(filepath.Clean(backup))]
	s.mu.Unlock()
	if sum == "" {
		return false
	}
	info, err := os.Lstat(backup)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	if own, err := cur.lstat(); err != nil || os.SameFile(own, info) {
		return false
	}
	saved, err := readFile(backup)
	if err != nil {
		return false
	}
	existing, err := cur.bytes()
	return err == nil && MatchesSum(saved, sum) && bytes.Equal(saved, existing)
}

// createUntrackedFileExclusive atomically claims path as a brand-new
// file holding content, or reports that something already occupies it,
// with the same capture/dry-run guards, project-root check, path
// locking, and transaction logging (so a Rollback later in the same
// pass removes the file this created) that WriteFile has, but it never
// touches capturing, recording (the gitignore block builder), or
// detailed recording (the sync ledger that proves ownership for the
// next run's orphan sweep).
//
// Used for side content a caller writes deliberately outside its own
// managed-output set, such as one candidate in MigrateLegacyPath's
// numbered `.bak` search: that file must survive the *next* sync
// untouched, not get treated as a generated output this run wrote and
// a later run stopped writing, which is exactly what the orphan sweep
// deletes (#1114 review).
//
// The claim goes through os.OpenFile with O_CREATE|O_EXCL, not a
// separate os.Stat-then-os.WriteFile: that TOCTOU pair both raced
// against a concurrent writer and, worse, let a dangling symlink at
// path pass the existence check (os.Stat follows the link and reports
// "not found" for a dangling target) only for the write that followed
// to create the file at wherever the link actually pointed, possibly
// outside the project. O_EXCL fails on any existing directory entry at
// path, symlink included, dangling or not, without ever following it
// (#1114 review). ok is false, with a nil error, exactly when path was
// already taken; err is any other I/O failure, and unmanaged is
// checked by the caller before this is reached, once per candidate,
// since a numbered search tries several paths and only the caller
// knows the whole set.
func (s *Session) createUntrackedFileExclusive(path, content string, dryRun bool) (ok bool, err error) {
	if s.IsCapturing() || dryRun {
		return false, nil
	}
	// O_EXCL below refuses a link at path itself; only its folders can
	// lead elsewhere.
	if err := s.checkProjectPath(path, "write", true); err != nil {
		return false, err
	}
	defer lockPath(path)()
	if err := mkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return false, fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	ownershipReads.RLock()
	defer ownershipReads.RUnlock()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, filePerm)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return false, nil
		}
		return false, fmt.Errorf("create %s: %w", path, err)
	}
	_, writeErr := f.WriteString(content)
	closeErr := f.Close()
	if err := firstNonNil(writeErr, closeErr); err != nil {
		_ = os.Remove(path)
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	if s.transacting {
		s.mu.Lock()
		s.txLog = append(s.txLog, txEntry{path: path, content: nil})
		s.mu.Unlock()
	}
	return true, nil
}

func firstNonNil(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
