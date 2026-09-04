// Package cline emits configs for the Cline VSCode extension and CLI.
//
// Rules emit as Markdown files under `.cline/rules/` and agents under
// `.cline/agents/`, the layout Cline's current config reference
// documents at
// `.cline/{rules,skills,hooks,agents,plugins,cron}/`
// (docs.cline.bot/getting-started/config, no mention of `.clinerules/`
// anywhere, not even as a fallback). The older
// docs.cline.bot/customization/cline-rules page still lists
// `.clinerules/` as the "Primary rule format", so this is a live
// migration rather than a completed removal (target-audit 2026-08-01,
// #534): set `outputs.cline.rules-dir: .clinerules` to keep emitting
// rules at the pre-migration path, the same opt-in-fallback shape the
// windsurf adapter uses for its own pre-rename path. A stale managed
// tree at the default `.clinerules` is swept on sync unless that
// override is set. Cline's `.cline/agents/` has no dedicated file-format
// doc page (its "Subagents" feature is an unrelated, ephemeral
// parallel-research tool, not a file-backed profile), so this adapter
// writes the spec body verbatim: no synthesized heading, and no
// invented frontmatter keys with no vendor confirmation behind them.
// A synthesized "# Agent: <name>" heading (the pre-migration rule-form
// shape) would round-trip back into the body on the next `import
// cline` and double itself on the next sync, since nothing downstream
// of a native, un-prefixed agent file expects to strip one back out.
// A rule that declares `alwaysApply: false` narrows through Cline's one
// conditional, `paths`: "Currently, `paths` is the supported
// conditional. It takes an array of glob patterns"
// (docs.cline.bot/customization/cline-rules). Explicit globs win over
// the rule's source-layout scope, which falls back to `<scope>/**`.
// Everything else stays bare, since "Rules without frontmatter are
// always active" and writing an empty block would churn every existing
// file for no behavior change. Before this existed no file carried
// frontmatter at all, so a rule scoped to `src/components/**` loaded on
// every request (target-audit 2026-08-27, #639).
//
// The wrinkle worth stating rather than papering over: the conditional
// -rules page says `.clinerules/` throughout and never names
// `.cline/rules/`, this adapter's default. The finding held either way,
// since no frontmatter was emitted at either path, and `paths` is the
// only activation key Cline documents anywhere. A rule that asks not to
// be always-on but gives nothing to match on has no Cline
// representation: `paths: []` is documented, but it "means the rule
// never activates", which disables the rule instead of narrowing it, so
// that case stays always-active and surfaces as a coverage note.
//
// Cline also reads the cross-tool root `AGENTS.md`, which is written
// centrally by `sync` as a slim pointer to the source specs (shared
// with codex, amp, warp, and zed).
//
// Skills emit as a folder per skill under `.cline/skills/<name>/
// SKILL.md`, Cline's recommended skills path
// (docs.cline.bot/customization/skills also accepts `.clinerules/
// skills/` and `.claude/skills/`, but never a flat file directly under
// `.clinerules/`); the SKILL.md frontmatter carries `name` +
// `description` and sibling assets copy byte-for-byte.
//
// When `outputs.cline.workflows-dir` is set, each agent additionally
// emits as a Markdown file at `<dir>/<name>.md`, in the shape this
// adapter calls a Workflow: invokable in chat as `/<name>.md`. Cline's
// own doc for this feature, docs.cline.bot/features/workflows, 404s,
// and `llms.txt` lists no project-scoped replacement: the current
// `customization/` tree covers Rules, `.clineignore`, Hooks, Plugins,
// and Skills only, no Workflows entry (target-audit 2026-08-08, #563).
// Treat this output as an unconfirmed export rather than a documented
// Cline surface until a current doc says otherwise.
package cline

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target           = "cline"
	defaultRulesDir  = ".cline/rules"
	defaultAgentsDir = ".cline/agents"
	defaultSkillsDir = ".cline/skills"
	// legacyDir is the pre-migration combined rules-and-agents
	// directory (every release through the one preceding this fix).
	// Cline's own docs currently disagree on whether it still applies
	// (see the package doc), so this adapter keeps it reachable as an
	// opt-in fallback via outputs.cline.rules-dir and sweeps a stale
	// managed copy there once the active rules-dir differs (i.e. the
	// user did not opt into the legacy path explicitly).
	legacyDir = ".clinerules"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule},
}

// Adapter emits Cline configs.
type Adapter struct{}

// New returns a Cline adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one .md per rule under the rules directory (default
// `.cline/rules`), one .md per agent under the agents directory
// (default `.cline/agents`), and one folder per skill under the skills
// directory (Cline's native SKILL.md layout; a flat file there never
// loads as a skill). A stale managed tree at the pre-migration
// `.clinerules` default is swept unless the user explicitly opted into
// that legacy path via outputs.cline.rules-dir. When
// `outputs.cline.workflows-dir` is set, each agent additionally emits
// as a Markdown file at `<dir>/<name>.md` in the shape this adapter
// calls a Workflow (see the package doc: the vendor doc for that
// surface is currently dead with no confirmed replacement); the native
// agent file emission stays in place either way.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	rulesDir := emit.OutputRulesDir(cfg, target, defaultRulesDir)
	if err := sess.RulesDirectory(b, emit.RulesDirOpts{
		Dir:        rulesDir,
		SkipAgents: true,
		SkipSkills: true,
		FormatRule: rule,
	}, dryRun); err != nil {
		return err
	}
	emit.NoteFieldNoOp(target, spec.KindRule, "alwaysApply", unscopableRules(b.Rules),
		"Cline's only conditional is `paths`; a rule with no globs and no scope stays always-active")
	// Sweep managed leftovers at the pre-migration path unless the user
	// explicitly opted to keep emitting there. Hand-authored files (no
	// provenance marker) survive.
	if rulesDir != legacyDir {
		if err := sess.RemoveGeneratedTree(legacyDir, dryRun); err != nil {
			return err
		}
	}

	agentsDir := emit.OutputAgentsDir(cfg, target, defaultAgentsDir)
	if err := emitAgentFiles(sess, b.Agents, agentsDir, dryRun); err != nil {
		return err
	}

	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)
	if err := sess.WriteSkillFolders(b.Skills, target, skillsDir, dryRun); err != nil {
		return err
	}
	return emitWorkflows(sess, b, cfg, dryRun)
}

// emitAgentFiles writes one `<dir>/<name>.md` per agent spec: the spec
// body verbatim, no synthesized heading (see the package doc for why).
func emitAgentFiles(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	for _, a := range agents {
		path := filepath.Join(dir, a.Name+".md")
		if err := sess.WriteFile(path, emit.WithHeader(agentFileMarkdown(a), emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
	}
	return nil
}

func agentFileMarkdown(a spec.Entry) string {
	return strings.TrimSpace(a.Body) + "\n"
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

// renderWorkflow returns the Markdown body for a Cline Workflow file.
// Cline workflows are plain Markdown with no required frontmatter; the
// description (when present) prefixes the body as an italic line so
// users can see at a glance what the workflow does when listed.
func renderWorkflow(e spec.Entry) string {
	if d := e.Description(); d != "" {
		return "_" + d + "_\n\n" + e.Body
	}
	return e.Body
}
