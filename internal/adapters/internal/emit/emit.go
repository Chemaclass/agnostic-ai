// Package emit holds shared helpers for adapter packages: file writing,
// frontmatter rendering, capability reporting, and the two common emission
// patterns (single merged document, one-file-per-rule directory).
package emit

import (
	"fmt"
	"io"
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

// WriteFile creates parent directories as needed and writes content to path.
// When dryRun is true the file is not written; the path and content are
// printed to stdout instead. When capture mode is active, the call is
// recorded and no IO occurs. When recording mode is active, the path is
// appended to the recorder.
func WriteFile(path, content string, dryRun bool) error {
	state.mu.Lock()
	capturing := state.capturing
	backup := state.backup
	recording := state.recording
	if capturing {
		state.captured = append(state.captured, CapturedFile{Path: path, Content: content})
	}
	if recording {
		state.recorded = append(state.recorded, path)
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
	if backup {
		if existing, err := os.ReadFile(path); err == nil && string(existing) != content {
			if err := os.WriteFile(path+".bak", existing, filePerm); err != nil {
				return fmt.Errorf("backup %s: %w", path, err)
			}
		}
	}
	if err := os.WriteFile(path, []byte(content), filePerm); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
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
