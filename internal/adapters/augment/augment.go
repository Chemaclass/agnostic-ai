// Package augment emits configs for Augment Code.
//
// Augment reads project instructions from the root `AGENTS.md` file,
// written centrally by `sync` as a slim pointer to the source specs
// with rule bodies inlined by default (one body shared with every
// other AGENTS.md-family target). Rules additionally emit natively as
// one file per rule at `.augment/rules/<name>.md` (override via
// outputs.augment.rules-dir): "Rules are files that live in the
// `.augment/rules` directory" (docs.augmentcode.com/setup-augment/
// guidelines, target-audit 2026-08-01). Frontmatter carries
// `type: agent_requested` plus a `description` (falls back to the rule
// name, since that type requires one) when the spec sets
// `alwaysApply: false`; the vendor default `type: always_apply` is
// left implicit by omitting the key. There is no `name` key for rules:
// Augment derives rule identity from the filename, same as every other
// AGENTS.md-family target's per-file surface.
//
// Augment also reads a second, older root file, `.augment-guidelines`:
// a single concatenated document of project guidelines. It stays
// opt-in via `outputs.augment.rules-file` for continuity, but the
// vendor's own precedence order truncates it first under budget
// pressure, which is why the native per-file directory above, not this
// document, is the adapter's default rules surface.
// `.augment-guidelines` reuses the same generic legacy-rules-file
// mechanism the zed, aider, warp, and antigravity adapters use for
// their own opt-in merged documents (`Session.EmitLegacyRulesFile`);
// there is no dedicated `guidelines-file` config key.
//
// Agents emit as one Markdown file per agent spec at
// `.augment/agents/<name>.md` (override via outputs.augment.agents-dir).
// Frontmatter carries `name` (required) plus `description` (falls back
// to the spec name), `color`, and `model` when the spec sets them.
// `tools` and `disabled_tools` are real Augment agent fields, but only
// in Augment's own tool vocabulary (`view`, `codebase-retrieval`,
// `str-replace-editor`, `save-file`, `remove-files`, `launch-process`,
// `github-api`, `web-fetch`, `web-search`), never Claude-style names
// (`Read`, `Grep`, `Bash`, ...). agnostic-ai's generic cross-tool
// `tools` field is always the latter, so this adapter never emits it
// under either key: doing so would silently restrict nothing, the same
// failure class as kilo's B2 (target-audit 2026-08-01). An agent that
// sets `tools` surfaces a coverage note instead of a silent no-op.
// `x-augment: {tools: [...]}` / `x-augment: {disabled_tools: [...]}`
// pass through verbatim: an author writing directly under the augment
// namespace is presumed to already be using Augment's own vocabulary,
// and that value may be either a YAML list or a comma-separated
// string, both vendor-documented, so passthrough never has to guess
// which.
//
// Skills emit into the shared `.agents/skills/<name>/SKILL.md` tree
// (override via outputs.augment.skills-dir), the same tree codex, amp,
// zed, crush, openhands, and windsurf already write byte-identically,
// so sync.shared-skills keeps deduping across it. Augment also reads
// `.claude/skills/` and `.augment/skills/` directly, but
// `.agents/skills/` covers it without a third on-disk copy: "Skills
// from all locations are loaded and made available to the agent", not
// first-match-wins. The one tradeoff: `.agents/skills/` is the lowest
// of Augment's six precedence slots, so a same-named skill a user
// hand-authors under `.augment/skills/` wins over the synced one; this
// only matters outside agnostic-ai's own managed tree.
//
// caps.Supports declares KindRule, KindAgent, and KindSkill. Hooks and
// MCP have no confirmed Augment surface yet and skip with a warning.
package augment

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target           = "augment"
	defaultRulesDir  = ".augment/rules"
	defaultAgentsDir = ".augment/agents"
	// defaultSkillsDir is the shared cross-tool skills tree Augment
	// scans (alongside .augment/skills/ and .claude/skills/); codex,
	// amp, zed, crush, openhands, and windsurf already write here, so
	// identical skill folders dedupe under sync.shared-skills.
	defaultSkillsDir = ".agents/skills"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindRule, spec.KindAgent, spec.KindSkill},
}

// Adapter emits Augment configs.
type Adapter struct{}

// New returns an Augment adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one `.augment/rules/<name>.md` per rule, one
// `.augment/agents/<name>.md` per agent, and one shared
// `.agents/skills/<name>/SKILL.md` folder per skill. The legacy
// concatenated `.augment-guidelines` document remains opt-in via
// `outputs.augment.rules-file`, scoped to rules only so an agent or
// skill body never leaks into it. The root AGENTS.md entry-point (with
// rule bodies inlined) is written centrally by `sync`, not here.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	if err := sess.EmitLegacyRulesFile(spec.Bundle{Rules: b.Rules}, cfg, target, emit.MergedOpts{
		Title: "Project guidelines",
	}, dryRun); err != nil {
		return err
	}
	if err := sess.RulesDirectory(b, emit.RulesDirOpts{
		Dir:        emit.OutputRulesDir(cfg, target, defaultRulesDir),
		FormatRule: func(e spec.Entry) string { return emit.WithHeader(ruleMarkdown(e), emit.FormatMarkdown) },
		// Agents and skills emit through their own native surfaces
		// below; a flattened rule-form copy would double-expose both.
		SkipAgents: true,
		SkipSkills: true,
	}, dryRun); err != nil {
		return err
	}
	agentsDir := emit.OutputAgentsDir(cfg, target, defaultAgentsDir)
	if err := emitAgents(sess, b.Agents, agentsDir, dryRun); err != nil {
		return err
	}
	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)
	return sess.WriteSkillFolders(b.Skills, target, skillsDir, dryRun)
}

