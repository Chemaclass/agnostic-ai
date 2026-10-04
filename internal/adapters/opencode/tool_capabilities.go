package opencode

import (
	"fmt"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

var toolCapabilities = emit.ToolCapabilityTable{
	"shell": {Permission: []string{"bash"}},
	"read":  {Permission: []string{"read"}},
	"edit":  {Permission: []string{"edit"}},
	"write": {Permission: []string{"edit"}},
	"Glob":  {Permission: []string{"glob"}},
	"Grep":  {Permission: []string{"grep"}},
	"Task":  {Permission: []string{"task"}},
	"Skill": {Permission: []string{"skill"}},
	"web":   {Permission: []string{"webfetch", "websearch"}},
}

func (Adapter) ToolCapabilities() emit.ToolCapabilityTable { return toolCapabilities }

func (Adapter) TranslatePermission(list, rule string) ([]string, bool) {
	tool, pattern, ok := translateRule(rule)
	if !ok {
		return nil, false
	}
	native := tool
	if pattern != "*" {
		native += "(" + pattern + ")"
	}
	out := []string{native}
	return out, true
}

func reportPermissionWidening(settings []spec.Entry, mode string) error {
	for _, entry := range settings {
		if _, overridden := nativePermission(entry)["edit"]; overridden {
			return nil
		}
	}
	for _, entry := range settings {
		permissions, _ := entry.Meta["permissions"].(map[string]any)
		for _, list := range []string{"allow", "ask", "deny"} {
			rules := emit.StringSlice(permissions[list])
			if slices.Contains(rules, "Edit") {
				continue
			}
			for _, rule := range rules {
				scope, _, scoped := spec.SplitPermissionRule(rule)
				if !scoped {
					scope = rule
				}
				if scope != "Write" {
					continue
				}
				line := fmt.Sprintf("permissions.%s rule %s becomes OpenCode's edit permission, which also covers edit and patch", list, rule)
				if err := emit.ReportWidening(target, entry.Path, line, mode); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
