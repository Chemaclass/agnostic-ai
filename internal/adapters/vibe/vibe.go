// Package vibe emits native Mistral Vibe v3.0.2 configuration.
package vibe

import (
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const target = "vibe"

var caps = emit.Capabilities{
	Target:      target,
	Supports:    []spec.Kind{spec.KindRule, spec.KindAgent, spec.KindSkill, spec.KindMCP, spec.KindHook, spec.KindSettings},
	SkillFields: emit.SkillFieldCoverage{Markdown: (Adapter{}).SkillMarkdown, Handled: []string{"disable-model-invocation", "allowed-tools"}},
}

type Adapter struct{}

func New() *Adapter                       { return &Adapter{} }
func (Adapter) Name() string              { return target }
func (Adapter) Capabilities() []spec.Kind { return caps.Supports }
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	doc, servers, err := prepareConfig(b, cfg)
	if err != nil {
		return err
	}
	emit.NoteFieldNoOp(target, spec.KindSettings, "permissions", emit.SpecsWithPermissions(b.Settings), "use native x-vibe.tools permission tables")
	if err := (Adapter{}).EmitAgents(sess, b.Agents, emit.OutputAgentsDir(cfg, target, ".vibe/agents"), dryRun); err != nil {
		return err
	}
	for _, s := range b.Skills {
		if err := validateSkill(s); err != nil {
			return err
		}
		dir, err := emit.ScopedSkillsDir(s.Scope, emit.OutputSkillsDir(cfg, target, ".vibe/skills"))
		if err != nil {
			return err
		}
		folder := filepath.Join(dir, s.Name)
		if err := sess.WriteFile(filepath.Join(folder, "SKILL.md"), emit.WithHeader((Adapter{}).SkillMarkdown(s), emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
		if err := sess.PropagateSkillAssets(s, folder, emit.SkipSKILLMd, dryRun); err != nil {
			return err
		}
	}
	if err := sess.EmitLegacyRulesFile(b, cfg, target, emit.MergedOpts{Title: "AGENTS.md"}, dryRun); err != nil {
		return err
	}
	if err := emitConfig(sess, doc, servers, cfg, dryRun); err != nil {
		return err
	}
	return emitHooks(sess, b.HooksFor(target), emit.OutputHooksFile(cfg, target, ".vibe/hooks.toml"), emit.HookScriptsDir(target), dryRun, runtime.GOOS)
}

var skillName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func validateSkill(s spec.Entry) error {
	meta := emit.ResolveMeta(s.Meta, target)
	d, _ := meta["description"].(string)
	if d == "" {
		d = s.Name
	}
	if !skillName.MatchString(s.Name) || len(s.Name) > 64 || len(d) == 0 || len(d) > 1024 {
		return fmt.Errorf("vibe: skill %s requires a lowercase dash-separated name (max 64) and description (1-1024 characters)", s.Name)
	}
	return nil
}
func (Adapter) SkillMarkdown(s spec.Entry) string {
	meta := emit.ResolveMeta(s.Meta, target)
	description, _ := meta["description"].(string)
	if description == "" {
		description = s.Name
	}
	out := map[string]any{"name": s.Name, "description": description}
	for _, key := range []string{"license", "compatibility", "metadata", "allowed-tools", "user-invocable", "disable-model-invocation"} {
		if value, ok := meta[key]; ok {
			out[key] = value
		}
	}
	return emit.Frontmatter(out) + "\n" + strings.TrimSpace(s.Body) + "\n"
}

func (Adapter) SkillSidecars(s spec.Entry) (map[string]string, error) { return nil, validateSkill(s) }

func emitHooks(sess *emit.Session, hooks []spec.Entry, path, scriptsDir string, dryRun bool, platform string) error {
	for i := range hooks {
		hooks[i].Meta = emit.ResolveMeta(hooks[i].Meta, target)
	}
	records, err := (Adapter{}).UserHookRecordsForPlatform(hooks, scriptsDir, platform)
	if err != nil {
		return err
	}
	if err := sess.MaterializeNeutralHookScripts(hooks, target, scriptsDir, dryRun); err != nil {
		return err
	}
	return writeDocument(sess, path, map[string]any{"hooks": records}, len(records) > 0, dryRun)
}
