// Package adapters exposes the per-target Adapter interface and the
// registry mapping target names to implementations.
package adapters

import (
	"errors"
	"fmt"
	"io"
	"os/exec"

	"github.com/chemaclass/agnostic-ai/internal/adapters/aider"
	"github.com/chemaclass/agnostic-ai/internal/adapters/amp"
	"github.com/chemaclass/agnostic-ai/internal/adapters/antigravity"
	"github.com/chemaclass/agnostic-ai/internal/adapters/claude"
	"github.com/chemaclass/agnostic-ai/internal/adapters/cline"
	"github.com/chemaclass/agnostic-ai/internal/adapters/codex"
	"github.com/chemaclass/agnostic-ai/internal/adapters/continueai"
	"github.com/chemaclass/agnostic-ai/internal/adapters/copilot"
	"github.com/chemaclass/agnostic-ai/internal/adapters/cursor"
	"github.com/chemaclass/agnostic-ai/internal/adapters/external"
	"github.com/chemaclass/agnostic-ai/internal/adapters/gemini"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/adapters/opencode"
	"github.com/chemaclass/agnostic-ai/internal/adapters/warp"
	"github.com/chemaclass/agnostic-ai/internal/adapters/windsurf"
	"github.com/chemaclass/agnostic-ai/internal/adapters/zed"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// CapturedFile mirrors emit.CapturedFile so callers outside the internal
// emit tree can consume capture output.
type CapturedFile = emit.CapturedFile

// WrittenFile mirrors emit.WrittenFile so callers outside the internal
// emit tree can consume detailed recording output.
type WrittenFile = emit.WrittenFile

// StartCapture redirects subsequent adapter writes to an in-memory buffer.
// Pair with StopCapture. Used by drift detection (`sync --check`, `doctor`)
// and `revert` to inspect what each adapter would emit without touching disk.
func StartCapture() { emit.StartCapture() }

// StopCapture returns the captured files and disables capture mode.
func StopCapture() []CapturedFile { return emit.StopCapture() }

// SetBackup toggles backup mode on the shared emit layer. When enabled,
// adapter writes copy any pre-existing target file to `<path>.bak` before
// overwriting. Used by `sync --backup` to leave a recovery trail that
// `revert` can restore from.
func SetBackup(b bool) { emit.SetBackup(b) }

// SetWarner redirects capability warnings emitted by adapters. The CLI uses
// this to suppress warnings under --quiet.
func SetWarner(w io.Writer) { emit.Warner = w }

// StartRecording begins collecting written paths alongside real writes.
// Unlike capture mode it does not suppress IO. Used by `sync` to learn
// every emitted path in a single pass for follow-up actions like
// .gitignore management.
func StartRecording() { emit.StartRecording() }

// StopRecording returns the recorded paths and disables recording.
func StopRecording() []string { return emit.StopRecording() }

// StartCounting begins tracking the number of files written to disk.
// Does not affect IO. Used by sync to record files_changed in the state file.
func StartCounting() { emit.StartCounting() }

// StopCounting returns the count of files written since StartCounting.
func StopCounting() int { return emit.StopCounting() }

// StartDetailedRecording begins collecting per-file write results alongside
// real writes. Determines each file's action (create/update/skip) by
// comparing new content against the existing file before writing.
func StartDetailedRecording() { emit.StartDetailedRecording() }

// StopDetailedRecording returns the collected write records and disables
// detailed recording mode.
func StopDetailedRecording() []WrittenFile { return emit.StopDetailedRecording() }

// WriteFile writes content to path through the shared emit layer,
// honoring the current capture, recording, and backup modes.
func WriteFile(path, content string, dryRun bool) error {
	return emit.WriteFile(path, content, dryRun)
}

// Adapter is the contract every target implementation satisfies.
type Adapter interface {
	// Name returns the target identifier used in config and CLI flags.
	Name() string
	// Emit renders the bundle as files for this target. dryRun prints
	// rather than writing.
	Emit(b spec.Bundle, cfg *config.Config, dryRun bool) error
}

var registry = map[string]Adapter{
	"claude":      claude.New(),
	"codex":       codex.New(),
	"gemini":      gemini.New(),
	"cursor":      cursor.New(),
	"copilot":     copilot.New(),
	"aider":       aider.New(),
	"cline":       cline.New(),
	"windsurf":    windsurf.New(),
	"continue":    continueai.New(),
	"amp":         amp.New(),
	"zed":         zed.New(),
	"warp":        warp.New(),
	"opencode":    opencode.New(),
	"antigravity": antigravity.New(),
}

// Get returns the adapter registered under name. Lookup is restricted to
// the in-tree registry; for the full lookup including external adapters
// discovered on PATH, use Resolve.
func Get(name string) (Adapter, bool) {
	a, ok := registry[name]
	return a, ok
}

// Resolve returns the adapter for name. The in-tree registry is checked
// first; if no built-in matches, an external adapter named
// `agnostic-ai-adapter-<name>` is looked up on PATH. Callers should use
// Resolve rather than Get so opt-in external targets work.
func Resolve(name string) (Adapter, error) {
	if a, ok := registry[name]; ok {
		return a, nil
	}
	a, err := external.New(name)
	if err == nil {
		return a, nil
	}
	if errors.Is(err, exec.ErrNotFound) {
		return nil, fmt.Errorf("unknown target: %s (no built-in adapter and no %s%s on PATH)", name, external.BinaryPrefix, name)
	}
	return nil, fmt.Errorf("unknown target: %s: %w", name, err)
}

// Names returns every registered target name.
func Names() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	return out
}
