// Package pi emits native configuration for @earendil-works/pi-coding-agent 1.4.0.
package pi

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const target = "pi"

var caps = emit.Capabilities{Target: target, Supports: []spec.Kind{spec.KindRule, spec.KindSkill, spec.KindCommand, spec.KindMCP, spec.KindSettings}, SettingsFields: []string{"effort"}, SkillFields: emit.SkillFieldCoverage{Markdown: (Adapter{}).SkillMarkdown}}

type Adapter struct{}

func New() *Adapter                       { return &Adapter{} }
func (Adapter) Name() string              { return target }
func (Adapter) Capabilities() []spec.Kind { return caps.Supports }
func (Adapter) SettingsEffortLevels() []string {
	return []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}
}

func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	servers, err := buildMCPServers(b.MCPs)
	if err != nil {
		return err
	}
	if err := writeSkills(sess, b.Skills, emit.OutputSkillsDir(cfg, target, emit.OutputSubDir(cfg, target, "skills", ".pi/skills")), dryRun); err != nil {
		return err
	}
	if err := emitCommands(sess, b.Commands, emit.OutputCommandsDir(cfg, target, emit.OutputSubDir(cfg, target, "prompts", ".pi/prompts")), dryRun); err != nil {
		return err
	}
	if err := sess.EmitLegacyRulesFile(b, cfg, target, emit.MergedOpts{Title: "AGENTS.md"}, dryRun); err != nil {
		return err
	}
	if len(servers) > 0 {
		projectServers := make(map[string]any, len(servers))
		for name, server := range servers {
			projectServers[name] = emit.OrderSensitiveJSON(server)
		}
		if err := sess.MergeJSONFileNested(emit.OutputMCPFile(cfg, target, emit.OutputSubDir(cfg, target, "mcp.json", ".pi/mcp.json")), map[string]any{"mcpServers": projectServers}, []string{"mcpServers"}, dryRun); err != nil {
			return err
		}
	}
	return emitSettings(sess, b.Settings, emit.OutputConfFile(cfg, target, emit.OutputSubDir(cfg, target, "settings.json", ".pi/settings.json")), dryRun)
}
