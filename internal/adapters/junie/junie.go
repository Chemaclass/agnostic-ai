// Package junie emits configs for JetBrains Junie.
//
// Junie's guidelines lookup is a strict precedence order, first match
// wins, not a merge: `.junie/AGENTS.md` ("the most preferred standard
// location"), then the root `AGENTS.md`, combined with
// `.junie/playbook.md` and every `.junie/rules/*.md` file, "if no file
// is found in the `.junie` folder"
// (junie.jetbrains.com/docs/guidelines-and-memory.html: "AGENTS.md file
// in the project root, combined with `.junie/playbook.md` and every
// `.junie/rules/*.md` file, if present."), then the legacy
// `.junie/guidelines.md` / `.junie/guidelines/`
// (junie.jetbrains.com/docs/junie-ide-plugin.html and
// guidelines-and-memory.html, target-audit 2026-08-08, #552). `sync`
// always writes `.junie/AGENTS.md` (see emitEntryPoint below), so step 1
// always matches: step 2, and everything it combines
// (`.junie/playbook.md`, `.junie/rules/*.md`), is pre-empted outright in
// a synced project (junie.jetbrains.com/docs/environment-variables.html:
// "If this file exists, it is used exclusively; no other guidelines
// files are combined with it."). The IDE plugin's own doc separately
// lists a Custom Path step ahead of `.junie/AGENTS.md`, an IDE Settings
// preference; the CLI-facing doc has none, and since that per-workspace
// setting is not usually committed, it rarely changes which file wins
// here (target-audit 2026-08-09, #590).
//
// Rule bodies inline directly into `.junie/AGENTS.md`, the only file
// Junie ever opens in a synced project, under a sentinel-marked
// `## Rules` block (emit.RenderRulesAppendix, the same mechanism
// codex/gemini/aider use for their own single-entry-point surface).
// There is no separate `.junie/rules/` output anymore: a prior version
// of this adapter wrote one .md per rule and per agent there. That
// directory is read, at step 2 alongside `.junie/playbook.md` (see
// above), but `.junie/AGENTS.md` always wins step 1 once `sync` has run,
// so a hand-authored `.junie/rules/*.md` file is shadowed rather than
// unread: real, just never reached in a synced project. Any
// agnostic-ai-managed leftovers from the old flattened layout are swept
// on sync (hand-authored files there survive; see sweepLegacyRulesDir).
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
// `disallowedTools`, `mcpServers`, `model`, `permissionMode`,
// `reasoningLevel`, `maxTurns`, `skills`, `allowPromptArgument`) are
// spelled exactly as a spec author already writes them, so nothing here
// needs translation, same as the commands renderer below.
// `reasoningLevel` also accepts `effort` as an alias, taking precedence
// when both are set; a spec author can write either key and both pass
// through unchanged, since there is no separate `effort` field to
// translate from. That same table constrains `name` to
// `[a-z][a-z0-9_-]*`, and the filename fallback ("If missing, the file
// name (without extension) is used") carries the constraint too, so an
// agent name breaking it fails before output is written (#857).
// Agent bodies no longer inline into `.junie/AGENTS.md`
// now that this native destination exists: the
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
// directory." The vendor documents two frontmatter fields for
// commands, `description` and `allowPromptArgument` (accepts free-form
// text via a `$prompt` placeholder); any other key an author sets
// still passes through verbatim, matching emitAgents. The body may
// reference `$argumentName` placeholders Junie substitutes at
// invocation.
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
// `mcpServers` map schema (the same shape Claude Code and Cursor use),
// minus two keys no Junie page documents. The vendor's own structure
// block for that file carries `command`/`args`/`env` on a local server
// and `url`/`headers` on a remote one, with no per-server `disabled`
// or `description` anywhere
// (junie.jetbrains.com/docs/junie-cli-mcp-configuration.html). Worse
// than inert, `disabled` reverses meaning on arrival: "Manually added
// configurations are imported to the list of MCP servers and enabled
// by default. To disable a server, list all the configured servers
// with the `/mcp` command, select the necessary server, and then
// select the `-> Disable` action." Both keys are stripped with a
// coverage note rather than written (target-audit 2026-09-18, #858),
// the same choice warp.go makes for its own two closed tables.
//
// Settings specs merge their last non-empty `model` into
// `.junie/config.json`, preserving unrelated native project settings.
//
// Ignore specs emit as `.aiignore` (override via
// outputs.junie.ignore-file), gitignore syntax under a `#` provenance
// header: "You can restrict Junie from processing the contents of
// specific files or folders by creating and configuring an `.aiignore`
// file in the project root directory" and "The `.aiignore` file follows
// the same syntax and pattern format as the `.gitignore` file"
// (junie.jetbrains.com/docs/junie-ide-plugin.html, target-audit
// 2026-09-11). The guarantee is weaker than a hard block: Junie "will
// ask for explicit approval before viewing or editing" a listed file
// rather than refuse it, only the contents are protected (file and
// folder names stay visible), and Brave Mode or an allowlisted command
// referencing the path bypasses the prompt entirely. Unlike every other
// junie surface this adapter writes, `.aiignore` is documented on the
// IDE-plugin page alone: no Junie CLI page names it, so an import-side
// reader must tolerate its absence rather than treat it as required.
package junie

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

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
	defaultConfigFile  = ".junie/config.json"
	// defaultIgnoreFile is Junie's project-root ignore file. It sits
	// outside `.junie/` because the vendor puts it there: "creating and
	// configuring an `.aiignore` file in the project root directory"
	// (junie.jetbrains.com/docs/junie-ide-plugin.html).
	defaultIgnoreFile = ".aiignore"
	// defaultEntryFile is Junie's own preferred entry-point location,
	// checked first in the lookup order (see the package doc). Fixed,
	// not user-overridable: it exists purely to make the canonical
	// pointer body, plus the inlined rules appendix, reachable at the
	// one path Junie opens.
	defaultEntryFile = ".junie/AGENTS.md"
)

