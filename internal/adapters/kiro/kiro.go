// Package kiro emits steering files, native agent profiles, hook
// definitions, and MCP config for AWS Kiro.
//
// Kiro loads Markdown steering documents from `.kiro/steering/`. Every
// file starts with a YAML frontmatter block (it must be the first
// content in the file, no blank line before it) whose `inclusion` key
// picks one of three loading modes:
//
//   - `always`: loaded on every interaction. Used for rules with no
//     glob or scope to target.
//   - `fileMatch` (+ `fileMatchPattern`): loaded when the active file
//     matches the pattern. Used for rules that carry `globs` or a
//     source-layout scope.
//
// Skills also become steering files, using a fourth mode Kiro reserves
// for skill-like matching:
//
//   - `auto` (+ `name`, `description`): Kiro matches the name and
//     description against the user's request and loads the file when
//     it looks relevant, mirroring skill semantics.
//
// A skill with bundled sibling assets cannot carry them in a flat
// steering file; those skills surface a coverage note instead of
// silently dropping the assets.
//
// Agents are a native Kiro surface, not a steering-file convention: one
// YAML-frontmatter Markdown file per agent at `.kiro/agents/<name>.md`,
// the tree Kiro's own agent picker reads (kiro.dev/docs/custom-agents/:
// "Workspace-level: `.kiro/agents/` ... Configuration lives in YAML
// frontmatter, your system prompt is the document body"). `description`
// (falls back to the agent's name) and `model` pass through; the full
// documented field set also includes `tools`, `mcpServers`,
// `permissions`, `hooks`, `keyboardShortcut`, and `welcomeMessage`
// (kiro.dev/docs/cli/custom-agents/configuration-reference/), of which
// only `tools` has an agnostic-ai spec equivalent. This adapter does not
// emit `tools`: the vendor doc names the field but never enumerates
// Kiro's own tool-identifier vocabulary, so there is no confirmed
// mapping from agnostic-ai's Claude-style tool names onto it, the same
// unconfirmed-vocabulary failure class already documented for Kilo Code
// and Augment. An agent that sets `tools` surfaces a coverage note
// instead of a silent no-op. `name` is never written: it is absent from
// Kiro's own field list, so identity comes from the filename, the same
// convention every per-agent surface with no documented `name` key
// uses. Arbitrary `x-kiro` keys pass through verbatim, so an author who
// already knows Kiro's own vocabulary can set `tools`, `mcpServers`,
// `permissions`, `hooks`, `keyboardShortcut`, or `welcomeMessage`
// directly. A prior version of this adapter flattened agents into
// `.kiro/steering/agent-<name>.md` with `inclusion: manual`; that path
// never reached Kiro's agent picker and dropped every field steering
// has no key for, so this adapter now sweeps any such file a prior sync
// left behind for a current agent name.
//
// Hooks are also native: one JSON file per hook spec at
// `.kiro/hooks/<name>.json` (kiro.dev/docs/hooks/: "Hooks are JSON
// files stored in `.kiro/hooks/` at the workspace level"), each holding
// `{"version": 1, "hooks": [...]}`. A hook entry carries `name`,
// `trigger` (the spec's `event`, passed through verbatim like every
// other adapter's hook event), an optional `matcher`, an `action`
// object, and an optional `timeout`. A spec's `command:` (string or
// list) always renders `action: {"type": "command", "command": ...}`; a
// list produces one entry per command in the same file, `name` suffixed
// `-2`, `-3`, ... to stay unique. Kiro also documents an `{"type":
// "agent", "prompt": ...}` action that invokes an agent instead of a
// shell command; agnostic-ai's hook spec has no generic prompt field,
// so this adapter never emits that shape. `disabled: true` on the spec
// writes `"enabled": false` (the vendor default, enabled, needs no
// explicit key), mirroring the `disabled`/`enabled` convention already
// used for MCP entries. Unlike Claude Code, Codex, Gemini, and Cursor,
// this adapter does not materialize stashed hook scripts from
// `.agnostic-ai/scripts/` into `.kiro/hooks/`: that directory is where
// Kiro looks for hook definitions, and there is no vendor confirmation
// that a plain script file living alongside them is safe.
//
// MCP servers write to `.kiro/settings/mcp.json` as a `mcpServers` map
// with `command`, `args`, and optional `env` per local server.
//
// The root `AGENTS.md` entry-point (which Kiro reads directly and
// always includes) is written centrally by `sync`, not by this
// adapter.
package kiro

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target             = "kiro"
	defaultSteeringDir = ".kiro/steering"
	defaultAgentsDir   = ".kiro/agents"
	defaultHooksDir    = ".kiro/hooks"
	defaultMCPFile     = ".kiro/settings/mcp.json"
	// legacyAgentPrefix names the flattened steering file this adapter
	// used to write per agent before agents moved to their native
	// `.kiro/agents/` surface (see the package doc). Kept only so
	// emitAgents can sweep away a stale file of this shape left behind
	// by an older sync.
	legacyAgentPrefix   = "agent-"
	skillFilenamePrefix = "skill-"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindMCP, spec.KindHook},
}

