// Package antigravity emits configs for Google Antigravity IDE.
//
// Antigravity reads project instructions from per-rule files under
// `.agents/rules/*.md`, custom subagents from
// `.agents/agents/<name>/agent.md`,
// and skills from a folder per skill under
// `.agents/skills/<name>/SKILL.md`. This adapter emits all three. The
// vendor's directory-scoped rules discovery lists `.agents/rules/*.md`
// "and legacy `<dir>/.agent/rules/*.md`" (antigravity.google/docs/rules),
// and the same wording covers skills (/docs/skills?tab=ide). This
// adapter defaults to the plural form and sweeps a stale managed tree at
// the pre-plural `.agent/rules` / `.agent/skills` paths on sync, the
// same pattern codex uses for its own pre-v0.43 `.codex/skills/`
// default. `.agents/skills`
// is also the shared tree codex, amp, zed, crush, openhands, and
// windsurf already emit into, so identical skill folders dedupe there
// once an adapter's default lands on it.
//
// Every rule file starts with YAML frontmatter, the first bytes of the
// file: "Every `.md` file inside `rules/` must start with YAML
// frontmatter declaring a valid `trigger`. ... If a file ... omits
// frontmatter or specifies an unrecognized `trigger` value ...,
// Antigravity silently discards the rule" (antigravity.google/docs/rules).
// `alwaysApply` decides first, mirroring windsurf and cursor: `true` or
// unset writes `trigger: always_on` outright, ignoring `globs`.
// `alwaysApply: false` then maps `globs` onto `trigger: glob` plus a
// quoted, comma-joined `globs` string, a bare `description` onto
// `trigger: model_decision`, and neither onto `trigger: manual`, the
// vendor's own "load only on an @-mention" mode. Every branch lands on
// a trigger whose vendor-required companion field is already in hand,
// so nothing here falls back or reports a coverage note. `description`
// carries through whenever the spec has one, on every trigger (see
// rule.go, #1113).
//
// The same page caps one rule file at 24,000 bytes: "Antigravity
// truncates any single rule file that exceeds 24,000 bytes (after
// expanding `@[label](path)` includes)". An over-cap rule still emits in
// full, since agnostic-ai never truncates on the author's behalf, but
// `sync` reports a coverage note naming the truncation so the author
// hears it before Antigravity cuts the file, not after. The count is
// taken on the bytes that land, frontmatter, provenance header, and
// heading included (target-audit 2026-09-24, #1114). A separate
// 20,000-token budget covers every `always_on` rule together: past it,
// Antigravity demotes its largest rules from inline text to a
// `path: description` pointer, so this adapter raises no note for that
// budget and a `description` is worth setting on every rule to keep the
// pointer useful.
//
// A rule with a `scope` emits into a copy of the rules directory nested
// at that scope, `<scope>/.agents/rules/<name>.md`, rather than dropping
// with a coverage note: "You can place ... a `.agents/rules/` directory
// ... in any subdirectory of your project. Whenever Antigravity reads or
// edits a file, it walks up the directory tree ..., loading rules at
// each level" (antigravity.google/docs/rules). The frontmatter is the
// same `rule()` renderer every unscoped file gets, and the file never
// nests deeper than one level inside the scope: "Antigravity scans only
// immediate `.md` children inside `.agents/rules/` ... ignores files
// nested in subdirectories," so a rule two directories deep loses its
// own subdirectory structure the same way an unscoped rule already does.
//
// `sync --global` writes skills to `~/.gemini/config/skills/`. The
// vendor's IDE tab rows that path as the global scope and calls
// `~/.gemini/antigravity/skills/` legacy, while its Antigravity 2.0 tab
// names the config path with no legacy alternative at all
// (antigravity.google/docs/skills?tab=ide). See
// internal/cli/global_targets.go.
//
// Agents emit as native subagent profiles at
// `.agents/agents/<name>/agent.md`, one of the two workspace forms in
// Antigravity's subagent reference. The nested form avoids colliding
// with Goose and OpenHands, which only read flat files in the shared
// `.agents/agents/` directory.
// Frontmatter carries `name` and `description` (both required); "The
// content following the YAML `---` delimiter defines the subagent's
// system prompt." This adapter previously flattened agents into
// `.agents/rules/agent-<name>.md`, which the subagent loader never
// reads (target-audit 2026-08-27, #638); a stale managed copy at the
// old name is swept for every current agent, at both the current and
// the pre-plural rules path.
//
// A generic `tools` list never reaches that file. Antigravity's
// vocabulary is its own (`view_file`, `replace_file_content`,
// `grep_search`, `run_command`, ...) with zero name overlap with
// agnostic-ai's Claude-style set, and the same page warns: "Specifying
// an unmapped or misspelled tool name in the `tools` list may cause the
// subagent process to hang during execution." So it drops with a
// coverage note, and `x-antigravity.tools` is the one channel trusted
// to already hold Antigravity's own names. `model` is a three-value
// tier enum (`inherit`, `flash`, `pro`), not a model ID, so a value
// outside it drops the same way. Every other documented key
// (`mainAgent`, `subagent`, `commandExecutionPolicy`, `mcpServers`,
// `skills`/`plugins`) reaches the file through `x-antigravity` too.
//
// The `.agents/AGENTS.md` entry-point is written centrally by `sync`
// as a slim pointer to the source specs (one body shared with every
// other target's entry-point file). The vendor now documents this
// exact path per subdirectory, `<dir>/.agents/AGENTS.md` alongside
// `<dir>/.agents/GEMINI.md`, "AGENTS.md and GEMINI.md do not use
// frontmatter ... continuously active (always_on)"
// (antigravity.google/docs/rules), still clear of the project-root
// `AGENTS.md` codex, amp, and warp own. It replaces the pre-#1114
// `.agent/AGENTS.md` default, which named no documented read path; Emit
// sweeps a managed leftover there aside to `.agent/AGENTS.md.bak`
// (sweepLegacyEntryPoint), the same intent as `amp` and `warp` migrating
// their own renamed entry-point files, written by hand here because the
// rename crosses directories, not just filenames, which
// emit.MigrateLegacyFile does not resolve on its own. `import
// antigravity` reads the new path first, falling back to the old one
// only when the new one is absent. Every rule body
// still lands in the documented `.agents/rules/` tree. When
// `outputs.antigravity.rules-file` is set, this adapter instead writes
// the legacy merged layout at that path so users on older workflows
// keep their behavior.
//
// MCP servers land in `.agents/mcp_config.json`, a single `mcpServers`
// object (antigravity.google/docs/mcp?tab=ide). Remote servers require the
// `serverUrl` field: "Legacy fields like `url` or `httpUrl` are not
// supported," so this adapter cannot reuse the shared
// `emit.MCPSchemaServersMap` builder, which emits `url` (see mcp.go).
// stdio servers carry `command`, `args`, `env`, and `cwd`; remote
// servers add `headers`; both accept `disabled` under that literal
// name, unlike codex and kilo which map it onto their own
// `enabled: false`. Any other documented field (`authProviderType`,
// `oauth`, `disabledTools`, same page) or any field the vendor adds
// next reaches the file through an entry's `x-antigravity` block
// (emit.MergeCustomTargetMeta), the same escape hatch zed and warp give
// their own unmapped fields (#588); `import antigravity` captures it
// back the same way.
//
// Hooks merge into `.agents/hooks.json`, keyed by hook definition name
// rather than by event (antigravity.google/docs/hooks?tab=ide). Each spec
// becomes its own named definition holding the one event it names, so
// `enabled: false` (the definition's own field, the inverse of the
// portable `disabled: true`) disables that spec alone. Five events run:
// PreToolUse and PostToolUse hold `{matcher, hooks}` groups, while
// PreInvocation, PostInvocation and Stop hold a handler list directly
// and ignore the matcher, so this adapter writes each shape where the
// vendor documents it. The hook payload's `transcriptPath` resolves
// under `~/.gemini/antigravity-ide`, the IDE's own app-data directory,
// confirming the IDE itself runs them (target-audit 2026-08-27, #563).
// Commands remain fully unconfirmed in the public-preview docs and skip
// with a warning.
package antigravity

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target           = "antigravity"
	defaultRulesDir  = ".agents/rules"
	defaultSkillsDir = ".agents/skills"
	defaultMCPFile   = ".agents/mcp_config.json"
	// legacyRulesDir and legacySkillsDir are the pre-plural defaults
	// (every release through the one preceding this fix). Antigravity
	// still reads them for backward compatibility, so Emit sweeps any
	// agnostic-ai-managed leftovers there once the active path differs
	// (i.e. the user did not opt into the legacy path explicitly via
	// the matching outputs.antigravity.* key). See the package doc.
	legacyRulesDir  = ".agent/rules"
	legacySkillsDir = ".agent/skills"
	// defaultEntryPointFile and legacyEntryPointFile mirror the
	// EntryPointPath map in internal/adapters/internal/emit/entrypoint.go
	// (adapter packages cannot import each other's private state, so the
	// literal is duplicated the way legacyRulesDir already is). sync
	// writes defaultEntryPointFile centrally; Emit only sweeps the
	// pre-#1114 legacy path aside once it carries the agnostic-ai
	// provenance marker. See the package doc.
	defaultEntryPointFile = ".agents/AGENTS.md"
	legacyEntryPointFile  = ".agent/AGENTS.md"
	// defaultAgentsDir is Antigravity's workspace subagent path:
	// Antigravity discovers nested workspace profiles at
	// `.agents/agents/<name>/agent.md` (antigravity.google/docs/subagents).
	defaultAgentsDir = ".agents/agents"
	// legacyAgentPrefix is the filename prefix agents carried while they
	// flattened into the rules directory. Emit sweeps a stale managed
	// copy for every current agent name, at both rules paths.
	legacyAgentPrefix = "agent-"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindMCP, spec.KindHook},
	AgentFieldReasons: map[string]string{
		"mcpServers": "Antigravity takes inline server objects only; set x-antigravity.mcpServers",
	},
}

