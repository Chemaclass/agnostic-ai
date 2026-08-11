// Package junie emits configs for JetBrains Junie.
//
// Junie's guidelines lookup is a strict precedence order, first match
// wins, not a merge: `.junie/AGENTS.md` ("the most preferred standard
// location"), then the root `AGENTS.md` "if no file is found in the
// `.junie` folder", then the legacy `.junie/guidelines.md` /
// `.junie/guidelines/`
// (junie.jetbrains.com/docs/junie-ide-plugin.html and
// guidelines-and-memory.html, target-audit 2026-08-08, #552). `sync`
// always writes `.junie/AGENTS.md` (see emitEntryPoint below), so step 1
// always matches and every location after it is unreachable in a synced
// project. The IDE plugin's own doc separately lists a Custom Path step
// ahead of `.junie/AGENTS.md`, an IDE Settings preference; the
// CLI-facing doc has none, and since that per-workspace setting is not
// usually committed, it rarely changes which file wins here
// (target-audit 2026-08-09, #590).
//
// Rule bodies inline directly into `.junie/AGENTS.md`, the only file
// Junie ever opens here, under a sentinel-marked `## Rules` block
// (emit.RenderRulesAppendix, the same mechanism codex/gemini/aider use
// for their own single-entry-point surface). There is no separate
// `.junie/rules/` output anymore: a prior version of this adapter wrote
// one .md per rule and per agent there, but that directory sits outside
// Junie's documented lookup order entirely and nothing ever read it. Any
// agnostic-ai-managed leftovers from that layout are swept on sync
// (hand-authored files there survive; see sweepLegacyRulesDir).
//
// Agents emit natively, one file per agent, at `.junie/agents/<name>.md`
// (junie.jetbrains.com/docs/junie-cli-subagents.html, target-audit
// 2026-08-11, #604): "Subagents are Markdown files with YAML metadata
// stored in the `.junie/agents/` or `.agents/` directory." This adapter
// defaults to `.junie/agents/`, not the shared `.agents/` alternative:
// the same page says Junie CLI detects `.cursor/agents/`,
// `.claude/agents/`, and `.codex/agents/` on open and offers to import
// them into `.junie/agents/` specifically, marking it the vendor's own
// preferred location the way `.codex/agents/` is Codex's. No other
// registered target defaults an agent file into the shared `.agents/`
// tree today (several write `.agents/skills/`, `.agents/rules/`,
// `.agents/commands/`, or `.agents/mcp_config.json`, but none write a
// bare `.agents/<name>.md`), so `.junie/agents/` dedupes with nothing
// and collides with nothing either way. Set
// `outputs.junie.agents-dir: .agents` for the shared alternative
// instead, mirroring how `outputs.codex.agents-dir: .agents/agents`
// opts Codex into its own community layout. Frontmatter passes through
// verbatim: Junie's documented fields (`name`, `description`, `tools`,
// `disallowedTools`, `mcpServers`, `model`, `reasoningLevel`,
// `maxTurns`, `skills`, `allowPromptArgument`) are spelled exactly as a
// spec author already writes them, so nothing here needs translation,
// same as the commands renderer below. Agent bodies no longer inline
// into `.junie/AGENTS.md` now that this native destination exists: the
// same rule Augment and Kilo Code follow once their own native agents
// directory (`.augment/agents/`, `.kilo/agents/`) exists. A project
// still carrying the pre-#604 inlined `## Agents` block loses it on its
// next sync without any extra sweep step, since `.junie/AGENTS.md` is
// fully regenerated from the canonical pointer body every run rather
// than patched in place (see emitEntryPoint).
//
// Slash commands emit natively, one file per command, at
// `.junie/commands/<name>.md`
// (junie.jetbrains.com/docs/custom-slash-commands.html, target-audit
// 2026-08-11, #605): "Project-specific commands are stored as Markdown
// files in the `.junie/commands` folder at your project's root
// directory." Frontmatter is `description` only on the vendor side, but
// any other key an author sets still passes through verbatim, matching
// emitAgents. The body may reference `$argumentName` placeholders Junie
// substitutes at invocation.
//
// Both subagents and slash commands are CLI-only surfaces:
// junie-ide-plugin.html mentions neither, confirmed by full-text search
// of that page.
//
// Skills emit into their own native folder tree at
// `.junie/skills/<name>/SKILL.md` (override via outputs.junie.skills-dir),
// unaffected by the above: Junie's Native Agent Skills feature shipped
// 2026-07-31 and requires exactly this layout ("Project scope:
// `<projectRoot>/.junie/skills/<skill-name>/`"; "The `SKILL.md` file is
// required. A folder without it is not recognized as a skill",
// junie.jetbrains.com/docs/agent-skills.html, target-audit 2026-08-01).
//
// MCP servers write to `.junie/mcp/mcp.json` using the standard
// `mcpServers` map schema (the same shape Claude Code and Cursor use).
package junie

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target = "junie"
	// legacyRulesDir is the pre-#552 flattened rules-and-agents
	// directory. It is no longer written (see the package doc); Emit
	// only ever touches it to sweep stale agnostic-ai-managed files,
	// honoring outputs.junie.rules-dir so a project that customized the
	// old default still gets swept at the path it used.
	legacyRulesDir     = ".junie/rules"
	defaultAgentsDir   = ".junie/agents"
	defaultSkillsDir   = ".junie/skills"
	defaultCommandsDir = ".junie/commands"
	defaultMCPFile     = ".junie/mcp/mcp.json"
	// defaultEntryFile is Junie's own preferred entry-point location,
	// checked first in the lookup order (see the package doc). Fixed,
	// not user-overridable: it exists purely to make the canonical
	// pointer body, plus the inlined rules appendix, reachable at the
	// one path Junie opens.
	defaultEntryFile = ".junie/AGENTS.md"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindMCP, spec.KindCommand},
}

