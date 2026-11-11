package emit

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Sentinel markers delimiting the generated rules appendix inside an
// entry-point file. Targets with no native rules directory (codex, amp,
// warp, gemini, aider, opencode) read their always-on context from a
// single entry-point file (AGENTS.md, GEMINI.md, CONVENTIONS.md, ...),
// so sync inlines every rule body here. Import strips the block before
// mirroring the body to AGNOSTIC_AI.md, keeping the canonical body free
// of target-specific rule copies.
const (
	RulesStartMarker = "<!-- agnostic-ai:rules:start -->"
	RulesEndMarker   = "<!-- agnostic-ai:rules:end -->"
)

// RenderRulesAppendix renders the sentinel-marked rules block for an
// entry-point file. Each rule contributes a "### <name>" section with
// its source provenance comment, optional description, and full body.
// Returns "" when the bundle has no rules, so callers can append the
// result unconditionally.
func RenderRulesAppendix(b spec.Bundle) string {
	if len(b.Rules) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, r := range b.Rules {
		if r.EffectiveScope() != "" {
			continue
		}
		WriteInlineRule(&sb, r)
	}
	if sb.Len() == 0 {
		return ""
	}
	return wrapRulesBlock(sb.String())
}

// wrapRulesBlock frames inner with the rules sentinel markers and the
// "## Rules" heading. Both the inline-body and the `@`-import appendix
// share this framing so the markers stay in one place.
func wrapRulesBlock(inner string) string {
	return RulesStartMarker + "\n\n## Rules\n\n" + inner + RulesEndMarker + "\n"
}

// AppendRulesAppendix returns body with the rendered rules block appended
// after one blank line, stripping any pre-existing rules block first so
// repeated syncs never stack appendixes. Returns body unchanged when
// appendix is empty.
func AppendRulesAppendix(body, appendix string) string {
	if appendix == "" {
		return body
	}
	body = StripRulesAppendix(body)
	return strings.TrimRight(body, "\n") + "\n\n" + appendix
}

// StripRulesAppendix removes the sentinel-marked rules block (markers
// included) from body. Returns body unchanged when no block is present.
func StripRulesAppendix(body string) string {
	return stripMarkedBlock(body, RulesStartMarker, RulesEndMarker)
}

// StripGeneratedAppendices reverses every sentinel-marked edit sync may
// make to an entry-point file, leaving the canonical pointer body. It
// removes the model tiers, rules, agents, built-in skills, project-local, memory, and target-overview blocks and restores
// resolved `@`-imports (inline mode) to their lone `@path` lines. Import
// uses it so the AGNOSTIC_AI.md round-trip stays lossless regardless of
// which transforms a target carried. The agents appendix (see
// agents_appendix.go) is junie-only today, but stripping it
// unconditionally costs nothing on bodies that never had one.
func StripGeneratedAppendices(body string) string {
	return restoreImportInlines(StripBuiltinSkills(StripAgentsAppendix(StripRulesAppendix(StripReviewSection(StripLocalInstructions(StripMemoryBlock(StripTargetOverview(StripModelTiers(body)))))))))
}

