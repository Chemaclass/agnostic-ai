package adapters

import (
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/adapters/junie"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

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

// EntryPointReaders returns, sorted, target and every configured target
// that reads the same entry-point file (re-exported from the emit layer).
func EntryPointReaders(cfg *config.Config, target string) []string {
	return emit.EntryPointReaders(cfg, target)
}

// ConventionalEntryPointPaths returns the distinct conventional root
// entry-point files across every known target, sorted. Used by import to
// detect a sibling entry-point that sync would overwrite.
func ConventionalEntryPointPaths() []string {
	return emit.ConventionalEntryPointPaths()
}

// WritesRulesFile reports whether target's adapter writes
// outputs.<target>.rules-file (re-exported from the emit layer).
func WritesRulesFile(target string) bool {
	return emit.WritesRulesFile(target)
}

// IgnoresRulesFile reports whether target is a built-in whose adapter
// writes no outputs.<target>.rules-file (re-exported from the emit
// layer).
func IgnoresRulesFile(target string) bool {
	return emit.IgnoresRulesFile(target)
}

// RulesFileWriters returns, sorted, the targets that write
// outputs.<target>.rules-file (re-exported from the emit layer).
func RulesFileWriters() []string {
	return emit.RulesFileWriters()
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

// EntryPointBody returns the template body, without the provenance
// header, that sync seeds into a missing AGNOSTIC_AI.md.
func EntryPointBody() string {
	return emit.EntryPointBody()
}

// LegacyEntryPointTemplateLead opens the long AGNOSTIC_AI.md body older
// releases seeded (re-exported from the emit layer).
const LegacyEntryPointTemplateLead = emit.LegacyEntryPointTemplateLead

// RenderEntryPoint returns a root entry-point file: the provenance
// header over the template body.
func RenderEntryPoint() string {
	return emit.RenderEntryPoint()
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

// specRefuser is the optional interface an adapter implements when its
// Emit refuses some specs or settings before writing anything. Lint calls
// it, so the refusal shows before sync stops for every target.
type specRefuser interface {
	RefuseSpecs(b spec.Bundle, cfg *config.Config) error
}

// Refusal is the error RefuseSpecs returns, naming the refused spec.
type Refusal = emit.Refusal

// RefuseSpecs returns the error the named target's Emit raises for b
// before writing, nil when the target accepts b or declares no checks.
func RefuseSpecs(name string, b spec.Bundle, cfg *config.Config) error {
	r, ok := registry[name].(specRefuser)
	if !ok {
		return nil
	}
	defer SkipNotes()()
	// A bundle sync cannot prepare fails there with its own error first.
	own, err := targetSkills(b.For(name), cfg, name)
	if err != nil {
		return nil
	}
	own, _, err = emit.PrepareScopedDocuments(own, cfg, name, nil)
	if err != nil {
		return nil
	}
	return r.RefuseSpecs(own, cfg)
}

// targetSkills adds the agents and commands target writes as skills.
func targetSkills(b spec.Bundle, cfg *config.Config, target string) (spec.Bundle, error) {
	own, err := withAgentsAsSkills(FlatCommandNames(b, target), cfg, target)
	if err != nil {
		return spec.Bundle{}, err
	}
	return withCommandsAsSkills(own, cfg, target), nil
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
// can detect and extract the agents appendix block older junie adapter
// versions wrote into `.junie/AGENTS.md`.
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

// RootInstructionsPath returns the root instructions file target reads
// at session start, AGENTS.md for Cursor (re-exported from the emit
// layer).
func RootInstructionsPath(cfg *config.Config, target string) string {
	return emit.RootInstructionsPath(cfg, target)
}

// TraeReadsEntryPoint reports whether the user declared that Trae reads
// AGENTS.md (re-exported from the emit layer).
func TraeReadsEntryPoint(cfg *config.Config) bool {
	return emit.TraeReadsEntryPoint(cfg)
}

// RuleInEntryPoint reports whether target skips its own file for rule r
// because the entry point it reads already carries r, with the same
// text, and that file would load r in every session too. To check many
// rules, build the set once with RulesInEntryPoint.
func RuleInEntryPoint(cfg *config.Config, b spec.Bundle, target string, r spec.Entry) bool {
	return RulesInEntryPoint(cfg, b, target)[r.Name]
}

// RulesInEntryPoint returns, by name, every rule RuleInEntryPoint
// reports for target.
func RulesInEntryPoint(cfg *config.Config, b spec.Bundle, target string) map[string]bool {
	own := slices.Clone(b.For(target).Rules)
	vals := emit.VarsFor(cfg, target)
	for i := range own {
		own[i].Body, _ = emit.ExpandVars(own[i].Body, vals)
		own[i].Body, _ = emit.ExpandRefs(own[i].Body, emit.RefForms[target])
	}
	skipped := entryPointRules(cfg, b, target, own)
	names := make(map[string]bool, len(skipped))
	for _, r := range skipped {
		names[r.Name] = true
	}
	return names
}

// entryPointRules returns the rules in own, target's rules as its files
// hold them, that the entry point target reads carries with the same
// body and description and that target would load in every session.
// A rule whose text differs, from a `::target` fence or an expanded
// variable, keeps its file: dropping it would lose that text.
func entryPointRules(cfg *config.Config, b spec.Bundle, target string, own []spec.Entry) []spec.Entry {
	inlined := entryPointInlinedRules(cfg, b, target)
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

// entryPointInlinedRules returns the rules, by name, that the entry
// point target reads carries in its inlined rules block, or nil.
func entryPointInlinedRules(cfg *config.Config, b spec.Bundle, target string) map[string]spec.Entry {
	inliner := emit.EntryPointRuleInliner(cfg, target)
	if inliner == "" {
		return nil
	}
	rules := map[string]spec.Entry{}
	for _, r := range EntryPointRules(b, inliner, cfg).Rules {
		rules[r.Name] = r
	}
	return rules
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

func StripBuiltinSkills(body string) string {
	return emit.StripBuiltinSkills(body)
}

// SupportsFileImports reports whether target's CLI resolves `@path`
// file-import lines in its entry-point file (re-exported from the emit
// layer).
func SupportsFileImports(target string) bool {
	return emit.SupportsFileImports(target)
}

type ImportSpan = emit.ImportSpan

func ApplyImportModeWithSpans(body, mode string) (string, []ImportSpan, error) {
	return emit.ApplyImportModeWithSpans(body, mode)
}

// VSCodeReadsRootAgents re-exports emit.VSCodeReadsRootAgents.
func VSCodeReadsRootAgents(cfg *config.Config) bool { return emit.VSCodeReadsRootAgents(cfg) }
