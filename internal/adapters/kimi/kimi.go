// Package kimi emits native Kimi Code CLI 2.3.1 configuration.
package kimi

import (
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const target = "kimi"

var caps = emit.Capabilities{Target: target, Supports: []spec.Kind{spec.KindRule, spec.KindAgent, spec.KindSkill, spec.KindMCP, spec.KindHook, spec.KindSettings}, SettingsFields: []string{"effort"}, SkillFields: emit.SkillFieldCoverage{Markdown: (Adapter{}).SkillMarkdown, Handled: []string{"disable-model-invocation"}}}

type Adapter struct{}

func New() *Adapter                       { return &Adapter{} }
func (Adapter) Name() string              { return target }
func (Adapter) Capabilities() []spec.Kind { return caps.Supports }
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	if err := (Adapter{}).EmitAgents(sess, b.Agents, emit.OutputAgentsDir(cfg, target, ".kimi-code/agents"), dryRun); err != nil {
		return err
	}
	for _, s := range b.Skills {
		if s.Scope != "" {
			return fmt.Errorf("kimi: skill %s: skills must be at the project root", s.Name)
		}
		if !nativeName.MatchString(s.Name) {
			return fmt.Errorf("kimi: skill %s requires a lowercase kebab-case name", s.Name)
		}
		folder := filepath.Join(emit.OutputSkillsDir(cfg, target, ".kimi-code/skills"), s.Name)
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
	servers, err := (Adapter{}).CheckedUserMCPServers(b.MCPs)
	if err != nil {
		return err
	}
	if len(servers) > 0 {
		if err := sess.MergeJSONFileNested(emit.OutputMCPFile(cfg, target, ".kimi-code/mcp.json"), map[string]any{"mcpServers": servers}, []string{"mcpServers"}, dryRun); err != nil {
			return err
		}
	}
	if root := emit.OutputRootMCPFile(cfg, target); root != "" && len(servers) > 0 {
		if err := sess.MergeJSONFileNested(root, map[string]any{"mcpServers": servers}, []string{"mcpServers"}, dryRun); err != nil {
			return err
		}
	}
	return emitConfig(sess, b, cfg, dryRun)
}

var nativeName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func native(meta map[string]any) map[string]any {
	m, _ := meta["x-kimi"].(map[string]any)
	out := maps.Clone(m)
	if out == nil {
		out = map[string]any{}
	}
	return out
}
func list(v any) []string {
	switch v := v.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		return spec.SplitToolList(v)
	}
	return nil
}
func (Adapter) SkillMarkdown(s spec.Entry) string {
	m := emit.ResolveMeta(s.Meta, target)
	d, _ := m["description"].(string)
	if d == "" {
		d = s.Name
	}
	out := native(s.Meta)
	out["name"] = s.Name
	out["description"] = d
	if v, ok := m["disable-model-invocation"]; ok {
		out["disableModelInvocation"] = v
	}
	for _, key := range []string{"type", "whenToUse", "arguments", "disableModelInvocation"} {
		if v, ok := m[key]; ok {
			out[key] = v
		}
	}
	return emit.Frontmatter(out) + "\n" + strings.TrimSpace(s.Body) + "\n"
}
func (Adapter) NativeArtifacts(cfg *config.Config) []emit.NativeArtifact {
	out := []emit.NativeArtifact{{Label: "Agents", Location: emit.OutputAgentsDir(cfg, target, ".kimi-code/agents") + "/"}, {Label: "Skills", Location: emit.OutputSkillsDir(cfg, target, ".kimi-code/skills") + "/"}, {Label: "MCP servers", Location: emit.OutputMCPFile(cfg, target, ".kimi-code/mcp.json")}}
	if conf := emit.OutputConfFile(cfg, target, ""); conf != "" {
		out = append(out, emit.NativeArtifact{Label: "Settings and hooks", Location: conf, Note: "explicit KIMI_CODE_HOME profile"})
	}
	return out
}
