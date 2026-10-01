// Package emit holds shared helpers for adapter packages: file writing,
// frontmatter rendering, capability reporting, and the two common emission
// patterns (single merged document, one-file-per-rule directory).
package emit

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// File permissions for emitted artifacts.
const (
	dirPerm        os.FileMode = 0o755
	filePerm       os.FileMode = 0o644
	executablePerm os.FileMode = 0o755
)

// CapturedFile is one (path, content) pair recorded during capture mode.
type CapturedFile struct {
	Path    string
	Content string
	// Merged, Keys, and Released carry a merged JSON write's claims, as
	// WrittenFile does, so `doctor --fix` can record what it writes.
	Merged   bool
	Keys     []MergedKey
	Released [][]string
}

// CapturedRemoval is one removal RemoveOwned would have made outside
// capture mode, with the ownership proof it was given. `doctor --fix`
// replays it through RemoveOwned so a file standing where a directory
// belongs gives way the same way it does on sync (#1064).
type CapturedRemoval struct {
	Path string
	Sum  string
}

// txEntry records the pre-write state of one file for transaction rollback.
// content is nil when the file did not exist before the write. link is the
// target of a symlink the transaction removed from path.
type txEntry struct {
	path    string
	content []byte
	mode    os.FileMode
	link    string
}

// WrittenFile is one write event recorded during detailed recording mode.
// Action is "create" (new file), "update" (existing file with changed
// content), "skip" (existing file with identical content, not rewritten),
// or "edited" (a hand edit KeepEditsSince left in place; Sum is the sum
// the last sync recorded, so the edit stays visible to the next check).
//
// Sum is the ContentSum of the bytes. The sync ledger stores it so a
// later orphan sweep can prove a header-less file is still the one
// agnostic-ai wrote (#785), and so `sync --check` can tell a hand edit
// from a spec change (#1270).
type WrittenFile struct {
	Path   string
	Bytes  int
	Action string
	Sum    string
	// Backup is the `<path>.bak` that holds a hand edit this write
	// replaced, empty when the file was not edited since the last sync.
	Backup string
	// Merged marks a JSON file sync merged into, which may also hold
	// keys sync did not write. Keys lists the values sync set there,
	// and Released the key paths it removed or left to the user.
	Merged   bool
	Keys     []MergedKey
	Released [][]string
}

// Session holds the mutable mode flags for one emission pass: capture,
// recording, counting, detailed recording, backup, and transaction
// buffers, plus the user-owned path set from sync.unmanaged. Each sync
// run owns its own Session (see NewSession) so two runs in the same
// process — concurrent library use, parallel wasm renders — never share
// capture/recording buffers or cross-talk. Every mode read and write
// takes the same mutex (the unmanaged set is an atomic pointer) so go
// test -race stays clean when a single Session is shared across
// goroutines.
type Session struct {
	mu          sync.Mutex
	capturing   bool
	captured    []CapturedFile
	removals    []CapturedRemoval
	backup      bool
	recording   bool
	recorded    []string
	counting    bool
	counted     int
	detailing   bool
	detailed    []WrittenFile
	transacting bool
	txLog       []txEntry
	// unmanaged holds the sync.unmanaged patterns. An atomic pointer, not
	// a field under mu, because skipUnmanaged runs first on every write
	// and removal: with nothing user-owned (nil) the check is one atomic
	// load and never contends on mu, while SetUnmanaged stays safe to call
	// on a session other goroutines already write through.
	unmanaged atomic.Pointer[[]string]
	skipped   []string // user-owned paths refused; may repeat a path
	userTier  bool
	// codexSkillsDir is a skills dir Codex was configured to emit into,
	// atomic for the same reason as unmanaged.
	codexSkillsDir atomic.Pointer[string]
	// skillsDirWriters maps each skills dir to the enabled targets that
	// write it, atomic for the same reason as unmanaged.
	skillsDirWriters atomic.Pointer[map[string][]string]
	// inlinedRules are the rules EmitWithProvenance left out of this
	// emit because the entry point carries them.
	inlinedRules []spec.Entry
	// keepSums, when non-nil, are the output sums of the last sync: a
	// write over a file whose bytes no longer match its sum is skipped
	// and recorded in kept (see KeepEditsSince).
	keepSums map[string]string
	kept     []string
	// backupSums, when non-nil, are the output sums of the last sync: a
	// write over a file whose bytes no longer match its sum first keeps
	// them as `<path>.bak` and records the path in overwrote (see
	// BackUpEditsSince). merging holds the paths a merged write is
	// writing, which hold user keys by design.
	backupSums map[string]string
	merging    map[string]bool
	overwrote  []string
	// backedUp holds the edits left in place because `<path>.bak`
	// already existed, so writing over them would lose one.
	backedUp []string
	// committedSum, when set, gives a path with no recorded sum the sum of
	// its committed version, "" when there is none (see SetCommittedSum).
	committedSum func(path string) string
}

