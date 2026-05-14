// Package emit holds shared helpers for adapter packages: file writing,
// frontmatter rendering, capability reporting, and the two common emission
// patterns (single merged document, one-file-per-rule directory).
package emit

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
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
	mu        sync.Mutex
	capturing bool
	captured  []CapturedFile
	backup    bool
	recording bool
	recorded  []string
	counting  bool
	counted   int
	detailing bool
	detailed  []WrittenFile
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

// WriteFile creates parent directories as needed and writes content to path.
// When dryRun is true the file is not written; the path and content are
// printed to stdout instead. When capture mode is active, the call is
// recorded and no IO occurs. When recording mode is active, the path is
// appended to the recorder. When detailed recording mode is active, the
// action (create/update/skip) is determined by comparing against existing
// content; unchanged files are skipped and not rewritten.
func WriteFile(path, content string, dryRun bool) error {
	return writeFileWithMode(path, content, filePerm, dryRun)
}

func writeFileWithMode(path, content string, mode os.FileMode, dryRun bool) error {
	state.mu.Lock()
	capturing := state.capturing
	backup := state.backup
	recording := state.recording
	detailing := state.detailing
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
// an empty string.
func Frontmatter(meta map[string]any) string {
	if len(meta) == 0 {
		return ""
	}
	data, err := yaml.Marshal(meta)
	if err != nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.Write(data)
	b.WriteString("---\n")
	return b.String()
}

// Warner accepts capability warnings. Defaults to writing to os.Stderr.
// Tests inject a buffer to assert messages.
var Warner io.Writer = os.Stderr
