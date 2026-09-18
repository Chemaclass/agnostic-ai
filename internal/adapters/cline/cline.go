// Package cline emits configs for the Cline VSCode extension and CLI.
//
// Rules emit as Markdown files under `.clinerules/`, the only project
// rules path any Cline surface reads. The shipping VS Code extension
// hardcodes it in `GlobalFileNames` (`clineRules: ".clinerules"`,
// apps/vscode/src/core/storage/disk.ts) and resolves it against the
// workspace root, and the README puts the CLI and the JetBrains plugin
// on the same path. `.cline/rules/` appears only in the project tree on
// docs.cline.bot/getting-started/config, which is the outlier: it
// disagrees with the extension it documents, and no loader in
// cline/cline reads it. Releases #534 through #853 defaulted there on
// that page alone, so every rule landed where nothing loaded it
// (target-audit 2026-09-18, #853). Set `outputs.cline.rules-dir:
// .cline/rules` to keep emitting at the documented-but-unread path; a
// stale managed tree there is swept on sync unless that override is
// set.
//
// Agents stay at `.cline/agents/`. That path is absent from
// `GlobalFileNames` too, but no source evidence contradicts the config
// page for it, and one page being wrong about rules does not settle
// agents. It needs its own runtime check before it moves. Skills stay
// at `.cline/skills/` on positive evidence: `clineSkillsDir:
// ".cline/skills"` sits in the same `GlobalFileNames` block.
//
// `.cline/agents/` has no dedicated file-format doc page either (the
// "Subagents" feature is an unrelated, ephemeral parallel-research
// tool, not a file-backed profile), so this adapter writes the spec
// body verbatim: no synthesized heading, and no invented frontmatter
// keys with no vendor confirmation behind them. A synthesized "# Agent:
// <name>" heading (the pre-#534 rule-form shape) would round-trip back
// into the body on the next `import cline` and double itself on the
// next sync, since nothing downstream of a native, un-prefixed agent
// file expects to strip one back out.
//
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
// A rule that asks not to be always-on but gives nothing to match on
// has no Cline representation: `paths: []` is documented, but it "means
// the rule never activates", which disables the rule instead of
// narrowing it, so that case stays always-active and surfaces as a
// coverage note.
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
	defaultRulesDir  = ".clinerules"
	defaultAgentsDir = ".cline/agents"
	defaultSkillsDir = ".cline/skills"
	// unreadRulesDir is the path the vendor config page shows and no
	// Cline loader reads (see the package doc). Releases #534 through
	// #853 defaulted there, so a sync that lands on the real path
	// sweeps a stale managed copy here, unless the user opted into it
	// explicitly via outputs.cline.rules-dir.
	unreadRulesDir = ".cline/rules"
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

func (Adapter) Capabilities() []spec.Kind { return caps.Supports }

// Emit writes one .md per rule under the rules directory (default
// `.clinerules`), one .md per agent under the agents directory
// (default `.cline/agents`), and one folder per skill under the skills
// directory (Cline's native SKILL.md layout; a flat file there never
// loads as a skill). A stale managed tree at the unread `.cline/rules`
// path is swept unless the user explicitly opted into it via
// outputs.cline.rules-dir. When
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
	// Sweep managed leftovers at the path no Cline surface reads unless
	// the user explicitly opted to keep emitting there. Hand-authored
	// files (no provenance marker) survive.
	if rulesDir != unreadRulesDir {
		if err := sess.RemoveGeneratedTree(unreadRulesDir, dryRun); err != nil {
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
