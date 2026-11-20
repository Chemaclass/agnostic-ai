package emit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/filealias"
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
	// Mode is set when emission enforces an asset or executable mode.
	Mode    os.FileMode `json:"-"`
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
// recording, detailed recording, backup, and transaction
// buffers, plus the user-owned path set from sync.unmanaged. Each sync
// run owns its own Session (see NewSession) so two runs in the same
// process — concurrent library use, parallel wasm renders — never share
// capture/recording buffers or cross-talk. Every mode read and write
// takes the same mutex (the unmanaged set is an atomic pointer) so go
// test -race stays clean when a single Session is shared across
// goroutines.
type Session struct {
	mu        sync.Mutex
	capturing bool
	// writesOnly skips tree sweeps during capture, for callers that
	// read the captured writes and never the removals.
	writesOnly bool
	captured   []CapturedFile
	removals   []CapturedRemoval
	// planned holds the content this session wrote, captured, or
	// previewed for each path an adapter watches, so an adapter can read
	// its own output back. Only watched paths are kept, which bounds it.
	planned     map[string]*string
	backup      bool
	recording   bool
	recorded    []string
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
	// collisionWinners maps each path several targets render differently
	// to the content sync.collision-policy chose, atomic for the same
	// reason as unmanaged.
	collisionWinners atomic.Pointer[map[string]string]
	userTier         bool
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
	// savedBackups are the `<path>.bak` sums the last sync recorded as
	// backups it saved.
	savedBackups    map[string]string
	priorPaths      map[string]string
	priorCandidates []string
	priorAliases    filealias.Cache
	merging         map[string]bool
	overwrote       []string
	// backedUp holds the edits left in place because `<path>.bak`
	// already existed, so writing over them would lose one.
	backedUp []string
	// backups lists every `<path>.bak` this session wrote.
	backups      []string
	removedEdits []string
	// committedSum, when set, gives a path with no recorded sum the sum of
	// its committed version, "" when there is none (see SetCommittedSum).
	committedSum func(path string) string
	// projectRoot, when set, bounds every absolute path the session
	// writes or removes (see SetProjectRoot).
	projectRoot string
}

// SetProjectRoot makes the session refuse an absolute path outside root.
// Project sync sets it before any write: its paths come from a config a
// cloned repository commits. Global sync leaves it unset, since it writes
// tool folders under the user's home.
func (s *Session) SetProjectRoot(root string) { s.projectRoot = root }

// checkProjectPath refuses a path that climbs above the project, lies in
// its .git folder, or, with a project root set, resolves outside it, and
// on Windows a write to a name with a colon. Removing such a name stays
// allowed, so an older sync's output can still be cleaned up. It
// then follows the links along path, inside the working directory, the
// project root, and refuses one that leads outside it or into a Git
// folder (see config.CheckProjectLinks), with a *config.LinkRefusal.
// parentsOnly follows only the folders above path, for a removal or an
// exclusive create.
func (s *Session) checkProjectPath(path, verb string, parentsOnly bool) error {
	switch {
	case !filepath.IsAbs(path) && config.EscapesProject(path):
		return fmt.Errorf("refusing to %s outside the project root: %s", verb, path)
	case !filepath.IsAbs(path) && config.UnsafeProjectPath(path),
		filepath.IsAbs(path) && s.projectRoot != "" && config.UnsafeProjectPathIn(s.projectRoot, path):
		return fmt.Errorf("refusing a path outside the project or inside .git: %s", path)
	case verb == "write" && config.NamesDataStream(path):
		return dataStreamRefusal(path)
	}
	return config.CheckProjectLinks(".", path, verb, parentsOnly)
}

func dataStreamRefusal(path string) error {
	return fmt.Errorf("refusing to write a name with a colon, which Windows stores as a hidden stream of the file before it: %s", path)
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

// WatchPlannedFile asks the session to keep what it next writes,
// captures, or previews at path, for PlannedFile.
func (s *Session) WatchPlannedFile(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.planned == nil {
		s.planned = map[string]*string{}
	}
	s.planned[path] = nil
}

// PlannedFile returns the content this session last wrote, captured, or
// previewed at a watched path. It reports false when the session left
// the file as it is on disk, such as a hand edit sync keeps or a path
// sync.unmanaged covers.
func (s *Session) PlannedFile(path string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	content := s.planned[path]
	if content == nil {
		return "", false
	}
	return *content, true
}

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

// SetCollisionWinners makes every write to one of these paths use the
// content the collision policy chose, so concurrent targets write the
// same bytes and a capture matches what sync wrote.
func (s *Session) SetCollisionWinners(winners map[string]string) {
	if len(winners) == 0 {
		s.collisionWinners.Store(nil)
		return
	}
	s.collisionWinners.Store(&winners)
}

func (s *Session) collisionWinner(path, content string) string {
	winners := s.collisionWinners.Load()
	if winners == nil {
		return content
	}
	if won, ok := (*winners)[filepath.Clean(path)]; ok {
		return won
	}
	return content
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
	byDir := make(map[string][]string, len(writers))
	for dir, targets := range writers {
		dir = filepath.Clean(dir)
		byDir[dir] = append(byDir[dir], targets...)
	}
	s.skillsDirWriters.Store(&byDir)
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

// RemovedEdits returns edited orphan paths saved before removal.
func (s *Session) RemovedEdits() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.removedEdits...)
}