// Adapter emits Junie configs.
type Adapter struct{}

// New returns a Junie adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes the `.junie/AGENTS.md` entry-point (pointer body plus
// inlined rules), one native file per agent under the agents directory,
// one folder per skill under the skills directory (Junie's native
// SKILL.md layout; a flat file there never loads as a skill), one
// native file per command under the commands directory, then the MCP
// server file when the bundle has any MCP entries. A stale managed
// tree at the pre-#552 `.junie/rules/` default (or its
// outputs.junie.rules-dir override) is swept.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	if err := emitEntryPoint(sess, b, cfg, dryRun); err != nil {
		return err
	}
	if err := sweepLegacyRulesDir(sess, cfg, dryRun); err != nil {
		return err
	}
	agentsDir := emit.OutputAgentsDir(cfg, target, defaultAgentsDir)
	if err := emitAgents(sess, b.Agents, agentsDir, dryRun); err != nil {
		return err
	}
	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)
	if err := sess.WriteSkillFolders(b.Skills, target, skillsDir, dryRun); err != nil {
		return err
	}
	commandsDir := emit.OutputCommandsDir(cfg, target, defaultCommandsDir)
	if err := emitCommands(sess, b.Commands, commandsDir, dryRun); err != nil {
		return err
	}
	return sess.WriteMCPFile(b.MCPs, emit.MCPSchemaServersMap,
		emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// emitEntryPoint writes .junie/AGENTS.md: the canonical pointer body
// (see entryPointBody) with the sentinel-marked rules appendix
// appended. Uses the same WriteSection rendering (`### <name>` +
// source comment + optional description + body) every other inlining
// target's entry-point carries. Agents no longer inline here (#604);
// they emit natively via emitAgents instead.
func emitEntryPoint(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	body, err := entryPointBody(cfg)
	if err != nil {
		return err
	}
	if emit.InlinesRulesIntoEntryPoint(target) {
		body = emit.AppendRulesAppendix(body, emit.RenderRulesAppendix(b))
	}
	return sess.WriteFile(defaultEntryFile, emit.WithHeader(body, emit.FormatMarkdown), dryRun)
}

// entryPointBody returns the canonical pointer body: the content of
// .agnostic-ai/AGNOSTIC_AI.md (header stripped) when that file already
// exists, or the generated template otherwise. Mirrors the read side
// of sync's own central entry-point resolution without creating
// AGNOSTIC_AI.md itself; that bootstrap stays sync's responsibility so
// this adapter never races it for the first write.
func entryPointBody(cfg *config.Config) (string, error) {
	data, err := os.ReadFile(emit.AgnosticEntryPointPath)
	if err == nil {
		return emit.StripHeader(string(data)), nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("%s: %w", emit.AgnosticEntryPointPath, err)
	}
	return emit.EntryPointBody(cfg), nil
}

// sweepLegacyRulesDir removes agnostic-ai-managed leftovers from the
// pre-#552 `.junie/rules/` layout (rules and agents flattened to one
// .md each). Runs unconditionally: there is no replacement directory to
// protect, unlike the "differs from the new default" sweeps other
// adapters use for a still-supported alternate path. It still resolves
// through outputs.junie.rules-dir so a project that customized the old
// default gets swept at the path it used. Files without the
// agnostic-ai provenance header (hand-authored) are left in place.
func sweepLegacyRulesDir(sess *emit.Session, cfg *config.Config, dryRun bool) error {
	return sess.RemoveGeneratedTree(emit.OutputRulesDir(cfg, target, legacyRulesDir), dryRun)
}

// emitAgents writes one native subagent file per agent at
// `<dir>/<name>.md` (see the package doc for the vendor schema and the
// `.junie/agents/` vs `.agents/` choice). Frontmatter passes through
// verbatim via emit.DocumentStyled: Junie's documented fields need no
// translation, since they are already spelled the way a spec author
// writes them.
func emitAgents(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	for _, a := range agents {
		path := filepath.Join(dir, a.Name+".md")
		body := emit.WithHeader(emit.DocumentStyled(a.Meta, a.MetaKeys, a.MetaStyles, a.Body, target), emit.FormatMarkdown)
		if err := sess.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// emitCommands writes one native slash-command file per command at
// `<dir>/<name>.md`. Frontmatter is `description` only on the vendor
// side, but emit.DocumentStyled passes through every other key
// verbatim too, exactly as emitAgents does.
func emitCommands(sess *emit.Session, commands []spec.Entry, dir string, dryRun bool) error {
	for _, c := range commands {
		path := filepath.Join(dir, c.Name+".md")
		body := emit.WithHeader(emit.DocumentStyled(c.Meta, c.MetaKeys, c.MetaStyles, c.Body, target), emit.FormatMarkdown)
		if err := sess.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}
	return nil
}
