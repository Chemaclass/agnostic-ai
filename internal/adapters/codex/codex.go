// Package codex emits AGENTS.md hierarchies, .agents/agents/*.toml files,
// and .agents/skills/<name>/ skill folders for the Codex CLI.
//
// The Codex CLI reads AGENTS.md for project conventions and supports
// nested AGENTS.md files in subdirectories that scope to that subtree.
// This adapter routes rules by their globs frontmatter:
//
//   - "src/**"        -> src/AGENTS.md
//   - "docs/api/**"   -> docs/api/AGENTS.md
//   - "**/*"          -> root
//   - "**/*.go"       -> root (no fixed prefix)
//
// Agents emit as one TOML file per agent under .agents/agents/ (override
// via outputs.codex.agents-dir) following the Codex subagents schema.
// `.agents/` is the community-shared root for subagent definitions: skills
// live under `.agents/skills/<name>/`, agents under `.agents/agents/<name>.toml`.
// The root AGENTS.md keeps a `## Agents` reference section listing each
// agent name, description, and source TOML path so humans browsing the
// document still see what is available.
//
// Skills emit as a folder per skill under .agents/skills/<name>/ (override
// via outputs.codex.skills-dir) per the Codex skills layout. Each folder
// contains a SKILL.md with `name` + `description` frontmatter plus the
// skill body. When the spec provides `x-codex.interface`, `x-codex.policy`,
// or `x-codex.dependencies`, an additional `agents/openai.yaml` is written
// alongside SKILL.md for UI customization and policy declarations.
//
// Codex's lifecycle hooks and MCP servers both live in
// `.codex/config.toml`, written separately from AGENTS.md. The hook
// engine (SessionStart, Stop, UserPromptSubmit, PreToolUse,
// PostToolUse, pre/post compact) emits as `[[hooks.<event>]]` array of
// tables; each MCP server emits as a `[mcp_servers.<name>]` table.
package codex

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target             = "codex"
	defaultOutFile     = "AGENTS.md"
	defaultAgentsDir   = ".agents/agents"
	defaultSkillsDir   = ".agents/skills"
	defaultCommandsDir = ".codex/prompts"
	defaultConfigFile  = ".codex/config.toml"
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

// Emit writes the root AGENTS.md, any nested AGENTS.md files implied by
// rule globs, and one .agents/agents/<name>.toml per agent.
func (Adapter) Emit(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}

	rootFile := emit.OutputFile(cfg, target, defaultOutFile)
	rootDir := filepath.Dir(rootFile)
	rootBase := filepath.Base(rootFile)
	agentsDir := emit.OutputAgentsDir(cfg, target, defaultAgentsDir)
	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)

	for _, a := range b.Agents {
		path := filepath.Join(agentsDir, a.Name+".toml")
		if err := emit.WriteFile(path, emit.WithHeader(agentTOML(a), emit.FormatTOML), dryRun); err != nil {
			return err
		}
	}

	for _, s := range b.Skills {
		if err := emitSkill(s, skillsDir, dryRun); err != nil {
			return err
		}
	}

	commandsDir := emit.OutputCommandsDir(cfg, target, defaultCommandsDir)
	for _, c := range b.Commands {
		path := filepath.Join(commandsDir, c.Name+".md")
		body := emit.Frontmatter(emit.ResolveMeta(c.Meta, target)) + "\n" + c.Body
		if err := emit.WriteFile(path, emit.WithHeader(body, emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
	}

	byDir := emit.GroupRulesByScope(b.Rules)

	dirs := slices.Sorted(maps.Keys(byDir))
	if !slices.Contains(dirs, "") && (len(b.Agents) > 0 || len(b.Skills) > 0) {
		dirs = append([]string{""}, dirs...)
	}

	for _, dir := range dirs {
		var sb strings.Builder
		writeHeader(&sb, dir)
		writeRules(&sb, byDir[dir])
		if dir == "" {
			writeAgents(&sb, b.Agents, agentsDir)
			writeSkills(&sb, b.Skills, skillsDir)
		}
		path := filepath.Join(rootDir, dir, rootBase)
		if err := emit.WriteFile(path, sb.String(), dryRun); err != nil {
			return err
		}
	}

	return emitConfigTOML(b, cfg, dryRun)
}

// emitConfigTOML writes `.codex/config.toml` with hooks and MCP servers
// when either is present. Codex's project-tier config.toml is
// agnostic-ai-managed: overwrite on each sync. Users who want to add
// non-managed Codex config keys should set them in the user-global
// `~/.codex/config.toml` instead.
func emitConfigTOML(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	body := renderConfigTOML(b.Hooks, b.MCPs)
	if body == "" {
		return nil
	}
	path := emit.OutputMCPFile(cfg, target, defaultConfigFile)
	return emit.WriteFile(path, body, dryRun)
}

func writeHeader(sb *strings.Builder, dir string) {
	if dir == "" {
		sb.WriteString("# AGENTS.md\n\n")
	} else {
		sb.WriteString("# AGENTS.md (" + dir + ")\n\n")
		sb.WriteString("Scoped to `" + dir + "/**`. Inherits root rules.\n\n")
	}
	sb.WriteString("Generated by agnostic-ai.\n\n")
}

func writeRules(sb *strings.Builder, rules []spec.Entry) {
	if len(rules) == 0 {
		return
	}
	sb.WriteString("## Conventions\n\n")
	for _, r := range rules {
		emit.WriteSection(sb, r.Name, r)
	}
}

// writeAgents renders a reference listing of agents in AGENTS.md. The
// real definitions live in <agentsDir>/<name>.toml; this section just
// helps humans see what is available without reading the TOML files.
func writeAgents(sb *strings.Builder, agents []spec.Entry, agentsDir string) {
	if len(agents) == 0 {
		return
	}
	sb.WriteString("## Agents\n\n")
	sb.WriteString("Custom Codex subagents. Definitions live in `" + agentsDir + "/`.\n\n")
	for _, a := range agents {
		sb.WriteString("### " + a.Name + "\n\n")
		sb.WriteString(emit.SourceComment(a.Path))
		if d := a.Description(); d != "" {
			sb.WriteString("_" + d + "_\n\n")
		}
		sb.WriteString("Source: `" + filepath.ToSlash(filepath.Join(agentsDir, a.Name+".toml")) + "`\n\n")
	}
}

// writeSkills renders a reference listing of skills in AGENTS.md. Each
// skill emits as its own folder under skillsDir per the Codex skills
// layout; this section just helps humans see what is available without
// browsing the directory.
func writeSkills(sb *strings.Builder, skills []spec.Entry, skillsDir string) {
	if len(skills) == 0 {
		return
	}
	sb.WriteString("## Skills\n\n")
	sb.WriteString("Codex skills. Definitions live in `" + skillsDir + "/<name>/SKILL.md`.\n\n")
	for _, s := range skills {
		sb.WriteString("### " + s.Name + "\n\n")
		sb.WriteString(emit.SourceComment(s.Path))
		if d := s.Description(); d != "" {
			sb.WriteString("_" + d + "_\n\n")
		}
		sb.WriteString("Source: `" + filepath.ToSlash(filepath.Join(skillsDir, s.Name, "SKILL.md")) + "`\n\n")
	}
}