var caps = emit.Capabilities{
	Target:      target,
	Supports:    []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindMCP, spec.KindCommand, spec.KindIgnore, spec.KindSettings},
	AgentFields: []string{"effort", "mcpServers"},
}

// agentNameRule is the regex the subagent frontmatter table states for
// `name`. It is looser than the opencode and zed skill rule (underscores
// are allowed, the first character must be a letter), so the shared
// emit.ValidateNames takes the pattern and the prose (#857).
var agentNameRule = emit.NameRule{
	Pattern: regexp.MustCompile(`^[a-z][a-z0-9_-]*$`),
	Rule:    "start with a lowercase letter, then use only lowercase letters, digits, hyphens, or underscores",
}

// Adapter emits Junie configs.
type Adapter struct{}

// New returns a Junie adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

func (Adapter) Capabilities() []spec.Kind { return caps.Supports }

// mcpDisabledNoOpReason and mcpDescriptionNoOpReason explain, in the
// flushed coverage notes, why neither key reaches `.junie/mcp/mcp.json`.
// The vendor's mcp.json structure block documents neither, and a server
// imported from that file "is enabled by default" whatever the file
// says, so a written `disabled: true` would claim a state Junie never
// enters (junie.jetbrains.com/docs/junie-cli-mcp-configuration.html,
// target-audit 2026-09-18, #858).
const (
	mcpDisabledNoOpReason    = "no per-server disable key in .junie/mcp/mcp.json; imported servers start enabled, so disable the server with /mcp -> Disable"
	mcpDescriptionNoOpReason = "no per-server description key in .junie/mcp/mcp.json; the server name is the only label Junie shows"
)

// permissionsUserTierOnlyReason explains why a portable permission
// policy reaches nothing on Junie. Its one rule-based approvals file is
// documented at the home tier alone: "You can manually add or remove
// allowed commands by editing the `~/.junie/allowlist.json` file"
// (junie.jetbrains.com/docs/action-allowlist-junie-cli.html). No
// project-root variant is named, and no flag relocates it, unlike
// `--config-location` for the config file.
//
// The project tier is real for other fields, which is what makes the
// absence a checked fact rather than an unresearched one: the CLI
// configuration page lists both "User scope: ~/.junie/config.json" and
// "Project scope: <project-root>/.junie/config.json", then tables that
// file's supported fields. None is an allow, deny, or ask list. The
// nearest knob there is `brave`, one boolean that skips every prompt.
// The IDE plugin's Action Allowlist is a settings panel, not a file
// (target-audit 2026-09-19, #917).
const permissionsUserTierOnlyReason = "Junie documents its allowlist.json at ~/.junie/ only, and the project-tier .junie/config.json field list has no allow, deny, or ask key"

