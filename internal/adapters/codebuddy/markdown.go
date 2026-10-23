package codebuddy

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

var toolTable = emit.ToolCapabilityTable{
	"read":  {Agent: []string{"Read"}, Permission: []string{"Read"}},
	"write": {Agent: []string{"Write"}, Permission: []string{"Write"}},
	"edit":  {Agent: []string{"Edit"}, Permission: []string{"Edit"}},
	"shell": {Agent: []string{"Bash"}, Permission: []string{"Bash"}},
	"web":   {Agent: []string{"WebFetch", "WebSearch"}, Permission: []string{"WebFetch", "WebSearch"}},
}

func (Adapter) ToolCapabilities() emit.ToolCapabilityTable { return toolTable }
func (Adapter) TranslateAgentCapability(rule string) ([]string, bool) {
	if server, tool, ok := mcpRuleParts(rule); ok && server != "*" && tool != "" && tool != "*" || nativeTool(rule) {
		return []string{rule}, true
	}
	return nil, false
}
func (Adapter) TranslatePermission(list string, rule string) ([]string, bool) {
	if server, _, ok := mcpRuleParts(rule); ok {
		if server != "*" || list == "deny" || list == "ask" {
			return []string{rule}, true
		}
		return nil, false
	}
	name, _, _ := strings.Cut(rule, "(")
	if nativeTool(name) {
		return []string{rule}, true
	}
	return nil, false
}

func nativeTool(name string) bool {
	return slices.Contains([]string{"Read", "Write", "Edit", "Bash", "Grep", "Glob", "WebFetch", "WebSearch"}, name)
}

