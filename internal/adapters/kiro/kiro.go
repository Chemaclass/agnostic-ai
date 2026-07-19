// Package kiro emits steering files for AWS Kiro.
//
// Kiro loads Markdown steering documents from `.kiro/steering/`. Every
// file starts with a YAML frontmatter block (it must be the first
// content in the file, no blank line before it) whose `inclusion` key
// picks one of three loading modes:
//
//   - `always`: loaded on every interaction. Used for rules with no
//     glob or scope to target.
//   - `fileMatch` (+ `fileMatchPattern`): loaded when the active file
//     matches the pattern. Used for rules that carry `globs` or a
//     source-layout scope.
//   - `manual`: loaded on demand via `#steering-file-name` in chat.
//     Used for agents, since Kiro has no native agent-profile surface.
//
// Skills also become steering files, using a fourth mode Kiro reserves
// for skill-like matching:
//
//   - `auto` (+ `name`, `description`): Kiro matches the name and
//     description against the user's request and loads the file when
//     it looks relevant, mirroring skill semantics.
//
// A skill with bundled sibling assets cannot carry them in a flat
// steering file; those skills surface a coverage note instead of
// silently dropping the assets.
//
// The root `AGENTS.md` entry-point (which Kiro reads directly and
// always includes) is written centrally by `sync`, not by this
// adapter.
//
// MCP servers write to `.kiro/settings/mcp.json` as a `mcpServers` map
// with `command`, `args`, and optional `env` per local server.
package kiro

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target              = "kiro"
	defaultSteeringDir  = ".kiro/steering"
	defaultMCPFile      = ".kiro/settings/mcp.json"
	agentFilenamePrefix = "agent-"
	skillFilenamePrefix = "skill-"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindMCP},
}

// Adapter emits AWS Kiro configs.
type Adapter struct{}

// New returns a Kiro adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one steering file per rule, agent, and skill into the
// steering directory, plus `.kiro/settings/mcp.json` when MCP entries
// exist.
func (Adapter) Emit(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	dir := emit.OutputRulesDir(cfg, target, defaultSteeringDir)
	if err := emitRules(b.Rules, dir, dryRun); err != nil {
		return err
	}
	if err := emitAgents(b.Agents, dir, dryRun); err != nil {
		return err
	}
	if err := emitSkills(b.Skills, dir, dryRun); err != nil {
		return err
	}
	return emit.WriteMCPFile(b.MCPs, emit.MCPSchemaServersMap,
		emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// emitRules writes one `<dir>/<name>.md` per rule. Rules that target a
// glob or a source-layout scope render `inclusion: fileMatch`;
// everything else renders `inclusion: always`.
func emitRules(rules []spec.Entry, dir string, dryRun bool) error {
	for _, r := range rules {
		path := filepath.Join(dir, r.Name+".md")
		body := emit.WithHeader(renderRule(r), emit.FormatMarkdown)
		if err := emit.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// emitAgents writes one `<dir>/agent-<name>.md` per agent with
// `inclusion: manual`, so an agent loads only when invoked by name.
func emitAgents(agents []spec.Entry, dir string, dryRun bool) error {
	for _, a := range agents {
		path := filepath.Join(dir, agentFilenamePrefix+a.Name+".md")
		body := emit.WithHeader(renderAgent(a), emit.FormatMarkdown)
		if err := emit.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// emitSkills writes one `<dir>/skill-<name>.md` per skill with
// `inclusion: auto`, `name`, and `description`, the mode Kiro matches
// against user requests. Folder-based skills that carry sibling assets
// beyond SKILL.md surface a coverage note, since a flat steering file
// cannot represent bundled files.
func emitSkills(skills []spec.Entry, dir string, dryRun bool) error {
	withAssets := 0
	for _, s := range skills {
		path := filepath.Join(dir, skillFilenamePrefix+s.Name+".md")
		body := emit.WithHeader(renderSkill(s), emit.FormatMarkdown)
		if err := emit.WriteFile(path, body, dryRun); err != nil {
			return err
		}
		if emit.SkillHasBundledAssets(s, emit.SkipSKILLMd) {
			withAssets++
		}
	}
	emit.NoteCoverageGap(target, spec.KindSkill, withAssets, "bundled assets stay in the source dir")
	return nil
}

// renderRule renders a rule's steering-file body: frontmatter first,
// then a blank line, then the spec body.
func renderRule(e spec.Entry) string {
	front, keys := ruleFrontmatter(e)
	return withFrontmatter(front, keys, e.Body)
}

// ruleFrontmatter picks `inclusion: fileMatch` (with `fileMatchPattern`)
// for a rule that targets a glob or a source-layout scope, otherwise
// `inclusion: always`.
func ruleFrontmatter(e spec.Entry) (map[string]any, []string) {
	if pattern := fileMatchPatternFor(e); pattern != "" {
		return map[string]any{
			"inclusion":        "fileMatch",
			"fileMatchPattern": pattern,
		}, []string{"inclusion", "fileMatchPattern"}
	}
	return map[string]any{"inclusion": "always"}, []string{"inclusion"}
}

// fileMatchPatternFor returns the fileMatchPattern glob for a rule.
// Explicit `globs` (resolved for target-specific overrides) wins;
// otherwise the source-layout scope (e.g. `rules/backend/auth.md` ->
// "backend/**"); otherwise "" (the rule has nothing to scope to and
// loads always).
func fileMatchPatternFor(e spec.Entry) string {
	m := emit.ResolveMeta(e.Meta, target)
	if g, _ := m["globs"].(string); g != "" {
		return g
	}
	if s := e.EffectiveScope(); s != "" {
		return s + "/**"
	}
	return ""
}

// renderAgent renders an agent's steering-file body with
// `inclusion: manual`, so Kiro loads it only on demand.
func renderAgent(e spec.Entry) string {
	front := map[string]any{"inclusion": "manual"}
	return withFrontmatter(front, []string{"inclusion"}, e.Body)
}

// renderSkill renders a skill's steering-file body with
// `inclusion: auto`, `name`, and `description`, so Kiro auto-matches it
// against user requests the way it would a skill. Description falls
// back to the skill's name when the spec has none.
func renderSkill(e spec.Entry) string {
	m := emit.ResolveMeta(e.Meta, target)
	desc, _ := m["description"].(string)
	if desc == "" {
		desc = e.Name
	}
	front := map[string]any{
		"inclusion":   "auto",
		"name":        e.Name,
		"description": desc,
	}
	return withFrontmatter(front, []string{"inclusion", "name", "description"}, e.Body)
}

// withFrontmatter joins a rendered frontmatter block with body,
// separated by a blank line, in the shape Kiro requires: frontmatter
// as the very first bytes of the file.
func withFrontmatter(front map[string]any, keys []string, body string) string {
	var b strings.Builder
	b.WriteString(emit.FrontmatterOrdered(front, keys))
	b.WriteString("\n")
	b.WriteString(body)
	return b.String()
}
