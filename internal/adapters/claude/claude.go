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
	"path/filepath"
	"sort"
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
		path := filepath.Join(dir, "settings.json")
		if err := emit.MergeJSONFile(path, settings, dryRun); err != nil {
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

// buildHookSettings renders {"hooks": {<event>: [{"matcher", "hooks": [...]}]}}
// keyed for `.claude/settings.json`.
//
// Specs with the same event and matcher merge into one matcher block:
// their commands stack inside the inner `hooks` array, matching the
// Claude Code schema where each matcher entry can run multiple commands.
// A spec's `command:` field accepts a string or a list of strings; each
// string becomes one `{type: "command", command: ...}` entry.
func buildHookSettings(hooks []spec.Entry) map[string]any {
	type matcherKey struct{ event, matcher string }

	byKey := map[matcherKey][]map[string]any{}
	keyOrder := []matcherKey{}

	for _, h := range hooks {
		event, _ := h.Meta["event"].(string)
		matcher, _ := h.Meta["matcher"].(string)
		if event == "" {
			continue
		}
		cmds := hookCommands(h.Meta["command"])
		if len(cmds) == 0 {
			continue
		}
		k := matcherKey{event: event, matcher: matcher}
		if _, seen := byKey[k]; !seen {
			keyOrder = append(keyOrder, k)
		}
		for _, cmd := range cmds {
			byKey[k] = append(byKey[k], map[string]any{"type": "command", "command": cmd})
		}
	}

	byEvent := map[string][]map[string]any{}
	for _, k := range keyOrder {
		byEvent[k.event] = append(byEvent[k.event], map[string]any{
			"matcher": k.matcher,
			"hooks":   byKey[k],
		})
	}
	// Stable iteration: events are emitted by Go in sorted order during
	// JSON encoding, but matcher groups within an event preserve insert
	// order. Sort matcher groups by matcher string for deterministic
	// output across sync runs.
	for event, groups := range byEvent {
		sort.SliceStable(groups, func(i, j int) bool {
			mi, _ := groups[i]["matcher"].(string)
			mj, _ := groups[j]["matcher"].(string)
			return mi < mj
		})
		byEvent[event] = groups
	}
	return map[string]any{"hooks": byEvent}
}

// hookCommands normalizes a `command:` field that may be a string or a
// list of strings into a single []string. Empty strings drop out.
func hookCommands(raw any) []string {
	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		out := make([]string, 0, len(v))
		for _, s := range v {
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
