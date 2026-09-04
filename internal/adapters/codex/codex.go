// Package codex emits Codex CLI configs.
//
// The project-root `AGENTS.md` is written centrally by `sync` as a
// slim pointer to the source specs (one body shared with every other
// target's entry-point file). When `outputs.codex.rules-file` is set,
// this adapter instead writes the legacy concatenated layout at that
// path so users on older workflows keep their behavior.
//
// Agents emit as one TOML file per agent under .codex/agents/ (override
// via outputs.codex.agents-dir) following the Codex subagents schema.
// .codex/ is Codex CLI's native lookup root; set agents-dir to
// .agents/agents for the community shared subagent layout.
//
// Skills emit as a folder per skill under .agents/skills/<name>/ (override
// via outputs.codex.skills-dir), the path Codex CLI scans from the cwd up
// to the repo root. Each folder contains a SKILL.md with `name` +
// `description` frontmatter plus the skill body. When the spec provides
// `x-codex.interface`, `x-codex.policy`, or `x-codex.dependencies`, an
// additional `agents/openai.yaml` is written alongside SKILL.md for UI
// customization and policy declarations. Amp reads the same layout at the
// same path; identical bytes dedupe, so co-enabling both is safe.
//
// Codex MCP servers live in `.codex/config.toml` (one
// `[mcp_servers.<name>]` table each). Lifecycle hooks (SessionStart,
// Stop, UserPromptSubmit, PreToolUse, PostToolUse, pre/post compact)
// emit to `.codex/hooks.json`, which preserves matcher metadata the
// inline `[[hooks.<event>]]` TOML form cannot. Override the hooks path
// via `outputs.codex.hooks-file`.
//
// An MCP spec's `disabled: true` maps to Codex's own `enabled = false`
// key (`learn.chatgpt.com/docs/config-file/config-reference` documents
// `mcp_servers.<id>.enabled: boolean`, not `disabled`), the one target
// where this field genuinely works. Stdio servers also accept `cwd`
// (working directory, part of the cross-tool MCP spec); http servers
// accept `auth` (`oauth` | `chatgpt`) and `http_headers_helper`
// ("Supported only for locally connected HTTP MCP servers"), read
// verbatim from the spec's top-level fields the same way
// `bearer_token_env_var` already is. `enabled_tools` and
// `disabled_tools` carry no transport restriction and pass through on
// any server (#661). Codex-specific `env_vars` and `env_http_headers`
// retain the vendor's mixed-array and environment-backed header shapes.
//
// Hook entries accept an optional `additionalContextLimit` (token
// threshold for how much hook output reaches the model), propagated
// into `.codex/hooks.json` the same way `commandWindows` already is.
// An explicit zero is preserved because Codex gives it distinct behavior.
// `async` (run the command hook in the background instead of blocking
// the session on it) propagates the same way; `import codex` reads it
// back from both `.codex/hooks.json` and a hand-authored
// `[[hooks.<event>]]` TOML block.
package codex

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target            = "codex"
	defaultAgentsDir  = ".codex/agents"
	defaultSkillsDir  = ".agents/skills"
	defaultConfigFile = ".codex/config.toml"
	// legacyAgentsDir is the pre-v0.26 agents default under `.agents/`.
	// legacySkillsDir is the v0.26..v0.42 skills default: Codex CLI
	// only scans `.agents/skills`, so `.codex/skills` was a dead path.
	// legacyCommandsDir is the pre-v0.43 prompts default: Codex reads
	// custom prompts only from `~/.codex/prompts` and deprecates them
	// in favor of skills, so the project-level copy was never loaded.
	// Sync sweeps each legacy tree after emitting to the current
	// default so user projects do not carry stale agnostic-ai-managed
	// copies at two paths. The sweep only fires when the active path
	// differs (i.e. the user did not opt into that path explicitly via
	// the matching `outputs.codex.*` key).
	legacyAgentsDir   = ".agents/agents"
	legacySkillsDir   = ".codex/skills"
	legacyCommandsDir = ".codex/prompts"
	// configOverlayPath is the project-relative path to the captured
	// non-hooks/non-mcp portion of `.codex/config.toml`. `agnostic-ai
	// import codex` writes this file; the emitter prepends it before
	// the spec-derived hooks + MCP sections so a re-sync from a fresh
	// checkout still carries the user's first-class Codex keys (model,
	// sandbox, profiles, model_providers, history, notify, ...).
	configOverlayPath = ".agnostic-ai/overlays/codex.config.toml"
)