// SetUserTier marks a session that writes a tool's user-level
// configuration, where user settings files are reachable.
func (s *Session) SetUserTier() { s.userTier = true }

// UserTier reports whether SetUserTier was called.
func (s *Session) UserTier() bool { return s.userTier }

// NewSession returns a Session with every mode off, ready to be threaded
// through one emission pass. Adapters and the CLI toggle modes on it and
// pass it to WriteFile and friends; each sync run constructs its own.
func NewSession() *Session { return &Session{} }

// SetUnmanaged installs the sync.unmanaged patterns. A matching path is
// never written, merged, copied, renamed, or removed by this session;
// each refusal is recorded for UnmanagedSkips so sync can report it.
// EmitWithProvenance calls this for every adapter emit; the serial
// entry-point session and the scoped-rule validators call it directly.
func (s *Session) SetUnmanaged(patterns []string) {
	if len(patterns) == 0 {
		s.unmanaged.Store(nil)
		return
	}
	s.unmanaged.Store(&patterns)
}

// SetCodexSkillsDir records the skills dir the codex target writes, so
// another target sharing it writes the same merged agents/openai.yaml.
func (s *Session) SetCodexSkillsDir(dir string) {
	if dir == "" {
		s.codexSkillsDir.Store(nil)
		return
	}
	s.codexSkillsDir.Store(&dir)
}

// SetSkillsDirWriters records which enabled targets write each skills dir.
func (s *Session) SetSkillsDirWriters(writers map[string][]string) {
	s.skillsDirWriters.Store(&writers)
}

// SkillsDirWrittenByOthers reports whether a target other than self
// writes dir in this sync. Only a dir no other target writes keeps its
// disk state while self emits, whatever order parallel targets run in.
func (s *Session) SkillsDirWrittenByOthers(dir, self string) bool {
	writers := s.skillsDirWriters.Load()
	if writers == nil {
		return false
	}
	for _, t := range (*writers)[filepath.Clean(dir)] {
		if t != self {
			return true
		}
	}
	return false
}

// SetInlinedRules records the rules the current emit leaves to the
// entry point, so an adapter can drop what it listed for them before.
func (s *Session) SetInlinedRules(rules []spec.Entry) {
	s.mu.Lock()
	s.inlinedRules = rules
	s.mu.Unlock()
}

// InlinedRules returns the rules SetInlinedRules recorded.
func (s *Session) InlinedRules() []spec.Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inlinedRules
}

// UnmanagedSkips returns every user-owned path this session refused to
// touch, in refusal order. A path refused twice appears twice: sync
// merges sessions and dedupes there anyway. Recorded in every mode
// (capture, dry-run, real) so the summary is the same whichever ran.
func (s *Session) UnmanagedSkips() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.skipped...)
}

// KeepEditsSince makes this session keep hand edits: a write over a
// file whose bytes differ both from the new content and from sums[path],
// the sum the last sync recorded, is skipped. A path with no recorded
// sum falls back on SetCommittedSum; with neither it has no proof of an
// edit and is written. Set before any write.
func (s *Session) KeepEditsSince(sums map[string]string) {
	if sums == nil {
		sums = map[string]string{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keepSums = sums
}

// BackUpEditsSince makes this session keep each hand edit it writes over:
// a file whose bytes differ both from the new content and from sums[path],
// the sum the last sync recorded, is copied to `<path>.bak` first. A path
// with no recorded sum has no proof of an edit. Set before any write.
func (s *Session) BackUpEditsSince(sums map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backupSums = sums
}

// OverwroteEdits returns the paths whose hand edit this session saved as
// `<path>.bak` before writing over it, in write order.
func (s *Session) OverwroteEdits() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.overwrote...)
}

// BackupBlockedEdits returns the paths whose hand edit this session left
// in place because `<path>.bak` already existed, in write order.
func (s *Session) BackupBlockedEdits() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.backedUp...)
}