// ruleMarkdown renders one `.augment/rules/<name>.md` file. `type`
// stays absent for the vendor default (`always_apply`); setting the
// spec's generic `alwaysApply: false` (the same field Cursor rules
// already use) switches it to `agent_requested`, which the vendor
// requires a `description` for, so one falls back to the rule name
// exactly like every other adapter's description fallback. An explicit
// `x-augment.type` still overrides either computed value, including to
// the IDE-only `manual` value the package doc calls out. `name` never
// emits: Augment has no such key for rules (see the package doc), so
// it stays excluded even through the x-augment escape hatch. `globs` /
// `alwaysApply` are Cursor's own spelling and have no Augment meaning,
// so neither is copied through.
func ruleMarkdown(e spec.Entry) string {
	resolved := emit.ResolveMeta(e.Meta, target)
	always := true
	if v, ok := resolved["alwaysApply"].(bool); ok {
		always = v
	}
	meta := map[string]any{}
	var keys []string
	if !always {
		meta["type"] = "agent_requested"
		keys = append(keys, "type")
	}
	desc, _ := resolved["description"].(string)
	if desc == "" && !always {
		desc = e.Name
	}
	if desc != "" {
		meta["description"] = desc
		keys = append(keys, "description")
	}
	// "type" is deliberately not excluded: an explicit x-augment.type
	// can still request the IDE-only `manual` value the package doc
	// calls out, or otherwise override the two values computed above.
	emit.MergeCustomTargetMeta(meta, &keys, e.Meta, target, "description", "name", "alwaysApply", "globs")
	front := emit.FrontmatterOrdered(meta, keys)
	body := strings.TrimSpace(e.Body)
	if front == "" {
		return body + "\n"
	}
	if body == "" {
		return front + "\n"
	}
	return front + "\n" + body + "\n"
}

// emitAgents writes one `<dir>/<name>.md` per agent spec. Agents whose
// spec declares a generic `tools` list get no restriction on Augment
// (see agentMarkdown), so the whole batch surfaces one coverage note
// instead of a silent drop.
func emitAgents(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	droppedTools := 0
	for _, a := range agents {
		path := filepath.Join(dir, a.Name+".md")
		md, dropped := agentMarkdown(a)
		if dropped {
			droppedTools++
		}
		if err := sess.WriteFile(path, emit.WithHeader(md, emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
	}
	emit.NoteFieldNoOp(target, spec.KindAgent, "tools", droppedTools,
		"Augment's tool names (view, codebase-retrieval, str-replace-editor, ...) differ from agnostic-ai's Claude-style tools; set x-augment.tools / x-augment.disabled_tools directly instead")
	return nil
}

// agentMarkdown renders a single `.augment/agents/<name>.md` file:
// `name` (required, from the spec name) and `description` (falls back
// to the spec name), plus `color` and `model` when the spec sets them,
// followed by the spec body as the agent's system prompt. `name` is
// excluded from the x-augment passthrough so it cannot diverge from
// the filename it also drives. `tools` / `disabled_tools` are excluded
// from the generic per-field copy (see the package doc): only an
// explicit x-augment.tools / x-augment.disabled_tools override reaches
// the frontmatter, via the MergeCustomTargetMeta passthrough below.
// droppedTools reports whether the spec declared the generic `tools`
// field with no x-augment override to rescue it, so the caller can
// fold every such agent into one coverage note per sync.
func agentMarkdown(a spec.Entry) (body string, droppedTools bool) {
	resolved := emit.ResolveMeta(a.Meta, target)
	desc, _ := resolved["description"].(string)
	if desc == "" {
		desc = a.Name
	}
	meta := map[string]any{
		"name":        a.Name,
		"description": desc,
	}
	keys := []string{"name", "description"}
	for _, k := range []string{"color", "model"} {
		if v, ok := resolved[k]; ok {
			meta[k] = v
			keys = append(keys, k)
		}
	}
	emit.MergeCustomTargetMeta(meta, &keys, a.Meta, target, "name", "description", "color", "model")
	front := emit.FrontmatterOrdered(meta, keys)
	droppedTools = len(emit.StringSlice(a.Meta["tools"])) > 0 && !xAugmentSetsTools(a.Meta)
	trimmed := strings.TrimSpace(a.Body)
	if trimmed == "" {
		return front + "\n", droppedTools
	}
	return front + "\n" + trimmed + "\n", droppedTools
}

// xAugmentSetsTools reports whether the spec already carries an
// explicit x-augment.tools or x-augment.disabled_tools override: the
// one channel this adapter trusts to already be Augment's own tool
// vocabulary rather than agnostic-ai's generic Claude-style names.
func xAugmentSetsTools(meta map[string]any) bool {
	x, _ := emit.CustomTargetMeta(meta, target)
	if x == nil {
		return false
	}
	_, tools := x["tools"]
	_, disabledTools := x["disabled_tools"]
	return tools || disabledTools
}
