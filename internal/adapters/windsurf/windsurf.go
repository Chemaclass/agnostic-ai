// Package windsurf emits .devin/rules/*.md, .devin/agents/*.md,
// .devin/mcp_config.json, and .devin/hooks.v1.json for Devin Desktop,
// the renamed Windsurf editor (2026-06).
//
// Devin Desktop prefers `.devin/rules/*.md` and keeps `.windsurf/rules/`
// as a backward-compat fallback (`.windsurfrules` is legacy). Rules emit
// into the preferred directory, always-on unless the rule declares
// otherwise (see activationFrontmatter); set
// `outputs.windsurf.rules-dir: .windsurf/rules` to stay on the old
// layout.
//
// A scoped rule goes to `<scope>/.devin/rules/<name>.md`. This layout
// follows Desktop's documented discovery of `.devin/rules` or
// `.windsurf/rules` in workspace subdirectories
// (docs.devin.ai/desktop/cascade/memories). Since v3000.11.1,
// Devin CLI also discovers rules recursively under `.devin/rules/`
// and `.windsurf/rules/` (docs.devin.ai/cli/changelog/stable.md).
// That CLI change does not establish Desktop recursion or equivalent
// scope semantics for nested rule files. The output layout stays as
// adopted in #628 (2026-08-27); sync still sweeps the old
// `.devin/rules/<scope>/<name>.md` tree through the ledger.
//
// Agents emit as native subagent profiles at `.devin/agents/<name>.md`:
// "Custom subagents are defined as markdown files under `agents/`",
// project layout `.devin/agents/`, "**Flat file** — `agents/<name>.md`"
// (docs.devin.ai/cli/subagents). Frontmatter carries `name`,
// `description`, `model`, `allowed-tools`, and `max-nesting`; the body
// after the closing delimiter is the subagent's system prompt. This
// adapter previously flattened agents into `.devin/rules/agent-<name>.md`,
// which reached the rules loader instead of the subagent loader and had
// no key for any of those five fields (target-audit 2026-08-27, #638);
// a stale managed copy at the old name is swept for every current agent.
// Scoped agents land flat here: Devin documents sub-directory discovery
// for rules only, so scoping the agents dir would write where nothing
// reads. The vendor caveat is worth carrying: "Custom subagents are
// **experimental**. The format, behavior, and configuration options may
// change in future releases."
//
// `allowed-tools` translates agnostic-ai's Claude-style names onto
// Devin's own vocabulary; see devinTool for the current mapping.
// `/cli/reference/permissions` still lists only five names, `read`,
// `edit`, `grep`, `glob`, `exec`, and lags its own changelog: the
// v3000.11.1 entry (September 21, 2026) grants `allowed-tools` a
// sixth, `write`, so `Write` and `Edit` now map onto distinct Devin
// tools rather than both collapsing onto `edit` (#1022). The subagent
// docs enumerate no vocabulary of their own, so that list is the only
// documented one for this key, and it is not the `permissions`
// vocabulary. A name outside that table is never guessed at: it drops
// from the list and folds into one coverage note per sync.
// `x-windsurf.allowed-tools` wins outright over the
// translated form for an author who already knows Devin's vocabulary,
// and `max-nesting`, which has no generic spec field, reaches the file
// the same way. `model` passes through verbatim, since the vendor's own
// worked example pins `model: sonnet`, the exact vocabulary a
// cross-target spec already carries.
//
// Devin CLI also reads the cross-tool root `AGENTS.md`, written
// centrally by `sync`: "Devin CLI reads this file automatically" and
// its supported-file-names table rows `AGENTS.md` as "Recommended"
// (docs.devin.ai/cli/extensibility/rules, #645). Without it a
// windsurf-only project had no root entry-point at all and could not
// dedupe with the other AGENTS.md consumers.
//
// Skills emit as a folder per skill under
// `.agents/skills/<name>/SKILL.md`. Devin Desktop's own primary skill
// path is `.windsurf/skills/` (workspace scope) or
// `~/.codeium/windsurf/skills/` (global); `.agents/skills/` and
// `~/.agents/skills/` are a separate, documented "cross-agent
// compatibility" discovery path behind those
// (docs.devin.ai/desktop/cascade/skills, target-audit 2026-08-08,
// #563). This adapter writes the compatibility path deliberately: the
// renderer matches codex, amp, zed, crush, and openhands byte-for-byte,
// so identical skills dedupe under `sync.shared-skills` instead of
// adding a ninth on-disk copy. The target keeps its `windsurf` name so
// existing configs and `x-windsurf` meta continue to work.
//
// `outputs.windsurf.workflows-dir` used to additionally emit each
// agent as a Workflow at `<dir>/<name>.md`, invokable in Cascade chat
// as `/<name>`. Devin Desktop v3.9.19 ("September 8, 2026") removed
// Cascade, the only agent that ever read a Workflow file: "Cascade has
// been removed. Devin Local is now the only agent available in Devin
// Desktop" (docs.devin.ai/desktop/changelog.md). Devin Local does not
// pick the surface back up: "Workflows are not available with the
// Devin Local agent. Migrate your workflows to skills with the Devin:
// Open Cascade Migration Wizard command"
// (docs.devin.ai/desktop/devin-local, Limitations; target-audit
// 2026-09-09, #707). Emit no longer writes there: setting the key now
// only warns, naming the vendor's migration path. The native subagent
// file emits either way, unaffected.
//
// MCP servers merge into `.devin/mcp_config.json` (override via
// outputs.windsurf.mcp-file) under a root `mcpServers` map, the same
// shape Claude Code's `.mcp.json` uses. This is the file Devin Local,
// the default agent for new Devin Desktop tabs, reads for project
// scope, not Cascade: "The MCP configuration on this page applies to
// the legacy Cascade agent only. The Devin Local agent ... configures
// MCP servers in the Devin CLI config files instead"
// (docs.devin.ai/desktop/cascade/mcp); "New tabs start with Devin
// Local when you haven't chosen a preferred agent"
// (docs.devin.ai/desktop/devin-local) confirms it is the default.
// Cascade's own MCP file, `~/.codeium/windsurf/mcp_config.json`, is
// user-tier and stays out of reach: agnostic-ai only emits
// project-tier files, so this is a new surface, not a restored one
// (target-audit 2026-08-09, #587).
//
// Local (stdio) servers carry `command` (required) plus optional
// `args` / `env`. Remote servers carry `url` (required) plus optional
// `transport` (`http`, the default for URL-based servers, or legacy
// `sse`), `headers`, `oauthClientId`, `oauthClientSecret`, and
// `oauthResource` (docs.devin.ai/cli/extensibility/mcp/configuration).
// Both transports accept `disabled`, which `devin mcp enable|disable`
// also toggles on this file. The shared `emit.MCPSchemaServersMap`
// builder always writes the transport discriminant under the key
// `type`; Devin's own field is spelled `transport`, so this adapter
// holds its own schema in mcp.go rather than reuse it, the same reason
// trae and antigravity do.
//
// The vendor documents that the file moved here in v3000.3 (Local
// 3.6): older versions keyed `mcpServers` inside `.devin/config.json`,
// and any entries found there migrate automatically to the dedicated
// file on startup, so writing `.devin/mcp_config.json` is correct
// whichever version reads it.
//
// Hooks merge into `.devin/hooks.v1.json` (override via
// outputs.windsurf.hooks-file): "Create `.devin/hooks.v1.json` in your
// project" (docs.devin.ai/cli/extensibility/hooks/overview, #629). The
// hooks object is the entire file, no wrapper key, unlike the
// Claude-shaped `{"hooks": {...}}` form Claude Code, Codex, Gemini,
// and Qoder share; that is the one divergence from an otherwise
// familiar shape (per-event arrays of `{matcher, hooks: [{type,
// command, timeout}]}`, `matcher` a regex on `tool_name`). `type` also
// accepts `"prompt"` in place of `"command"`, evaluating an LLM prompt
// instead of running a shell command; agnostic-ai's generic hook spec
// has no `prompt` field, so that shape is reachable only through a
// hand-authored `type: prompt` plus a `prompt` Meta key. Devin CLI's
// own tool vocabulary is lowercase and snake_case (`exec`, `edit`,
// `read`, ...), not Claude's, so a Claude-style matcher parses but
// matches nothing; that case surfaces a coverage note rather than a
// guessed rename, the same treatment openhands and antigravity give
// their own mismatched vocabularies. `import windsurf` reads this
// file back the same way it already does for rules, agents, skills,
// and MCP: see internal/cli/import_windsurf_hooks.go.
//
// Settings specs merge into `.devin/config.json` (override via
// outputs.windsurf.conf-file) under `permissions`, the committed
// project policy Devin's own approval prompts write to: "Allow for
// project | `.devin/config.json` | Yes"
// (docs.devin.ai/cli/reference/permissions). Only that one key is set,
// so `read_config_from`, `hooks`, and any sibling inside `permissions`
// itself survive the sync; those three are the only keys Devin accepts
// there anyway ("Only `permissions`, `read_config_from`, and `hooks`
// are available in project configs",
// docs.devin.ai/cli/reference/configuration/config-file).
//
// A portable `model` deliberately stays out: the same page marks the
// `agent` block user only, so writing `agent.model` into a project
// config would reach nothing. It surfaces a coverage note instead.
//
// Devin's rule vocabulary is its own, so each rule translates rather
// than copies; see devinPermissionRule for the table. Its bare tool
// names are keyed separately from the subagent `allowed-tools` map,
// because the two vocabularies come from different pages and have
// already moved apart: `permissions` accepts `web_search` since CLI
// v3000.10.21 while the subagent docs enumerate no list at all (#951).
// A `Bash(...)` rule is never written verbatim: Devin spells shell
// execution `Exec(...)`. An exact `Bash(cmd)` has no faithful form at
// all, since Devin's `Exec` only ever prefix-matches, so the
// translation widens. Which way that cuts depends on the list.
// On `allow` and `ask` it approves commands the
// author never wrote, so the rule drops into a coverage note. On `deny`
// it blocks more than was asked for, and Devin's changelog for
// v3000.10.31 states a command deny outranks a broader allow or ask, so
// the widened `Exec(cmd)` is written rather than leaving the command
// unblocked (target-audit 2026-09-19, #887).
// `x-windsurf.permissions` passes through untranslated for an author
// who already knows Devin's spelling. See settings.go (target-audit
// 2026-09-18, #856).
//
// Ignore specs emit as both `.devinignore` (override via
// outputs.windsurf.ignore-file) and `.windsurfignore`, gitignore syntax
// under a `#` provenance header. The two names are not one path and its
// legacy alias; they cover different things. "you can add a
// `.devinignore` file to your repo root, with the same syntax as
// .gitignore" governs Devin Desktop Indexing, while "The agent
// additionally respects `.windsurfignore` files when accessing files"
// (docs.devin.ai/desktop/context-awareness/windsurf-ignore). Only
// `.codeiumignore` is legacy on that page, and it is an alias for the
// indexing file, reachable through the ignore-file override. Writing
// just the indexing file put a user's `secrets/**` out of the index and
// left the agent free to read and edit it (target-audit 2026-09-18,
// #863).
package windsurf

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target     = "windsurf"
	defaultDir = ".devin/rules"
	// legacyDir is the pre-rename rules path. Devin Desktop still reads
	// it, but managed files there predate the sync ledger on old
	// projects, so Emit sweeps the tree explicitly (header-guarded).
	legacyDir = ".windsurf/rules"
	// defaultAgentsDir is Devin CLI's native custom-subagent path:
	// "Custom subagents are defined as markdown files under `agents/`",
	// project layout `.devin/agents/`, flat file per profile
	// (docs.devin.ai/cli/subagents).
	defaultAgentsDir = ".devin/agents"
	// legacyAgentPrefix is the filename prefix agents carried while they
	// flattened into the rules directory. Emit sweeps a stale managed
	// copy for every current agent name.
	legacyAgentPrefix = "agent-"
	// defaultSkillsDir is the shared cross-tool skills tree Devin
	// Desktop scans; codex, amp, zed, crush, and openhands already
	// write here, so identical skill folders dedupe.
	defaultSkillsDir = ".agents/skills"
	// defaultIgnoreFile is the file Devin Desktop Indexing reads. Its
	// legacy sibling is `.codeiumignore`, which the vendor says "can be
	// used together" with this one.
	defaultIgnoreFile = ".devinignore"
	// agentIgnoreFile is the second, differently-scoped ignore file:
	// "The agent additionally respects `.windsurfignore` files when
	// accessing files." Indexing and file access are separate surfaces,
	// so one ignore spec writes both.
	agentIgnoreFile = ".windsurfignore"
	// defaultMCPFile is the project-scoped MCP config Devin Local
	// reads. The legacy Cascade agent has no project-tier MCP file of
	// its own to preserve compatibility with.
	defaultMCPFile = ".devin/mcp_config.json"
	// defaultConfigFile is Devin's committed project config, the file
	// its permission prompts write an "Allow for project" grant to.
	// This adapter only ever sets the `permissions` key on it.
	defaultConfigFile = ".devin/config.json"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindIgnore, spec.KindMCP, spec.KindHook, spec.KindSettings},
}