// backsUpEdit reports whether a write of content to path replaces a hand
// edit BackUpEditsSince must keep. Call it under the path lock, so a
// second target writing the same bytes sees the first one's write.
func (s *Session) backsUpEdit(path, content string) bool {
	s.mu.Lock()
	sum := s.backupSums[path]
	merging := s.merging[path]
	committed := s.committedSum
	s.mu.Unlock()
	if sum == "" || merging {
		return false
	}
	// A link's target may live outside the project; never copy it in.
	if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() {
		return false
	}
	existing, err := os.ReadFile(path)
	if err != nil || string(existing) == content {
		return false
	}
	got := ContentSum(string(existing))
	// A checkout or pull, not the user, changed a file that matches Git.
	return got != sum && (committed == nil || got != committed(path))
}

// SetCommittedSum sets the lookup KeepEditsSince falls back on for a path
// with no recorded sum: the ContentSum of the version Git committed, or ""
// when the path has none. Set before any write.
func (s *Session) SetCommittedSum(lookup func(path string) string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.committedSum = lookup
}

// KeepsEdits reports whether KeepEditsSince was called.
func (s *Session) KeepsEdits() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.keepSums != nil
}

// KeptEdits returns the paths this session left alone because they were
// edited since the last sync, in write order.
func (s *Session) KeptEdits() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.kept...)
}

// keepsEdit reports whether a write of content to path must leave a hand
// edit in place, and returns the sum the last sync recorded for it.
func (s *Session) keepsEdit(path, content string) (string, bool) {
	s.mu.Lock()
	sum := s.keepSums[path]
	keeping := s.keepSums != nil
	committed := s.committedSum
	s.mu.Unlock()
	if !keeping {
		return "", false
	}
	existing, err := os.ReadFile(path)
	if err != nil || string(existing) == content {
		return "", false
	}
	if sum == "" && committed != nil {
		sum = committed(path)
	}
	if sum == "" || ContentSum(string(existing)) == sum {
		return "", false
	}
	return sum, true
}

// IsUnmanaged reports whether path matches sync.unmanaged, without
// recording a refusal. Callers use it to tell a user-owned path from
// one the session declined to touch for another reason.
func (s *Session) IsUnmanaged(path string) bool {
	patterns := s.unmanaged.Load()
	return patterns != nil && config.MatchUnmanaged(*patterns, path)
}

// skipUnmanaged reports whether path is user-owned, recording the hit.
// Callers return immediately when it is true, before capture, recording,
// or any disk write, so the path is invisible to every mode.
func (s *Session) skipUnmanaged(path string) bool {
	if !s.IsUnmanaged(path) {
		return false
	}
	s.mu.Lock()
	s.skipped = append(s.skipped, path)
	s.mu.Unlock()
	return true
}

// SetBackup toggles backup mode. When enabled, WriteFile copies an
// existing file to `<path>.bak` before overwriting (only when the new
// content differs). Pair SetBackup(true) with SetBackup(false) once the
// sync pass completes.
func (s *Session) SetBackup(b bool) {
	s.mu.Lock()
	s.backup = b
	s.mu.Unlock()
}

// StartCapture redirects subsequent WriteFile calls to an in-memory buffer
// instead of touching disk or stdout. Used by `sync --check`, `doctor`,
// and `revert` to inspect what each adapter would emit.
func (s *Session) StartCapture() {
	s.mu.Lock()
	s.capturing = true
	s.captured = nil
	s.removals = nil
	s.mu.Unlock()
}

// IsCapturing reports whether capture mode is active. Adapters that
// perform side-effects beyond `WriteFile` (e.g. one-shot file renames
// for migrations) should consult it and no-op when true, so dry-check
// modes do not mutate the working tree.
func (s *Session) IsCapturing() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.capturing
}

// StopCapture returns the captured files and disables capture mode.
func (s *Session) StopCapture() []CapturedFile {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.capturing = false
	out := s.captured
	s.captured = nil
	return out
}

// CapturedRemovals returns the removals recorded since the last
// StartCapture. Unlike StopCapture it does not clear them.
func (s *Session) CapturedRemovals() []CapturedRemoval {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.removals
}

// StartRecording begins collecting written paths alongside real writes.
// Unlike capture mode this does not suppress IO. Used by `sync` to learn
// every emitted path in a single pass for follow-up actions like
// .gitignore management.
func (s *Session) StartRecording() {
	s.mu.Lock()
	s.recording = true
	s.recorded = nil
	s.mu.Unlock()
}

