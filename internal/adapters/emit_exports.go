package adapters

import (
	"io"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// CapturedFile mirrors emit.CapturedFile so callers outside the internal
// emit tree can consume capture output.
type CapturedFile = emit.CapturedFile

// CapturedRemoval mirrors emit.CapturedRemoval so `doctor --fix` can
// replay a removal the capture pass recorded.
type CapturedRemoval = emit.CapturedRemoval

// OrderedJSON mirrors emit.OrderedJSON so the CLI layer can read and
// write settings.json overlays without losing source key order.
type OrderedJSON = emit.OrderedJSON

// NewOrderedJSON returns an empty OrderedJSON ready for Set / Get.
func NewOrderedJSON() *OrderedJSON { return emit.NewOrderedJSON() }

// MarshalJSONIndentWith renders v as indented JSON without HTML
// escaping, with a caller-supplied indent string ("  ", "    ", "\t").
// Preserves OrderedJSON insertion order when v is an OrderedJSON. Used
// by import-side overlay writers that preserve the indent of the
// captured file.
func MarshalJSONIndentWith(v any, indent string) ([]byte, error) {
	return emit.MarshalJSONIndentWith(v, indent)
}

// DetectJSONIndent re-exports emit.DetectJSONIndent so import-side
// helpers can sniff a captured file's indent before re-emitting it.
func DetectJSONIndent(data []byte) string { return emit.DetectJSONIndent(data) }

// WholeNumber reads a decoded number that has no fraction, as sync does.
func WholeNumber(v any) (int, bool) { return emit.WholeNumber(v) }

// WrittenFile mirrors emit.WrittenFile so callers outside the internal
// emit tree can consume detailed recording output.
type WrittenFile = emit.WrittenFile

// MergedRelease mirrors emit.MergedRelease: what releasing a merged JSON
// file did with it.
type MergedRelease = emit.MergedRelease

const (
	MergedKept      = emit.MergedKept
	MergedUnchanged = emit.MergedUnchanged
	MergedStripped  = emit.MergedStripped
	MergedRemoved   = emit.MergedRemoved
	MergedEdited    = emit.MergedEdited
)

// MergedKey mirrors emit.MergedKey: one value sync set in a JSON or
// YAML file it merges into.
type MergedKey = emit.MergedKey

// ParsesMerged mirrors emit.ParsesMerged.
func ParsesMerged(path string) bool { return emit.ParsesMerged(path) }

// SetPriorMergedKeys tells the merge writers where to find the values
// the last sync recorded for a merged file.
func SetPriorMergedKeys(prior func(path string) []MergedKey) { emit.PriorMergedKeys = prior }

// SetPriorMergedCreated tells the merge writers whether the last sync
// created a merged file, rather than merging into one that was there.
func SetPriorMergedCreated(prior func(path string) bool) { emit.PriorMergedCreated = prior }

// SetPriorMergedRecorded tells the merge writers whether the last sync
// recorded a file as a merged write.
func SetPriorMergedRecorded(prior func(path string) bool) { emit.PriorMergedRecorded = prior }

// Session mirrors emit.Session so the cli and cmd layers — which cannot
// import the internal emit tree — can construct one emission session per
// sync run and drive its capture / recording / backup / transaction
// modes and file writes directly through the exported methods.
type Session = emit.Session

// NewSession returns a fresh emission Session with every mode off. Each
// sync run constructs its own so two runs in the same process never
// share capture or recording buffers.
func NewSession() *Session { return emit.NewSession() }

// ContentSum mirrors emit.ContentSum: the fingerprint the sync ledger
// records for every output.
func ContentSum(content string) string { return emit.ContentSum(content) }

// MatchesSum mirrors emit.MatchesSum: data holds the content of sum, with
// LF or CRLF line endings.
func MatchesSum(data []byte, sum string) bool { return emit.MatchesSum(data, sum) }

// SetWarner redirects capability warnings emitted by adapters. The CLI uses
// this to suppress warnings under --quiet.
func SetWarner(w io.Writer) { emit.Warner = w }

// Warner returns the writer capability warnings go to, so a caller that
// silences them can put it back.
func Warner() io.Writer { return emit.Warner }

// ResetCapabilityWarnings clears buffered capability warnings without
// printing. Used by tests and by `sync --watch` before each new pass.
func ResetCapabilityWarnings() { emit.ResetCapabilityWarnings() }

// ResolveMeta re-exports emit.ResolveMeta: a spec's meta as target sees
// it, with its `x-<target>` keys flattened on top.
func ResolveMeta(meta map[string]any, target string) map[string]any {
	return emit.ResolveMeta(meta, target)
}

// RewriteHookPath re-exports emit.RewriteHookPath, which points a hook
// command at target's own hooks directory.
func RewriteHookPath(cmd, target string, metadata ...map[string]any) string {
	return emit.RewriteHookPath(cmd, target, metadata...)
}

func RewriteGlobalWindowsHookPath(command, scriptsDir string) string {
	return emit.RewriteGlobalWindowsHookPath(command, scriptsDir)
}

func RewriteWindowsNeutralHookPath(command, scriptsDir string) string {
	return emit.RewriteWindowsNeutralHookPath(command, scriptsDir)
}

func RewriteGlobalHookPath(command, target, scriptsDir string, metadata ...map[string]any) string {
	return emit.RewriteGlobalHookPath(command, target, scriptsDir, metadata...)
}

type HookScript = emit.HookScript

func NeutralHookScripts(command, target, sourceDir, outputDir string, literal ...bool) ([]HookScript, error) {
	return emit.NeutralHookScripts(command, target, sourceDir, outputDir, literal...)
}

func NeutralHookScriptNames(command string, literal bool) []string {
	return emit.NeutralHookScriptNames(command, literal)
}

func NeutralHookScriptsFor(hook spec.Entry, command, target, sourceDir, outputDir string, literal ...bool) ([]HookScript, error) {
	return emit.NeutralHookScriptsFor(hook, command, target, sourceDir, outputDir, literal...)
}

func RewriteGlobalHookRoot(command, target string, metadata ...map[string]any) string {
	return emit.RewriteGlobalHookRoot(command, target, metadata...)
}

func ReportHookProjectRoot(target string, hooks []spec.Entry, mode string, global bool) error {
	return emit.ReportHookProjectRoot(target, hooks, mode, global)
}

// FlushCapabilityWarnings prints any buffered capability warnings,
// grouped by kind, then clears the buffer. Call once at the end of a
// sync pass.
func FlushCapabilityWarnings() { emit.FlushCapabilityWarnings() }

// RenderDroppedSummary writes a per-target summary of buffered capability
// warnings and coverage notes (what each target could not fully emit)
// without clearing either buffer. Call before the flushes.
func RenderDroppedSummary(w io.Writer) { emit.RenderDroppedSummary(w) }

// CapabilityWarningsDigest returns a stable hex digest of the currently
// buffered capability warnings, "" when none. Used by sync to suppress
// unchanged warning sets across runs.
func CapabilityWarningsDigest() string { return emit.CapabilityWarningsDigest() }

// PendingCapabilityWarningsCount returns the number of distinct
// (target, kind) capability warnings currently buffered.
func PendingCapabilityWarningsCount() int { return emit.PendingCapabilityWarningsCount() }

// KnownOverlay is one path under `.agnostic-ai/overlays/` a target reads.
type KnownOverlay = emit.KnownOverlay

// KnownOverlays returns every overlay some target reads at sync.
func KnownOverlays() []KnownOverlay { return emit.KnownOverlays() }

// NoteProject buffers a project-wide note that belongs to no single
// target, so it flushes and sticky-suppresses with the coverage notes.
func NoteProject(text string) { emit.NoteProject(text) }

// ClaudeModel reports whether model is a Claude Code model name.
func ClaudeModel(model string) bool { return emit.ClaudeModel(model) }

// FlowScalar quotes a model value for a flow YAML hint (re-exported from
// the emit layer).
func FlowScalar(s string) string { return emit.FlowScalar(s) }

// ModelAliasIn names the vendor model alias target resolves from meta, or
// "" for an exact id (re-exported from the emit layer).
func ModelAliasIn(meta map[string]any, target string) string { return emit.ModelAliasIn(meta, target) }

// ModelAlias returns the id alias names for target, and whether it is one
// (re-exported from the emit layer).
func ModelAlias(target, alias string) (string, bool) { return emit.ModelAlias(target, alias) }

// TakeResolvedAliases re-exports emit.TakeResolvedAliases: the vendor
// model aliases resolved since the last call, by target.
func TakeResolvedAliases() map[string]map[string]string { return emit.TakeResolvedAliases() }

// ForeignClaudeModel reports whether model is a Claude model name that
// target's agent `model` key cannot load.
func ForeignClaudeModel(target, model string) bool {
	a, ok := registry[target].(interface{ ForeignClaudeModels() []string })
	return ok && emit.ForeignClaudeModel(a.ForeignClaudeModels(), model)
}

// SharedModel returns the model meta gives target through a value every
// target shares: a string `model`, or the map's `default`.
func SharedModel(meta map[string]any, target string) string { return emit.SharedModel(meta, target) }

// Note is one buffered capability warning or coverage note in structured
// form (re-exported from the emit layer).
type Note = emit.Note

// Note shapes (re-exported from the emit layer).
const (
	NoteUnsupportedKind = emit.NoteUnsupportedKind
	NoteGap             = emit.NoteGap
	NoteField           = emit.NoteField
	NoteSurface         = emit.NoteSurface
)

// SetAsideNotes silences capability warnings and stashes the buffered
// ones until the returned func runs (re-exported from the emit layer).
func SetAsideNotes() (restore func()) { return emit.SetAsideNotes() }

// SkipNotes stops recording coverage notes until the returned func runs
// (re-exported from the emit layer).
func SkipNotes() (restore func()) { return emit.SkipNotes() }

// DrainNotes returns and clears every buffered capability warning and
// coverage note, for callers that attribute them to one spec.
func DrainNotes() []Note { return emit.DrainNotes() }

// ResetCoverageNotes clears buffered coverage notes without printing.
// Used by tests and by `sync --watch` before each new pass.
func ResetCoverageNotes() { emit.ResetCoverageNotes() }

// DiscardCoverageNotes clears buffered coverage notes without printing
// and keeps the record of entries each target left out.
func DiscardCoverageNotes() { emit.DiscardCoverageNotes() }

// OmittedEntry reports whether target's output left out the spec of
// kind named name in the current sync pass, with a coverage note saying
// why.
func OmittedEntry(target string, kind spec.Kind, name string) bool {
	return emit.OmittedEntry(target, kind, name)
}

// NoteEntryOmitted records that target's output leaves out the spec of
// kind named name (re-exported from the emit layer). Call it next to
// the coverage note that says why.
func NoteEntryOmitted(target string, kind spec.Kind, name string) {
	emit.NoteEntryOmitted(target, kind, name)
}

// FlushCoverageNotes prints any buffered coverage notes, grouped by
// kind, then clears the buffer. Call once at the end of a sync pass.
func FlushCoverageNotes() { emit.FlushCoverageNotes() }

// CoverageNotesDigest returns a stable hex digest of the currently
// buffered coverage notes, "" when none. Used by sync to suppress
// unchanged note sets across runs.
func CoverageNotesDigest() string { return emit.CoverageNotesDigest() }

// PendingCoverageNotesCount returns the number of distinct
// (target, kind, via) coverage notes currently buffered.
func PendingCoverageNotesCount() int { return emit.PendingCoverageNotesCount() }

// AcceptedNote is a coverage note a coverage.accept entry matched
// (re-exported from the emit layer).
type AcceptedNote = emit.AcceptedNote

// AcceptCoverageNotes takes the buffered coverage notes accept matches
// out of the buffers and returns them, with the entries that matched
// none.
func AcceptCoverageNotes(accept []config.CoverageAccept) ([]AcceptedNote, []config.CoverageAccept) {
	return emit.AcceptCoverageNotes(accept)
}

// PrintAcceptedNotes prints each accepted note with its reason.
func PrintAcceptedNotes(accepted []AcceptedNote) { emit.PrintAcceptedNotes(accepted) }

// PendingTargetCoverageNotesCount returns the number of distinct
// buffered coverage notes that name a target.
func PendingTargetCoverageNotesCount() int { return emit.PendingTargetCoverageNotesCount() }

// MCPLaunchField is the MCP launch field sync writes for the server's
// transport: `args` for stdio, `url` for a remote server.
func MCPLaunchField(meta map[string]any) string { return emit.MCPLaunchField(meta) }

// MCPLaunchRefSyntax returns the reference form target expands in an
// MCP `url` or `args` value, EnvRefNone when it documents none.
func MCPLaunchRefSyntax(target, field string) spec.EnvRefSyntax {
	return emit.MCPLaunchRefSyntax(target, field)
}

// DropRecord is one buffered capability warning or coverage note for one
// target (re-exported from the emit layer).
type DropRecord = emit.DropRecord

// PendingCapabilityWarnings returns the buffered capability warnings,
// one per target and kind, without clearing them.
func PendingCapabilityWarnings() []DropRecord { return emit.PendingCapabilityWarnings() }

// PendingCoverageNotes returns the buffered coverage notes, one per
// target and note, without clearing them.
func PendingCoverageNotes() []DropRecord { return emit.PendingCoverageNotes() }

// DropRecordFingerprint identifies the full diagnostic and its fix.
func DropRecordFingerprint(r DropRecord) string { return emit.DropRecordFingerprint(r) }

// SuppressKnownCapabilityWarnings removes previously reported warnings.
func SuppressKnownCapabilityWarnings(known map[string]bool) int {
	return emit.SuppressKnownCapabilityWarnings(known)
}

// SuppressKnownCoverageNotes removes previously reported notes.
func SuppressKnownCoverageNotes(known map[string]bool) int {
	return emit.SuppressKnownCoverageNotes(known)
}

// OrderBufferedDropsByTarget reorders the buffered capability warnings and
// coverage notes to the given target sequence so their flushed output is
// deterministic regardless of the order concurrent emission appended them
// in (sync --jobs > 1). A no-op for serial emission.
func OrderBufferedDropsByTarget(order []string) { emit.OrderBufferedDropsByTarget(order) }

// SetProvenanceEnabled overrides the process-wide provenance-header toggle
// and returns the previous value. The parallel sync path pins it per
// provenance-homogeneous batch so concurrently emitting adapters never
// observe a half-applied toggle; see internal/adapters/internal/emit/header.go.
func SetProvenanceEnabled(b bool) bool { return emit.SetProvenanceEnabled(b) }

// ProvenanceEnabled reports the current provenance-header toggle state.
func ProvenanceEnabled() bool { return emit.ProvenanceEnabled() }

// PortableHookCommand returns one command of hook h on target, run
// through the portable hook wrapper when h needs it.
func PortableHookCommand(h spec.Entry, target, inner string, rewrite func(string) string) string {
	return emit.PortableHookCommand(h, target, inner, rewrite)
}

// PortableHookWrapperScript is the wrapper file for target in dir, or
// false when no hook in hooks needs it.
func PortableHookWrapperScript(hooks []spec.Entry, target, dir string) (HookScript, bool) {
	return emit.PortableHookWrapperScript(hooks, target, dir)
}

// EntrySourcePath is the provenance path sync stamps for e: its spec path,
// or `builtin:<name>` for a built-in (re-exported from the emit layer).
func EntrySourcePath(e spec.Entry) string {
	return emit.EntrySourcePath(e)
}

// SetPriorOutputSum tells the merge writers where to find the sum the
// last sync recorded for a file it wrote whole.
func SetPriorOutputSum(prior func(path string) string) { emit.PriorOutputSum = prior }