// Adapter emits Windsurf configs.
type Adapter struct{}

// New returns a Windsurf adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

func (Adapter) Capabilities() []spec.Kind { return caps.Supports }

// Emit writes one .md per rule into the rules directory (default
// `.devin/rules`, the path Devin Desktop prefers), or into
// `<scope>/<rules-dir>` for a scoped one, one .md per agent into the
// agents directory (default `.devin/agents`, Devin CLI's native
// subagent path), and one folder per skill under the skills directory
// (default `.agents/skills`, Devin Desktop's documented
// cross-agent-compatibility SKILL.md tree behind its own
// `.windsurf/skills/`; a flat file there never loads as a skill).
// Ignore specs merge into `.devinignore` (default; override via
// outputs.windsurf.ignore-file) for indexing and `.windsurfignore` for
// agent file access. MCP servers merge into
// `.devin/mcp_config.json` (default; override via
// outputs.windsurf.mcp-file), the file Devin Local reads. Hooks merge
// into `.devin/hooks.v1.json` (default; override via
// outputs.windsurf.hooks-file), Devin CLI's project-scoped hooks file
// (#629). `outputs.windsurf.workflows-dir`, still set on an old
// config, no longer writes anything: Emit warns instead
// (warnWorkflowsDirRemoved).
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	rulesDir := emit.OutputRulesDir(cfg, target, defaultDir)
	if err := sess.RulesDirectory(b, emit.RulesDirOpts{
		Dir:         rulesDir,
		SkipAgents:  true,
		SkipSkills:  true,
		ScopeAtRoot: true,
		FormatRule:  rule,
	}, dryRun); err != nil {
		return err
	}
	// Sweep managed leftovers at the pre-rename path unless the user
	// explicitly opted to keep emitting there. Hand-authored files (no
	// provenance marker) survive.
	if rulesDir != legacyDir {
		if err := sess.RemoveGeneratedTree(legacyDir, dryRun); err != nil {
			return err
		}
	}
	agentsDir := emit.OutputAgentsDir(cfg, target, defaultAgentsDir)
	if err := emitAgents(sess, b.Agents, agentsDir, rulesDir, dryRun); err != nil {
		return err
	}
	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)
	if err := sess.WriteSkillFolders(b.Skills, target, skillsDir, dryRun); err != nil {
		return err
	}
	if err := emitIgnores(sess, b.Ignores, cfg, dryRun); err != nil {
		return err
	}
	if err := emitMCP(sess, b.MCPs, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun); err != nil {
		return err
	}
	if err := emitHooks(sess, b.Hooks, cfg, dryRun); err != nil {
		return err
	}
	if err := emitConfig(sess, b.Settings, emit.OutputConfFile(cfg, target, defaultConfigFile), dryRun); err != nil {
		return err
	}
	warnWorkflowsDirRemoved(sess, cfg)
	return nil
}

