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

var (
	capturing bool
	captured  []CapturedFile
	backup    bool
)

// SetBackup toggles backup mode. When enabled, WriteFile renames an
// existing file to `<path>.bak` before overwriting. The flag is process
// global; pair every SetBackup(true) with SetBackup(false) once the sync
// pass completes.
func SetBackup(b bool) { backup = b }

// StartCapture redirects subsequent WriteFile calls to an in-memory buffer
// instead of touching disk or stdout. Used by `sync --check` and `doctor`
// to compare what would be emitted against current files.
func StartCapture() {
	capturing = true
	captured = nil
}

// StopCapture returns the captured files and disables capture mode.
func StopCapture() []CapturedFile {
	capturing = false
	out := captured
	captured = nil
	return out
}

// WriteFile creates parent directories as needed and writes content to path.
// When dryRun is true the file is not written; the path and content are
// printed to stdout instead. When capture mode is active, the call is
// recorded and no IO occurs.
func WriteFile(path, content string, dryRun bool) error {
	if capturing {
		captured = append(captured, CapturedFile{Path: path, Content: content})
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
		if existing, err := os.ReadFile(path); err == nil {
			if string(existing) != content {
				if err := os.WriteFile(path+".bak", existing, filePerm); err != nil {
					return fmt.Errorf("backup %s: %w", path, err)
				}
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