// Adapter emits Antigravity configs.
type Adapter struct{}

// New returns an Antigravity adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

func (Adapter) Capabilities() []spec.Kind { return caps.Supports }

// Emit writes per-rule files under .agents/rules/, each carrying the
// mandatory `trigger` frontmatter (see rule.go), a nested copy under
// `<scope>/.agents/rules/` for a scoped rule, one subagent file
// per agent under .agents/agents/<name>/agent.md, a folder per skill under
// .agents/skills/<name>/SKILL.md, .agents/mcp_config.json for MCP
// servers, .agents/hooks.json for hooks, and, when opted in via
// outputs.antigravity.rules-file, a
// legacy merged document at that path. A stale managed tree at the
// pre-plural `.agent/rules` / `.agent/skills` defaults is swept unless
// the user explicitly opted into that legacy path. The `.agents/AGENTS.md`
// entry-point is written by `sync`, not here; Emit only sweeps a managed
// leftover at the pre-#1114 `.agent/AGENTS.md` path aside.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	if err := sweepLegacyEntryPoint(sess, cfg, dryRun); err != nil {
		return err
	}
	rulesDir := emit.OutputRulesDir(cfg, target, defaultRulesDir)
	// Agents and skills emit through their native layouts below, so
	// suppress the rule-form `agent-<name>.md` / `skill-<name>.md`
	// output from RulesDirectory. FormatRule writes the mandatory
	// trigger frontmatter every rule file needs (#1113). ScopeAtRoot
	// places a scoped rule's copy of the rules directory inside the
	// scope (`<scope>/.agents/rules/<name>.md`), matching the vendor's
	// own directory-scoped discovery (#1114).
	if err := sess.RulesDirectory(b, emit.RulesDirOpts{
		Dir:         rulesDir,
		SkipAgents:  true,
		SkipSkills:  true,
		FormatRule:  rule,
		ScopeAtRoot: true,
	}, dryRun); err != nil {
		return err
	}
	noteOversizedRules(b.Rules)
	noteInvalidTriggerOverrides(b.Rules)
	if rulesDir != legacyRulesDir {
		if err := sess.RemoveGeneratedTree(legacyRulesDir, dryRun); err != nil {
			return err
		}
	}

	agentsDir := emit.OutputAgentsDir(cfg, target, defaultAgentsDir)
	if err := emitAgents(sess, b.Agents, agentsDir, []string{rulesDir, legacyRulesDir}, dryRun); err != nil {
		return err
	}

	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)
	if err := sess.WriteSkillFolders(b.Skills, target, skillsDir, dryRun); err != nil {
		return err
	}
	if skillsDir != legacySkillsDir {
		if err := sess.RemoveGeneratedTree(legacySkillsDir, dryRun); err != nil {
			return err
		}
	}

	if err := emitMCP(sess, b.MCPs, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun); err != nil {
		return err
	}

	if err := emitHooks(sess, b.Hooks, cfg, dryRun); err != nil {
		return err
	}

	if err := emit.WritePluginManifests(sess, pluginRoots(
		rulesDir, agentsDir, skillsDir,
		emit.OutputMCPFile(cfg, target, defaultMCPFile),
		emit.OutputHooksFile(cfg, target, defaultHooksFile),
	), dryRun); err != nil {
		return err
	}

	return sess.EmitLegacyRulesFile(b, cfg, target, emit.MergedOpts{Title: "AGENTS.md"}, dryRun)
}