// emitIgnores writes one Ignore spec to both files Devin reads, because
// they cover different things: `.devinignore` is what Devin Desktop
// Indexing skips, and "The agent additionally respects
// `.windsurfignore` files when accessing files"
// (docs.devin.ai/desktop/context-awareness/windsurf-ignore). Writing
// only the indexing file left the agent free to read and edit
// everything a user had excluded (target-audit 2026-09-18, #863).
//
// outputs.windsurf.ignore-file moves the indexing file, including onto
// the legacy `.codeiumignore` name. Pointed at `.windsurfignore`
// itself, the one file covers both surfaces and is written once.
func emitIgnores(sess *emit.Session, ignores []spec.Entry, cfg *config.Config, dryRun bool) error {
	indexFile := emit.OutputIgnoreFile(cfg, target, defaultIgnoreFile)
	if err := sess.WriteIgnoreFile(ignores, target, indexFile, dryRun); err != nil {
		return err
	}
	if filepath.Clean(indexFile) == agentIgnoreFile {
		return nil
	}
	return sess.WriteIgnoreFile(ignores, target, agentIgnoreFile, dryRun)
}

// The activation modes agnostic-ai emits through the `trigger`
// frontmatter key on a `.devin/rules/*.md` file
// (docs.devin.ai/cli/extensibility/rules,
// docs.devin.ai/desktop/cascade/memories). Devin's fifth documented
// value, `agent`, and its `always_on` default have no key written here:
// see activationFrontmatter for why always-on files stay bare, and
// docs/site/content/docs/targets/windsurf.md for why `agent` is out of reach.
const (
	triggerGlob          = "glob"
	triggerModelDecision = "model_decision"
	triggerManual        = "manual"
)

