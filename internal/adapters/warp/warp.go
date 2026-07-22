// Package warp emits configs for the Warp terminal AI.
//
// The project-root `AGENTS.md` is written centrally by `sync` as a
// slim pointer to the source specs (one body shared with every other
// target's entry-point file). When `outputs.warp.rules-file` is set,
// this adapter instead writes the legacy concatenated layout at that
// path so users on older workflows keep their behavior.
//
// When `outputs.warp.workflows-dir` is set, each agent emits as a Warp
// Workflow YAML at `<dir>/<name>.yaml`. Previous releases of this
// adapter wrote `WARP.md` (the legacy name), which newer Warp versions
// no longer read.
package warp

import (
	"fmt"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target         = "warp"
	defaultOutFile = "AGENTS.md"
	defaultMCPFile = ".warp/.mcp.json"
	legacyOutFile  = "WARP.md"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindMCP},
}

// Adapter emits Warp configs.
type Adapter struct{}

// New returns a Warp adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes any Warp Workflow YAMLs (when `outputs.warp.workflows-dir`
// is set), `.warp/.mcp.json`, and—when opted in via
// outputs.warp.rules-file—a legacy concatenated rules document. The
// project-root AGENTS.md is written by `sync`, not here. Legacy
// agnostic-generated WARP.md is migrated to WARP.md.bak on first sync.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}

	sess.MigrateLegacyFile(cfg, target, legacyOutFile, defaultOutFile, dryRun)

	if err := emitWorkflows(sess, b, cfg, dryRun); err != nil {
		return err
	}
	if err := sess.EmitLegacyRulesFile(b, cfg, target, emit.MergedOpts{
		Title:              "AGENTS.md",
		AgentSectionPrefix: "Agent: ",
	}, dryRun); err != nil {
		return err
	}
	return sess.WriteMCPFile(b.MCPs, emit.MCPSchemaServersMap,
		emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// emitWorkflows writes one .warp/workflows/<name>.yaml per agent. The
// agent body becomes the workflow `command:`; description and tags are
// pulled from frontmatter when present.
func emitWorkflows(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	// Warp has no native skill surface: skills never reach it regardless of
	// workflows-dir, so note the gap unconditionally.
	emit.NoteCoverageGap(target, spec.KindSkill, len(b.Skills), "no native skill surface")
	dir := emit.OutputWorkflowsDir(cfg, target, "")
	if dir == "" {
		emit.NoteCoverageGap(target, spec.KindAgent, len(b.Agents),
			"outputs.warp.workflows-dir")
		return nil
	}
	for _, a := range b.Agents {
		doc, err := workflowYAML(a)
		if err != nil {
			return err
		}
		path := filepath.Join(dir, a.Name+".yaml")
		if err := sess.WriteFile(path, emit.WithHeader(doc, emit.FormatYAML), dryRun); err != nil {
			return err
		}
	}
	return nil
}

// workflowYAML renders one agent as a Warp Workflow per the
// docs.warp.dev schema (name, command, description, tags). The body
// goes into `command:` verbatim; users tailor it to a Warp-friendly
// shell snippet from there.
func workflowYAML(e spec.Entry) (string, error) {
	m := emit.ResolveMeta(e.Meta, target)
	desc, _ := m["description"].(string)
	tags := emit.StringSlice(m["tags"])

	doc := map[string]any{
		"name":    e.Name,
		"command": e.Body,
	}
	if desc != "" {
		doc["description"] = desc
	}
	if len(tags) > 0 {
		doc["tags"] = tags
	}

	raw, err := yaml.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("marshal workflow %s: %w", e.Name, err)
	}
	return string(raw), nil
}
