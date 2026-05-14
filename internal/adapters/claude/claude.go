// Package claude emits Claude Code configs.
//
// Claude Code natively supports all five spec kinds:
//   - agents -> <dir>/agents/<name>.md
//   - skills -> <dir>/skills/<name>/SKILL.md
//   - rules  -> <dir>/rules/<name>.md (one file per rule)
//   - hooks  -> <dir>/settings.json
//   - mcps   -> .mcp.json
//
// Rules emit one file per spec under `.claude/rules/` so a hand-authored
// CLAUDE.md is never clobbered. Reference the per-rule files from CLAUDE.md
// via `@.claude/rules/<name>.md` imports if you want Claude Code to load
// them. Set `outputs.claude.rules-file: CLAUDE.md` to fall back to the
// legacy concatenated single-file layout.
package claude

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target          = "claude"
	defaultDir      = ".claude"
	defaultRulesDir = ".claude/rules"
	defaultMCPFile  = ".mcp.json"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindHook, spec.KindMCP},
}

// Adapter emits Claude Code configs.
type Adapter struct{}

// New returns a Claude adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes the agents, skills, rules, and hook settings.
func (Adapter) Emit(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}

	dir := emit.OutputDir(cfg, target, defaultDir)

	for _, a := range b.Agents {
		path := filepath.Join(dir, "agents", a.Name+".md")
		if err := emit.WriteFile(path, emit.Frontmatter(emit.ResolveMeta(a.Meta, target))+"\n"+a.Body, dryRun); err != nil {
			return err
		}
	}

	for _, s := range b.Skills {
		path := filepath.Join(dir, "skills", s.Name, "SKILL.md")
		if err := emit.WriteFile(path, emit.Frontmatter(emit.ResolveMeta(s.Meta, target))+"\n"+s.Body, dryRun); err != nil {
			return err
		}
	}

	if err := writeRules(b.Rules, cfg, dryRun); err != nil {
		return err
	}

	if len(b.Hooks) > 0 {
		settings := buildHookSettings(b.Hooks)
		raw, err := emit.MarshalJSONIndent(settings)
		if err != nil {
			return fmt.Errorf("hooks settings: %w", err)
		}
		if err := emit.WriteFile(filepath.Join(dir, "settings.json"), string(raw)+"\n", dryRun); err != nil {
			return err
		}
	}

	return emit.WriteMCPFile(b.MCPs, emit.MCPSchemaServersMap, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// writeRules emits rules per-file under `.claude/rules/<name>.md` by
// default. Setting `outputs.claude.rules-file` switches back to the
// legacy single-file concatenated layout (typically CLAUDE.md).
func writeRules(rules []spec.Entry, cfg *config.Config, dryRun bool) error {
	if len(rules) == 0 {
		return nil
	}
	if rulesFile := emit.OutputRulesFile(cfg, target, ""); rulesFile != "" {
		var sb strings.Builder
		for _, r := range rules {
			sb.WriteString("## " + r.Name + "\n\n" + r.Body + "\n\n")
		}
		return emit.WriteFile(rulesFile, sb.String(), dryRun)
	}
	rulesDir := emit.OutputRulesDir(cfg, target, defaultRulesDir)
	for _, r := range rules {
		path := filepath.Join(rulesDir, r.Name+".md")
		if err := emit.WriteFile(path, "# "+r.Name+"\n\n"+r.Body, dryRun); err != nil {
			return err
		}
	}
	return nil
}

func buildHookSettings(hooks []spec.Entry) map[string]any {
	byEvent := map[string][]map[string]any{}
	for _, h := range hooks {
		event, _ := h.Meta["event"].(string)
		matcher, _ := h.Meta["matcher"].(string)
		command, _ := h.Meta["command"].(string)
		if event == "" {
			continue
		}
		byEvent[event] = append(byEvent[event], map[string]any{
			"matcher": matcher,
			"hooks": []map[string]any{
				{"type": "command", "command": command},
			},
		})
	}
	return map[string]any{"hooks": byEvent}
}