// rule renders one rule file: optional activation frontmatter, then the
// `# <name>` heading and body the shared rules-directory renderer
// writes.
//
// An always-on rule stays bare. Devin loads a rule file with no
// frontmatter as always-on and its Always On mode puts the full body in
// the system prompt on every message, so `description` has no job
// there; writing it would churn every existing file for no behavior
// change.
func rule(e spec.Entry) string {
	fm := activationFrontmatter(e)
	if fm == "" {
		// Byte-identical to emit's default rule renderer, which this
		// replaces only to add the frontmatter above.
		return emit.Header(emit.FormatMarkdown) + "\n# " + e.Name + "\n\n" + e.Body
	}
	return emit.WithHeader(fm+"# "+e.Name+"\n\n"+e.Body, emit.FormatMarkdown)
}

// activationFrontmatter maps a rule's generic activation fields onto
// Devin's `trigger`, and returns "" for an always-on rule so the file
// stays bare. The mapping mirrors the one cursor's `.mdc` renderer
// applies to the same three fields:
//
//	alwaysApply true or unset          -> always_on (no frontmatter)
//	alwaysApply false + globs          -> glob, with the globs verbatim
//	alwaysApply false + description    -> model_decision
//	alwaysApply false, neither         -> manual
//
// Before this existed the file carried no frontmatter at all, so a rule
// that declared `alwaysApply: false` was silently promoted to always-on
// (#628).
func activationFrontmatter(e spec.Entry) string {
	m := emit.ResolveMeta(e.Meta, target)
	if always, ok := m["alwaysApply"].(bool); !ok || always {
		return ""
	}
	desc, _ := m["description"].(string)
	globs, _ := m["globs"].(string)
	trigger := triggerManual
	switch {
	case globs != "":
		trigger = triggerGlob
	case desc != "":
		trigger = triggerModelDecision
	}
	var b strings.Builder
	b.WriteString("---\n")
	if desc != "" {
		b.WriteString("description: " + emit.YAMLScalar(desc) + "\n")
	}
	if globs != "" {
		b.WriteString("globs: " + emit.YAMLScalar(globs) + "\n")
	}
	b.WriteString("trigger: " + trigger + "\n")
	b.WriteString("---\n\n")
	return b.String()
}

