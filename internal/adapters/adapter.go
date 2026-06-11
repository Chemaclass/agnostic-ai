// Package adapters exposes the per-target Adapter interface and the
// registry mapping target names to implementations.
package adapters

import (
	"errors"
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
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// CapturedFile mirrors emit.CapturedFile so callers outside the internal
// emit tree can consume capture output.
type CapturedFile = emit.CapturedFile

// OrderedJSON mirrors emit.OrderedJSON so the CLI layer can read and
// write settings.json overlays without losing source key order.
type OrderedJSON = emit.OrderedJSON

// NewOrderedJSON returns an empty OrderedJSON ready for Set / Get.
func NewOrderedJSON() *OrderedJSON { return emit.NewOrderedJSON() }

// MarshalJSONIndent renders v as indented JSON without HTML escaping,
// preserving OrderedJSON insertion order when v is an OrderedJSON.
func MarshalJSONIndent(v any) ([]byte, error) { return emit.MarshalJSONIndent(v) }

// MarshalJSONIndentWith is MarshalJSONIndent with a caller-supplied
// indent string ("  ", "    ", "\t"). Used by import-side overlay
// writers that preserve the indent of the captured file.
func MarshalJSONIndentWith(v any, indent string) ([]byte, error) {
	return emit.MarshalJSONIndentWith(v, indent)
}

// DetectJSONIndent re-exports emit.DetectJSONIndent so import-side
// helpers can sniff a captured file's indent before re-emitting it.
func DetectJSONIndent(data []byte) string { return emit.DetectJSONIndent(data) }

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

// ResetCapabilityWarnings clears buffered capability warnings without
// printing. Used by tests and by `sync --watch` before each new pass.
func ResetCapabilityWarnings() { emit.ResetCapabilityWarnings() }

// FlushCapabilityWarnings prints any buffered capability warnings,
// grouped by kind, then clears the buffer. Call once at the end of a
// sync pass.
func FlushCapabilityWarnings() { emit.FlushCapabilityWarnings() }

// CapabilityWarningsDigest returns a stable hex digest of the currently
// buffered capability warnings, "" when none. Used by sync to suppress
// unchanged warning sets across runs.
func CapabilityWarningsDigest() string { return emit.CapabilityWarningsDigest() }

// PendingCapabilityWarningsCount returns the number of distinct
// (target, kind) capability warnings currently buffered.
func PendingCapabilityWarningsCount() int { return emit.PendingCapabilityWarningsCount() }

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

// RemoveGenerated deletes path when it carries the agnostic-ai
// provenance header, leaving user-authored files untouched. Honors
// the same dryRun / capture / detailing / transaction modes as
// WriteFile. Re-exported so the cli package can use it for the
// sync-ledger orphan sweep without crossing the emit internal
// boundary.
func RemoveGenerated(path string, dryRun bool) error {
	return emit.RemoveGenerated(path, dryRun)
}

// StartTransaction begins recording pre-write file state so that Rollback
// can undo all writes if a sync pass fails partway through.
func StartTransaction() { emit.StartTransaction() }

// Commit clears the transaction log after a successful sync.
func Commit() { emit.Commit() }

// Rollback undoes all file writes recorded since StartTransaction.
func Rollback() error { return emit.Rollback() }

// AgnosticEntryPointPath is the canonical CLI-agnostic entry-point
// path under .agnostic-ai/ (re-exported from the emit layer for
// callers in the cli package).
const AgnosticEntryPointPath = emit.AgnosticEntryPointPath

// EntryPointPath returns the project-relative entry-point file for
// target, honoring outputs.<target>.file. Returns "" for targets
// without an entry-point convention.
func EntryPointPath(cfg *config.Config, target string) string {
	return emit.EntryPointPath(cfg, target)
}

// HasLegacyRulesFile reports whether the user opted into the legacy
// concatenated rules-file layout for target.
func HasLegacyRulesFile(cfg *config.Config, target string) bool {
	return emit.HasLegacyRulesFile(cfg, target)
}

// EntryPointBody returns the raw (no header) pointer body for entry-point
// files. Use when the caller needs to prepend its own header or compare
// against existing content.
func EntryPointBody(cfg *config.Config) string {
	return emit.EntryPointBody(cfg)
}

// RenderEntryPoint returns the canonical entry-point content (header
// + pointer body) shared by .agnostic-ai/AGNOSTIC_AI.md and every
// per-target entry-point file.
func RenderEntryPoint(cfg *config.Config) string {
	return emit.RenderEntryPoint(cfg)
}

// NativeArtifact mirrors emit.NativeArtifact so callers outside the
// internal emit tree can consume target-overview data.
type NativeArtifact = emit.NativeArtifact

// TargetArtifacts mirrors emit.TargetArtifacts.
type TargetArtifacts = emit.TargetArtifacts

// OverviewStartMarker mirrors emit.OverviewStartMarker so callers (and
// tests) outside the internal emit tree can detect the appendix block.
const OverviewStartMarker = emit.OverviewStartMarker

// nativeOverviewer is the optional interface an adapter implements to
// describe where its generated artifacts live for a given config. The
// sync layer renders the result into the target-overview appendix of
// the adapter's entry-point file when sync.target-overview is enabled.
type nativeOverviewer interface {
	NativeArtifacts(cfg *config.Config) []NativeArtifact
}

// NativeArtifactsFor returns the native artifacts the named target
// declares, nil when the adapter does not implement nativeOverviewer.
// Lookup is in-tree only: external adapters have no native-artifacts
// protocol yet, so an external target's section is silently absent
// from the overview appendix.
func NativeArtifactsFor(name string, cfg *config.Config) []NativeArtifact {
	a, ok := registry[name]
	if !ok {
		return nil
	}
	o, ok := a.(nativeOverviewer)
	if !ok {
		return nil
	}
	return o.NativeArtifacts(cfg)
}

// RenderTargetOverview renders the sentinel-marked overview appendix
// for one entry-point file (re-exported from the emit layer).
func RenderTargetOverview(sections []TargetArtifacts) string {
	return emit.RenderTargetOverview(sections)
}

// AppendTargetOverview appends a rendered overview to body, stripping
// any pre-existing block first (re-exported from the emit layer).
func AppendTargetOverview(body, overview string) string {
	return emit.AppendTargetOverview(body, overview)
}

// StripTargetOverview removes the sentinel-marked overview block from
// body (re-exported from the emit layer). Import uses it so the
// AGNOSTIC_AI.md round-trip stays lossless.
func StripTargetOverview(body string) string {
	return emit.StripTargetOverview(body)
}

// RulesStartMarker and RulesEndMarker mirror the emit-layer sentinels so
// callers (and tests) outside the internal emit tree can detect and
// extract the rules appendix block.
const (
	RulesStartMarker = emit.RulesStartMarker
	RulesEndMarker   = emit.RulesEndMarker
)

// InlinesRulesIntoEntryPoint reports whether target delivers rule bodies
// by inlining them into its entry-point file (re-exported from the emit
// layer).
func InlinesRulesIntoEntryPoint(target string) bool {
	return emit.InlinesRulesIntoEntryPoint(target)
}

// RenderRulesAppendix renders the sentinel-marked rules block for an
// entry-point file (re-exported from the emit layer).
func RenderRulesAppendix(b spec.Bundle) string {
	return emit.RenderRulesAppendix(b)
}

// AppendRulesAppendix appends a rendered rules block to body, stripping
// any pre-existing block first (re-exported from the emit layer).
func AppendRulesAppendix(body, appendix string) string {
	return emit.AppendRulesAppendix(body, appendix)
}

// StripGeneratedAppendices removes every sentinel-marked block sync may
// append to an entry-point file (re-exported from the emit layer).
func StripGeneratedAppendices(body string) string {
	return emit.StripGeneratedAppendices(body)
}

// Adapter is the contract every target implementation satisfies.
type Adapter interface {
	// Name returns the target identifier used in config and CLI flags.
	Name() string
	// Emit renders the bundle as files for this target. dryRun prints
	// rather than writing.
	Emit(b spec.Bundle, cfg *config.Config, dryRun bool) error
}

// EmitWithProvenance wraps adapter.Emit with the per-target provenance
// header toggle and per-target spec scoping. Filters the bundle through
// `spec.Bundle.For(target)` so adapters only see entries whose
// `target:` / `targets:` / `target-exclude:` / `targets-exclude:`
// frontmatter allows the named target (closes #292). Reads
// `outputs.<target>.provenance-header` from cfg (default true) and
// installs the toggle for the duration of the emit so adapter calls to
// emit.WithHeader honor it. CLI dispatch sites (sync, check, status,
// revert, render, collision) should prefer this wrapper over a bare
// adapter.Emit.
func EmitWithProvenance(a Adapter, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	defer emit.ProvenanceFor(cfg, a.Name())()
	return a.Emit(b.For(a.Name()), cfg, dryRun)
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
		return nil, errs.Coded(errs.CodeSyncTargetUnknown,
			"unknown target: %s (no built-in adapter and no %s%s on PATH)",
			name, external.BinaryPrefix, name)
	}
	return nil, errs.Coded(errs.CodeSyncTargetUnknown,
		"unknown target: %s: %w", name, err)
}

// Names returns every registered target name.
func Names() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	return out
}
