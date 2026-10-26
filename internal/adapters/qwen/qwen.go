// Package qwen emits native Qwen Code configuration for released CLI v0.26.0.
//
// Unscoped rules are inlined into QWEN.md. A rule with file globs goes to
// `.qwen/rules/<name>.md` with `paths:` frontmatter, which Qwen keeps out of
// the prompt until a tool reads or edits a matching file
// (packages/core/src/config/rulesDiscovery.ts at v0.26.0).
package qwen

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target              = "qwen"
	defaultOutFile      = "QWEN.md"
	defaultSettingsFile = ".qwen/settings.json"
	defaultAgentsDir    = ".qwen/agents"
	defaultSkillsDir    = ".qwen/skills"
	defaultCommandsDir  = ".qwen/commands"
	defaultRulesDir     = ".qwen/rules"
	defaultIgnoreFile   = ".qwenignore"
)

var caps = emit.Capabilities{
	Target:            target,
	Supports:          []spec.Kind{spec.KindRule, spec.KindAgent, spec.KindSkill, spec.KindCommand, spec.KindMCP, spec.KindHook, spec.KindSettings, spec.KindIgnore},
	AgentFields:       []string{"maxTurns"},
	SkillFields:       emit.SkillFieldCoverage{Markdown: (Adapter{}).SkillMarkdown},
	AgentFieldReasons: map[string]string{"mcpServers": "Qwen requires inline server records; use x-qwen.mcpServers"},
}

type Adapter struct{}

func New() *Adapter                       { return &Adapter{} }
func (Adapter) Name() string              { return target }
func (Adapter) Capabilities() []spec.Kind { return caps.Supports }

func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	if err := (Adapter{}).EmitAgents(sess, b.Agents, emit.OutputAgentsDir(cfg, target, defaultAgentsDir), dryRun); err != nil {
		return err
	}
	if err := sess.WriteScopedSkillFoldersWith(b.Skills, emit.OutputSkillsDir(cfg, target, defaultSkillsDir), dryRun, (Adapter{}).SkillMarkdown); err != nil {
		return err
	}
	if err := emitCommands(sess, b.Commands, emit.OutputCommandsDir(cfg, target, defaultCommandsDir), dryRun); err != nil {
		return err
	}
	if err := sess.EmitLegacyRulesFile(b, cfg, target, emit.MergedOpts{Title: defaultOutFile}, dryRun); err != nil {
		return err
	}
	if err := emitPathRules(sess, b.Rules, cfg, dryRun); err != nil {
		return err
	}
	if err := emitSettings(sess, b, cfg, dryRun); err != nil {
		return err
	}
	return sess.WriteIgnoreFile(b.Ignores, target, emit.OutputIgnoreFile(cfg, target, defaultIgnoreFile), dryRun)
}

func (Adapter) RendersSpecFile(e spec.Entry, _ *config.Config, _ string) bool {
	if e.Kind != spec.KindSettings {
		return true
	}
	keys := map[string]any{}
	emit.MergeSettingsCustomKeys(keys, []spec.Entry{e}, target, "mcpServers")
	emit.MergeSettingsCustomRecordMap(keys, []spec.Entry{e}, target, "mcpServers")
	return emit.SettingsModel([]spec.Entry{e}, target) != "" || emit.SettingsPermissions([]spec.Entry{e}) != nil || len(keys) > 0
}
