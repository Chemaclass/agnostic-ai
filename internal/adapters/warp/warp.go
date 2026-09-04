// Package warp emits configs for the Warp terminal AI.
//
// The project-root `AGENTS.md` is written centrally by `sync` as a
// slim pointer to the source specs (one body shared with every other
// target's entry-point file). When `outputs.warp.rules-file` is set,
// this adapter instead writes the legacy concatenated layout at that
// path so users on older workflows keep their behavior.
//
// When `outputs.warp.workflows-dir` is set, each agent emits as a Warp
// Workflow YAML at `<dir>/<name>.yaml` (`name`/`command`/`description`/
// `tags`). Other documented workflow fields (`shells`, `arguments`,
// `source_url`, `author`, `author_url`) pass through when declared
// under `x-warp`; `import warp` captures them back the same way.
// Previous releases of this adapter wrote `WARP.md` (the legacy name),
// which newer Warp versions no longer read.
//
// Skills emit natively as one folder per skill at
// `.agents/skills/<name>/SKILL.md`. Warp's docs
// (docs.warp.dev/agents/capabilities/skills) list ten directories it
// scans — `.agents/skills/` (recommended), `.warp/skills/`,
// `.claude/skills/`, `.codex/skills/`, `.cursor/skills/`,
// `.gemini/skills/`, `.copilot/skills/`, `.factory/skills/`,
// `.github/skills/`, and `.opencode/skills/`. A `WARP_SKILL_DIRS` env
// var (added in the 2026-08-07 changelog) indexes further directories,
// but only for Cloud agents: the vendor scopes it to skills that live
// outside the repositories a cloud run already has, not a general
// extension of the ten scanned directories above (target-audit
// 2026-09-03, #663; first cited 2026-08-09, #590). `.opencode/skills/`
// is OpenCode's own default, so a project running both tools gets
// Warp's scan for free. This adapter defaults to `.agents/skills/`,
// the vendor's own recommended path and the tree codex, amp, zed,
// crush, and others already emit into, so identical skill folders
// dedupe there (#557).
//
// MCP servers write to `.warp/.mcp.json` (mcp.go). A stdio server's
// `working_directory` reads from the spec's cross-tool `cwd` field
// (docs.warp.dev/agents/capabilities/mcp, #606); `import warp` renames
// it back on the way in. Remote servers carry no `type` discriminant,
// since Warp's own schema has none (#592). The emitted key set is
// exactly the two vendor tables: `description`, `disabled`, and `roots`
// are documented nowhere on that page and no longer emit, with
// `disabled` raising a coverage note and the other two reachable
// through `x-warp` (#641).
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
	target           = "warp"
	defaultOutFile   = "AGENTS.md"
	defaultSkillsDir = ".agents/skills"
	defaultMCPFile   = ".warp/.mcp.json"
	legacyOutFile    = "WARP.md"
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

// Emit writes one native skill folder per skill under .agents/skills/,
// any Warp Workflow YAMLs (when `outputs.warp.workflows-dir` is set),
// `.warp/.mcp.json`, and—when opted in via outputs.warp.rules-file—a
// legacy concatenated rules document. The project-root AGENTS.md is
// written by `sync`, not here. Legacy agnostic-generated WARP.md is
// migrated to WARP.md.bak on first sync.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}

	sess.MigrateLegacyFile(cfg, target, legacyOutFile, defaultOutFile, dryRun)

	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)
	if err := sess.WriteSkillFolders(b.Skills, target, skillsDir, dryRun); err != nil {
		return err
	}
	if err := emitWorkflows(sess, b, cfg, dryRun); err != nil {
		return err
	}
	if err := sess.EmitLegacyRulesFile(b, cfg, target, emit.MergedOpts{
		Title:              "AGENTS.md",
		AgentSectionPrefix: "Agent: ",
	}, dryRun); err != nil {
		return err
	}
	return emitMCP(sess, b.MCPs, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// emitWorkflows writes one .warp/workflows/<name>.yaml per agent. The
// agent body becomes the workflow `command:`; description and tags are
// pulled from frontmatter when present.
func emitWorkflows(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
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
// shell snippet from there. Any other documented workflow field
// (`shells`, `arguments`, `source_url`, `author`, `author_url`, ...)
// passes through when declared under `x-warp`, the same
// emit.MergeCustomTargetMeta pattern opencode.go's commandFile uses.
// yaml.Marshal on a plain map sorts keys alphabetically, so the
// resulting key order is deterministic regardless of merge order.
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
	var keys []string
	emit.MergeCustomTargetMeta(doc, &keys, e.Meta, target, "name", "command", "description", "tags")

	raw, err := yaml.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("marshal workflow %s: %w", e.Name, err)
	}
	return string(raw), nil
}
