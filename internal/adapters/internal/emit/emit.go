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
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
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

// state holds the package-global mode flags. Adapters and the CLI mutate
// state through SetBackup, StartCapture, StopCapture, and StartRecording;
// every read in WriteFile takes the same mutex so go test -race stays
// clean and library reuse from concurrent goroutines is safe.
var state struct {
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

// SetBackup toggles backup mode. When enabled, WriteFile copies an
// existing file to `<path>.bak` before overwriting (only when the new
// content differs). Pair SetBackup(true) with SetBackup(false) once the
// sync pass completes.
func SetBackup(b bool) {
	state.mu.Lock()
	state.backup = b
	state.mu.Unlock()
}

// StartCapture redirects subsequent WriteFile calls to an in-memory buffer
// instead of touching disk or stdout. Used by `sync --check`, `doctor`,
// and `revert` to inspect what each adapter would emit.
func StartCapture() {
	state.mu.Lock()
	state.capturing = true
	state.captured = nil
	state.mu.Unlock()
}

// IsCapturing reports whether capture mode is active. Adapters that
// perform side-effects beyond `WriteFile` (e.g. one-shot file renames
// for migrations) should consult it and no-op when true, so dry-check
// modes do not mutate the working tree.
func IsCapturing() bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.capturing
}

// StopCapture returns the captured files and disables capture mode.
func StopCapture() []CapturedFile {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.capturing = false
	out := state.captured
	state.captured = nil
	return out
}

// StartRecording begins collecting written paths alongside real writes.
// Unlike capture mode this does not suppress IO. Used by `sync` to learn
// every emitted path in a single pass for follow-up actions like
// .gitignore management.
func StartRecording() {
	state.mu.Lock()
	state.recording = true
	state.recorded = nil
	state.mu.Unlock()
}

// StopRecording returns the recorded paths and disables recording.
func StopRecording() []string {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.recording = false
	out := state.recorded
	state.recorded = nil
	return out
}

// StartCounting begins tracking the number of files written to disk.
// Does not affect IO.
func StartCounting() {
	state.mu.Lock()
	state.counting = true
	state.counted = 0
	state.mu.Unlock()
}

// StopCounting returns the count of files written since StartCounting
// and disables counting mode.
func StopCounting() int {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.counting = false
	n := state.counted
	return n
}

// StartDetailedRecording begins collecting per-file write results alongside
// real writes. Unlike capture mode this does not suppress IO. Unlike counting
// and recording modes it also determines the action ("create", "update", or
// "skip") by comparing the new content against what is on disk before writing.
// Files whose content is unchanged are not rewritten and are recorded with
// action "skip".
func StartDetailedRecording() {
	state.mu.Lock()
	state.detailing = true
	state.detailed = nil
	state.mu.Unlock()
}

// StopDetailedRecording returns the collected write records and disables
// detailed recording mode.
func StopDetailedRecording() []WrittenFile {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.detailing = false
	out := state.detailed
	state.detailed = nil
	return out
}

// StartTransaction begins recording pre-write file state so that Rollback
// can undo all writes if a sync pass fails partway through. Commit clears
// the log on success.
func StartTransaction() {
	state.mu.Lock()
	state.transacting = true
	state.txLog = nil
	state.mu.Unlock()
}

// Commit clears the transaction log and disables transaction mode. Call
// after a successful sync to release the log.
func Commit() {
	state.mu.Lock()
	state.transacting = false
	state.txLog = nil
	state.mu.Unlock()
}

// Rollback undoes all file writes recorded since StartTransaction. New files
// are removed; overwritten files are restored from their pre-write content.
// All entries are attempted; errors are joined and returned.
func Rollback() error {
	state.mu.Lock()
	log := state.txLog
	state.txLog = nil
	state.transacting = false
	state.mu.Unlock()

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
func WriteFile(path, content string, dryRun bool) error {
	return writeFileWithMode(path, normalizeTrailingNewline(content), filePerm, dryRun)
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

func writeFileWithMode(path, content string, mode os.FileMode, dryRun bool) error {
	state.mu.Lock()
	capturing := state.capturing
	backup := state.backup
	recording := state.recording
	detailing := state.detailing
	transacting := state.transacting
	if capturing {
		state.captured = append(state.captured, CapturedFile{Path: path, Content: content})
	}
	if recording {
		state.recorded = append(state.recorded, path)
	}
	if state.counting && !capturing && !dryRun {
		state.counted++
	}
	state.mu.Unlock()

	if capturing {
		return nil
	}
	if dryRun {
		fmt.Printf("--- %s ---\n%s\n", path, content)
		return nil
	}
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
			state.mu.Lock()
			state.detailed = append(state.detailed, WrittenFile{Path: path, Bytes: len(content), Action: "skip"})
			state.mu.Unlock()
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
			state.mu.Lock()
			state.txLog = append(state.txLog, txEntry{path: path, content: pre})
			state.mu.Unlock()
		}
		if backup && action == "update" {
			if err := os.WriteFile(path+".bak", existing, filePerm); err != nil {
				return fmt.Errorf("backup %s: %w", path, err)
			}
		}
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		state.mu.Lock()
		state.detailed = append(state.detailed, WrittenFile{Path: path, Bytes: len(content), Action: action})
		state.mu.Unlock()
		return nil
	}

	// Log pre-write state for rollback.
	if transacting {
		pre, readErr := os.ReadFile(path)
		state.mu.Lock()
		switch {
		case readErr == nil:
			state.txLog = append(state.txLog, txEntry{path: path, content: pre})
		case os.IsNotExist(readErr):
			state.txLog = append(state.txLog, txEntry{path: path, content: nil})
		}
		// Other read errors: skip logging; the write below will also fail.
		state.mu.Unlock()
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
func CopyTree(srcDir, dstDir string, skip func(rel string) bool, dryRun bool) error {
	info, err := os.Stat(srcDir)
	if err != nil {
		if os.IsNotExist(err) {
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
		return writeFileWithMode(dst, string(data), fi.Mode().Perm(), dryRun)
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
// Co-fixes three round-trip noise sources versus the legacy
// `yaml.Marshal(map)` path:
//
//   - Source key order is preserved (frontmatter authored with
//     `name` first stays with `name` first instead of being sorted
//     to the bottom by the YAML library).
//   - Sequence indent is forced to 2 spaces (yaml.v3 default is 4).
//   - Single-quoted scalars are promoted to double quotes, matching
//     the convention used by hand-authored CLI configs.
func FrontmatterOrdered(meta map[string]any, keys []string) string {
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
// scalars to double-quoted. yaml.v3's default style for a scalar that
// needs quoting (e.g. one starting with `[`) is single quotes; CLI
// configs typically author them as double quotes, so the round-trip
// diff stays clean.
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
