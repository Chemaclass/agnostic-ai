package spec

import (
	"fmt"
	"maps"
	"strings"
)

// PermissionLists are the settings `permissions` lists that hold rules.
var PermissionLists = []string{"allow", "ask", "deny"}

const permissionsKey = "permissions"

// PermissionRules returns the Claude Code rules one permission rule
// stands for, or why it cannot be read. Besides the agent capabilities,
// read(<path>) and edit(<path>) scope a rule to files. A Claude Code
// rule passes through as an alias.
func PermissionRules(list, rule string) ([]string, string) {
	return capabilityToolsOf(rule, capabilityForm{field: "permissions." + list + ":", paths: true})
}

// SettingsPermissionProblems returns why each permission rule in a
// settings spec cannot be read. A list that is not a list of strings is
// a problem too: an unquoted `: ` turns a rule into a mapping, which
// every target drops, so a deny rule would vanish.
func SettingsPermissionProblems(meta map[string]any) []string {
	perms, _ := meta[permissionsKey].(map[string]any)
	var out []string
	for _, list := range PermissionLists {
		value, set := perms[list]
		if !set || value == nil {
			continue
		}
		rules, ok := value.([]any)
		if !ok {
			out = append(out, fmt.Sprintf("permissions.%s: must be a list of rules", list))
			continue
		}
		for _, raw := range rules {
			rule, ok := raw.(string)
			if !ok {
				out = append(out, fmt.Sprintf("permissions.%s: entry %v is not a rule; quote a rule that holds \": \"", list, raw))
				continue
			}
			if _, problem := PermissionRules(list, rule); problem != "" {
				out = append(out, problem)
			}
		}
	}
	return out
}

// NeutralPermission returns the capability a Claude Code rule stands for
// alone, or false when no capability maps to it one to one.
// PermissionRules turns the result back into exactly rule.
func NeutralPermission(rule string) (string, bool) {
	for name, tool := range pathTools {
		if arg, ok := strings.CutPrefix(rule, tool+"("); ok {
			if p, closed := strings.CutSuffix(arg, ")"); closed && strings.TrimSpace(p) != "" {
				return name + "(" + p + ")", true
			}
			return "", false
		}
	}
	return NeutralCapability(rule)
}

// NativePermissions returns the settings spec with each capability in
// its permission lists replaced by the Claude Code rules it stands for.
// A rule that cannot be read stays as written, as any unknown rule did
// before, and sync stops on it first.
func (e Entry) NativePermissions() Entry {
	if e.Kind != KindSettings {
		return e
	}
	perms, ok := e.Meta[permissionsKey].(map[string]any)
	if !ok {
		return e
	}
	var native map[string]any
	for _, list := range PermissionLists {
		rules, ok := perms[list].([]any)
		if !ok {
			continue
		}
		out := make([]any, 0, len(rules))
		changed := false
		for _, raw := range rules {
			rule, ok := raw.(string)
			names, problem := PermissionRules(list, rule)
			if !ok || problem != "" || len(names) == 1 && names[0] == rule {
				out = append(out, raw)
				continue
			}
			changed = true
			for _, n := range names {
				out = append(out, n)
			}
		}
		if !changed {
			continue
		}
		if native == nil {
			native = maps.Clone(perms)
		}
		native[list] = out
	}
	if native == nil {
		return e
	}
	meta := maps.Clone(e.Meta)
	meta[permissionsKey] = native
	e.Meta = meta
	return e
}
