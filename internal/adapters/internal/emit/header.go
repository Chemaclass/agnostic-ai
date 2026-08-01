package emit

import (
	"sync/atomic"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// provenanceEnabled gates whether WithHeader prepends the agnostic-ai
// provenance line to emitted content. Defaults to true. Each adapter's
// Emit() sets the per-run value from `outputs.<target>.provenance-header`
// via SetProvenanceEnabled, then restores the previous value on return.
//
// Adapters run sequentially today (see internal/cli/sync_run.go), so the
// shared atomic does not race. If sync ever fans out adapters in
// parallel, the toggle moves into a per-call argument and this comment
// becomes a load-bearing reminder.
var provenanceEnabled atomic.Bool

func init() { provenanceEnabled.Store(true) }

// SetProvenanceEnabled flips the global toggle and returns the previous
// value so callers can `defer SetProvenanceEnabled(prev)`.
func SetProvenanceEnabled(b bool) bool {
	return provenanceEnabled.Swap(b)
}

// ProvenanceEnabled reports the current toggle state.
func ProvenanceEnabled() bool { return provenanceEnabled.Load() }

// ProvenanceFor reads the per-target `provenance-header` config flag,
// installs the resulting toggle, and returns a restore func suitable
// for `defer`. Idiomatic call inside each adapter's Emit():
//
//	defer emit.ProvenanceFor(cfg, "<target>")()
func ProvenanceFor(cfg *config.Config, target string) func() {
	prev := SetProvenanceEnabled(cfg.ProvenanceHeaderEnabled(target))
	return func() { SetProvenanceEnabled(prev) }
}

// Format is the file-format hint adapters pass to Header / WithHeader.
// Re-exported from internal/adapters/header so adapters keep a single
// emit import while the cli package (which lives outside
// internal/adapters/) can still reuse the same definitions via the
// header package directly.
type Format = header.Format

const (
	FormatMarkdown = header.FormatMarkdown
	FormatTOML     = header.FormatTOML
	FormatYAML     = header.FormatYAML
	FormatShell    = header.FormatShell
	FormatJSON     = header.FormatJSON
)

// Header returns the comment line that marks a generated file in the
// given format, terminated by a newline. Returns "" for FormatJSON and
// for every format when the per-run provenance toggle is off so adapters
// that prepend the line via StringBuilder still honor
// `outputs.<target>.provenance-header: false` without an explicit guard
// at each call site.
func Header(format Format) string {
	if !provenanceEnabled.Load() {
		return ""
	}
	return header.Line(format)
}

// HeaderBlock returns the provenance header followed by a blank line so
// builder-style callers (codex `config.toml`, claude single-file rules)
// can prepend a clean header section in one call. Returns "" when the
// per-run toggle is off so no stray blank line leaks into the emitted
// file.
func HeaderBlock(format Format) string {
	if !provenanceEnabled.Load() {
		return ""
	}
	return header.Line(format) + "\n"
}

// WithHeader prepends Header(format) to content. For Markdown content
// with YAML frontmatter, the header is inserted right after the
// closing delimiter so the frontmatter parser stays valid. Re-exports
// header.With.
//
// Honors SetProvenanceEnabled: when the per-run toggle is false, the
// header is suppressed and content returns unchanged. Used by users
// who want byte-stable round-trip from hand-authored sources.
func WithHeader(content string, format Format) string {
	if !provenanceEnabled.Load() {
		return content
	}
	return header.With(content, format)
}

// StripHeader removes a leading agnostic-ai provenance header from
// content, the read-side counterpart to WithHeader. Re-exports
// header.Strip. Adapters that read back a previously generated file
// (e.g. mirroring .agnostic-ai/AGNOSTIC_AI.md's body into a
// target-native entry-point copy) call this before re-wrapping the
// body in their own WithHeader call, so the two headers never stack.
func StripHeader(content string) string {
	return header.Strip(content)
}
