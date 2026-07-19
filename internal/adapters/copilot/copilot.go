// Package copilot emits GitHub Copilot custom instructions.
//
// Rules with `globs` (or a source-layout scope) emit as per-file
// `.github/instructions/<name>.instructions.md` with an `applyTo:`
// frontmatter, matching Copilot's native path-scoped instructions
// format.
//
// The `.github/copilot-instructions.md` entry-point is written
// centrally by `sync` as a slim pointer to the source specs (one body
// shared with every other target's entry-point file). When
// `outputs.copilot.rules-file` is set, this adapter instead writes the
// legacy concatenated always-on-rules layout at that path so users on
// older workflows keep their behavior.
//
// Agents emit natively as custom-agent profiles at
// `.github/agents/<name>.agent.md` and skills as one folder per skill
// at `.github/skills/<name>/SKILL.md` with bundled assets, the two
// surfaces Copilot discovers directly.
package copilot

import (
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target                 = "copilot"
	defaultInstructionsDir = ".github/instructions"
	defaultAgentsDir       = ".github/agents"
	defaultSkillsDir       = ".github/skills"
	defaultMCPFile         = ".vscode/mcp.json"
	instructionFileSuffix  = ".instructions.md"
	agentFileSuffix        = ".agent.md"
	catchAllApplyTo        = "**"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindMCP},
}

// Adapter emits GitHub Copilot configs.
type Adapter struct{}

