// Package adapters exposes the per-target Adapter interface and the
// registry mapping target names to implementations.
package adapters

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/aider"
	"github.com/chemaclass/agnostic-ai/internal/adapters/amp"
	"github.com/chemaclass/agnostic-ai/internal/adapters/claude"
	"github.com/chemaclass/agnostic-ai/internal/adapters/cline"
	"github.com/chemaclass/agnostic-ai/internal/adapters/codex"
	"github.com/chemaclass/agnostic-ai/internal/adapters/continueai"
	"github.com/chemaclass/agnostic-ai/internal/adapters/copilot"
	"github.com/chemaclass/agnostic-ai/internal/adapters/cursor"
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

// StartCapture redirects subsequent adapter writes to an in-memory buffer.
// Pair with StopCapture. Used by drift detection (`sync --check`, `doctor`).
func StartCapture() { emit.StartCapture() }

// StopCapture returns the captured files and disables capture mode.
func StopCapture() []CapturedFile { return emit.StopCapture() }

// SetBackup toggles backup mode on the shared emit layer. When enabled,
// adapter writes copy any pre-existing target file to `<path>.bak` before
// overwriting. Used by `sync --backup` to leave a recovery trail that
// `revert` can restore from.
func SetBackup(b bool) { emit.SetBackup(b) }

// Adapter is the contract every target implementation satisfies.
type Adapter interface {
	// Name returns the target identifier used in config and CLI flags.
	Name() string
	// Emit renders the bundle as files for this target. dryRun prints
	// rather than writing.
	Emit(b spec.Bundle, cfg *config.Config, dryRun bool) error
}

var registry = map[string]Adapter{
	"claude":   claude.New(),
	"codex":    codex.New(),
	"gemini":   gemini.New(),
	"cursor":   cursor.New(),
	"copilot":  copilot.New(),
	"aider":    aider.New(),
	"cline":    cline.New(),
	"windsurf": windsurf.New(),
	"continue": continueai.New(),
	"amp":      amp.New(),
	"zed":      zed.New(),
	"warp":     warp.New(),
	"opencode": opencode.New(),
}

// Get returns the adapter registered under name.
func Get(name string) (Adapter, bool) {
	a, ok := registry[name]
	return a, ok
}

// Names returns every registered target name.
func Names() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	return out
}
