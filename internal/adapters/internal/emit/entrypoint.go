package emit

import (
	"path/filepath"
	"sort"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// AgnosticEntryPointPath is the project-relative path of the canonical
// CLI-agnostic entry-point file. Every target's native entry-point
// (CLAUDE.md, AGENTS.md, GEMINI.md, ...) carries the same body so a
// hand-edit propagates through `agnostic-ai import` to every consumer.
const AgnosticEntryPointPath = ".agnostic-ai/AGNOSTIC_AI.md"

// entryPointPaths maps target name to the conventional root entry-point
// file for that target. Targets absent from the map have no entry-point
// (they emit only per-file artifacts under their own directory, e.g.
// cursor's .cursor/rules/<name>.mdc).
//
// zed is the one target whose path is not the vendor's preferred file.
// Zed reads "the first matching file in this list"
// (zed.dev/docs/ai/instructions), first match wins with no merge:
// `.rules`, `.cursorrules`, `.windsurfrules`, `.clinerules`,
// `.github/copilot-instructions.md`, `AGENT.md`, `AGENTS.md`,
// `CLAUDE.md`, `GEMINI.md`. AGENTS.md sits at rank 7, behind copilot's
// entry-point at rank 5, and that file carries the pointer body only
// (copilot delivers rules through `.github/instructions/`, so it is
// absent from inlineRulesTargets). Enabling both targets handed Zed a
// file with no rule bodies in it and every rule silently stopped
// applying (#624). `.rules` is rank 1, so nothing agnostic-ai emits
// can outrank it. See TestVendorLookupOrder_WinningFileCarriesRules.
var entryPointPaths = map[string]string{
	"codebuddy":   "CODEBUDDY.md",
	"claude":      "CLAUDE.md",
	"codex":       "AGENTS.md",
	"vibe":        "AGENTS.md",
	"kimi":        ".kimi-code/AGENTS.md",
	"amp":         "AGENTS.md",
	"warp":        "AGENTS.md",
	"cline":       "AGENTS.md",
	"windsurf":    "AGENTS.md",
	"junie":       "AGENTS.md",
	"kiro":        "AGENTS.md",
	"crush":       "AGENTS.md",
	"trae":        "AGENTS.md",
	"jules":       "AGENTS.md",
	"goose":       "AGENTS.md",
	"augment":     "AGENTS.md",
	"qoder":       "AGENTS.md",
	"pi":          "AGENTS.md",
	"openhands":   "AGENTS.md",
	"factory":     "AGENTS.md",
	"kilo":        "AGENTS.md",
	"opencode":    "AGENTS.md",
	"gemini":      "GEMINI.md",
	"qwen":        "QWEN.md",
	"aider":       "CONVENTIONS.md",
	"zed":         ".rules",
	"copilot":     ".github/copilot-instructions.md",
	"antigravity": ".agents/AGENTS.md",
}

// ConventionalEntryPointPaths returns the distinct conventional root
// entry-point files across every known target (CLAUDE.md, AGENTS.md,
// GEMINI.md, ...), sorted for stable output. Output-path overrides are
// ignored: the set drives import's detection of a hand-authored sibling
// entry-point that sync would otherwise overwrite (#415).
func ConventionalEntryPointPaths() []string {
	seen := make(map[string]struct{}, len(entryPointPaths))
	out := make([]string, 0, len(entryPointPaths))
	for _, p := range entryPointPaths {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// EntryPointPath returns the project-relative entry-point file for
// target, respecting outputs.<target>.file overrides. Returns "" when
// the target has no entry-point convention.
func EntryPointPath(cfg *config.Config, target string) string {
	if cfg != nil {
		if o, ok := cfg.Outputs[target]; ok && o.File != "" {
			return o.File
		}
	}
	if target == "kimi" && cfg != nil {
		for _, peer := range cfg.Targets {
			if peer == target {
				continue
			}
			path := EntryPointPath(cfg, peer)
			if filepath.Clean(path) == "AGENTS.md" && !cfg.IsUnmanaged(path) {
				return path
			}
		}
	}
	return entryPointPaths[target]
}

// rulesFileWriters says, for each built-in target, whether its adapter
// writes outputs.<target>.rules-file. On a built-in that does not, the
// key has no effect: sync delivers rules and writes the entry point as
// if it were unset, and lint reports it (LINT062). A name missing here
// is an external adapter, which receives outputs and may write the
// file, so the key keeps its meaning for it.
// TestRulesFileWriters_MatchAdapters keeps this table in step with the
// adapters.
var rulesFileWriters = map[string]bool{
	"aider":       true,
	"amp":         true,
	"antigravity": true,
	"augment":     true,
	"claude":      true,
	"cline":       false,
	"codebuddy":   false,
	"codex":       true,
	"continue":    false,
	"copilot":     true,
	"crush":       false,
	"cursor":      false,
	"ellipsis":    false,
	"factory":     false,
	"gemini":      true,
	"goose":       true,
	"grok":        false,
	"jules":       false,
	"junie":       false,
	"kilo":        false,
	"kimi":        true,
	"kiro":        false,
	"opencode":    true,
	"openhands":   false,
	"pi":          true,
	"qoder":       false,
	"qwen":        true,
	"trae":        false,
	"vibe":        true,
	"warp":        true,
	"windsurf":    false,
	"zed":         true,
}

// WritesRulesFile reports whether target's adapter writes
// outputs.<target>.rules-file.
func WritesRulesFile(target string) bool {
	return rulesFileWriters[target]
}

// IgnoresRulesFile reports whether target is a built-in whose adapter
// writes no outputs.<target>.rules-file, so sync ignores the key.
func IgnoresRulesFile(target string) bool {
	writes, builtin := rulesFileWriters[target]
	return builtin && !writes
}

// RulesFileWriters returns, sorted, the built-in targets that write
// outputs.<target>.rules-file.
func RulesFileWriters() []string {
	var out []string
	for t, writes := range rulesFileWriters {
		if writes {
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out
}

// RulesFileTargets returns, sorted, every target rulesFileWriters names.
func RulesFileTargets() []string {
	out := make([]string, 0, len(rulesFileWriters))
	for t := range rulesFileWriters {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// HasLegacyRulesFile reports whether the user opted into the legacy
// concatenated rules-file layout for a target that may write one. When
// true, the adapter owns rule delivery: it concatenates rule bodies
// into outputs.<target>.rules-file and the central sync layer neither
// inlines nor `@`-imports them anywhere else.
func HasLegacyRulesFile(cfg *config.Config, target string) bool {
	if cfg == nil || IgnoresRulesFile(target) {
		return false
	}
	o, ok := cfg.Outputs[target]
	return ok && o.RulesFile != ""
}

// LegacyRulesFileOwnsEntryPoint reports whether target's legacy
// concatenated rules-file is the target's own entry-point file, the
// documented form (`outputs.claude.rules-file: CLAUDE.md`,
// `outputs.codex.rules-file: AGENTS.md`). Two writers on one path
// collide, so the central sync layer skips the pointer-body write and
// lets the adapter own the file.
//
// A rules-file pointing anywhere else is not a collision, and skipping
// the entry-point write for it left the target with no entry-point file
// at all. On claude that is a routing inversion, not a cosmetic gap:
// Claude Code reads AGENTS.md as project instructions whenever no
// CLAUDE.md exists at or above the working directory, so a project with
// codex enabled handed Claude Code the rule bodies its own config had
// routed away from claude.
func LegacyRulesFileOwnsEntryPoint(cfg *config.Config, target string) bool {
	if !HasLegacyRulesFile(cfg, target) {
		return false
	}
	entry := EntryPointPath(cfg, target)
	return entry != "" && samePath(cfg.Outputs[target].RulesFile, entry)
}

// samePath compares two project-relative output paths the way the
// filesystem does, so `./CLAUDE.md` and `CLAUDE.md` are one file.
func samePath(a, b string) bool {
	return filepath.Clean(filepath.FromSlash(a)) == filepath.Clean(filepath.FromSlash(b))
}

// entryPointTemplate is the body sync seeds into a missing
// .agnostic-ai/AGNOSTIC_AI.md. Every entry point loads it each session,
// so it stays short.
const entryPointTemplate = "# Project instructions\n\n" +
	"CLAUDE.md, AGENTS.md, and the other AI tool files are generated from `.agnostic-ai/`. " +
	"Edit `.agnostic-ai/AGNOSTIC_AI.md` and the source specs, then run `agnostic-ai sync`.\n\n" +
	"<!-- Replace this comment with the instructions every AI tool should follow in this project. -->\n"

// LegacyEntryPointTemplateLead opens the long body sync seeded before
// entryPointTemplate. Projects created then still carry it.
const LegacyEntryPointTemplateLead = "# AI Project Conventions\n\nThis file is regenerated by [agnostic-ai](https://github.com/chemaclass/agnostic-ai)."

// EntryPointBody returns the template body, without the provenance
// header, that sync seeds into a missing AGNOSTIC_AI.md.
func EntryPointBody() string {
	return entryPointTemplate
}

// RenderEntryPoint returns a root entry-point file (provenance header +
// template body). The string is byte-identical regardless of target, so
// collision detection can deduplicate writes when two targets share the
// same path (e.g. codex + amp both default to AGENTS.md).
func RenderEntryPoint() string {
	return WithHeader(EntryPointBody(), FormatMarkdown)
}

// EmitLegacyRulesFile writes a concatenated MergedDocument at
// outputs.<target>.rules-file when the user opted into the legacy
// layout. No-op when the field is unset, so adapters call it
// unconditionally without an outer if-guard.
//
// opts.OutFile is overwritten with the rules-file path; opts.Intro
// defaults to the provenance line when empty. The remaining fields
// (Title, AgentSectionPrefix, ...) pass through unchanged so each
// adapter controls its own heading and section labeling.
//
// When rulesFile is the target's own entry point, the model tiers follow
// the intro and the project-local instructions follow last (see
// AppendLegacyEntryPointLocal).
func (s *Session) EmitLegacyRulesFile(b spec.Bundle, cfg *config.Config, target string, opts MergedOpts, dryRun bool) error {
	rulesFile := OutputRulesFile(cfg, target, "")
	if rulesFile == "" {
		return nil
	}
	local, err := legacyEntryPointLocal(cfg, target)
	if err != nil {
		return err
	}
	opts.local = local
	if LegacyRulesFileOwnsEntryPoint(cfg, target) {
		opts.tiers = RenderModelTiers(cfg.Models)
	}
	opts.OutFile = rulesFile
	if opts.Intro == "" {
		opts.Intro = "Generated by agnostic-ai."
	}
	return s.MergedDocument(b, opts, dryRun)
}