func mcpRuleParts(rule string) (string, string, bool) {
	name, ok := strings.CutPrefix(rule, "mcp__")
	if !ok {
		return "", "", false
	}
	parts := strings.Split(name, "__")
	if len(parts) > 2 {
		return "", "", false
	}
	for _, part := range parts {
		if part == "" || part != "*" && strings.ContainsAny(part, "*?[]() \t\r\n") {
			return "", "", false
		}
	}
	if len(parts) == 1 {
		return parts[0], "", true
	}
	if parts[0] == "*" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
func validateAgentTools(agents []spec.Entry) error {
	for _, a := range agents {
		m := emit.ResolveMeta(a.Meta, target)
		if custom, ok := a.Meta["x-codebuddy"].(map[string]any); ok {
			if _, ok := custom["tools"]; ok {
				continue
			}
		}
		if _, set := m["tools"]; set && len(list(m["tools"])) == 0 {
			return fmt.Errorf("%s: codebuddy: an empty tools list inherits all tools; remove the restriction or use explicit x-codebuddy.tools", a.Path)
		}
		for _, tool := range list(m["tools"]) {
			if _, ok := (Adapter{}).TranslateAgentCapability(tool); !ok {
				return fmt.Errorf("%s: codebuddy: tool %q has no verified native mapping; set x-codebuddy.tools explicitly", a.Path, tool)
			}
		}
	}
	return nil
}

func (Adapter) ReportAgentWidening(agents []spec.Entry, mode string) error {
	if err := validateAgentTools(agents); err != nil {
		return err
	}
	for _, a := range agents {
		m := emit.ResolveMeta(a.Meta, target)
		memory, _ := m["memory"].(string)
		if !slices.Contains([]string{"user", "project", "local"}, memory) {
			continue
		}
		tools := list(m["tools"])
		if len(tools) == 0 {
			continue
		}
		var missing []string
		for _, tool := range []string{"Read", "Write", "Edit"} {
			if !slices.Contains(tools, tool) {
				missing = append(missing, tool)
			}
		}
		if len(missing) > 0 {
			if err := emit.ReportWidening(target, a.Path, "memory automatically adds "+strings.Join(missing, ", ")+" to the agent's tools", mode); err != nil {
				return err
			}
		}
	}
	return nil
}
func list(v any) []string {
	if s, ok := v.(string); ok {
		return spec.SplitToolList(s)
	}
	if values, ok := v.([]string); ok {
		return values
	}
	return emit.StringSlice(v)
}
func markdown(meta map[string]any, body string) string {
	return emit.Frontmatter(meta) + "\n" + strings.TrimSpace(body) + "\n"
}
func selected(meta map[string]any, keys ...string) map[string]any {
	out := map[string]any{}
	for _, key := range keys {
		if v, ok := meta[key]; ok {
			out[key] = v
		}
	}
	return out
}
func (Adapter) EmitAgents(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	if err := validateAgentTools(agents); err != nil {
		return err
	}
	for _, a := range agents {
		if a.Name == "" || strings.Trim(a.Name, "abcdefghijklmnopqrstuvwxyz-") != "" {
			return fmt.Errorf("%s: codebuddy: agent name %q must use lowercase letters and hyphens", a.Path, a.Name)
		}
		a = emit.WithValidAgentLimits(a, target, emit.AgentMaxTurns, emit.AgentBackground)
		m := emit.ResolveMeta(a.Meta, target)
		meta := selected(m, "description", "model", "tools", "disallowedTools", "skills", "mcpServers", "effort", "permissionMode", "maxTurns", "background", "initialPrompt", "memory", "hooks")
		if mode, ok := meta["permissionMode"].(string); ok && !slices.Contains([]string{"default", "acceptEdits", "bypassPermissions", "plan", "ignore"}, mode) {
			return fmt.Errorf("%s: codebuddy: unsupported agent permissionMode %q", a.Path, mode)
		}
		meta["name"] = a.Name
		if meta["description"] == nil {
			meta["description"] = a.Name
		}
		for _, k := range []string{"tools", "skills"} {
			if v, ok := meta[k]; ok {
				meta[k] = strings.Join(list(v), ", ")
			}
		}
		if v, ok := meta["effort"]; ok && !slices.Contains((Adapter{}).SettingsEffortLevels(), fmt.Sprint(v)) {
			return fmt.Errorf("%s: codebuddy: unsupported agent effort %v", a.Path, v)
		}
		if err := sess.WriteFile(filepath.Join(dir, a.Name+".md"), emit.WithHeader(markdown(meta, a.Body), emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
	}
	return nil
}
func (Adapter) SkillMarkdown(s spec.Entry) string {
	m := emit.ResolveMeta(s.Meta, target)
	meta := selected(m, "description", "allowed-tools", "disable-model-invocation", "user-invocable", "context", "agent", "model", "hooks")
	meta["name"] = s.Name
	if meta["description"] == nil {
		meta["description"] = s.Name
	}
	if v, ok := meta["allowed-tools"]; ok {
		meta["allowed-tools"] = strings.Join(list(v), ", ")
	}
	return markdown(meta, s.Body)
}
func commandMarkdown(c spec.Entry) string {
	m := emit.ResolveMeta(c.Meta, target)
	meta := selected(m, "description", "argument-hint", "model", "allowed-tools", "disable-model-invocation")
	if v, ok := meta["allowed-tools"]; ok {
		meta["allowed-tools"] = strings.Join(list(v), ", ")
	}
	return markdown(meta, c.Body)
}
func ruleMeta(r spec.Entry) map[string]any {
	m := emit.ResolveMeta(r.Meta, target)
	meta := selected(m, "description", "enabled", "alwaysApply", "paths")
	if _, ok := meta["paths"]; !ok {
		if v, exists := m["globs"]; exists {
			meta["paths"] = v
		}
	}
	if _, ok := r.Meta["globs"]; ok {
		meta["alwaysApply"] = false
	} else if _, ok := r.Meta["paths"]; ok {
		meta["alwaysApply"] = false
	}
	return meta
}
func ruleMarkdown(r spec.Entry) string {
	return emit.WithHeader(markdown(ruleMeta(r), r.Body), emit.FormatMarkdown)
}
func (Adapter) AlwaysOnRule(r spec.Entry) bool {
	if custom, _ := r.Meta["x-codebuddy"].(map[string]any); custom["instruction-file"] != nil {
		return false
	}
	m := ruleMeta(r)
	if m["enabled"] == false || m["alwaysApply"] == false {
		return false
	}
	return m["paths"] == nil
}

func (Adapter) ValidateSkillTools(skills []spec.Entry) error {
	for _, s := range skills {
		custom, _ := s.Meta["x-codebuddy"].(map[string]any)
		if _, set := custom["allowed-tools"]; set {
			continue
		}
		if value, set := s.Meta["allowed-tools"]; set {
			tools := list(value)
			if len(tools) == 0 {
				return fmt.Errorf("%s: codebuddy: empty allowed-tools has no verified restriction", s.Path)
			}
			for _, tool := range tools {
				if _, ok := (Adapter{}).TranslateAgentCapability(tool); !ok {
					return fmt.Errorf("%s: codebuddy: allowed tool %q has no verified native mapping; set x-codebuddy.allowed-tools explicitly", s.Path, tool)
				}
			}
		}
	}
	return nil
}