// sweepLegacyEntryPoint renames a managed `.agent/AGENTS.md` left by a
// pre-#1114 sync aside to `.agent/AGENTS.md.bak`, so the user notices
// the move to the documented `.agents/AGENTS.md` path. It delegates to
// emit.Session.MigrateLegacyPath, which MigrateLegacyFile cannot do
// here: that helper resolves the legacy name against the new default's
// own directory, which fits amp's `AGENT.md` -> `AGENTS.md` and warp's
// `WARP.md` -> `AGENTS.md` (same directory, different filename) and not
// this rename, which crosses directories (`.agent/` to `.agents/`).
// MigrateLegacyPath moves both halves through the session's
// transaction-aware WriteFile and RemoveOwned, so a Rollback later in
// the same sync pass restores `.agent/AGENTS.md` instead of leaving the
// move half-done, and it never overwrites an existing `.bak`. A
// project with `outputs.antigravity.file` set is skipped entirely: the
// pre-#1114 default only ever wrote `.agent/AGENTS.md` when
// unconfigured, so an override never had a legacy file to sweep.
func sweepLegacyEntryPoint(sess *emit.Session, cfg *config.Config, dryRun bool) error {
	if emit.EntryPointPath(cfg, target) != defaultEntryPointFile {
		return nil
	}
	return sess.MigrateLegacyPath(target, legacyEntryPointFile, defaultEntryPointFile, dryRun)
}
