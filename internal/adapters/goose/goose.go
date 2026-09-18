// Package goose emits configs for Block's Goose CLI.
//
// Goose reads project instructions from the root `AGENTS.md` file,
// written centrally by `sync` as a slim pointer to the source specs
// with rule bodies inlined by default (one body shared with every
// other AGENTS.md-family target). Goose also reads a second, older
// file, `.goosehints`: a single concatenated document of project
// hints. This adapter writes that document only when the user opts in
// via `outputs.goose.rules-file`, so a fresh `init` followed by `sync`
// never surprises a project with an extra root file.
//
// goose-docs.ai/docs/guides/context-engineering/using-goosehints/
// documents nested discovery: "As goose reads or modifies files in
// nested subdirectories, it can also discover and load additional hint
// files from those directories automatically", checking each directory
// for one of `CONTEXT_FILE_NAMES` (default `AGENTS.md`, `.goosehints`).
// A rule carrying a source-layout or frontmatter scope (e.g.
// `backend/`) therefore routes into a nested `<scope>/.goosehints`
// instead of flattening into the root document (#608): once the
// rules-file opt-in is set, every rule still routes through it, scoped
// or not, so the gate stays a single on/off switch rather than an
// inconsistent one for the root file only.
//
// `.goosehints` reuses the same generic legacy-rules-file mechanism the
// zed, aider, warp, and antigravity adapters use for their own opt-in
// merged documents (`Session.EmitLegacyRulesFile`) for the root
// document; there is no dedicated `hints-file` config key. Only rule
// bodies go into the document.
//
// Skills emit as one folder per skill at `.agents/skills/<name>/SKILL.md`
// (override via outputs.goose.skills-dir), the same cross-tool tree
// codex, amp, zed, and crush already write byte-identically, so the
// shared tree dedupes into one write. Goose's own docs name it "the
// recommended standard" (github.com/aaif-goose/goose, using-skills.md):
// ".agents/skills/ — Project-level skills, scoped to the current
// project"; a legacy `.goose/skills/`, `.claude/skills/`, and others
// are also discovered but not written here.
//
// Project agents emit as flat `.agents/agents/<name>.md` profiles with
// the portable name, description, model, and prompt body. OpenHands
// reads the same path and fields, so both adapters use one shared
// renderer and their writes dedupe byte-for-byte.
//
// Hooks emit as an Open Plugins package under
// `.agents/plugins/agnostic-ai/`: a required `plugin.json` manifest plus
// `hooks/hooks.json`. `outputs.goose.hooks-file` can move the hook file;
// the manifest follows at the parent plugin root.
//
// Skills are the plugin's other component: "A plugin can provide skills,
// hooks, or both", and "A plugin is a directory with a plugin manifest
// and optional component directories"
// (documentation/docs/guides/context-engineering/plugins.md). So
// `outputs.goose.skills-dir: .agents/plugins/<name>/skills` writes the
// same manifest, with or without a hook spec. Until this fix the
// manifest existed only as a side effect of emitting hooks, and a
// skills-only bundle was undiscoverable (target-audit 2026-09-18,
// #862). Goose namespaces a plugin's skills: "The `review` skill in
// `my-plugin` is loaded as `my-plugin:review`". One plugin carrying
// both components gets one manifest.
//
// Reviews emit plain bodies to .agents/REVIEW.md at the root and in each
// scope, with same-scope specs concatenated. goose review composes changed-
// file directories and their ancestors. outputs.goose.review-file overrides
// the path relative to each scope.
package goose

import (
	"path/filepath"
	"sort"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target           = "goose"
	defaultAgentsDir = ".agents/agents"
	defaultSkillsDir = ".agents/skills"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindRule, spec.KindSkill, spec.KindHook, spec.KindReview},
}

// Adapter emits Goose configs.
type Adapter struct{}

// New returns a Goose adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

func (Adapter) Capabilities() []spec.Kind { return caps.Supports }

