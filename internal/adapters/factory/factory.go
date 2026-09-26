// Package factory emits configs for Factory's Droid CLI.
//
// The project-root AGENTS.md is written centrally by `sync` as a slim
// pointer to the source specs (one body shared with every other
// target's entry-point file). Droid CLI reads AGENTS.md directly as
// its single source of truth for project rules, delivered centrally;
// this adapter never writes a rules file of its own.
//
// Custom droids (Droid CLI's subagent surface) emit as one Markdown
// file per agent spec at `.factory/droids/<name>.md` (override via
// outputs.factory.agents-dir), the top-level directory Droid CLI scans
// for droid definitions — nested subdirectories are not read, so every
// droid lands flat in that one folder. Frontmatter carries `name`,
// `description`, optional `model`, and optional `tools`; arbitrary
// `x-factory` keys pass through verbatim so the rest of the documented
// schema is reachable without waiting on this adapter's allowlist.
//
// Skills emit as one folder per skill at `.agents/skills/<name>/SKILL.md`
// (override via outputs.factory.skills-dir), the same cross-tool tree
// codex, amp, zed, crush, and others already write byte-identically, so
// the shared tree dedupes into one write. docs.factory.ai/harness/skills
// documents this compatibility path alongside a second one this adapter
// does not additionally write, `.agent/skills/**/SKILL.md`: "Droid can
// load skills from several scopes. A skill is any directory under a
// `skills/` folder that contains SKILL.md."
// Scoped skills emit at `<scope>/.factory/skills/<name>/SKILL.md`.
// An explicit skills-dir override applies beneath each scope.
//
// `tools` is translated, not passed through. Droid CLI's tool IDs are
// its own vocabulary and "Arrays must use valid IDs from this table or
// exact registered MCP tool IDs. Unknown IDs cause a validation error"
// (docs.factory.ai/harness/subagents), so a Claude-style name Factory
// does not know costs the author the whole droid at load time, not just
// that one tool. factoryToolID renames `Bash` to `Execute`, `Write` to
// `Create`, and `WebFetch` to `FetchUrl`, the same three renames
// Factory's own Claude Code importer performs, and passes through the
// seven names that are already valid IDs. Three load-time rules from
// the same page shape the rest: `TodoWrite` and `Skill` are "always
// included for every droid ... You do not list them", so they drop
// without a note (the droid keeps them either way); `ExitSpecMode` and
// `GenerateDroid` "cannot be enabled by a custom droid; listing either
// one is a validation error", so they have no table entry and drop like
// any unknown name; and "The literal value `tools: all` is rejected",
// which needs no code because a scalar is not a list and never reaches
// the frontmatter, leaving the key omitted, Factory's own way to spell
// "allow every tool". Anything else drops rather than being written
// unconfirmed, and every agent that lost a name folds into one coverage
// note per sync while the names that do translate still emit.
// `x-factory.tools` bypasses the table outright: it is the only way to
// reach a category name (`read-only`, `edit`, `execute`, `web`, `mcp`)
// or a registered MCP tool ID, neither of which the cross-tool list can
// express.
//
// The portable `readonly: true` maps onto that same `read-only` category
// ("Analysis and file exploration" over `Read`, `LS`, `Grep`, `Glob`,
// docs.factory.ai/harness/subagents), so a portable readonly agent gets
// a real Factory tool boundary instead of the field silently dropping.
// It wins outright over any portable `tools` list on the same agent
// rather than narrowing it, because the category can never grant more
// than read-only and a narrowed list still could (an empty intersection
// would omit `tools` and grant every tool). The override folds into a
// coverage note naming `x-factory.tools` as the escape hatch for a
// custom list. An explicit `x-factory.tools` still wins over `readonly`
// itself, the same as it wins over the generic list, and `readonly:
// false` is a no-op.
//
// Droid CLI's own schema requires a non-empty system prompt after the
// frontmatter ("The body after the frontmatter is the system prompt
// and cannot be empty", docs.factory.ai/harness/subagents), so an
// agent spec with an empty body is skipped rather than written as a
// file Droid CLI itself would call invalid; the skip surfaces through
// a coverage note instead of failing silently.
//
// MCP servers are written to `.factory/mcp.json` (override via
// outputs.factory.mcp-file) under a root `mcpServers` map, the same
// shape emit.MCPSchemaServersMap already produces for Claude Code and
// Cursor: stdio carries `command`/`args`/`env` with no `type`; HTTP
// and SSE carry an explicit `type` plus `url`/`headers`, both
// documented by Factory. WebSocket entries are skipped with a coverage
// note because Factory documents a closed transport set of `stdio`,
// `http`, and `sse`. Unlike Claude Code, Cursor, and Copilot,
// Factory's own schema documents a working per-server `disabled`
// boolean (default false), so this adapter does not strip it the way
// those three do.
//
// "Written to", not "merged into": emit.WriteMCPFile is a plain
// WriteFile, so this adapter owns the whole file, the same as claude,
// cursor, junie, and kiro. Say it plainly here because the vendor sends
// users to hand-edit it: "**Project servers cannot be removed** with
// `droid mcp remove` or the `/mcp` manager. To remove them, edit
// `.factory/mcp.json` directly" (docs.factory.ai/harness/mcp). That
// edit is lost on the next sync (target-audit 2026-09-11, #737).
//
// Hooks are written to `.factory/hooks.json` (override via
// outputs.factory.hooks-file), keyed directly by event name with no
// surrounding "hooks" wrapper: "Standalone `hooks.json` files are
// keyed directly by event name" (docs.factory.ai/harness/hooks), the
// one other divergence Windsurf/Devin CLI's own `.devin/hooks.v1.json`
// also carries. Nine events: `PreToolUse`, `PostToolUse`,
// `UserPromptSubmit`, `Notification`, `Stop`, `SubagentStop`,
// `PreCompact`, `SessionStart`, `SessionEnd`. `timeout` is seconds
// (vendor default 60 when absent), not milliseconds. See hooks.go for
// the field mapping, the matcher-vocabulary note, and the vendor
// quote (#629). "Written to", not "merged into", for the same reason
// as `.factory/mcp.json` above: this is a plain WriteFile, so a hand
// edit to the file is lost on the next sync (#745).
//
// Factory MCP options preserve disabledTools, timeout and connectTimeout
// (milliseconds, including zero). HTTP/SSE oauth accepts false or Factory
// metadata fields. x-factory overrides corresponding top-level options.
// Commands emit at `.factory/commands/<name>.md` with the documented
// description and argument-hint frontmatter. Factory recommends Skills for
// new reusable workflows, but continues to load this project command surface.
//
// Settings specs merge into `.factory/settings.json` (override via
// outputs.factory.conf-file). Factory documents the project tier on its
// hierarchical-settings page rather than the CLI settings page, whose
// "Where settings live" table lists `~/.factory/settings.json` alone:
// "Settings are authored in `.factory/` folders, using the same schema
// at every level", levels table row "**Project** |
// `<git-root>/.factory/`", and "Each `.factory/` folder can contain:
// `settings.json`: general settings (models, safety, preferences,
// telemetry)"
// (docs.factory.ai/enterprise/hierarchical-settings-and-org-control).
// The skills page names the same file: "the **Project** tab writes to
// `<project>/.factory/settings.json`" (docs.factory.ai/harness/skills).
// `model`, the three command lists, and any key written under
// `x-factory` are set, and through MergeJSONFile, so `disabledSkills`
// and every other key in that file survive the sync.
//
// The portable permission lists reach Factory's three command lists,
// which the vendor now types as `string[]` of "Shell command patterns"
// and gives examples for in both spellings, bare and `prefix *`. The
// mapping is not the one the names suggest: `commandDenylist` prompts
// and can be approved, so portable `ask` lands there and portable
// `deny` lands in `commandBlocklist`, the key with no approval path.
// Rules outside `Bash` have no spelling among the three and raise a
// coverage note (target-audit 2026-09-20, #948). See settings.go.
package factory

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target             = "factory"
	defaultDroidsDir   = ".factory/droids"
	defaultSkillsDir   = ".agents/skills"
	defaultCommandsDir = ".factory/commands"
	defaultMCPFile     = ".factory/mcp.json"

	// defaultSettingsFile is the project tier of Factory's settings
	// hierarchy: "Settings are authored in `.factory/` folders, using
	// the same schema at every level", levels table row "**Project** |
	// `<git-root>/.factory/`"
	// (docs.factory.ai/enterprise/hierarchical-settings-and-org-control).
	defaultSettingsFile = ".factory/settings.json"
)

