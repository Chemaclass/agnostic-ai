// Package cline emits .clinerules/*.md for the Cline VSCode extension.
//
// Rules and agents emit as `.clinerules/*.md` (always-on). Skills emit
// as a folder per skill under `.cline/skills/<name>/SKILL.md`, Cline's
// recommended skills path (docs.cline.bot/customization/skills also
// accepts `.clinerules/skills/` and `.claude/skills/`, but never a flat
// file directly under `.clinerules/`); the SKILL.md frontmatter carries
// `name` + `description` and sibling assets copy byte-for-byte. Cline
// also reads the cross-tool root `AGENTS.md`, which is written
// centrally by `sync` as a slim pointer to the source specs (shared
// with codex, amp, warp, and zed). When `outputs.cline.workflows-dir`
// is set, each agent additionally emits as a Cline Workflow at
// `<dir>/<name>.md`, invokable in chat as `/<name>.md` (Cline's
// slash-command surface for workflows).
package cline

import (
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target           = "cline"
	defaultDir       = ".clinerules"
	defaultSkillsDir = ".cline/skills"
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

// Emit writes one .md per rule and per agent into the rules directory,
// and one folder per skill under the skills directory (Cline's native
// SKILL.md layout; a flat file there never loads as a skill). When
// `outputs.cline.workflows-dir` is set, each agent additionally emits
// as a Cline Workflow at `<dir>/<name>.md`; the existing
// `.clinerules/agent-<name>.md` rule-form emission stays in place so
// users that depend on it keep working.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	if err := sess.RulesDirectory(b, emit.RulesDirOpts{
		Dir:         emit.OutputRulesDir(cfg, target, defaultDir),
		AgentPrefix: "agent-",
		SkipSkills:  true,
	}, dryRun); err != nil {
		return err
	}
	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)
	if err := sess.WriteSkillFolders(b.Skills, target, skillsDir, dryRun); err != nil {
		return err
	}
	return emitWorkflows(sess, b, cfg, dryRun)
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
