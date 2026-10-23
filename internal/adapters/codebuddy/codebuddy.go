// Package codebuddy emits Tencent CodeBuddy Code's native project files.
package codebuddy

import (
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const target = "codebuddy"

var caps = emit.Capabilities{Target: target, Supports: []spec.Kind{spec.KindRule, spec.KindAgent, spec.KindSkill, spec.KindCommand, spec.KindHook, spec.KindSettings, spec.KindMCP}, AgentFields: []string{"effort", "mcpServers", "maxTurns", "background"}, SettingsFields: []string{"effort"}, SkillFields: emit.SkillFieldCoverage{Markdown: (Adapter{}).SkillMarkdown}}

type Adapter struct{}

func New() *Adapter                       { return &Adapter{} }
func (Adapter) Name() string              { return target }
func (Adapter) Capabilities() []spec.Kind { return caps.Supports }
func (Adapter) SettingsEffortLevels() []string {
	return []string{"minimal", "low", "medium", "high", "xhigh", "max"}
}
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	if err := (Adapter{}).ValidateSkillTools(b.Skills); err != nil {
		return err
	}
	if err := (Adapter{}).ReportAgentWidening(b.Agents, cfg.OnUnsupported); err != nil {
		return err
	}
	rules, err := emitNativeInstructions(sess, b.Rules, cfg, dryRun)
	if err != nil {
		return err
	}
	if err := sess.RulesDirectory(spec.Bundle{Rules: rules}, emit.RulesDirOpts{Dir: emit.OutputRulesDir(cfg, target, emit.OutputSubDir(cfg, target, "rules", ".codebuddy/rules")), FormatRule: ruleMarkdown, SkipAgents: true, SkipSkills: true}, dryRun); err != nil {
		return err
	}
	if err := (Adapter{}).EmitAgents(sess, b.Agents, emit.OutputAgentsDir(cfg, target, emit.OutputSubDir(cfg, target, "agents", ".codebuddy/agents")), dryRun); err != nil {
		return err
	}
	skillsDir := emit.OutputSkillsDir(cfg, target, emit.OutputSubDir(cfg, target, "skills", ".codebuddy/skills"))
	for _, s := range b.Skills {
		folder := filepath.Join(skillsDir, s.Name)
		if err := sess.WriteFile(filepath.Join(folder, "SKILL.md"), emit.WithHeader((Adapter{}).SkillMarkdown(s), emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
		if err := sess.PropagateSkillAssets(s, folder, emit.SkipSKILLMd, dryRun); err != nil {
			return err
		}
	}
	commandsDir := emit.OutputCommandsDir(cfg, target, emit.OutputSubDir(cfg, target, "commands", ".codebuddy/commands"))
	for _, c := range b.Commands {
		if err := sess.WriteFile(filepath.Join(commandsDir, c.Name+".md"), emit.WithHeader(commandMarkdown(c), emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
	}
	if err := writeSettings(sess, b.HooksFor(target), b.Settings, cfg, dryRun); err != nil {
		return err
	}
	path := emit.OutputRootMCPFile(cfg, target)
	if path == "" {
		if len(b.MCPs) > 0 {
			return emit.ReportWidening(target, "MCP servers", "set outputs.codebuddy.root-mcp-file: .mcp.json to emit project servers", cfg.OnUnsupported)
		}
		return nil
	}
	servers, err := mcpServers(b.MCPs)
	if err != nil {
		return err
	}
	if len(servers) == 0 {
		return nil
	}
	return sess.MergeJSONFile(path, map[string]any{"mcpServers": emit.MergeJSONEntries(servers)}, dryRun)
}
