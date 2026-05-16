// Package codex emits Codex CLI configs.
//
// The project-root `AGENTS.md` is written centrally by `sync` as a
// slim pointer to the source specs (one body shared with every other
// target's entry-point file). When `outputs.codex.rules-file` is set,
// this adapter instead writes the legacy concatenated layout at that
// path so users on older workflows keep their behavior.
//
// Agents emit as one TOML file per agent under .agents/agents/ (override
// via outputs.codex.agents-dir) following the Codex subagents schema.
// `.agents/` is the community-shared root for subagent definitions: skills
// live under `.agents/skills/<name>/`, agents under `.agents/agents/<name>.toml`.
//
// Skills emit as a folder per skill under .agents/skills/<name>/ (override
// via outputs.codex.skills-dir) per the Codex skills layout. Each folder
// contains a SKILL.md with `name` + `description` frontmatter plus the
// skill body. When the spec provides `x-codex.interface`, `x-codex.policy`,
// or `x-codex.dependencies`, an additional `agents/openai.yaml` is written
// alongside SKILL.md for UI customization and policy declarations.
//
// Codex's lifecycle hooks and MCP servers both live in
// `.codex/config.toml`. The hook engine (SessionStart, Stop,
// UserPromptSubmit, PreToolUse, PostToolUse, pre/post compact) emits
// as `[[hooks.<event>]]` array of tables; each MCP server emits as a
// `[mcp_servers.<name>]` table.
package codex

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target             = "codex"
	defaultAgentsDir   = ".agents/agents"
	defaultSkillsDir   = ".agents/skills"
	defaultCommandsDir = ".codex/prompts"
	defaultConfigFile  = ".codex/config.toml"
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
	// Codex consumes agents, rules, skills (the latter as listings),
	// commands (slash prompts under .codex/prompts/), plus lifecycle
	// hooks and MCP servers via .codex/config.toml.
	Supports: []spec.Kind{spec.KindAgent, spec.KindRule, spec.KindSkill, spec.KindHook, spec.KindMCP, spec.KindCommand},
}

// Adapter emits Codex configs.
type Adapter struct{}

// New returns a Codex adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one TOML per agent, one folder per skill, one prompt per
// command, the .codex/config.toml (hooks + MCP), and—when opted in via
// outputs.codex.rules-file—a legacy concatenated rules document. The
// project-root AGENTS.md is written by `sync`, not here.
func (Adapter) Emit(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}

	agentsDir := emit.OutputAgentsDir(cfg, target, defaultAgentsDir)
	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)

	for _, a := range b.Agents {
		path := filepath.Join(agentsDir, a.Name+".toml")
		if err := emit.WriteFile(path, emit.WithHeader(agentTOML(a), emit.FormatTOML), dryRun); err != nil {
			return err
		}
	}

	if codexEmitsSkills(cfg) {
		for _, s := range b.Skills {
			if err := emitSkill(s, skillsDir, dryRun); err != nil {
				return err
			}
		}
	}

	commandsDir := emit.OutputCommandsDir(cfg, target, defaultCommandsDir)
	for _, c := range b.Commands {
		path := filepath.Join(commandsDir, c.Name+".md")
		rmeta, rkeys := emit.ResolveMetaOrdered(c.Meta, c.MetaKeys, target)
		body := emit.FrontmatterOrdered(rmeta, rkeys) + "\n" + c.Body
		if err := emit.WriteFile(path, emit.WithHeader(body, emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
	}

	if err := emit.EmitLegacyRulesFile(b, cfg, target, emit.MergedOpts{Title: "AGENTS.md"}, dryRun); err != nil {
		return err
	}

	return emitConfigTOML(b, cfg, dryRun)
}

// codexEmitsSkills reports whether the codex adapter should write the
// `.agents/skills/<name>/` tree on this sync.
//
// The default depends on whether claude is also enabled:
//   - claude + codex: false (claude already owns skills at
//     `.claude/skills/`; `.agents/skills/` would just duplicate them
//     byte-for-byte and codex does not natively read that tree yet).
//   - codex alone: true (no other adapter writes skills, so codex emits
//     to the community `.agents/skills/` layout).
//
// Explicit `outputs.codex.shared-subagents` in `agnostic-ai.yaml` wins
// over the conditional default in both directions.
func codexEmitsSkills(cfg *config.Config) bool {
	if cfg == nil {
		return true
	}
	if o, ok := cfg.Outputs[target]; ok && o.SharedSubagents != nil {
		return *o.SharedSubagents
	}
	for _, t := range cfg.Targets {
		if t == "claude" {
			return false
		}
	}
	return true
}

// emitConfigTOML writes `.codex/config.toml` with the captured overlay,
// first-class config, hooks, and MCP servers when any content exists.
// The project-tier config.toml is agnostic-ai-managed: overwrite on each
// sync. The overlay (`.agnostic-ai/overlays/codex.config.toml`) carries
// every user-authored key outside hooks/mcp_servers so a wipe of
// `.codex/` between import and sync does not destroy them.
func emitConfigTOML(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	var codexCfg *config.CodexConfig
	if o, ok := cfg.Outputs[target]; ok {
		codexCfg = o.Config
	}
	overlay, overlayKeys, err := loadConfigOverlay(dryRun)
	if err != nil {
		return err
	}
	body := renderConfigTOML(b.Hooks, b.MCPs, codexCfg, overlay, overlayKeys)
	if body == "" {
		return nil
	}
	path := emit.OutputMCPFile(cfg, target, defaultConfigFile)
	return emit.WriteFile(path, body, dryRun)
}

// loadConfigOverlay returns the overlay body bytes and the set of
// top-level keys it defines. Returns ("", nil, nil) when the overlay is
// absent. Skips disk in dryRun so `--dry-run` previews remain pure.
func loadConfigOverlay(dryRun bool) (string, map[string]bool, error) {
	if dryRun {
		return "", nil, nil
	}
	data, err := os.ReadFile(configOverlayPath)
	if errors.Is(err, fs.ErrNotExist) {
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
