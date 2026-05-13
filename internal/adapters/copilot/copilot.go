// Package copilot emits GitHub Copilot custom instructions.
//
// Rules with `globs` (or a source-layout scope) emit as per-file
// `.github/instructions/<name>.instructions.md` with an `applyTo:`
// frontmatter, matching Copilot's native path-scoped instructions
// format. Always-on rules (no globs / no scope, or `alwaysApply: true`)
// merge into `.github/copilot-instructions.md`.
//
// Agents and skills always emit as catch-all instructions
// (`applyTo: "**"`) with `agent-` or `skill-` filename prefixes so
// they remain discoverable to Copilot's tools surface.
package copilot

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target                 = "copilot"
	defaultMainFile        = ".github/copilot-instructions.md"
	defaultInstructionsDir = ".github/instructions"
	defaultMCPFile         = ".vscode/mcp.json"
	instructionFileSuffix  = ".instructions.md"
	agentFilenamePrefix    = "agent-"
	skillFilenamePrefix    = "skill-"
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

// Emit writes per-file instructions, the always-on main file, and the
// `.vscode/mcp.json` when MCP entries exist. When
// `outputs.copilot.chatmodes-dir` is set, also writes one Copilot
// Custom Chat Mode per agent at that directory; the catch-all
// instruction-form emission still happens for back-compat.
func (Adapter) Emit(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	if err := emitInstructionFiles(b, cfg, dryRun); err != nil {
		return err
	}
	if err := emitChatmodes(b, cfg, dryRun); err != nil {
		return err
	}
	if err := emitMainFile(b, cfg, dryRun); err != nil {
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
		if err := emit.WriteFile(path, renderChatmode(a), dryRun); err != nil {
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

// emitInstructionFiles writes one `.instructions.md` per scoped rule, agent, and skill.
func emitInstructionFiles(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	dir := emit.OutputInstructionsDir(cfg, target, defaultInstructionsDir)

	for _, r := range b.Rules {
		if isAlwaysOn(r) {
			continue
		}
		if err := writeInstruction(dir, "", applyToFor(r), r, dryRun); err != nil {
			return err
		}
	}
	for _, a := range b.Agents {
		if err := writeInstruction(dir, agentFilenamePrefix, catchAllApplyTo, a, dryRun); err != nil {
			return err
		}
	}
	for _, s := range b.Skills {
		if err := writeInstruction(dir, skillFilenamePrefix, catchAllApplyTo, s, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// writeInstruction writes one `<prefix><name>.instructions.md` into dir.
func writeInstruction(dir, prefix, applyTo string, e spec.Entry, dryRun bool) error {
	path := filepath.Join(dir, prefix+e.Name+instructionFileSuffix)
	return emit.WriteFile(path, renderInstruction(e, applyTo), dryRun)
}

// emitMainFile writes always-on rules to `.github/copilot-instructions.md`.
// No file is written when no rule qualifies as always-on.
func emitMainFile(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	alwaysOn := alwaysOnRules(b.Rules)
	if len(alwaysOn) == 0 {
		return nil
	}
	var sb strings.Builder
	sb.WriteString("# GitHub Copilot Instructions\n\n")
	sb.WriteString("Generated by agnostic-ai. Do not edit by hand.\n\n")
	sb.WriteString("## Rules\n\n")
	for _, r := range alwaysOn {
		emit.WriteSection(&sb, r.Name, r)
	}
	return emit.WriteFile(emit.OutputFile(cfg, target, defaultMainFile), sb.String(), dryRun)
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
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("applyTo: \"" + applyTo + "\"\n")
	b.WriteString("---\n\n")
	if d := e.Description(); d != "" {
		b.WriteString("_" + d + "_\n\n")
	}
	b.WriteString(e.Body)
	return b.String()
}