// warnWorkflowsDirRemoved fires once per real sync when
// outputs.windsurf.workflows-dir is still configured. This adapter
// used to write one Workflow file per agent there; it no longer does.
//
// Devin Desktop v3.9.19 ("September 8, 2026") removed Cascade, the
// only agent that ever read a Workflow file: "Cascade has been
// removed. Devin Local is now the only agent available in Devin
// Desktop" (docs.devin.ai/desktop/changelog.md). Devin Local does not
// pick the surface back up: "Workflows are not available with the
// Devin Local agent. Migrate your workflows to skills with the Devin:
// Open Cascade Migration Wizard command"
// (docs.devin.ai/desktop/devin-local, Limitations; target-audit
// 2026-09-09, #707). Unlike the rules legacyDir fallback above, which
// a real still-supported older layout still reads, there is no reader
// left for a Workflow file at all, so writing one is pure dead weight
// forever, not a compat tradeoff. This only warns and names the
// vendor's own migration path; any file it wrote to this directory
// before the removal carries the provenance marker and is swept by
// the sync ledger's orphan sweep on the next full sync, same as any
// other output this adapter stops emitting.
func warnWorkflowsDirRemoved(sess *emit.Session, cfg *config.Config) {
	if sess.IsCapturing() {
		return
	}
	dir := emit.OutputWorkflowsDir(cfg, target, "")
	if dir == "" {
		return
	}
	_, _ = fmt.Fprintf(emit.Warner,
		"%s: outputs.windsurf.workflows-dir (%s) is set, but Devin Desktop removed Cascade in v3.9.19, the only agent that read Workflows; Devin Local does not support them. Nothing is written there anymore. Migrate to skills instead (Devin: Open Cascade Migration Wizard), or remove outputs.windsurf.workflows-dir to silence this.\n",
		target, dir)
}
