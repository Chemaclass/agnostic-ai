// Package windsurf emits .devin/rules/*.md, .devin/agents/*.md, and
// .devin/mcp_config.json for Devin Desktop, the renamed Windsurf editor
// (2026-06).
//
// Devin Desktop prefers `.devin/rules/*.md` and keeps `.windsurf/rules/`
// as a backward-compat fallback (`.windsurfrules` is legacy). Rules emit
// into the preferred directory, always-on unless the rule declares
// otherwise (see activationFrontmatter); set
// `outputs.windsurf.rules-dir: .windsurf/rules` to stay on the old
// layout.
//
// A scoped rule goes to `<scope>/.devin/rules/<name>.md`, not to a
// subdirectory inside the rules dir. Devin reads "`.devin/rules` or
// `.windsurf/rules` in any sub-directory of your workspace"
// (docs.devin.ai/desktop/cascade/memories) and globs each one
// single-level as `.devin/rules/*.md`
// (docs.devin.ai/cli/extensibility/rules), so the nested form reached
// no documented discovery path (target-audit 2026-08-27, #628). The old
// tree is swept through the sync ledger.
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
// `allowed-tools` translates agnostic-ai's Claude-style names onto the
// five Devin publishes as its complete set, `read`, `edit`, `grep`,
// `glob`, `exec` (docs.devin.ai/cli/reference/permissions); see
// devinTool for what each collapses onto. A name outside that table is
// never guessed at: it drops from the list and folds into one coverage
// note per sync. `x-windsurf.allowed-tools` wins outright over the
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
// When `outputs.windsurf.workflows-dir` is set, each agent additionally
// emits as a Workflow at `<dir>/<name>.md`, invokable in Cascade chat
// as `/<name>` (upstream still documents `.windsurf/workflows/` only).
// The native subagent file emits either way.
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
// Ignore specs merge into `.devinignore` (override via
// outputs.windsurf.ignore-file), gitignore syntax under a `#`
// provenance header: "you can add a `.devinignore` file to your repo
// root, with the same syntax as .gitignore" (docs.devin.ai/desktop/
// context-awareness/windsurf-ignore). Devin Desktop also still respects
// the legacy `.codeiumignore` filename and `.windsurfignore`, but
// `.devinignore` is the vendor-documented current path, so it is the
// one this adapter writes.
package windsurf

import (
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
	// defaultIgnoreFile is the vendor-documented current ignore-file
	// path. Devin Desktop also still reads the legacy `.codeiumignore`
	// and `.windsurfignore` filenames, but this adapter only writes the
	// current one.
	defaultIgnoreFile = ".devinignore"
	// defaultMCPFile is the project-scoped MCP config Devin Local
	// reads. The legacy Cascade agent has no project-tier MCP file of
	// its own to preserve compatibility with.
	defaultMCPFile = ".devin/mcp_config.json"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindIgnore, spec.KindMCP},
}

// Adapter emits Windsurf configs.
type Adapter struct{}

// New returns a Windsurf adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one .md per rule into the rules directory (default
// `.devin/rules`, the path Devin Desktop prefers), or into
// `<scope>/<rules-dir>` for a scoped one, one .md per agent into the
// agents directory (default `.devin/agents`, Devin CLI's native
// subagent path), and one folder per skill under the skills directory
// (default `.agents/skills`, Devin Desktop's documented
// cross-agent-compatibility SKILL.md tree behind its own
// `.windsurf/skills/`; a flat file there never loads as a skill). When
// `outputs.windsurf.workflows-dir` is set, each agent additionally
// emits as a Workflow at `<dir>/<name>.md`. Ignore specs merge into
// `.devinignore` (default; override via outputs.windsurf.ignore-file).
// MCP servers merge into `.devin/mcp_config.json` (default; override
// via outputs.windsurf.mcp-file), the file Devin Local reads.
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
	if err := sess.WriteIgnoreFile(b.Ignores, emit.OutputIgnoreFile(cfg, target, defaultIgnoreFile), dryRun); err != nil {
		return err
	}
	if err := emitMCP(sess, b.MCPs, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun); err != nil {
		return err
	}
	return emitWorkflows(sess, b, cfg, dryRun)
}

// The activation modes agnostic-ai emits through the `trigger`
// frontmatter key on a `.devin/rules/*.md` file
// (docs.devin.ai/cli/extensibility/rules,
// docs.devin.ai/desktop/cascade/memories). Devin's fifth documented
// value, `agent`, and its `always_on` default have no key written here:
// see activationFrontmatter for why always-on files stay bare, and
// targets.md for why `agent` is out of reach.
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

// emitWorkflows writes one workflow per agent under the configured
// workflows directory. No-op when the dir is unset.
func emitWorkflows(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	dir := emit.OutputWorkflowsDir(cfg, target, "")
	if dir == "" {
		return nil
	}
	for _, a := range b.Agents {
		path := filepath.Join(dir, a.Name+".md")
		if err := sess.WriteFile(path, emit.WithHeader(renderWorkflow(a), emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
	}
	return nil
}

// renderWorkflow renders a Windsurf Workflow file. Frontmatter holds
// the `description` Cascade shows in the slash-command picker; the
// body is the prompt the agent runs when the workflow is invoked.
func renderWorkflow(e spec.Entry) string {
	desc := e.Description()
	var b strings.Builder
	b.WriteString("---\n")
	if desc != "" {
		b.WriteString("description: " + desc + "\n")
	}
	b.WriteString("---\n\n")
	b.WriteString(e.Body)
	return b.String()
}