// Emit writes the `.junie/AGENTS.md` entry-point (pointer body plus
// inlined rules), one native file per agent under the agents directory,
// one folder per skill under the skills directory (Junie's native
// SKILL.md layout; a flat file there never loads as a skill), one
// native file per command under the commands directory, `.aiignore`
// when ignore entries exist, `.junie/config.json` when a settings spec
// selects a model, then the MCP server file when the bundle has any MCP
// entries. A stale managed
// tree at the pre-#552 `.junie/rules/` default (or its
// outputs.junie.rules-dir override) is swept.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	if err := emit.ValidateNames(b.Agents, target, "agent", agentNameRule); err != nil {
		return err
	}
	if err := emitEntryPoint(sess, b, cfg, dryRun); err != nil {
		return err
	}
	if err := sweepLegacyRulesDir(sess, cfg, dryRun); err != nil {
		return err
	}
	agentsDir := emit.OutputAgentsDir(cfg, target, defaultAgentsDir)
	if err := (Adapter{}).EmitAgents(sess, b.Agents, agentsDir, dryRun); err != nil {
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
	if err := sess.WriteIgnoreFile(b.Ignores, target, emit.OutputIgnoreFile(cfg, target, defaultIgnoreFile), dryRun); err != nil {
		return err
	}
	emit.NoteFieldNoOp(target, spec.KindSettings, "permissions",
		emit.SpecsWithPermissions(b.Settings), permissionsUserTierOnlyReason)
	if err := emitProjectConfig(sess, b.Settings, dryRun); err != nil {
		return err
	}
	mcps := emit.StripMCPDisabled(target, b.MCPs, mcpDisabledNoOpReason)
	mcps = emit.StripMCPDescription(target, mcps, mcpDescriptionNoOpReason)
	return sess.WriteMCPFile(mcps, emit.MCPSchemaServersMap,
		emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// emitEntryPoint writes .junie/AGENTS.md: the canonical pointer body
// (see entryPointBody) with the sentinel-marked rules appendix
// appended. Uses the same WriteSection rendering (`### <name>` +
// source comment + optional description + body) every other inlining
// target's entry-point carries. Agents no longer inline here (#604);
// they emit natively via emitAgents instead. The project-local
// instructions (.agnostic-ai/local/AGNOSTIC_AI.md) follow last, as in
// every entry point the central renderer writes.
func emitEntryPoint(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	body, err := entryPointBody(cfg)
	if err != nil {
		return err
	}
	if emit.InlinesRulesIntoEntryPoint(target) {
		body = emit.AppendRulesAppendix(body, emit.RenderRulesAppendix(b))
	}
	local, err := emit.ReadLocalInstructions()
	if err != nil {
		return err
	}
	body = emit.AppendLocalInstructions(body, spec.FilterFences(local, []string{target}))
	return sess.WriteFile(defaultEntryFile, emit.WithHeader(body, emit.FormatMarkdown), dryRun)
}

// entryPointBody returns the canonical pointer body: the content of
// .agnostic-ai/AGNOSTIC_AI.md (header stripped) when that file already
// exists, or the generated template otherwise. Mirrors the read side
// of sync's own central entry-point resolution without creating
// AGNOSTIC_AI.md itself; that bootstrap stays sync's responsibility so
// this adapter never races it for the first write.
//
// .junie/AGENTS.md is a file only junie itself ever reads (unlike the
// shared root AGENTS.md, which the central entry-point renderer already
// fence-filters per its several readers), so the body is filtered here
// for the single reader "junie": a ::target fence for another tool must
// not leak its paragraph, or its marker lines, into this file.
func entryPointBody(cfg *config.Config) (string, error) {
	data, err := os.ReadFile(emit.AgnosticEntryPointPath)
	if err == nil {
		return spec.FilterFences(emit.StripHeader(string(data)), []string{target}), nil
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

// EmitAgents writes one native subagent file per agent at
// `<dir>/<name>.md` (see the package doc for the vendor schema and the
// `.junie/agents/` vs `.agents/` choice). Frontmatter passes through
// verbatim via emit.DocumentStyled: Junie's documented fields need no
// translation, since they are already spelled the way a spec author
// writes them.
func (Adapter) EmitAgents(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	if err := emit.ValidateNames(agents, target, "agent", agentNameRule); err != nil {
		return err
	}
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
// `<dir>/<name>.md`. The vendor documents `description` and
// `allowPromptArgument` as command frontmatter fields, but
// emit.DocumentStyled passes through every other key verbatim too,
// exactly as emitAgents does.
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

// emitProjectConfig merges the portable default model into
// `.junie/config.json`, alongside any key an author wrote under
// `x-junie`. Routes through MergeJSONFile so every other key in that
// file survives the sync.
//
// The `x-junie` block is the settings-kind escape hatch: Junie's
// project config carries fields this project does not model, and
// without the hatch they reach nothing and say nothing (#949). It
// merges with the managed keys rather than replacing them; `model` is
// the only one this adapter writes, and a scalar has no parts to keep,
// so `x-junie.model` still wins outright (#966).
// No file is written when neither source contributes.
func emitProjectConfig(sess *emit.Session, settings []spec.Entry, dryRun bool) error {
	keys := map[string]any{}
	if model := emit.LastSettingsModel(settings); model != "" {
		keys["model"] = model
	}
	emit.MergeSettingsCustomKeys(keys, settings, target)
	if len(keys) == 0 {
		return nil
	}
	return sess.MergeJSONFile(defaultConfigFile, keys, dryRun)
}