// StopRecording returns the recorded paths and disables recording.
func (s *Session) StopRecording() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recording = false
	out := s.recorded
	s.recorded = nil
	return out
}

// StartCounting begins tracking the number of files written to disk.
// Does not affect IO.
func (s *Session) StartCounting() {
	s.mu.Lock()
	s.counting = true
	s.counted = 0
	s.mu.Unlock()
}

// StopCounting returns the count of files written since StartCounting
// and disables counting mode.
func (s *Session) StopCounting() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counting = false
	n := s.counted
	return n
}

// StartDetailedRecording begins collecting per-file write results alongside
// real writes. Unlike capture mode this does not suppress IO. Unlike counting
// and recording modes it also determines the action ("create", "update", or
// "skip") by comparing the new content against what is on disk before writing.
// Files whose content is unchanged are not rewritten and are recorded with
// action "skip".
func (s *Session) StartDetailedRecording() {
	s.mu.Lock()
	s.detailing = true
	s.detailed = nil
	s.mu.Unlock()
}

// StopDetailedRecording returns the collected write records and disables
// detailed recording mode.
func (s *Session) StopDetailedRecording() []WrittenFile {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.detailing = false
	out := s.detailed
	s.detailed = nil
	return out
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
	for i := len(log) - 1; i >= 0; i-- {
		e := log[i]
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

// escapesProjectRoot reports whether a project-relative output path climbs
// above the project root via `..`. Adapters always emit inside the project,
// so a relative path that escapes upward signals a traversal (e.g. from a
// crafted spec `name:` or `scope:`). Defense-in-depth behind the loader's
// name check; absolute paths are left to the caller.
func escapesProjectRoot(path string) bool {
	if filepath.IsAbs(path) {
		return false
	}
	clean := filepath.Clean(path)
	return clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator))
}

// pathLocks serializes concurrent writes to the same output path across
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
// This is pure write coordination, not shared business state: the map holds
// one mutex per distinct path for the process lifetime (bounded by the
// output tree, freed on exit) and carries no data of its own.
var pathLocks sync.Map // map[string]*sync.Mutex

