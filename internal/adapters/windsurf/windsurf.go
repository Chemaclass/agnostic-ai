// Package windsurf emits .devin/rules/*.md for Devin Desktop, the
// renamed Windsurf editor (2026-06).
//
// Devin Desktop prefers `.devin/rules/*.md` and keeps `.windsurf/rules/`
// as a backward-compat fallback (`.windsurfrules` is legacy). Rules and
// agents emit as always-on files into the preferred directory; set
// `outputs.windsurf.rules-dir: .windsurf/rules` to stay on the old
// layout. Skills emit as a folder per skill under
// `.agents/skills/<name>/SKILL.md`, the tree Devin Desktop discovers
// skills from (docs.devin.ai/desktop/cascade/skills); the renderer
// matches codex, amp, zed, crush, and openhands byte-for-byte, so
// identical skills dedupe under `sync.shared-skills`. The target keeps
// its `windsurf` name so existing configs and `x-windsurf` meta
// continue to work.
//
// When `outputs.windsurf.workflows-dir` is set, each agent additionally
// emits as a Workflow at `<dir>/<name>.md`, invokable in Cascade chat
// as `/<name>` (upstream still documents `.windsurf/workflows/` only).
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
	// defaultSkillsDir is the shared cross-tool skills tree Devin
	// Desktop scans; codex, amp, zed, crush, and openhands already
	// write here, so identical skill folders dedupe.
	defaultSkillsDir = ".agents/skills"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule},
}

// Adapter emits Windsurf configs.
type Adapter struct{}

// New returns a Windsurf adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one .md per rule and per agent into the rules directory
// (default `.devin/rules`, the path Devin Desktop prefers), and one
// folder per skill under the skills directory (default
// `.agents/skills`, Devin Desktop's native SKILL.md tree; a flat file
// there never loads as a skill). When `outputs.windsurf.workflows-dir`
// is set, each agent additionally emits as a Workflow at
// `<dir>/<name>.md`; the rule-form `agent-<name>.md` emission stays in
// place so users that depend on it keep working.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	rulesDir := emit.OutputRulesDir(cfg, target, defaultDir)
	if err := sess.RulesDirectory(b, emit.RulesDirOpts{
		Dir:         rulesDir,
		AgentPrefix: "agent-",
		SkipSkills:  true,
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