// Backups returns every `<path>.bak` this session wrote, for a hand edit
// or under --backup, in write order.
func (s *Session) Backups() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.backups...)
}

// noteBackup records a `<path>.bak` this session wrote.
func (s *Session) noteBackup(path string) {
	s.mu.Lock()
	s.backups = append(s.backups, path+".bak")
	s.mu.Unlock()
}

// noteKeptEdit records a hand edit the session left in place on list,
// and as an "edited" record under detailed recording.
func (s *Session) noteKeptEdit(list *[]string, edit WrittenFile) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.noteKeptEditLocked(list, edit, s.detailing)
}

// noteKeptEditLocked is noteKeptEdit for a caller that holds s.mu and
// read detailing earlier in the same write.
func (s *Session) noteKeptEditLocked(list *[]string, edit WrittenFile, detailing bool) {
	*list = append(*list, edit.Path)
	if detailing {
		edit.Action = "edited"
		s.detailed = append(s.detailed, edit)
	}
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
func (s *Session) backsUpEdit(cur *onDisk, content string) bool {
	path := cur.path
	s.mu.Lock()
	sum := s.priorChecksum(path, s.backupSums)
	sums := s.backupSums
	merging := s.merging[path]
	committed := s.committedSum
	s.mu.Unlock()
	if sum == "" || merging {
		return false
	}
	// Read before the type check: the write reads this file anyway, and
	// an unchanged output then needs no stat.
	existing, err := cur.bytes()
	if err != nil || string(existing) == content {
		return false
	}
	if info, err := cur.stat(); err != nil || !info.Mode().IsRegular() {
		return false
	}
	gitPath := path
	if cur.isLink() {
		if linksToOutputSum(path, existing, sums) {
			return false
		}
		if target, err := projectLinkTarget(path); err == nil {
			gitPath = target
		}
	}
	// A checkout or pull, not the user, changed a file that matches Git.
	return !MatchesSum(existing, sum) && (committed == nil || !MatchesSum(existing, committed(gitPath))) && !matchesAliasSum(path, existing, sums)
}

// projectLinkTarget returns the file the link at path leads to, relative
// to the working folder, or an error when it leads outside that folder.
func projectLinkTarget(path string) (string, error) {
	root, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if root, err = filepath.EvalSymlinks(root); err != nil {
		return "", err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	target, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s links outside the project", path)
	}
	return rel, nil
}

// linksToOutputSum reports whether the link at path leads to another
// output that still holds the bytes the last sync recorded for it.
func linksToOutputSum(path string, existing []byte, sums map[string]string) bool {
	own, err := os.Stat(path)
	if err != nil {
		return false
	}
	for prior, sum := range sums {
		if prior == path || !MatchesSum(existing, sum) {
			continue
		}
		if info, err := os.Stat(prior); err == nil && os.SameFile(own, info) {
			return true
		}
	}
	return false
}

// TrustBackupsSince sets the sums of the `<path>.bak` files the last
// sync saved. A hand edit whose bytes such a backup still holds is
// written over, since the backup keeps them.
func (s *Session) TrustBackupsSince(sums map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.savedBackups = sums
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
	sum := s.priorChecksum(path, s.keepSums)
	sums := s.keepSums
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
	if sum == "" || MatchesSum(existing, sum) || matchesAliasSum(path, existing, sums) {
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
	s.writesOnly = false
	s.captured = nil
	s.removals = nil
	s.mu.Unlock()
}

// StartWritesOnlyCapture is StartCapture for callers that read only the
// captured writes. It skips legacy tree sweeps, which walk and read every
// file under a directory that grows with the spec count, so emitting one
// spec at a time stays linear.
func (s *Session) StartWritesOnlyCapture() {
	s.mu.Lock()
	s.capturing = true
	s.writesOnly = true
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
	s.writesOnly = false
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

// StartDetailedRecording begins collecting per-file write results alongside
// real writes. Unlike capture mode this does not suppress IO. Unlike recording
// mode it also determines the action ("create", "update", or
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
