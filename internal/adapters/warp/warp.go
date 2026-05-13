// Package warp emits configs for the Warp terminal AI.
//
// Warp's current Rules docs specify `AGENTS.md` (per the open AGENTS.md
// standard) plus optional subtree `AGENTS.md` files that scope rules to
// their directory. Previous releases of this adapter wrote `WARP.md`
// (the legacy name), which newer Warp versions no longer read.
//
// Agents inline their bodies into the root AGENTS.md by default. When
// `outputs.warp.workflows-dir` is set, each agent additionally emits
// as a Warp Workflow YAML at `<dir>/<name>.yaml`, and the AGENTS.md
// `## Agents` section switches to reference pointers (description plus
// source path) so each agent body lives in exactly one place.
package warp

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

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

// Emit writes hierarchical AGENTS.md files and migrates any legacy
// agnostic-generated WARP.md to WARP.md.bak.
func (Adapter) Emit(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}

	emit.MigrateLegacyFile(cfg, target, legacyOutFile, defaultOutFile, dryRun)

	if err := emitAgentsTree(b, cfg, dryRun); err != nil {
		return err
	}
	if err := emitWorkflows(b, cfg, dryRun); err != nil {
		return err
	}
	return emit.WriteMCPFile(b.MCPs, emit.MCPSchemaServersMap,
		emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// workflowsEnabled reports whether the user opted into per-agent
// workflow YAML emission. When true, AGENTS.md downgrades agents to
// reference pointers so each body lives only in its workflow file.
func workflowsEnabled(cfg *config.Config) bool {
	return emit.OutputWorkflowsDir(cfg, target, "") != ""
}

// emitWorkflows writes one .warp/workflows/<name>.yaml per agent. The
// agent body becomes the workflow `command:`; description and tags are
// pulled from frontmatter when present.
func emitWorkflows(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	dir := emit.OutputWorkflowsDir(cfg, target, "")
	if dir == "" {
		return nil
	}
	for _, a := range b.Agents {
		doc, err := workflowYAML(a)
		if err != nil {
			return err
		}
		path := filepath.Join(dir, a.Name+".yaml")
		if err := emit.WriteFile(path, doc, dryRun); err != nil {
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

// emitAgentsTree writes one AGENTS.md per scope. Root document holds
// all rules at the root scope plus the agent (full bodies) and skill
// (reference) sections; scoped documents hold only their own rules.
func emitAgentsTree(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if len(b.Rules) == 0 && len(b.Agents) == 0 && len(b.Skills) == 0 {
		return nil
	}
	rootFile := emit.OutputFile(cfg, target, defaultOutFile)
	rootDir := filepath.Dir(rootFile)
	rootBase := filepath.Base(rootFile)

	byScope := emit.GroupRulesByScope(b.Rules)
	scopes := slices.Sorted(maps.Keys(byScope))
	if !slices.Contains(scopes, "") && (len(b.Agents) > 0 || len(b.Skills) > 0) {
		scopes = append([]string{""}, scopes...)
	}

	asReferences := workflowsEnabled(cfg)
	for _, scope := range scopes {
		var sb strings.Builder
		writeHeader(&sb, scope)
		writeRulesSection(&sb, byScope[scope])
		if scope == "" {
			writeAgentsSection(&sb, b.Agents, asReferences)
			writeSkillsSection(&sb, b.Skills)
		}
		path := filepath.Join(rootDir, scope, rootBase)
		if err := emit.WriteFile(path, sb.String(), dryRun); err != nil {
			return err
		}
	}
	return nil
}

func writeHeader(sb *strings.Builder, scope string) {
	if scope == "" {
		sb.WriteString("# AGENTS.md\n\n")
	} else {
		sb.WriteString("# AGENTS.md (" + scope + ")\n\n")
		sb.WriteString("Scoped to `" + scope + "/**`. Inherits root rules.\n\n")
	}
	sb.WriteString("Generated by agnostic-ai. Do not edit by hand.\n\n")
}

func writeRulesSection(sb *strings.Builder, rules []spec.Entry) {
	if len(rules) == 0 {
		return
	}
	sb.WriteString("## Rules\n\n")
	for _, r := range rules {
		emit.WriteSection(sb, r.Name, r)
	}
}

// writeAgentsSection inlines each agent body by default. When
// asReferences is true, the section instead lists each agent as a
// pointer (description + source path); the body lives in a Warp
// Workflow YAML so each agent has exactly one home.
func writeAgentsSection(sb *strings.Builder, agents []spec.Entry, asReferences bool) {
	if len(agents) == 0 {
		return
	}
	sb.WriteString("## Agents\n\n")
	if asReferences {
		sb.WriteString("Each agent ships as a Warp Workflow. Read the source file or invoke from the Warp launcher.\n\n")
		for _, a := range agents {
			emit.WriteReference(sb, a, a.Path)
		}
		return
	}
	for _, a := range agents {
		emit.WriteSection(sb, "Agent: "+a.Name, a)
	}
}

// writeSkillsSection lists skills as reference pointers (no execution
// model in Warp) so the file stays scannable.
func writeSkillsSection(sb *strings.Builder, skills []spec.Entry) {
	if len(skills) == 0 {
		return
	}
	sb.WriteString("## Skills\n\n")
	sb.WriteString("Reference material. Read the source file to use a skill.\n\n")
	for _, s := range skills {
		emit.WriteReference(sb, s, s.Path)
	}
}
