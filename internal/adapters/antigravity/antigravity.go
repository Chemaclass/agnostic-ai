// Package antigravity emits configs for Google Antigravity IDE.
//
// Antigravity reads project instructions from per-rule files under
// `.agents/rules/*.md` and skills from a folder per skill under
// `.agents/skills/<name>/SKILL.md`. This adapter emits both. Antigravity
// "now defaults to `.agents/rules`, but still maintains backward support
// for `.agent/rules`" (antigravity.google/docs/ide/rules), and the
// same wording covers skills (/docs/ide/skills). This adapter defaults to
// the plural form and sweeps a stale managed tree at the pre-plural
// `.agent/rules` / `.agent/skills` paths on sync, the same pattern codex
// uses for its own pre-v0.43 `.codex/skills/` default. `.agents/skills`
// is also the shared tree codex, amp, zed, crush, openhands, and
// windsurf already emit into, so identical skill folders dedupe there
// once an adapter's default lands on it.
//
// The `.agent/AGENTS.md` entry-point is written centrally by `sync`
// as a slim pointer to the source specs (one body shared with every
// other target's entry-point file). When `outputs.antigravity.rules-file`
// is set, this adapter instead writes the legacy merged layout at that
// path so users on older workflows keep their behavior.
//
// MCP servers land in `.agents/mcp_config.json`, a single `mcpServers`
// object (antigravity.google/docs/ide/mcp). Remote servers require the
// `serverUrl` field: "Legacy fields like `url` or `httpUrl` are not
// supported," so this adapter cannot reuse the shared
// `emit.MCPSchemaServersMap` builder, which emits `url` (see mcp.go).
// stdio servers carry `command`, `args`, `env`, and `cwd`; remote
// servers add `headers`; both accept `disabled` under that literal
// name, unlike codex and kilo which map it onto their own
// `enabled: false`. Any other documented field (`authProviderType`,
// `oauth`, `disabledTools`, same page) or any field the vendor adds
// next reaches the file through an entry's `x-antigravity` block
// (emit.MergeCustomTargetMeta), the same escape hatch zed and warp give
// their own unmapped fields (#588); `import antigravity` captures it
// back the same way. Hooks now have a documented schema
// (antigravity.google/docs/ide/hooks: `.agents/hooks.json`, five
// events, PreToolUse/PostToolUse/PreInvocation/PostInvocation/Stop),
// but whether the IDE itself executes them stays unconfirmed
// (target-audit 2026-08-08, #563), so this adapter still skips hooks
// with a warning; commands remain fully unconfirmed in the
// public-preview docs and skip the same way.
package antigravity

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target           = "antigravity"
	defaultRulesDir  = ".agents/rules"
	defaultSkillsDir = ".agents/skills"
	defaultMCPFile   = ".agents/mcp_config.json"
	// legacyRulesDir and legacySkillsDir are the pre-plural defaults
	// (every release through the one preceding this fix). Antigravity
	// still reads them for backward compatibility, so Emit sweeps any
	// agnostic-ai-managed leftovers there once the active path differs
	// (i.e. the user did not opt into the legacy path explicitly via
	// the matching outputs.antigravity.* key). See the package doc.
	legacyRulesDir  = ".agent/rules"
	legacySkillsDir = ".agent/skills"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindMCP},
}

// Adapter emits Antigravity configs.
type Adapter struct{}

// New returns an Antigravity adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes per-rule files under .agents/rules/, a folder per skill
// under .agents/skills/<name>/SKILL.md, .agents/mcp_config.json for MCP
// servers, and, when opted in via outputs.antigravity.rules-file, a
// legacy merged document at that path. A stale managed tree at the
// pre-plural `.agent/rules` / `.agent/skills` defaults is swept unless
// the user explicitly opted into that legacy path. The `.agent/AGENTS.md`
// entry-point is written by `sync`, not here.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	rulesDir := emit.OutputRulesDir(cfg, target, defaultRulesDir)
	// Skills emit through the native folder layout below, so suppress
	// the rule-form `skill-<name>.md` output from RulesDirectory.
	if err := sess.RulesDirectory(b, emit.RulesDirOpts{
		Dir:         rulesDir,
		AgentPrefix: "agent-",
		SkipSkills:  true,
	}, dryRun); err != nil {
		return err
	}
	if rulesDir != legacyRulesDir {
		if err := sess.RemoveGeneratedTree(legacyRulesDir, dryRun); err != nil {
			return err
		}
	}

	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)
	if err := sess.WriteSkillFolders(b.Skills, target, skillsDir, dryRun); err != nil {
		return err
	}
	if skillsDir != legacySkillsDir {
		if err := sess.RemoveGeneratedTree(legacySkillsDir, dryRun); err != nil {
			return err
		}
	}

	if err := emitMCP(sess, b.MCPs, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun); err != nil {
		return err
	}

	return sess.EmitLegacyRulesFile(b, cfg, target, emit.MergedOpts{Title: "AGENTS.md"}, dryRun)
}