// droidHandBuiltKeys names the frontmatter keys this adapter builds
// itself, so the x-factory passthrough never writes them a second time.
// `tools` is deliberately absent: an x-factory.tools override is the one
// channel trusted to already speak Factory's own vocabulary, so it
// reaches the frontmatter through the passthrough rather than the
// translation table (see tools.go).
var droidHandBuiltKeys = []string{"name", "description", "model", "mcpServers", "effort", "reasoningEffort"}

var caps = emit.Capabilities{
	Target: target,
	// KindRule is declared even though this adapter never writes a
	// rules file itself: Droid CLI reads project rules exclusively
	// from the shared AGENTS.md entry-point sync writes centrally.
	Supports: []spec.Kind{spec.KindRule, spec.KindAgent, spec.KindSkill, spec.KindMCP, spec.KindHook, spec.KindCommand, spec.KindSettings},
	// effort: frontmatter_policy.go notes the values outside the enum.
	AgentFields:    []string{"effort", "mcpServers"},
	SettingsFields: []string{"effort"},
}

// Adapter emits Factory Droid CLI configs.
type Adapter struct{}

// New returns a Factory adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

func (Adapter) Capabilities() []spec.Kind { return caps.Supports }

// Emit writes one droid Markdown file per agent spec under
// `.factory/droids/`, one skill folder per skill spec under
// `.agents/skills/`, a managed `.factory/mcp.json` for MCP servers,
// `.factory/hooks.json` for hook specs, one native command file per
// command spec, and a merged `.factory/settings.json` for settings
// specs. The project-root
// AGENTS.md (rules' single source of truth for Droid CLI) is written
// by `sync`, not here.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	dir := emit.OutputAgentsDir(cfg, target, defaultDroidsDir)
	if err := (Adapter{}).EmitAgents(sess, b.Agents, dir, dryRun); err != nil {
		return err
	}
	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)
	noteDroppedManualOnly(b.Skills)
	for _, skill := range b.Skills {
		skillDir := skillsDir
		if skill.Scope != "" {
			var err error
			skillDir, err = emit.ScopedSkillsDir(skill.Scope, emit.OutputSkillsDir(cfg, target, ".factory/skills"))
			if err != nil {
				return err
			}
		}
		if err := sess.WriteSkillFolder(skill, target, skillDir, dryRun); err != nil {
			return err
		}
	}
	if err := emitCommands(sess, b.Commands, emit.OutputCommandsDir(cfg, target, defaultCommandsDir), dryRun); err != nil {
		return err
	}
	if err := emitHooks(sess, b.Hooks, cfg, dryRun); err != nil {
		return err
	}
	if err := emitSettings(sess, b.Settings, emit.OutputConfFile(cfg, target, defaultSettingsFile), dryRun); err != nil {
		return err
	}
	// Factory's schema documents a working `disabled` key (unlike
	// Claude Code, Cursor, and Copilot), so the shared builder's
	// existing `disabled` output is correct here as-is; no strip.
	return sess.WriteMCPFile(factoryMCPs(b.MCPs), emit.MCPSchemaServersMap, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun, emit.WithFactoryMCPExtras())
}