// lockPath acquires the write mutex for path (keyed by its cleaned form so
// spellings of the same file coincide) and returns the unlock func.
func lockPath(path string) func() {
	m, _ := pathLocks.LoadOrStore(filepath.Clean(path), &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
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
func mkdirAll(dir string, perm os.FileMode) error {
	var err error
	delay := time.Millisecond
	for attempt := 0; attempt < 5; attempt++ {
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
	if s.skipUnmanaged(path) {
		return nil
	}
	s.mu.Lock()
	capturing := s.capturing
	backup := s.backup
	recording := s.recording
	detailing := s.detailing
	transacting := s.transacting
	if capturing {
		s.captured = append(s.captured, CapturedFile{Path: path, Content: content})
	}
	if recording {
		s.recorded = append(s.recorded, path)
	}
	if s.counting && !capturing && !dryRun {
		s.counted++
	}
	s.mu.Unlock()

	if capturing {
		return nil
	}
	// No lock needed: targets sharing a path write identical bytes, so
	// once one rewrites an unedited file the others see it current.
	if sum, keep := s.keepsEdit(path, content); keep {
		s.mu.Lock()
		s.kept = append(s.kept, path)
		if detailing {
			s.detailed = append(s.detailed, WrittenFile{Path: path, Bytes: len(content), Action: "edited", Sum: sum})
		}
		s.mu.Unlock()
		return nil
	}
	if dryRun {
		fmt.Printf("--- %s ---\n%s\n", path, content)
		return nil
	}
	if escapesProjectRoot(path) {
		return fmt.Errorf("refusing to write outside the project root: %s", path)
	}
	// Serialize the read-classify-write below against any other session
	// targeting the same path so concurrent emitters cannot both see it
	// missing. Held until the write and its rollback/detail bookkeeping
	// complete. Ordering is always pathLock (outer) then s.mu (inner); the
	// early s.mu section above has already been released, so no goroutine
	// holds s.mu while acquiring a path lock and the two never deadlock.
	defer lockPath(path)()
	if err := mkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	editBackup := s.backsUpEdit(path, content)
	var backupPath string
	if editBackup {
		backupPath = path + ".bak"
		switch err := backUpEdit(path, backupPath); {
		case errors.Is(err, fs.ErrExist):
			s.mu.Lock()
			s.backedUp = append(s.backedUp, path)
			if detailing {
				s.detailed = append(s.detailed, WrittenFile{Path: path, Bytes: len(content), Action: "edited", Sum: s.backupSums[path]})
			}
			s.mu.Unlock()
			return nil
		case err != nil:
			return fmt.Errorf("backup %s: %w", path, err)
		}
		if transacting {
			// A rollback removes it, so a retry does not take it for an
			// earlier backup and leave the edit stuck in place.
			s.mu.Lock()
			s.txLog = append(s.txLog, txEntry{path: backupPath})
			s.mu.Unlock()
		}
	}

	// Detailed recording: inspect existing content to classify the action.
	if detailing {
		existing, err := os.ReadFile(path)
		info, statErr := os.Stat(path)
		var action string
		switch {
		case os.IsNotExist(err):
			action = "create"
		case err == nil && string(existing) == content && (!enforceMode || statErr == nil && info.Mode().Perm() == mode.Perm()):
			// File is already up to date; skip the write.
			s.mu.Lock()
			s.detailed = append(s.detailed, WrittenFile{Path: path, Bytes: len(content), Action: "skip", Sum: ContentSum(content)})
			s.mu.Unlock()
			return nil
		default:
			action = "update"
		}
		// Log pre-write state for rollback (only for actual writes, not skips).
		if transacting {
			var pre []byte
			var preMode os.FileMode
			if action == "update" {
				pre = existing
				if statErr == nil {
					preMode = info.Mode().Perm()
				}
			}
			s.mu.Lock()
			s.txLog = append(s.txLog, txEntry{path: path, content: pre, mode: preMode})
			s.mu.Unlock()
		}
		if backup && action == "update" {
			if err := os.WriteFile(path+".bak", existing, filePerm); err != nil {
				return fmt.Errorf("backup %s: %w", path, err)
			}
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

	// Log pre-write state for rollback.
	if transacting {
		pre, readErr := os.ReadFile(path)
		info, statErr := os.Stat(path)
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
		if existing, err := os.ReadFile(path); err == nil && string(existing) != content {
			if err := os.WriteFile(path+".bak", existing, filePerm); err != nil {
				return fmt.Errorf("backup %s: %w", path, err)
			}
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

// backUpEdit copies path to backup, which must not exist yet in any form,
// so an earlier backup is never lost and a planted link is never followed.
func backUpEdit(path, backup string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
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
	if escapesProjectRoot(path) {
		return false, fmt.Errorf("refusing to write outside the project root: %s", path)
	}
	defer lockPath(path)()
	if err := mkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return false, fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
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
func (s *Session) remove(path, sum string, existing []byte, handWritten, dryRun bool) (removed bool, err error) {
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
		if err := os.WriteFile(path+".bak", existing, filePerm); err != nil {
			return false, fmt.Errorf("backup %s: %w", path, err)
		}
	}
	if err := os.Remove(path); err != nil && !IsAbsent(err) {
		return false, fmt.Errorf("remove %s: %w", path, err)
	}

	if detailing {
		s.mu.Lock()
		s.detailed = append(s.detailed, WrittenFile{Path: path, Bytes: 0, Action: "delete"})
		s.mu.Unlock()
	}
	return true, nil
}

// ContentSum returns the hex sha256 of content, the fingerprint the sync
// ledger records for every output.
func ContentSum(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
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
// Empty source dir is a no-op. Symlinks and other irregular entries
// are skipped silently.
//
// Each copied file flows through the same mode pipeline as WriteFile,
// so dryRun, capture, recording, detailed recording, and backup all
// behave identically. skip is an optional predicate keyed on the path
// relative to srcDir (forward slashes). Returning true skips the file
// — adapters use this to exclude SKILL.md when they re-render the
// frontmatter themselves and only want sibling assets propagated.
func (s *Session) CopyTree(srcDir, dstDir string, skip func(rel string) bool, dryRun bool) error {
	info, err := os.Stat(srcDir)
	if err != nil {
		if IsAbsent(err) {
			return nil
		}
		return fmt.Errorf("stat %s: %w", srcDir, err)
	}
	if !info.IsDir() {
		return nil
	}
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
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

// Frontmatter renders meta as a YAML frontmatter block. Empty meta returns
// an empty string. Keys are emitted in alphabetical order; callers that
// need to preserve source key ordering should use FrontmatterOrdered.
func Frontmatter(meta map[string]any) string {
	return FrontmatterOrdered(meta, nil)
}

// FrontmatterOrdered renders meta as a YAML frontmatter block with keys
// emitted in the given order. Keys missing from `keys` are appended
// alphabetically. Empty meta returns "".
//
// Equivalent to FrontmatterStyled(meta, keys, nil): when no source
// styles are available the emitter still preserves source key order
// and forces 2-space sequence indent, and still promotes
// single-quoted scalars to double-quoted (yaml.v3's quoted default).
func FrontmatterOrdered(meta map[string]any, keys []string) string {
	return FrontmatterStyled(meta, keys, nil)
}

// FrontmatterStyled renders meta as a YAML frontmatter block. `keys`
// hints at source order; missing keys are appended alphabetically.
// `styles` carries per-key value styles captured at parse time so
// scalars round-trip byte-equivalently:
//
//   - A double-quoted source scalar stays double-quoted.
//   - A plain source scalar stays plain (no auto-promotion to quotes,
//     even when the value contains characters like `<` that look like
//     they want quoting).
//   - Single-quoted source scalars get promoted to double-quoted,
//     matching the convention used by hand-authored CLI configs.
//   - Keys missing from styles fall through to yaml.v3's encoder
//     default, which is plain whenever the value is unambiguous.
//
// Empty meta returns "".
func FrontmatterStyled(meta map[string]any, keys []string, styles map[string]yaml.Style) string {
	if len(meta) == 0 {
		return ""
	}
	ordered := orderedMetaKeys(meta, keys)
	root := &yaml.Node{Kind: yaml.MappingNode}
	for _, k := range ordered {
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: k}
		valNode := &yaml.Node{}
		if err := valNode.Encode(meta[k]); err != nil {
			return ""
		}
		applySourceStyle(valNode, styles[k])
		preferDoubleQuotes(valNode)
		root.Content = append(root.Content, keyNode, valNode)
	}
	var buf strings.Builder
	enc := yaml.NewEncoder(&yamlWriter{b: &buf})
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		_ = enc.Close()
		return ""
	}
	if err := enc.Close(); err != nil {
		return ""
	}
	var out strings.Builder
	out.WriteString("---\n")
	out.WriteString(buf.String())
	out.WriteString("---\n")
	return out.String()
}

// applySourceStyle stamps a non-zero source style onto a scalar leaf so
// the encoder reproduces the author's quoting. No-op for zero (plain),
// for non-scalar nodes, and for nodes whose value has nested structure
// where forcing a scalar style would be invalid.
func applySourceStyle(n *yaml.Node, style yaml.Style) {
	if n == nil || style == 0 || n.Kind != yaml.ScalarNode {
		return
	}
	n.Style = style
}

// yamlWriter adapts strings.Builder to io.Writer so the YAML encoder
// can stream into it without an intermediate bytes.Buffer.
type yamlWriter struct{ b *strings.Builder }

func (w *yamlWriter) Write(p []byte) (int, error) { return w.b.Write(p) }

// orderedMetaKeys returns the union of meta's keys with `hint` consulted
// first (preserving the caller's preferred order) and any remaining keys
// appended in alphabetical order. Keys in hint that are absent from
// meta are skipped.
func orderedMetaKeys(meta map[string]any, hint []string) []string {
	seen := make(map[string]bool, len(meta))
	out := make([]string, 0, len(meta))
	for _, k := range hint {
		if _, ok := meta[k]; ok && !seen[k] {
			out = append(out, k)
			seen[k] = true
		}
	}
	rest := make([]string, 0, len(meta))
	for k := range meta {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}

// preferDoubleQuotes walks a yaml.Node tree and promotes single-quoted
// scalars to double-quoted (yaml.v3 defaults to single quotes when a
// scalar must be quoted, e.g. starts with `[`; CLI authors typically
// use double quotes). Plain scalars stay plain — source-level style
// preservation now flows through MetaStyles / applySourceStyle, so
// the previous angle-bracket auto-promotion is unnecessary and was
// breaking round-trip for hand-authored plain `<ver>` scalars.
func preferDoubleQuotes(n *yaml.Node) {
	if n == nil {
		return
	}
	if n.Kind == yaml.ScalarNode && n.Style == yaml.SingleQuotedStyle {
		n.Style = yaml.DoubleQuotedStyle
	}
	for _, c := range n.Content {
		preferDoubleQuotes(c)
	}
}

// Warner accepts capability warnings. Defaults to writing to os.Stderr.
// Tests inject a buffer to assert messages.
var Warner io.Writer = os.Stderr
