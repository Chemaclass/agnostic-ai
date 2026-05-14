// Package windsurf emits .windsurf/rules/*.md for the Windsurf editor.
//
// Rules and agents emit as `.windsurf/rules/*.md` (always-on). When
// `outputs.windsurf.workflows-dir` is set, each agent additionally
// emits as a Windsurf Workflow at `<dir>/<name>.md`, invokable in
// Cascade chat as `/<name>`.
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
	defaultDir = ".windsurf/rules"
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

// Emit writes one .md per rule and per agent into the rules directory.
// When `outputs.windsurf.workflows-dir` is set, each agent additionally
// emits as a Windsurf Workflow at `<dir>/<name>.md`; the existing
// `.windsurf/rules/agent-<name>.md` rule-form emission stays in place
// so users that depend on it keep working.
func (Adapter) Emit(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	if err := emit.RulesDirectory(b, emit.RulesDirOpts{
		Dir:         emit.OutputRulesDir(cfg, target, defaultDir),
		AgentPrefix: "agent-",
	}, dryRun); err != nil {
		return err
	}
	return emitWorkflows(b, cfg, dryRun)
}

// emitWorkflows writes one workflow per agent under the configured
// workflows directory. No-op when the dir is unset.
func emitWorkflows(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	dir := emit.OutputWorkflowsDir(cfg, target, "")
	if dir == "" {
		return nil
	}
	for _, a := range b.Agents {
		path := filepath.Join(dir, a.Name+".md")
		if err := emit.WriteFile(path, emit.WithHeader(renderWorkflow(a), emit.FormatMarkdown), dryRun); err != nil {
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