// Emit writes one skill folder per skill spec under `.agents/skills/`,
// plus the legacy concatenated `.goosehints`-style document only when
// `outputs.goose.rules-file` is set, scoped to rules so native agent
// profiles never leak into the document. Root-scoped rules concatenate
// into that path unchanged; a
// rule carrying a source-layout or frontmatter scope concatenates into
// a sibling `<scope>/<basename>` file instead, matching Goose's own
// nested-discovery mechanism (see the package doc). The root AGENTS.md
// entry-point (with rule bodies inlined) is written centrally by
// `sync`, not here.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	agentsDir := emit.OutputAgentsDir(cfg, target, defaultAgentsDir)
	if err := sess.WriteSharedAgentFiles(b.Agents, target, agentsDir, dryRun); err != nil {
		return err
	}
	noteDroppedAgentTools(b.Agents)
	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)
	if err := sess.WriteSkillFolders(b.Skills, target, skillsDir, dryRun); err != nil {
		return err
	}
	root, scoped := splitRulesByScope(b.Rules)
	if err := sess.EmitLegacyRulesFile(spec.Bundle{Rules: root}, cfg, target, emit.MergedOpts{
		Title: "Project rules",
	}, dryRun); err != nil {
		return err
	}
	if err := emitScopedRulesFiles(sess, scoped, cfg, dryRun); err != nil {
		return err
	}
	hooksPlugin, err := emitHooks(sess, b.Hooks, cfg, dryRun)
	if err != nil {
		return err
	}
	if err := emitReviews(sess, b.Reviews, cfg, dryRun); err != nil {
		return err
	}
	var skillsPlugin string
	if len(b.Skills) > 0 {
		skillsPlugin = pluginSkillsRoot(skillsDir)
	}
	return emit.WritePluginManifests(sess, []string{hooksPlugin, skillsPlugin}, dryRun)
}

func noteDroppedAgentTools(agents []spec.Entry) {
	dropped := 0
	for _, agent := range agents {
		if emit.SharedAgentToolsDropped(agent, target) {
			dropped++
		}
	}
	emit.NoteFieldNoOp(target, spec.KindAgent, "tools", dropped,
		"Goose does not document a tools field for project agents")
}

// splitRulesByScope buckets rules into root-scoped (EffectiveScope() ==
// "") and a map keyed by every other scope, each preserving the rules'
// original relative order.
func splitRulesByScope(rules []spec.Entry) (root []spec.Entry, scoped map[string][]spec.Entry) {
	scoped = map[string][]spec.Entry{}
	for _, r := range rules {
		if s := r.EffectiveScope(); s != "" {
			scoped[s] = append(scoped[s], r)
			continue
		}
		root = append(root, r)
	}
	return root, scoped
}

// emitScopedRulesFiles writes one merged `.goosehints`-style document
// per non-root scope, into `<scope>/<basename>` where `<basename>` is
// the last path element of the configured (or default) rules-file, so
// a custom `outputs.goose.rules-file` still nests under the same name
// Goose looks for automatically. No-op when the rules-file opt-in is
// unset, so a scoped rule produces no output until the same opt-in
// EmitLegacyRulesFile itself requires for the root document (#608): the
// gate stays one on/off switch, not two.
func emitScopedRulesFiles(sess *emit.Session, byScope map[string][]spec.Entry, cfg *config.Config, dryRun bool) error {
	rulesFile := emit.OutputRulesFile(cfg, target, "")
	if rulesFile == "" || len(byScope) == 0 {
		return nil
	}
	base := filepath.Base(rulesFile)
	for _, scope := range sortedScopeKeys(byScope) {
		opts := emit.MergedOpts{
			OutFile: filepath.Join(scope, base),
			Title:   "Project rules",
			Intro:   "Generated by agnostic-ai.",
		}
		if err := sess.MergedDocument(spec.Bundle{Rules: byScope[scope]}, opts, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// sortedScopeKeys returns byScope's keys sorted for stable output order
// across runs.
func sortedScopeKeys(byScope map[string][]spec.Entry) []string {
	out := make([]string, 0, len(byScope))
	for k := range byScope {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