// Adapter emits AWS Kiro configs.
type Adapter struct{}

// New returns a Kiro adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one steering file per rule and skill, one native agent
// profile per agent, one hook definition file per hook, plus
// `.kiro/settings/mcp.json` when MCP entries exist.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	dir := emit.OutputRulesDir(cfg, target, defaultSteeringDir)
	if err := emitRules(sess, b.Rules, dir, dryRun); err != nil {
		return err
	}
	agentsDir := emit.OutputAgentsDir(cfg, target, defaultAgentsDir)
	if err := emitAgents(sess, b.Agents, agentsDir, dir, dryRun); err != nil {
		return err
	}
	if err := emitSkills(sess, b.Skills, dir, dryRun); err != nil {
		return err
	}
	hooksDir := emit.OutputHooksDir(cfg, target, defaultHooksDir)
	if err := emitHooks(sess, b.HooksFor(target), hooksDir, dryRun); err != nil {
		return err
	}
	return sess.WriteMCPFile(b.MCPs, emit.MCPSchemaServersMap,
		emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// emitRules writes one `<dir>/<name>.md` per rule. Rules that target a
// glob or a source-layout scope render `inclusion: fileMatch`;
// everything else renders `inclusion: always`.
func emitRules(sess *emit.Session, rules []spec.Entry, dir string, dryRun bool) error {
	for _, r := range rules {
		path := filepath.Join(dir, r.Name+".md")
		body := emit.WithHeader(renderRule(r), emit.FormatMarkdown)
		if err := sess.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// emitAgents writes one native `<agentsDir>/<name>.md` per agent (see
// agentMarkdown) and sweeps the legacy flattened steering file at
// `<steeringDir>/agent-<name>.md` a prior sync may have left behind for
// the same name. Agents whose spec declares a generic `tools` list get
// no restriction on Kiro, so the whole batch surfaces one coverage note
// instead of a silent drop.
func emitAgents(sess *emit.Session, agents []spec.Entry, agentsDir, steeringDir string, dryRun bool) error {
	droppedTools := 0
	for _, a := range agents {
		path := filepath.Join(agentsDir, a.Name+".md")
		md, dropped := agentMarkdown(a)
		if dropped {
			droppedTools++
		}
		if err := sess.WriteFile(path, emit.WithHeader(md, emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
		legacy := filepath.Join(steeringDir, legacyAgentPrefix+a.Name+".md")
		if err := sess.RemoveGenerated(legacy, dryRun); err != nil {
			return err
		}
	}
	emit.NoteFieldNoOp(target, spec.KindAgent, "tools", droppedTools,
		"Kiro's own tool-identifier vocabulary is unconfirmed against agnostic-ai's Claude-style names; set x-kiro.tools directly once you know Kiro's own names")
	return nil
}

// emitSkills writes one `<dir>/skill-<name>.md` per skill with
// `inclusion: auto`, `name`, and `description`, the mode Kiro matches
// against user requests. Folder-based skills that carry sibling assets
// beyond SKILL.md surface a coverage note, since a flat steering file
// cannot represent bundled files.
func emitSkills(sess *emit.Session, skills []spec.Entry, dir string, dryRun bool) error {
	withAssets := 0
	for _, s := range skills {
		path := filepath.Join(dir, skillFilenamePrefix+s.Name+".md")
		body := emit.WithHeader(renderSkill(s), emit.FormatMarkdown)
		if err := sess.WriteFile(path, body, dryRun); err != nil {
			return err
		}
		if emit.SkillHasBundledAssets(s, emit.SkipSKILLMd) {
			withAssets++
		}
	}
	emit.NoteCoverageGap(target, spec.KindSkill, withAssets, "bundled assets stay in the source dir")
	return nil
}

// renderRule renders a rule's steering-file body: frontmatter first,
// then a blank line, then the spec body.
func renderRule(e spec.Entry) string {
	front, keys := ruleFrontmatter(e)
	return withFrontmatter(front, keys, e.Body)
}

// ruleFrontmatter picks `inclusion: fileMatch` (with `fileMatchPattern`)
// for a rule that targets a glob or a source-layout scope, otherwise
// `inclusion: always`.
func ruleFrontmatter(e spec.Entry) (map[string]any, []string) {
	if pattern := fileMatchPatternFor(e); pattern != "" {
		return map[string]any{
			"inclusion":        "fileMatch",
			"fileMatchPattern": pattern,
		}, []string{"inclusion", "fileMatchPattern"}
	}
	return map[string]any{"inclusion": "always"}, []string{"inclusion"}
}

// fileMatchPatternFor returns the fileMatchPattern glob for a rule.
// Explicit `globs` (resolved for target-specific overrides) wins;
// otherwise the source-layout scope (e.g. `rules/backend/auth.md` ->
// "backend/**"); otherwise "" (the rule has nothing to scope to and
// loads always).
func fileMatchPatternFor(e spec.Entry) string {
	m := emit.ResolveMeta(e.Meta, target)
	if g, _ := m["globs"].(string); g != "" {
		return g
	}
	if s := e.EffectiveScope(); s != "" {
		return s + "/**"
	}
	return ""
}

// agentMarkdown renders a single `.kiro/agents/<name>.md` file:
// `description` (falls back to the spec name) and optional `model`,
// plus arbitrary x-kiro passthrough (mcpServers, permissions, hooks,
// keyboardShortcut, welcomeMessage, or an explicit tools override
// already in Kiro's own vocabulary), followed by the spec body as the
// agent's system prompt. Kiro's agent schema has no `name` key, so
// identity comes from the filename; `name` and `model` are excluded
// from the x-kiro passthrough merge below only because they are already
// handled by hand above (excluding them here just prevents emitting the
// same key twice, not a ban on x-kiro overriding model: ResolveMeta
// already flattens x-kiro.model onto the value this function reads).
// `tools` is deliberately not read from the resolved meta: agnostic-ai's
// generic `tools` field is Claude-style and Kiro's own vocabulary is
// unconfirmed (see the package doc), so only an explicit `x-kiro.tools`
// override reaches the frontmatter, via the same passthrough merge.
// droppedTools reports whether the spec declared the generic `tools`
// field with no x-kiro override to rescue it, so the caller can fold
// every such agent into one coverage note per sync.
func agentMarkdown(a spec.Entry) (body string, droppedTools bool) {
	resolved := emit.ResolveMeta(a.Meta, target)
	desc, _ := resolved["description"].(string)
	if desc == "" {
		desc = a.Name
	}
	meta := map[string]any{"description": desc}
	keys := []string{"description"}
	if model, _ := resolved["model"].(string); model != "" {
		meta["model"] = model
		keys = append(keys, "model")
	}
	emit.MergeCustomTargetMeta(meta, &keys, a.Meta, target, "name", "description", "model")
	front := emit.FrontmatterOrdered(meta, keys)
	droppedTools = len(emit.StringSlice(a.Meta["tools"])) > 0 && !xKiroSetsTools(a.Meta)
	trimmed := strings.TrimSpace(a.Body)
	if trimmed == "" {
		return front + "\n", droppedTools
	}
	return front + "\n" + trimmed + "\n", droppedTools
}

// xKiroSetsTools reports whether the spec already carries an explicit
// x-kiro.tools override: the one channel this adapter trusts to already
// be Kiro's own tool vocabulary rather than agnostic-ai's generic
// Claude-style names.
func xKiroSetsTools(meta map[string]any) bool {
	x, _ := emit.CustomTargetMeta(meta, target)
	if x == nil {
		return false
	}
	_, tools := x["tools"]
	return tools
}

// renderSkill renders a skill's steering-file body with
// `inclusion: auto`, `name`, and `description`, so Kiro auto-matches it
// against user requests the way it would a skill. Description falls
// back to the skill's name when the spec has none.
func renderSkill(e spec.Entry) string {
	m := emit.ResolveMeta(e.Meta, target)
	desc, _ := m["description"].(string)
	if desc == "" {
		desc = e.Name
	}
	front := map[string]any{
		"inclusion":   "auto",
		"name":        e.Name,
		"description": desc,
	}
	return withFrontmatter(front, []string{"inclusion", "name", "description"}, e.Body)
}

// withFrontmatter joins a rendered frontmatter block with body,
// separated by a blank line, in the shape Kiro requires: frontmatter
// as the very first bytes of the file.
func withFrontmatter(front map[string]any, keys []string, body string) string {
	var b strings.Builder
	b.WriteString(emit.FrontmatterOrdered(front, keys))
	b.WriteString("\n")
	b.WriteString(body)
	return b.String()
}