// New returns a Copilot adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes per-rule instructions, one native agent profile per
// agent, one native skill folder per skill, the optional legacy
// concatenated always-on rules file (only when
// `outputs.copilot.rules-file` is set), and `.vscode/mcp.json` when MCP
// entries exist. When `outputs.copilot.chatmodes-dir` is set, also
// writes one Copilot Custom Chat Mode per agent at that directory. The
// `.github/copilot-instructions.md` entry-point is written by `sync`,
// not here.
func (Adapter) Emit(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	if err := emitInstructionFiles(b, cfg, dryRun); err != nil {
		return err
	}
	if err := emitAgents(b.Agents, emit.OutputAgentsDir(cfg, target, defaultAgentsDir), dryRun); err != nil {
		return err
	}
	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)
	for _, s := range b.Skills {
		if err := emitSkill(s, skillsDir, dryRun); err != nil {
			return err
		}
	}
	if err := emitChatmodes(b, cfg, dryRun); err != nil {
		return err
	}
	if err := emitLegacyRulesFile(b, cfg, dryRun); err != nil {
		return err
	}
	return emit.WriteMCPFile(b.MCPs, emit.MCPSchemaVSCodeServers,
		emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// emitChatmodes writes one `.chatmode.md` per agent under the
// configured chat-modes directory. No-op when the dir is unset, so
// existing setups are unaffected.
func emitChatmodes(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	dir := emit.OutputChatmodesDir(cfg, target, "")
	if dir == "" {
		return nil
	}
	for _, a := range b.Agents {
		path := filepath.Join(dir, a.Name+".chatmode.md")
		body := emit.WithHeader(renderChatmode(a), emit.FormatMarkdown)
		if err := emit.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// renderChatmode renders a Copilot Custom Chat Mode file. Frontmatter
// is restricted to the keys Copilot reads (description, tools, model);
// extras would noisy-warn on activation.
func renderChatmode(e spec.Entry) string {
	m := emit.ResolveMeta(e.Meta, target)
	desc, _ := m["description"].(string)
	model, _ := m["model"].(string)
	tools := emit.StringSlice(m["tools"])
	var b strings.Builder
	b.WriteString("---\n")
	if desc != "" {
		b.WriteString("description: " + desc + "\n")
	}
	if model != "" {
		b.WriteString("model: " + model + "\n")
	}
	if len(tools) > 0 {
		b.WriteString("tools: [")
		for i, t := range tools {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(t)
		}
		b.WriteString("]\n")
	}
	b.WriteString("---\n\n")
	b.WriteString(e.Body)
	return b.String()
}

// emitInstructionFiles writes one `.instructions.md` per rule. Scoped
// rules carry their `applyTo` glob; always-on rules emit as catch-all
// (`applyTo: "**"`) instructions so they reach Copilot by default. The
// one exception: when the user opted into the legacy concatenated
// layout via `outputs.copilot.rules-file`, always-on rules go there
// instead and are skipped here to avoid duplication.
func emitInstructionFiles(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	dir := emit.OutputInstructionsDir(cfg, target, defaultInstructionsDir)
	legacyRulesFile := emit.OutputRulesFile(cfg, target, "") != ""

	for _, r := range b.Rules {
		if isAlwaysOn(r) && legacyRulesFile {
			continue
		}
		if err := writeInstruction(dir, applyToFor(r), r, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// writeInstruction writes one `<name>.instructions.md` into dir.
func writeInstruction(dir, applyTo string, e spec.Entry, dryRun bool) error {
	path := filepath.Join(dir, e.Name+instructionFileSuffix)
	body := emit.WithHeader(renderInstruction(e, applyTo), emit.FormatMarkdown)
	return emit.WriteFile(path, body, dryRun)
}

// emitLegacyRulesFile writes always-on rules to the
// outputs.copilot.rules-file path (when set). No-op when the user has
// not opted into the legacy concatenated layout; `sync` writes the
// canonical pointer body to `.github/copilot-instructions.md` instead.
func emitLegacyRulesFile(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	rulesFile := emit.OutputRulesFile(cfg, target, "")
	if rulesFile == "" {
		return nil
	}
	alwaysOn := alwaysOnRules(b.Rules)
	if len(alwaysOn) == 0 {
		return nil
	}
	var sb strings.Builder
	sb.WriteString("# GitHub Copilot Instructions\n\n")
	sb.WriteString("Generated by agnostic-ai.\n\n")
	sb.WriteString("## Rules\n\n")
	for _, r := range alwaysOn {
		emit.WriteSection(&sb, r.Name, r)
	}
	return emit.WriteFile(rulesFile, sb.String(), dryRun)
}

func alwaysOnRules(rules []spec.Entry) []spec.Entry {
	out := make([]spec.Entry, 0, len(rules))
	for _, r := range rules {
		if isAlwaysOn(r) {
			out = append(out, r)
		}
	}
	return out
}

// isAlwaysOn reports whether a rule belongs in the merged main file
// rather than as a scoped per-file instruction. An explicit
// `alwaysApply: true` wins; otherwise a rule is always-on only when it
// has neither `globs` nor a source-layout scope to target.
func isAlwaysOn(r spec.Entry) bool {
	m := emit.ResolveMeta(r.Meta, target)
	if v, ok := m["alwaysApply"].(bool); ok && v {
		return true
	}
	if g, _ := m["globs"].(string); g != "" {
		return false
	}
	return r.EffectiveScope() == ""
}

// applyToFor returns the `applyTo` glob for a per-file instruction.
// Explicit `globs` wins; otherwise the source-layout scope (e.g.
// `rules/backend/auth.md` -> "backend/**"); otherwise the catch-all.
func applyToFor(e spec.Entry) string {
	m := emit.ResolveMeta(e.Meta, target)
	if g, _ := m["globs"].(string); g != "" {
		return g
	}
	if s := e.EffectiveScope(); s != "" {
		return s + "/**"
	}
	return catchAllApplyTo
}

// renderInstruction renders a single `.instructions.md` body with
// `applyTo:` frontmatter, an italic description (when present), and
// the spec body.
func renderInstruction(e spec.Entry, applyTo string) string {
	front := map[string]any{"applyTo": applyTo}
	keys := []string{"applyTo"}
	// applyTo stays double-quoted (its glob can start with `*`, which a
	// plain scalar cannot). Forcing the source style keeps existing files
	// byte-identical while custom x-copilot keys append below it. See #367.
	styles := map[string]yaml.Style{"applyTo": yaml.DoubleQuotedStyle}
	emit.MergeCustomTargetMeta(front, &keys, e.Meta, target, "applyTo")
	var b strings.Builder
	b.WriteString(emit.FrontmatterStyled(front, keys, styles))
	b.WriteString("\n")
	if d := e.Description(); d != "" {
		b.WriteString("_" + d + "_\n\n")
	}
	b.WriteString(e.Body)
	return b.String()
}
