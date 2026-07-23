// Package emit holds shared helpers for adapter packages: file writing,
// frontmatter rendering, capability reporting, and the two common emission
// patterns (single merged document, one-file-per-rule directory).
package emit

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
)

// File permissions for emitted artifacts.
const (
	dirPerm  os.FileMode = 0o755
	filePerm os.FileMode = 0o644
)

// CapturedFile is one (path, content) pair recorded during capture mode.
type CapturedFile struct {
	Path    string
	Content string
}

// txEntry records the pre-write state of one file for transaction rollback.
// content is nil when the file did not exist before the write.
type txEntry struct {
	path    string
	content []byte
}

// WrittenFile is one write event recorded during detailed recording mode.
// Action is "create" (new file), "update" (existing file with changed
// content), or "skip" (existing file with identical content, not rewritten).
type WrittenFile struct {
	Path   string
	Bytes  int
	Action string
}

// Session holds the mutable mode flags for one emission pass: capture,
// recording, counting, detailed recording, backup, and transaction
// buffers. Each sync run owns its own Session (see NewSession) so two
// runs in the same process — concurrent library use, parallel wasm
// renders — never share capture/recording buffers or cross-talk. Every
// read and write takes the same mutex so go test -race stays clean when
// a single Session is shared across goroutines.
type Session struct {
	mu          sync.Mutex
	capturing   bool
	captured    []CapturedFile
	backup      bool
	recording   bool
	recorded    []string
	counting    bool
	counted     int
	detailing   bool
	detailed    []WrittenFile
	transacting bool
	txLog       []txEntry
}

// NewSession returns a Session with every mode off, ready to be threaded
// through one emission pass. Adapters and the CLI toggle modes on it and
// pass it to WriteFile and friends; each sync run constructs its own.
func NewSession() *Session { return &Session{} }

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

// Rollback undoes all file writes recorded since StartTransaction. New files
// are removed; overwritten files are restored from their pre-write content.
// All entries are attempted; errors are joined and returned.
func (s *Session) Rollback() error {
	s.mu.Lock()
	log := s.txLog
	s.txLog = nil
	s.transacting = false
	s.mu.Unlock()

	var errs []error
	for i := len(log) - 1; i >= 0; i-- {
		e := log[i]
		if e.content == nil {
			if err := os.Remove(e.path); err != nil && !os.IsNotExist(err) {
				errs = append(errs, fmt.Errorf("rollback %s: %w", e.path, err))
			}
		} else {
			if err := os.WriteFile(e.path, e.content, filePerm); err != nil {
				errs = append(errs, fmt.Errorf("rollback %s: %w", e.path, err))
			}
		}
	}
	return errors.Join(errs...)
}

// WriteFile creates parent directories as needed and writes content to path.
// When dryRun is true the file is not written; the path and content are
// printed to stdout instead. When capture mode is active, the call is
// recorded and no IO occurs. When recording mode is active, the path is
// appended to the recorder. When detailed recording mode is active, the
// action (create/update/skip) is determined by comparing against existing
// content; unchanged files are skipped and not rewritten.
func (s *Session) WriteFile(path, content string, dryRun bool) error {
	return s.writeFileWithMode(path, normalizeTrailingNewline(content), filePerm, dryRun)
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

func (s *Session) writeFileWithMode(path, content string, mode os.FileMode, dryRun bool) error {
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
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}

	// Detailed recording: inspect existing content to classify the action.
	if detailing {
		existing, err := os.ReadFile(path)
		var action string
		switch {
		case os.IsNotExist(err):
			action = "create"
		case err == nil && string(existing) == content:
			// File is already up to date; skip the write.
			s.mu.Lock()
			s.detailed = append(s.detailed, WrittenFile{Path: path, Bytes: len(content), Action: "skip"})
			s.mu.Unlock()
			return nil
		default:
			action = "update"
		}
		// Log pre-write state for rollback (only for actual writes, not skips).
		if transacting {
			var pre []byte
			if action == "update" {
				pre = existing
			}
			s.mu.Lock()
			s.txLog = append(s.txLog, txEntry{path: path, content: pre})
			s.mu.Unlock()
		}
		if backup && action == "update" {
			if err := os.WriteFile(path+".bak", existing, filePerm); err != nil {
				return fmt.Errorf("backup %s: %w", path, err)
			}
		}
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		s.mu.Lock()
		s.detailed = append(s.detailed, WrittenFile{Path: path, Bytes: len(content), Action: action})
		s.mu.Unlock()
		return nil
	}

	// Log pre-write state for rollback.
	if transacting {
		pre, readErr := os.ReadFile(path)
		s.mu.Lock()
		switch {
		case readErr == nil:
			s.txLog = append(s.txLog, txEntry{path: path, content: pre})
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
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
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
// provenance marker are user-authored and left untouched.
//
// dryRun prints the intended removal instead of touching disk so
// `sync --dry-run` previews stay side-effect-free. Capture mode is a
// no-op for the same reason WriteFile diverts there. Detailed
// recording logs the removal as a "delete" action so `sync` accounting
// includes the cleaned-up file. Transaction logging captures the
// pre-removal bytes so Rollback can restore the file.
func (s *Session) RemoveGenerated(path string, dryRun bool) error {
	existing, err := os.ReadFile(path)
	if IsAbsent(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if !header.Has(string(existing)) {
		return nil
	}

	s.mu.Lock()
	capturing := s.capturing
	detailing := s.detailing
	transacting := s.transacting
	s.mu.Unlock()

	if capturing {
		return nil
	}
	if dryRun {
		fmt.Printf("--- rm %s ---\n", path)
		return nil
	}

	if transacting {
		s.mu.Lock()
		s.txLog = append(s.txLog, txEntry{path: path, content: existing})
		s.mu.Unlock()
	}

	if err := os.Remove(path); err != nil && !IsAbsent(err) {
		return fmt.Errorf("remove %s: %w", path, err)
	}

	if detailing {
		s.mu.Lock()
		s.detailed = append(s.detailed, WrittenFile{Path: path, Bytes: 0, Action: "delete"})
		s.mu.Unlock()
	}
	return nil
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
	// Remove empty directories bottom-up. Non-empty dirs (user-authored
	// files survived) stay put.
	return removeEmptyDirs(dir)
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
		if err := os.Remove(dirs[i]); err != nil && !IsAbsent(err) {
			return fmt.Errorf("remove %s: %w", dirs[i], err)
		}
	}
	return nil
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
		return s.writeFileWithMode(dst, string(data), fi.Mode().Perm(), dryRun)
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
