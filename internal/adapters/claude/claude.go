// Package claude emits Claude Code configs.
//
// Claude Code natively supports all five spec kinds:
//   - agents -> <dir>/agents/<name>.md
//   - skills -> <dir>/skills/<name>/SKILL.md
//   - rules  -> CLAUDE.md (concatenated)
//   - hooks  -> <dir>/settings.json
//   - mcps   -> .mcp.json
package claude

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target           = "claude"
	defaultDir       = ".claude"
	defaultRulesFile = "CLAUDE.md"
	defaultMCPFile   = ".mcp.json"
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
	rulesFile := emit.OutputRulesFile(cfg, target, defaultRulesFile)

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

	if len(b.Rules) > 0 {
		var sb strings.Builder
		for _, r := range b.Rules {
			sb.WriteString("## " + r.Name + "\n\n" + r.Body + "\n\n")
		}
		if err := emit.WriteFile(rulesFile, sb.String(), dryRun); err != nil {
			return err
		}
	}

	if len(b.Hooks) > 0 {
		settings := buildHookSettings(b.Hooks)
		raw, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return fmt.Errorf("hooks settings: %w", err)
		}
		if err := emit.WriteFile(filepath.Join(dir, "settings.json"), string(raw)+"\n", dryRun); err != nil {
			return err
		}
	}

	return emit.WriteMCPFile(b.MCPs, emit.MCPSchemaServersMap, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
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