// inlineRulesTargets are the entry-point targets whose underlying CLI has
// no native rules directory: their only always-on context surface is the
// single entry-point file, so sync inlines rule bodies there. Targets
// that emit a real per-rule destination (claude .claude/rules/, cursor
// .cursor/rules/, cline .clinerules/, qoder .qoder/rules/,
// continue/windsurf/antigravity rules dirs) are absent: they deliver
// rules without polluting the pointer. Zed inlines since 1.4.2 retired
// its rules library, leaving instruction files as its only always-on
// surface (its entry-point is `.rules`, see entryPointPaths); Crush and
// the batch-2 AGENTS.md tools jules, goose, openhands, and factory have
// no rules directory either.
//
// augment and kilo are two deliberate exceptions: both gained a
// native per-rule directory (`.augment/rules/`, `.kilo/rules/`) but
// stay on this list, since each vendor loads a root AGENTS.md in every
// session. Their always-on rules inline here and skip the native
// directory, so each rule loads once (entryPointFirstTargets, #1224).
//
// This map and entryPointPaths are independent, so a target absent
// here still writes an entry-point file, holding the pointer body
// alone. For a vendor that reads the first match in a documented list
// of paths, that pointer-only file can shadow a rules-carrying one
// (#624); TestVendorLookupOrder_WinningFileCarriesRules holds that line.
//
// junie fits the main rule (no native rules directory: `.junie/rules/`
// never was one, Junie's guidelines lookup never reads it, see
// internal/adapters/junie's package doc, #552) but is the one entry
// consulted from two places instead of one. Every other member here
// only matters to the central multi-consumer dispatch in
// internal/cli/entrypoint.go, which renders the shared root AGENTS.md.
// junie.go's own Emit also reads this map directly to decide whether
// its adapter-owned `.junie/AGENTS.md` (Junie's actual read path)
// gets the rules appendix, independent of what entrypoint.go does with
// the root AGENTS.md junie also happens to share with codex et al.
var inlineRulesTargets = map[string]bool{
	"codebuddy": true,

	"pi":        true,
	"codex":     true,
	"vibe":      true,
	"kimi":      true,
	"amp":       true,
	"warp":      true,
	"zed":       true,
	"gemini":    true,
	"qwen":      true,
	"aider":     true,
	"opencode":  true,
	"crush":     true,
	"jules":     true,
	"goose":     true,
	"augment":   true,
	"openhands": true,
	"factory":   true,
	"kilo":      true,
	"junie":     true,
}

// InlinesRulesIntoEntryPoint reports whether target delivers rule bodies
// by inlining them into its entry-point file. The legacy concatenated
// rules-file layout (outputs.<target>.rules-file) overrides this: the
// adapter owns that write and the central inline is skipped.
func InlinesRulesIntoEntryPoint(target string) bool {
	return inlineRulesTargets[target]
}

// entryPointFirstTargets write rule files of their own and also load
// their root entry point in every session by default. When that file
// carries the inlined rules block, an always-on rule file holding the
// same text would load it twice, so they leave it to the entry point
// (#1224):
//
//   - cline: AGENTS.md is a detected rule source whose toggle starts on,
//     like a .clinerules/ file's (Cline's rule-helpers.ts).
//   - kiro: AGENTS.md files "do not support inclusion modes and are
//     always included".
//   - qoder: the IDE reads AGENTS.md with "No additional configuration";
//     rules win only a conflict, and a skipped copy cannot conflict.
//   - kilo: AGENTS.md "cannot be individually disabled".
//   - augment: the IDE always includes a root AGENTS.md, and Auggie
//     ranks it above `.augment/rules/`.
//   - cursor: the CLI "reads AGENTS.md and CLAUDE.md at the project
//     root ... and applies them as rules alongside .cursor/rules". It
//     writes no entry point; another target writes AGENTS.md (#3230).
//   - windsurf: Devin reads AGENTS.md "as always-on rules" next to
//     `.devin/rules/` (#3230).
//
// trae reads AGENTS.md only after "Include AGENTS.md in the context" is
// switched on under Settings > Rules & Memories, which the user declares
// with `outputs.trae.rules-mode: entry-point`.
var entryPointFirstTargets = map[string]bool{
	"codebuddy": true,
	"cline":     true,
	"kiro":      true,
	"qoder":     true,
	"kilo":      true,
	"augment":   true,
	"cursor":    true,
	"windsurf":  true,
}

// TraeReadsEntryPoint reports whether the user declared Trae's "Include
// AGENTS.md in the context" switch on (`outputs.trae.rules-mode:
// entry-point`). Trae's docs give no default and tell the user to turn
// it on, so it counts as off until declared.
func TraeReadsEntryPoint(cfg *config.Config) bool {
	return cfg != nil && cfg.Outputs["trae"].RulesMode == "entry-point"
}

func loadsEntryPointRules(cfg *config.Config, target string) bool {
	return entryPointFirstTargets[target] || target == "trae" && TraeReadsEntryPoint(cfg)
}

// RootInstructionsPath returns the conventional root instructions file
// target reads at session start, or "" when an output override moves it
// off the file its tool reads. Cursor writes no entry point but reads
// the project-root AGENTS.md when another target writes it.
func RootInstructionsPath(cfg *config.Config, target string) string {
	if target == "cursor" {
		return "AGENTS.md"
	}
	path := EntryPointPath(cfg, target)
	if path == "" || path != entryPointPaths[target] {
		return ""
	}
	return path
}

