package opencode

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
)

type projectPermission struct {
	legacy, native json.RawMessage
}

// readProjectPermission reads the permissions opencode.json holds after
// this sync: the content the session just planned for it, or the file
// on disk when the session left it alone. A dry run plans from an empty
// file, so there the planned keys go over the copy on disk, as the real
// merge would put them.
func readProjectPermission(sess *emit.Session, path string, dryRun bool) (*agentProject, error) {
	data, err := os.ReadFile(path)
	if err != nil && !emit.IsAbsent(err) {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	onDisk, err := permissionOf(string(data), path)
	if err != nil {
		return nil, err
	}
	permission := onDisk
	if content, planned := sess.PlannedFile(path); planned {
		if permission, err = permissionOf(content, path); err != nil {
			return nil, err
		}
		if dryRun {
			permission.legacy = overlayPermission(onDisk.legacy, permission.legacy)
			if permission.native == nil {
				permission.native = onDisk.native
			}
		}
	}
	project := &agentProject{rules: slices.Clone(builtinPermission), native: permission.native != nil, path: path}
	if err := project.appendLegacy(permission.legacy); err != nil {
		return nil, err
	}
	if project.native {
		for i := range project.rules {
			project.rules[i].key = project.actionKey(project.rules[i].key)
		}
		if err := project.appendNative(permission.native); err != nil {
			return nil, err
		}
	}
	return project, nil
}

func (p *agentProject) appendLegacy(permission json.RawMessage) error {
	if permission == nil {
		return nil
	}
	var action string
	if json.Unmarshal(permission, &action) == nil {
		p.rules = append(p.rules, permissionRule{catchAllPattern, catchAllPattern, action})
		return nil
	}
	keys := emit.NewOrderedJSON()
	if err := json.Unmarshal(permission, keys); err != nil {
		return fmt.Errorf("parse %s permission: %w", p.path, err)
	}
	for _, key := range keys.Keys() {
		value, _ := keys.Get(key)
		if json.Unmarshal(value, &action) == nil {
			p.rules = append(p.rules, permissionRule{key, catchAllPattern, action})
			continue
		}
		patterns := emit.NewOrderedJSON()
		if json.Unmarshal(value, patterns) != nil {
			continue
		}
		for _, pattern := range patterns.Keys() {
			if a := ruleAction(patterns, pattern); a != "" {
				p.rules = append(p.rules, permissionRule{key, pattern, a})
			}
		}
	}
	return nil
}

func (p *agentProject) appendNative(raw json.RawMessage) error {
	var rules []json.RawMessage
	if err := json.Unmarshal(raw, &rules); err != nil {
		return fmt.Errorf("parse %s permissions: %w", p.path, err)
	}
	if rules == nil {
		return fmt.Errorf("parse %s permissions: expected an array of action/resource/effect rules", p.path)
	}
	for i, rawRule := range rules {
		var rule map[string]json.RawMessage
		if err := json.Unmarshal(rawRule, &rule); err != nil {
			return fmt.Errorf("parse %s permissions[%d]: %w", p.path, i, err)
		}
		var action, resource, effect *string
		if json.Unmarshal(rule["action"], &action) != nil || json.Unmarshal(rule["resource"], &resource) != nil || json.Unmarshal(rule["effect"], &effect) != nil || action == nil || resource == nil || effect == nil || permissionSeverity[*effect] == 0 {
			return fmt.Errorf("parse %s permissions[%d]: action and resource must be strings; effect must be allow, ask, or deny", p.path, i)
		}
		p.rules = append(p.rules, permissionRule{*action, *resource, *effect})
	}
	return nil
}

// V2 migrates exact legacy names before matching native action wildcards.
func (p *agentProject) actionKey(key string) string {
	if p.native {
		switch key {
		case "bash":
			return "shell"
		case "task":
			return "subagent"
		case "write", "patch", "apply_patch":
			return "edit"
		}
	}
	return key
}

func (p *agentProject) validateAgentPermission(effective any, tools []string, authored any) error {
	permissions, ok := effective.(map[string]any)
	if !p.native || !ok {
		return nil
	}
	grants := map[string][]string{externalDirectoryKey: {catchAllPattern}}
	for _, tool := range tools {
		key, pattern, known := translateRule(tool)
		if known && (key == "read" || key == "edit") {
			grants[key] = append(grants[key], pattern)
		}
	}
	overrides, _ := authored.(map[string]any)
	for _, key := range []string{"read", "edit", externalDirectoryKey} {
		if _, overridden := overrides[key]; overridden {
			continue
		}
		if _, scoped := permissions[key].(*yaml.Node); !scoped {
			continue
		}
		patterns := grants[key]
		whole := slices.Contains(patterns, catchAllPattern)
		homeGrant := slices.ContainsFunc(patterns, homeResource)
		for _, rule := range p.rules {
			if !wildcardOverlap(rule.key, key, true) {
				continue
			}
			homeRule := homeResource(rule.pattern)
			if homeRule && rule.key != key {
				return fmt.Errorf("%s permissions: home-relative resource %q changes meaning when action %q becomes %s; use absolute resources or an explicit agent permission override", p.path, rule.pattern, rule.key, key)
			}
			if !whole && (homeRule || (homeGrant && rule.pattern != catchAllPattern)) {
				return fmt.Errorf("%s permissions: home-relative resources cannot be replayed for scoped %s without OpenCode's runtime home; use absolute resources or an explicit agent permission override", p.path, key)
			}
		}
	}
	for _, key := range []string{"question", "websearch", "doom_loop"} {
		if _, scoped := permissions[key].(*yaml.Node); scoped {
			return fmt.Errorf("%s permissions: resource rules for %s cannot be preserved in legacy agent frontmatter", p.path, key)
		}
	}
	return nil
}

// V2 expands these prefixes only for exact read, edit, and external_directory actions.
func homeResource(resource string) bool {
	return resource == "~" || resource == "$HOME" || strings.HasPrefix(resource, "~/") || strings.HasPrefix(resource, "$HOME/") || strings.HasPrefix(resource, "$HOME\\")
}

func permissionOf(content, path string) (projectPermission, error) {
	if strings.TrimSpace(content) == "" {
		return projectPermission{}, nil
	}
	stripped, _ := emit.StripJSONC([]byte(content))
	doc := emit.NewOrderedJSON()
	if err := json.Unmarshal(stripped, doc); err != nil {
		return projectPermission{}, fmt.Errorf("parse %s: %w", path, err)
	}
	legacy, _ := doc.Get(permissionKey)
	native, _ := doc.Get("permissions")
	return projectPermission{legacy: legacy, native: native}, nil
}

// overlayPermission sets each planned tool key over the disk copy,
// keeping the disk order and adding new keys after it. A bare action on
// either side replaces the whole value.
func overlayPermission(onDisk, planned json.RawMessage) json.RawMessage {
	base, over := emit.NewOrderedJSON(), emit.NewOrderedJSON()
	if onDisk == nil || json.Unmarshal(onDisk, base) != nil || planned == nil || json.Unmarshal(planned, over) != nil {
		if planned == nil {
			return onDisk
		}
		return planned
	}
	for _, key := range over.Keys() {
		value, _ := over.Get(key)
		base.SetRaw(key, value)
	}
	merged, err := json.Marshal(base)
	if err != nil {
		return planned
	}
	return merged
}