func emitCommands(sess *emit.Session, commands []spec.Entry, dir string, dryRun bool) error {
	for _, command := range commands {
		path := filepath.Join(dir, command.Name+".md")
		resolved := emit.ResolveMeta(command.Meta, target)
		front := map[string]any{}
		var keys []string
		for _, key := range []string{"description", "argument-hint"} {
			if value, ok := resolved[key]; ok {
				front[key] = value
				keys = append(keys, key)
			}
		}
		emit.MergeCustomTargetMeta(front, &keys, command.Meta, target, "description", "argument-hint")
		body := emit.FrontmatterOrdered(front, keys) + "\n" + command.Body
		if err := sess.WriteFile(path, emit.WithHeader(body, emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
	}
	return nil
}

func factoryMCPs(entries []spec.Entry) []spec.Entry {
	return emit.DropMCPWebSocket(target, entries,
		"WebSocket transport is not supported; Factory documents only stdio, http, and sse")
}

// EmitAgents writes one `<dir>/<name>.md` per agent spec whose body is
// non-empty. Droid CLI's own schema calls a frontmatter-only body
// invalid, so an agent spec with an empty (or whitespace-only) body is
// skipped instead of written as a file the tool itself would reject;
// the skip count surfaces through a coverage note so a spec left empty
// by accident does not disappear without a trace. A `tools` name with
// no Factory ID drops the same way, folded into one field note per sync
// (see tools.go), and a portable tools list superseded by `readonly:
// true` folds into its own note (see droidMarkdown).
func (Adapter) EmitAgents(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	noteUnsupportedEffort(agents)
	var emptyBody, droppedTools, readonlyOverrodeTools int
	for _, a := range agents {
		if strings.TrimSpace(a.Body) == "" {
			emptyBody++
			continue
		}
		path := filepath.Join(dir, a.Name+".md")
		md, dropped, overrodeList := droidMarkdown(a)
		if dropped {
			droppedTools++
		}
		if overrodeList {
			readonlyOverrodeTools++
		}
		if err := sess.WriteFile(path, emit.WithHeader(md, emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
	}
	emit.NoteCoverageGap(target, spec.KindAgent, emptyBody,
		"empty spec body; Droid CLI requires a non-empty system prompt")
	emit.NoteFieldNoOp(target, spec.KindAgent, "tools", droppedTools,
		"name(s) outside Factory's tool-ID table (Read, LS, Grep, Glob, Create, Edit, ApplyPatch, Execute, WebSearch, FetchUrl) fail Droid CLI's load-time validation and are dropped; set x-factory.tools for a category name or an MCP tool ID")
	emit.NoteFieldNoOp(target, spec.KindAgent, "tools", readonlyOverrodeTools,
		"readonly: true replaces the portable tools list with the tools: read-only category so the droid never exceeds Read, LS, Grep, Glob; set x-factory.tools to keep a custom list instead")
	return nil
}

// droidMarkdown renders a single droid definition: `name`,
// `description` (falls back to the spec name), optional `model`,
// optional `tools` translated onto Factory's own tool IDs (see
// tools.go), plus arbitrary x-factory passthrough, followed by the spec
// body as the droid's system prompt. Callers only reach this with a
// non-empty (trimmed) body; EmitAgents skips the empty case before this
// ever runs. `tools` is read from the raw, unresolved meta rather than
// the resolved map: ResolveMeta would already have flattened an
// x-factory.tools override onto it, and running that value back through
// the Claude-style table would misread Factory's own vocabulary as
// unknown. xFactorySetsTools guards the same case, so the override wins
// outright instead of merging alongside a translated value.
//
// A portable `readonly: true` maps onto Factory's own `read-only`
// category ("Analysis and file exploration", `Read`, `LS`, `Grep`,
// `Glob`; docs.factory.ai/harness/subagents), the same coarse mapping
// #1149 gives Codex's `sandbox_mode = "read-only"` and #1152 gives
// Cursor's own `readonly` field. It wins outright over any portable
// `tools` list rather than narrowing it: Factory's category never
// grants more than read-only, so replacing the list is the direction
// that can never grant more than the author asked for, and
// readonlyOverrodeTools reports the case so the caller can fold every
// such agent into one coverage note naming the escape hatch.
// hasDroppedTools reports whether any declared name had no Factory ID,
// so the caller can fold every such agent into one note per sync.
func droidMarkdown(e spec.Entry) (body string, hasDroppedTools, readonlyOverrodeTools bool) {
	resolved := emit.ResolveMeta(e.Meta, target)
	desc, _ := resolved["description"].(string)
	if desc == "" {
		desc = e.Name
	}
	meta := map[string]any{
		"name":        e.Name,
		"description": desc,
	}
	keys := []string{"name", "description"}
	if model, _ := resolved["model"].(string); model != "" {
		meta["model"] = model
		keys = append(keys, "model")
	}
	if !xFactorySetsTools(e.Meta) {
		raw := emit.StringSlice(e.Meta["tools"])
		if resolved["readonly"] == true {
			meta["tools"] = "read-only"
			keys = append(keys, "tools")
			readonlyOverrodeTools = len(raw) > 0
		} else if len(raw) > 0 {
			mapped, dropped := translateTools(raw)
			if len(mapped) > 0 {
				meta["tools"] = mapped
				keys = append(keys, "tools")
			}
			// An emitted droid with no `tools` key may use every
			// tool, so losing the whole list is a lost restriction
			// even when each dropped name was an always-on one.
			hasDroppedTools = dropped || len(mapped) == 0
		}
	}
	// An empty list is written too: Factory reads `mcpServers: []` as
	// no servers at all, while an absent key inherits every server.
	if _, set := resolved["mcpServers"].([]any); set {
		meta["mcpServers"] = append([]string{}, emit.StringSlice(resolved["mcpServers"])...)
		keys = append(keys, "mcpServers")
	}
	if effort, ok := droidReasoningEffort(resolved); ok {
		meta["reasoningEffort"] = effort
		keys = append(keys, "reasoningEffort")
	}
	emit.MergeCustomTargetMeta(meta, &keys, e.Meta, target, droidHandBuiltKeys...)
	front := emit.FrontmatterOrdered(meta, keys)
	return front + "\n" + strings.TrimSpace(e.Body) + "\n", hasDroppedTools, readonlyOverrodeTools
}