// EntryPointRuleInliner returns the target whose rules block sync
// inlines into the entry point target reads, when target may load that
// block in place of its own always-on rule files: target itself or a
// configured reader of the same file. Returns "" when target keeps its
// rule files: it is not in entryPointFirstTargets, its entry point is
// not the conventional root file its tool reads, or sync does not write
// that file. Mirrors the reader grouping in internal/cli/entrypoint.go.
func EntryPointRuleInliner(cfg *config.Config, target string) string {
	if cfg == nil || !loadsEntryPointRules(cfg, target) || HasLegacyRulesFile(cfg, target) {
		return ""
	}
	path := RootInstructionsPath(cfg, target)
	if path == "" || cfg.IsUnmanaged(path) {
		return ""
	}
	for _, t := range append([]string{target}, cfg.Targets...) {
		if InlinesRulesIntoEntryPoint(t) && !HasLegacyRulesFile(cfg, t) && EntryPointPath(cfg, t) == path {
			return t
		}
	}
	return ""
}

// importRulesDir maps each target whose CLI auto-loads its entry-point
// file but NOT its per-rule directory to that directory's default. These
// targets emit one file per rule but the runtime never reads the folder,
// so `outputs.<target>.rules-mode: import` wires the files into the
// entry-point via `@`-import lines pointing at this dir. Claude is the
// only such target today.
var importRulesDir = map[string]struct{ sub, def string }{
	"claude": {"rules", ".claude/rules"},
}

// ImportsRulesIntoEntryPoint reports whether target wires its per-rule
// files into its entry-point file via `@`-import lines, opted in with
// `outputs.<target>.rules-mode: import`. Only targets that emit a native
// per-rule directory the CLI does not auto-load qualify (claude). The
// legacy concatenated rules-file layout overrides it: that adapter owns
// the entry-point write.
func ImportsRulesIntoEntryPoint(cfg *config.Config, target string) bool {
	if cfg == nil || importRulesDir[target].def == "" || HasLegacyRulesFile(cfg, target) {
		return false
	}
	o, ok := cfg.Outputs[target]
	return ok && o.RulesMode == "import"
}

// RenderLegacyRulesFileImportAppendix renders a one-line `@`-import
// block pointing at the legacy concatenated rules-file, for a target
// whose CLI auto-loads its entry-point file but not that path. Returns
// "" unless target sets outputs.<target>.rules-file to something other
// than its own entry point, which is the only case where the entry
// point is written by sync and the rule bodies live elsewhere.
//
// Without it the concatenated file reaches no session at all:
// `.claude/RULES.md` is on no documented Claude Code load path. Reuses
// the rules sentinel markers so import strips the block on round-trip.
func RenderLegacyRulesFileImportAppendix(cfg *config.Config, target string) string {
	if importRulesDir[target].def == "" || LegacyRulesFileOwnsEntryPoint(cfg, target) {
		return ""
	}
	rulesFile := OutputRulesFile(cfg, target, "")
	if rulesFile == "" {
		return ""
	}
	return wrapRulesBlock("These rule files are loaded into context on every session:\n\n" +
		"@" + filepath.ToSlash(rulesFile) + "\n")
}

// RenderRulesImportAppendix renders the sentinel-marked block of Claude
// `@`-import lines, one per rule, pointing at the per-rule files the
// adapter emits under its rules directory (honoring an
// `outputs.<target>.rules-dir` override). Returns "" when the bundle has
// no rules or the target does not support import mode. Reuses the rules
// sentinel markers so import strips the block on round-trip.
func RenderRulesImportAppendix(cfg *config.Config, target string, b spec.Bundle) string {
	def := importRulesDir[target]
	if def.def == "" || len(b.Rules) == 0 {
		return ""
	}
	rulesDir := OutputRulesDir(cfg, target, OutputSubDir(cfg, target, def.sub, def.def))
	var sb strings.Builder
	sb.WriteString("These rule files are loaded into context on every session:\n\n")
	for _, r := range b.Rules {
		if r.EffectiveScope() != "" {
			continue
		}
		sb.WriteString("@" + path.Join(rulesDir, r.EffectiveScope(), r.Name+".md") + "\n")
	}
	return wrapRulesBlock(sb.String())
}