var caps = emit.Capabilities{
	Target: target,
	// Codex consumes agents, rules, skills, MCP servers via
	// .codex/config.toml, and lifecycle hooks via .codex/hooks.json.
	// Commands stay declared-supported but emit only behind
	// outputs.codex.commands-dir: Codex loads custom prompts from
	// ~/.codex/prompts only and deprecates them in favor of skills, so
	// a project-level prompts tree would never be read.
	Supports: []spec.Kind{spec.KindAgent, spec.KindRule, spec.KindSkill, spec.KindHook, spec.KindMCP, spec.KindCommand},
}

// Adapter emits Codex configs.
type Adapter struct{}

// New returns a Codex adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one TOML per agent, one folder per skill,
// .codex/config.toml (MCP), .codex/hooks.json (hooks), and—when opted
// in via outputs.codex.rules-file—a legacy concatenated rules document.
// Commands emit only when outputs.codex.commands-dir is set (Codex
// deprecated custom prompts and never reads a project-level tree). The
// project-root AGENTS.md is written by `sync`, not here.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}

	agentsDir := emit.OutputAgentsDir(cfg, target, defaultAgentsDir)
	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)

	droppedAgentTools := 0
	for _, a := range b.Agents {
		path := filepath.Join(agentsDir, a.Name+".toml")
		if err := sess.WriteFile(path, emit.WithHeader(agentTOML(a), emit.FormatTOML), dryRun); err != nil {
			return err
		}
		if len(emit.StringSlice(a.Meta["tools"])) > 0 {
			droppedAgentTools++
		}
	}
	emit.NoteFieldNoOp(target, spec.KindAgent, "tools", droppedAgentTools,
		"Codex uses tools as a configuration table, not a Claude-style allowlist; set x-codex.tools for Codex-native tool settings")

	if codexEmitsSkills(cfg) {
		for _, s := range b.Skills {
			if err := emitSkill(sess, s, skillsDir, dryRun); err != nil {
				return err
			}
		}
	}

	commandsDir := emit.OutputCommandsDir(cfg, target, "")
	if commandsDir != "" {
		for _, c := range b.Commands {
			path := filepath.Join(commandsDir, c.Name+".md")
			rmeta, rkeys := emit.ResolveMetaOrdered(c.Meta, c.MetaKeys, target)
			body := emit.FrontmatterStyled(rmeta, rkeys, c.MetaStyles) + "\n" + c.Body
			if err := sess.WriteFile(path, emit.WithHeader(body, emit.FormatMarkdown), dryRun); err != nil {
				return err
			}
		}
	} else {
		emit.NoteCoverageGap(target, spec.KindCommand, len(b.Commands),
			"Codex loads custom prompts from ~/.codex/prompts only and deprecates them for skills")
	}

	if err := sess.EmitLegacyRulesFile(b, cfg, target, emit.MergedOpts{Title: "AGENTS.md"}, dryRun); err != nil {
		return err
	}

	if err := emitConfigTOML(sess, b, cfg, dryRun); err != nil {
		return err
	}
	if err := sweepLegacyTrees(sess, agentsDir, skillsDir, commandsDir, dryRun); err != nil {
		return err
	}
	hooks := b.HooksFor(target)
	if err := emitHooksJSON(sess, hooks, cfg, dryRun); err != nil {
		return err
	}
	if err := emitExecPolicies(sess, cfg, dryRun); err != nil {
		return err
	}
	if err := materializeHookScripts(hooks, dryRun); err != nil {
		return err
	}
	return sess.RestoreHelperFiles(target, dryRun)
}

// sweepLegacyTrees removes agnostic-ai-managed leftovers from prior
// codex default layouts when the user is now emitting to the current
// defaults (or any other path that differs from the legacy ones):
// pre-v0.26 agents under `.agents/agents`, v0.26..v0.42 skills under
// `.codex/skills` (a path Codex CLI never scans), and pre-v0.43
// prompts under `.codex/prompts` (Codex reads prompts from
// `~/.codex/prompts` only). Files without the agnostic-ai header are
// left untouched so hand-authored content survives the sweep. Each
// sweep is skipped when the active path matches the legacy one, since
// that means the user explicitly opted into that layout.
func sweepLegacyTrees(sess *emit.Session, agentsDir, skillsDir, commandsDir string, dryRun bool) error {
	if agentsDir != legacyAgentsDir {
		// Scoped to .toml: antigravity's vendor-documented subagent path
		// is this same directory, and its `<name>.md` files carry the
		// same provenance header a whole-tree sweep deletes on (#638).
		// Codex agents are TOML at both the native and the legacy path,
		// so nothing codex-managed escapes this filter.
		if err := sess.RemoveGeneratedTreeExt(legacyAgentsDir, ".toml", dryRun); err != nil {
			return err
		}
	}
	if skillsDir != legacySkillsDir {
		if err := sess.RemoveGeneratedTree(legacySkillsDir, dryRun); err != nil {
			return err
		}
	}
	if commandsDir != legacyCommandsDir {
		if err := sess.RemoveGeneratedTree(legacyCommandsDir, dryRun); err != nil {
			return err
		}
	}
	// Keep the shared `.agents/` parent even when the legacy agents tree
	// was its last child. Other adapters write `.agents/rules`, skills,
	// and commands concurrently, so removing the parent races with their
	// directory creation. RemoveGeneratedTree already removes the
	// target-owned `.agents/agents` subtree completely.
	return nil
}

