package emit

import (
	"path"
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
		WriteSection(&sb, r.Name, r)
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
// removes the rules and target-overview appendices and restores resolved
// `@`-imports (inline mode) to their lone `@path` lines. Import uses it
// so the AGNOSTIC_AI.md round-trip stays lossless regardless of which
// transforms a target carried.
func StripGeneratedAppendices(body string) string {
	return RestoreImportInlines(StripRulesAppendix(StripTargetOverview(body)))
}

// inlineRulesTargets are the entry-point targets whose underlying CLI has
// no native rules directory: their only always-on context surface is the
// single entry-point file, so sync inlines rule bodies there. Targets
// that emit a real per-rule destination (claude .claude/rules/, cursor
// .cursor/rules/, cline/continue/windsurf/antigravity .agent/rules/, zed
// .rules) are absent: they deliver rules without polluting the pointer.
var inlineRulesTargets = map[string]bool{
	"codex":    true,
	"amp":      true,
	"warp":     true,
	"gemini":   true,
	"aider":    true,
	"opencode": true,
}

// InlinesRulesIntoEntryPoint reports whether target delivers rule bodies
// by inlining them into its entry-point file. The legacy concatenated
// rules-file layout (outputs.<target>.rules-file) overrides this: the
// adapter owns that write and the central inline is skipped.
func InlinesRulesIntoEntryPoint(target string) bool {
	return inlineRulesTargets[target]
}

// importRulesDir maps each target whose CLI auto-loads its entry-point
// file but NOT its per-rule directory to that directory's default. These
// targets emit one file per rule but the runtime never reads the folder,
// so `outputs.<target>.rules-mode: import` wires the files into the
// entry-point via `@`-import lines pointing at this dir. Claude is the
// only such target today.
var importRulesDir = map[string]string{
	"claude": ".claude/rules",
}

// ImportsRulesIntoEntryPoint reports whether target wires its per-rule
// files into its entry-point file via `@`-import lines, opted in with
// `outputs.<target>.rules-mode: import`. Only targets that emit a native
// per-rule directory the CLI does not auto-load qualify (claude). The
// legacy concatenated rules-file layout overrides it: that adapter owns
// the entry-point write.
func ImportsRulesIntoEntryPoint(cfg *config.Config, target string) bool {
	if cfg == nil || importRulesDir[target] == "" || HasLegacyRulesFile(cfg, target) {
		return false
	}
	o, ok := cfg.Outputs[target]
	return ok && o.RulesMode == "import"
}

// RenderRulesImportAppendix renders the sentinel-marked block of Claude
// `@`-import lines, one per rule, pointing at the per-rule files the
// adapter emits under its rules directory (honoring an
// `outputs.<target>.rules-dir` override). Returns "" when the bundle has
// no rules or the target does not support import mode. Reuses the rules
// sentinel markers so import strips the block on round-trip.
func RenderRulesImportAppendix(cfg *config.Config, target string, b spec.Bundle) string {
	def := importRulesDir[target]
	if def == "" || len(b.Rules) == 0 {
		return ""
	}
	rulesDir := OutputRulesDir(cfg, target, def)
	var sb strings.Builder
	sb.WriteString("These rule files are loaded into context on every session:\n\n")
	for _, r := range b.Rules {
		sb.WriteString("@" + path.Join(rulesDir, r.EffectiveScope(), r.Name+".md") + "\n")
	}
	return wrapRulesBlock(sb.String())
}
