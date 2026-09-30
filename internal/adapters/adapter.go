// Package adapters exposes the per-target Adapter interface and the
// registry mapping target names to implementations.
package adapters

import (
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"

	"github.com/chemaclass/agnostic-ai/internal/adapters/aider"
	"github.com/chemaclass/agnostic-ai/internal/adapters/amp"
	"github.com/chemaclass/agnostic-ai/internal/adapters/antigravity"
	"github.com/chemaclass/agnostic-ai/internal/adapters/augment"
	"github.com/chemaclass/agnostic-ai/internal/adapters/claude"
	"github.com/chemaclass/agnostic-ai/internal/adapters/cline"
	"github.com/chemaclass/agnostic-ai/internal/adapters/codex"
	"github.com/chemaclass/agnostic-ai/internal/adapters/continueai"
	"github.com/chemaclass/agnostic-ai/internal/adapters/copilot"
	"github.com/chemaclass/agnostic-ai/internal/adapters/crush"
	"github.com/chemaclass/agnostic-ai/internal/adapters/cursor"
	"github.com/chemaclass/agnostic-ai/internal/adapters/external"
	"github.com/chemaclass/agnostic-ai/internal/adapters/factory"
	"github.com/chemaclass/agnostic-ai/internal/adapters/gemini"
	"github.com/chemaclass/agnostic-ai/internal/adapters/goose"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/adapters/jules"
	"github.com/chemaclass/agnostic-ai/internal/adapters/junie"
	"github.com/chemaclass/agnostic-ai/internal/adapters/kilo"
	"github.com/chemaclass/agnostic-ai/internal/adapters/kiro"
	"github.com/chemaclass/agnostic-ai/internal/adapters/opencode"
	"github.com/chemaclass/agnostic-ai/internal/adapters/openhands"
	"github.com/chemaclass/agnostic-ai/internal/adapters/qoder"
	"github.com/chemaclass/agnostic-ai/internal/adapters/trae"
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

// WrittenFile mirrors emit.WrittenFile so callers outside the internal
// emit tree can consume detailed recording output.
type WrittenFile = emit.WrittenFile

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

// SetWarner redirects capability warnings emitted by adapters. The CLI uses
// this to suppress warnings under --quiet.
func SetWarner(w io.Writer) { emit.Warner = w }

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
func RewriteHookPath(cmd, target string) string { return emit.RewriteHookPath(cmd, target) }

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

// NoteProject buffers a project-wide note that belongs to no single
// target, so it flushes and sticky-suppresses with the coverage notes.
func NoteProject(text string) { emit.NoteProject(text) }

// ClaudeModel reports whether model is a Claude Code model name.
func ClaudeModel(model string) bool { return emit.ClaudeModel(model) }

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

// DrainNotes returns and clears every buffered capability warning and
// coverage note, for callers that attribute them to one spec.
func DrainNotes() []Note { return emit.DrainNotes() }

// ResetCoverageNotes clears buffered coverage notes without printing.
// Used by tests and by `sync --watch` before each new pass.
func ResetCoverageNotes() { emit.ResetCoverageNotes() }

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

// AgnosticEntryPointPath is the canonical CLI-agnostic entry-point
// path under .agnostic-ai/ (re-exported from the emit layer for
// callers in the cli package).
const AgnosticEntryPointPath = emit.AgnosticEntryPointPath

// SharedInstructionsMirror returns the file an adapter writes from the
// AGNOSTIC_AI.md body itself, outside sync's central entry points, or ""
// when target writes none. Junie's .junie/AGENTS.md is the only one.
func SharedInstructionsMirror(target string) string {
	if target == "junie" {
		return junie.EntryFile
	}
	return ""
}

// ProjectLocalEntryPointPath is the ignored project-local instructions
// file that extends AgnosticEntryPointPath in every entry point
// (re-exported from the emit layer).
const ProjectLocalEntryPointPath = emit.ProjectLocalEntryPointPath

// EntryPointPath returns the project-relative entry-point file for
// target, honoring outputs.<target>.file. Returns "" for targets
// without an entry-point convention.
func EntryPointPath(cfg *config.Config, target string) string {
	return emit.EntryPointPath(cfg, target)
}

// ConventionalEntryPointPaths returns the distinct conventional root
// entry-point files across every known target, sorted. Used by import to
// detect a sibling entry-point that sync would overwrite.
func ConventionalEntryPointPaths() []string {
	return emit.ConventionalEntryPointPaths()
}

// HasLegacyRulesFile reports whether the user opted into the legacy
// concatenated rules-file layout for target.
func HasLegacyRulesFile(cfg *config.Config, target string) bool {
	return emit.HasLegacyRulesFile(cfg, target)
}

// LegacyRulesFileOwnsEntryPoint reports whether target's legacy
// concatenated rules-file is the target's own entry-point file, in
// which case the adapter owns that write and sync skips the pointer
// body (re-exported from the emit layer).
func LegacyRulesFileOwnsEntryPoint(cfg *config.Config, target string) bool {
	return emit.LegacyRulesFileOwnsEntryPoint(cfg, target)
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

// alwaysOnRuler is the optional interface an adapter implements when it
// writes rules to files its tool loads by itself. AlwaysOnRule reports
// whether the tool loads rule r in every session, before any file
// matches, and comes from the code that renders the rule's activation.
type alwaysOnRuler interface {
	AlwaysOnRule(r spec.Entry) bool
}

// AlwaysOnRule reports whether the named target loads rule r from its
// own rule files in every session. It is false for a target that
// writes no rule files of its own, such as one that only inlines rules
// into its entry point. Lookup is in-tree only.
func AlwaysOnRule(name string, r spec.Entry) bool {
	a, ok := registry[name].(alwaysOnRuler)
	return ok && a.AlwaysOnRule(r)
}

// gitignoreHinter is the optional interface an adapter implements to
// declare local artifacts the tool creates but agnostic-ai never emits
// (e.g. a memory store, a per-user local settings file). The sync layer
// folds the result into the managed `.gitignore` block so those paths
// stay ignored without hand maintenance.
type gitignoreHinter interface {
	GitignoreHints(cfg *config.Config) []string
}

// GitignoreHintsFor returns the ignore-only paths the named target
// declares, nil when the adapter does not implement gitignoreHinter.
// Lookup is in-tree only: external adapters have no hint protocol yet.
func GitignoreHintsFor(name string, cfg *config.Config) []string {
	a, ok := registry[name]
	if !ok {
		return nil
	}
	h, ok := a.(gitignoreHinter)
	if !ok {
		return nil
	}
	return h.GitignoreHints(cfg)
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

// RulesStartMarker and RulesEndMarker mirror the emit-layer sentinels so
// callers (and tests) outside the internal emit tree can detect and
// extract the rules appendix block.
const (
	RulesStartMarker = emit.RulesStartMarker
	RulesEndMarker   = emit.RulesEndMarker
)

// AgentsStartMarker and AgentsEndMarker mirror the emit-layer sentinels
// so callers outside the internal emit tree (the junie importer today)
// can detect and extract the agents appendix block junie.go writes into
// `.junie/AGENTS.md`.
const (
	AgentsStartMarker = emit.AgentsStartMarker
	AgentsEndMarker   = emit.AgentsEndMarker
)

// LocalStartMarker and LocalEndMarker mirror the emit-layer sentinels
// around the project-local instructions sync appends to an entry point.
const (
	LocalStartMarker = emit.LocalStartMarker
	LocalEndMarker   = emit.LocalEndMarker
)

// ReadLocalInstructions returns the trimmed text of the project-local
// `.agnostic-ai/local/AGNOSTIC_AI.md`, or "" when absent (re-exported
// from the emit layer).
func ReadLocalInstructions() (string, error) {
	return emit.ReadLocalInstructions()
}

// AppendLocalInstructions appends the sentinel-marked local block to
// body, replacing any earlier one (re-exported from the emit layer).
func AppendLocalInstructions(body, local string) string {
	return emit.AppendLocalInstructions(body, local)
}

// StripLocalInstructions removes the sentinel-marked local block from
// body (re-exported from the emit layer).
func StripLocalInstructions(body string) string {
	return emit.StripLocalInstructions(body)
}

// SplitAgentsCompanion reports whether a CLAUDE.md imports the AGENTS.md
// beside it and returns the rest of its text (re-exported from the emit
// layer).
func SplitAgentsCompanion(text string) (string, bool) {
	return emit.SplitAgentsCompanion(text)
}

// IsAgentsCompanion reports whether a CLAUDE.md holds only an import of
// the AGENTS.md beside it (re-exported from the emit layer).
func IsAgentsCompanion(text string) bool {
	return emit.IsAgentsCompanion(text)
}

// InlinesRulesIntoEntryPoint reports whether target delivers rule bodies
// by inlining them into its entry-point file (re-exported from the emit
// layer).
func InlinesRulesIntoEntryPoint(target string) bool {
	return emit.InlinesRulesIntoEntryPoint(target)
}

// EntryPointRuleInliner returns the target whose inlined rules block
// the entry point target reads carries in place of target's own
// always-on rule files, or "" (re-exported from the emit layer).
func EntryPointRuleInliner(cfg *config.Config, target string) string {
	return emit.EntryPointRuleInliner(cfg, target)
}

// RuleInEntryPoint reports whether target skips its own file for rule r
// because the entry point it reads already carries r, with the same
// text, and that file would load r in every session too.
func RuleInEntryPoint(cfg *config.Config, b spec.Bundle, target string, r spec.Entry) bool {
	own := slices.Clone(b.For(target).Rules)
	vals := varsFor(cfg, target)
	for i := range own {
		own[i].Body, _ = emit.ExpandVars(own[i].Body, vals)
	}
	for _, skipped := range entryPointRules(cfg, b, target, own) {
		if skipped.Name == r.Name {
			return true
		}
	}
	return false
}

// entryPointRules returns the rules in own, target's rules as its files
// hold them, that the entry point target reads carries with the same
// body and description and that target would load in every session.
// A rule whose text differs, from a `::target` fence or an expanded
// variable, keeps its file: dropping it would lose that text.
func entryPointRules(cfg *config.Config, b spec.Bundle, target string, own []spec.Entry) []spec.Entry {
	inlined := emit.EntryPointInlinedRules(cfg, b, target)
	if len(inlined) == 0 {
		return nil
	}
	var out []spec.Entry
	for _, r := range own {
		i, ok := inlined[r.Name]
		if ok && i.Body == r.Body && i.Description() == r.Description() && AlwaysOnRule(target, r) {
			out = append(out, r)
		}
	}
	return out
}

// withoutEntryPointRules drops the rules entryPointRules reports, so
// target loads each of them once, and records them on sess.
func withoutEntryPointRules(sess *Session, cfg *config.Config, b spec.Bundle, target string, own []spec.Entry) []spec.Entry {
	inlined := entryPointRules(cfg, b, target, own)
	sess.SetInlinedRules(inlined)
	if len(inlined) == 0 {
		return own
	}
	skip := make(map[string]bool, len(inlined))
	for _, r := range inlined {
		skip[r.Name] = true
	}
	out := make([]spec.Entry, 0, len(own))
	for _, r := range own {
		if !skip[r.Name] {
			out = append(out, r)
		}
	}
	return out
}

// RenderRulesAppendix renders the sentinel-marked rules block for an
// entry-point file (re-exported from the emit layer).
func RenderRulesAppendix(b spec.Bundle) string {
	return emit.RenderRulesAppendix(b)
}

// ImportsRulesIntoEntryPoint reports whether target wires its per-rule
// files into its entry-point via `@`-import lines (re-exported from the
// emit layer).
func ImportsRulesIntoEntryPoint(cfg *config.Config, target string) bool {
	return emit.ImportsRulesIntoEntryPoint(cfg, target)
}

// RenderRulesImportAppendix renders the sentinel-marked block of
// `@`-import lines pointing at a target's per-rule files (re-exported
// from the emit layer).
func RenderRulesImportAppendix(cfg *config.Config, target string, b spec.Bundle) string {
	return emit.RenderRulesImportAppendix(cfg, target, b)
}

// RenderLegacyRulesFileImportAppendix renders the sentinel-marked
// one-line `@`-import block pointing at a target's legacy concatenated
// rules-file (re-exported from the emit layer).
func RenderLegacyRulesFileImportAppendix(cfg *config.Config, target string) string {
	return emit.RenderLegacyRulesFileImportAppendix(cfg, target)
}

// AppendRulesAppendix appends a rendered rules block to body, stripping
// any pre-existing block first (re-exported from the emit layer).
func AppendRulesAppendix(body, appendix string) string {
	return emit.AppendRulesAppendix(body, appendix)
}

// StripGeneratedAppendices reverses every sentinel-marked edit sync may
// make to an entry-point file, leaving the canonical body (re-exported
// from the emit layer).
func StripGeneratedAppendices(body string) string {
	return emit.StripGeneratedAppendices(body)
}

// SupportsFileImports reports whether target's CLI resolves `@path`
// file-import lines in its entry-point file (re-exported from the emit
// layer).
func SupportsFileImports(target string) bool {
	return emit.SupportsFileImports(target)
}

// ApplyImportMode rewrites `@path` file-import lines in body per mode for
// a target that cannot resolve them (re-exported from the emit layer).
func ApplyImportMode(body, mode string) (string, error) {
	return emit.ApplyImportMode(body, mode)
}

// Adapter is the contract every target implementation satisfies.
type Adapter interface {
	// Name returns the target identifier used in config and CLI flags.
	Name() string
	// Capabilities returns the portable spec kinds the target emits natively.
	// The playground and documentation use this declaration directly, so a
	// target-audit fix has one capability source to update.
	Capabilities() []spec.Kind
	// Emit renders the bundle as files for this target, routing every
	// write through sess so the caller owns the capture / recording /
	// backup / transaction buffers. dryRun prints rather than writing.
	Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error
}

// TargetCapabilities is the public, read-only capability view for one
// registered target.
type TargetCapabilities struct {
	Name     string
	Supports []spec.Kind
}

// CapabilityMatrix returns every built-in target and its declared native
// spec kinds sorted by target name. Returned slices are copies so callers cannot
// mutate adapter declarations.
func CapabilityMatrix() []TargetCapabilities {
	names := Names()
	matrix := make([]TargetCapabilities, 0, len(names))
	for _, name := range names {
		a := registry[name]
		matrix = append(matrix, TargetCapabilities{
			Name:     name,
			Supports: append([]spec.Kind(nil), a.Capabilities()...),
		})
	}
	return matrix
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
// adapter.Emit. Installs the user-owned path set from `sync.unmanaged`
// on sess so every adapter, current and future, honors it without
// per-adapter code.
func EmitWithProvenance(sess *Session, a Adapter, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	defer emit.ProvenanceFor(cfg, a.Name())()
	if cfg != nil {
		sess.SetUnmanaged(cfg.Sync.Unmanaged)
		codexSkills := ""
		if slices.Contains(cfg.Targets, "codex") {
			codexSkills = emit.OutputSkillsDir(cfg, "codex", emit.CodexSkillsRoot)
		}
		sess.SetCodexSkillsDir(codexSkills)
		writers := map[string][]string{}
		for _, t := range cfg.Targets {
			if dir := varsFor(cfg, t)[emit.VarSkillsDir]; dir != "" {
				writers[filepath.Clean(dir)] = append(writers[filepath.Clean(dir)], t)
			}
		}
		sess.SetSkillsDirWriters(writers)
	}
	own := expandBundleVars(b.For(a.Name()), cfg, a.Name())
	own.Rules = withoutEntryPointRules(sess, cfg, b, a.Name(), own.Rules)
	prepared, files, err := emit.PrepareScopedDocuments(own, cfg, a.Name(), ReviewSections(b, cfg, a.Name()))
	if err != nil {
		return err
	}
	if err := a.Emit(sess, prepared, cfg, dryRun); err != nil {
		return err
	}
	if slices.Contains(a.Capabilities(), spec.KindEnvironment) {
		emit.RecordEnvironmentFields(a.Name(), prepared.Environments)
	}
	for _, f := range files {
		if err := sess.WriteFile(f.Path, f.Content, dryRun); err != nil {
			return err
		}
	}
	return nil
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
	"junie":       junie.New(),
	"kiro":        kiro.New(),
	"crush":       crush.New(),
	"trae":        trae.New(),
	"jules":       jules.New(),
	"goose":       goose.New(),
	"augment":     augment.New(),
	"qoder":       qoder.New(),
	"openhands":   openhands.New(),
	"factory":     factory.New(),
	"kilo":        kilo.New(),
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
		if s := SuggestName(name, Names()); s != "" {
			return nil, errs.Coded(errs.CodeSyncTargetUnknown,
				"unknown target: %s (did you mean %s? no %s%s on PATH)",
				name, s, external.BinaryPrefix, name)
		}
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
	sort.Strings(out)
	return out
}