// materializeHookScripts copies each hook's stashed script body from
// `.agnostic-ai/scripts/` into `.codex/hooks/` so the emitted
// config.toml has the actual script alongside the path it references.
func materializeHookScripts(hooks []spec.Entry, dryRun bool) error {
	for _, h := range hooks {
		cmds := hookCommands(h.Meta["command"])
		for _, raw := range cmds {
			sourceTool, _ := emit.SourceToolFromHookCommand(raw)
			rewritten := emit.RewriteHookPath(raw, target)
			if err := emit.MaterializeHookScript(rewritten, target, sourceTool, dryRun); err != nil {
				return err
			}
		}
	}
	return nil
}

// codexEmitsSkills reports whether the codex adapter should write the
// per-skill tree on this sync. Defaults to true so codex picks up the
// emitted skills at its native `.agents/skills/<name>/` lookup path.
//
// Amp emits the same SKILL.md layout to the same default path. Both
// renderers produce identical bytes for a spec without per-target
// overrides, so the double write dedupes; a spec with divergent
// `x-codex`/`x-amp` overrides surfaces through the collision check.
// Users who explicitly want to skip codex skill emission opt out with
// `outputs.codex.shared-subagents: false`.
func codexEmitsSkills(cfg *config.Config) bool {
	if cfg == nil {
		return true
	}
	if o, ok := cfg.Outputs[target]; ok && o.SharedSubagents != nil {
		return *o.SharedSubagents
	}
	return true
}

// emitConfigTOML writes `.codex/config.toml` with the captured overlay,
// first-class config, hooks, and MCP servers when any content exists.
// The project-tier config.toml is agnostic-ai-managed: overwrite on each
// sync. The overlay (`.agnostic-ai/overlays/codex.config.toml`) carries
// every user-authored key outside hooks/mcp_servers so a wipe of
// `.codex/` between import and sync does not destroy them.
func emitConfigTOML(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	var codexCfg *config.CodexConfig
	if o, ok := cfg.Outputs[target]; ok {
		codexCfg = o.Config
	}
	overlay, overlayKeys, err := loadConfigOverlay(dryRun)
	if err != nil {
		return err
	}
	body := renderConfigTOML(b.HooksFor(target), b.MCPs, codexCfg, overlay, overlayKeys)
	path := emit.OutputMCPFile(cfg, target, defaultConfigFile)
	if body == "" {
		// Nothing to render this sync: a prior sync may have left a stale
		// agnostic-ai-managed config.toml on disk (e.g. the user removed
		// the last MCP/hook/overlay). Clean it up so target files never
		// drift from the source specs.
		return sess.RemoveGenerated(path, dryRun)
	}
	return sess.WriteFile(path, body, dryRun)
}

// loadConfigOverlay returns the overlay body bytes and the set of
// top-level keys it defines. Returns ("", nil, nil) when the overlay is
// absent. Skips disk in dryRun so `--dry-run` previews remain pure.
func loadConfigOverlay(dryRun bool) (string, map[string]bool, error) {
	if dryRun {
		return "", nil, nil
	}
	data, err := os.ReadFile(configOverlayPath)
	if emit.IsAbsent(err) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, fmt.Errorf("read %s: %w", configOverlayPath, err)
	}
	doc := map[string]any{}
	if _, err := toml.Decode(string(data), &doc); err != nil {
		return "", nil, fmt.Errorf("parse %s: %w", configOverlayPath, err)
	}
	keys := make(map[string]bool, len(doc))
	for k := range doc {
		keys[k] = true
	}
	return string(data), keys, nil
}
