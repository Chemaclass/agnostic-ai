// Package grok emits native Grok Build project configuration.
package grok

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const target = "grok"

var caps = emit.Capabilities{Target: target, Supports: []spec.Kind{spec.KindRule, spec.KindAgent, spec.KindSkill, spec.KindCommand, spec.KindMCP, spec.KindHook, spec.KindSettings}, AgentFields: []string{"effort", "mcpServers", "maxTurns", "background"}, SkillFields: emit.SkillFieldCoverage{Markdown: (Adapter{}).SkillMarkdown}}

type Adapter struct{}

func New() *Adapter                       { return &Adapter{} }
func (Adapter) Name() string              { return target }
func (Adapter) Capabilities() []spec.Kind { return slices.Clone(caps.Supports) }
func rulesOpts(cfg *config.Config) emit.RulesDirOpts {
	return emit.RulesDirOpts{Dir: emit.OutputRulesDir(cfg, target, emit.OutputSubDir(cfg, target, "rules", ".grok/rules")), ScopeAtRoot: true, SkipAgents: true, SkipSkills: true, FormatRule: func(e spec.Entry) string { return emit.WithHeader(e.Body, emit.FormatMarkdown) }}
}
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	if err := validateBundle(b); err != nil {
		return err
	}
	opts := rulesOpts(cfg)
	if err := validateRulesVisible(b.Rules, cfg, opts); err != nil {
		return err
	}
	if err := sess.RulesDirectory(b, opts, dryRun); err != nil {
		return err
	}
	if err := (Adapter{}).EmitAgents(sess, b.Agents, emit.OutputAgentsDir(cfg, target, emit.OutputSubDir(cfg, target, "agents", ".grok/agents")), dryRun); err != nil {
		return err
	}
	if err := sess.WriteScopedSkillFoldersWith(b.Skills, emit.OutputSkillsDir(cfg, target, emit.OutputSubDir(cfg, target, "skills", ".grok/skills")), dryRun, (Adapter{}).SkillMarkdown); err != nil {
		return err
	}
	for _, e := range b.Commands {
		dir, err := emit.ScopedSkillsDir(e.Scope, emit.OutputCommandsDir(cfg, target, emit.OutputSubDir(cfg, target, "commands", ".grok/commands")))
		if err != nil {
			return err
		}
		if err := sess.WriteFile(filepath.Join(dir, e.Name+".md"), emit.WithHeader(commandMarkdown(e), emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
	}
	if err := emitHooks(sess, b.Hooks, cfg, dryRun); err != nil {
		return err
	}
	mcpPath := emit.OutputMCPFile(cfg, target, emit.OutputSubDir(cfg, target, "config.toml", ".grok/config.toml"))
	settingsPath := emit.OutputConfFile(cfg, target, mcpPath)
	if settingsPath == mcpPath {
		return emitConfig(sess, b.MCPs, b.Settings, mcpPath, dryRun)
	}
	if err := emitConfig(sess, b.MCPs, nil, mcpPath, dryRun); err != nil {
		return err
	}
	return emitConfig(sess, nil, b.Settings, settingsPath, dryRun)
}
func validateBundle(b spec.Bundle) error {
	for _, group := range []struct {
		kind    spec.Kind
		entries []spec.Entry
	}{{spec.KindRule, b.Rules}, {spec.KindAgent, b.Agents}, {spec.KindSkill, b.Skills}, {spec.KindCommand, b.Commands}, {spec.KindHook, b.Hooks}, {spec.KindMCP, b.MCPs}, {spec.KindSettings, b.Settings}} {
		for _, e := range group.entries {
			if err := spec.ValidateName(group.kind, e.Name); err != nil {
				return fmt.Errorf("grok %s %q: %w", group.kind, e.Name, err)
			}
			if e.Scope != "" {
				if err := emit.CheckScopePath(e.Scope); err != nil {
					return fmt.Errorf("grok %s %s: %w", group.kind, e.Name, err)
				}
			}
		}
	}
	for _, e := range b.Agents {
		if _, err := agentMeta(e); err != nil {
			return err
		}
	}
	for _, e := range b.Hooks {
		if _, err := HookFile(e); err != nil {
			return err
		}
	}
	for _, e := range b.Skills {
		if err := (Adapter{}).ValidateSkill(e); err != nil {
			return err
		}
	}
	for _, e := range b.Commands {
		if err := validateSkillIdentity(e, nil); err != nil {
			return err
		}
	}

	for _, e := range b.MCPs {
		if _, err := mcpEntry(e); err != nil {
			return err
		}
	}
	return nil
}

func OutputPaths(cfg *config.Config) map[string]string { return emit.VarsFor(cfg, target) }
