package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// globalImportPermissionsSpec is where import --global puts the
// permission rules it reads from the user settings file. It is the
// import's own file, so a later import adds the rules it lacks.
const globalImportPermissionsSpec = "settings/" + claudePermissionsSpec + ".yaml"

// globalPermissionRules returns, per list, the rules the settings specs
// give target at user level, in the spelling the target reads: each
// spec's portable permissions, then its x-<target>.permissions, in spec
// order, each rule once.
func globalPermissionRules(target string, settings []spec.Entry) map[string][]string {
	out := map[string][]string{}
	for _, entry := range settings {
		if !entry.EmitsTo(target) {
			continue
		}
		entry = entry.NativePermissions()
		layers := []any{entry.Meta["permissions"]}
		if hatch, ok := entry.Meta["x-"+target].(map[string]any); ok {
			layers = append(layers, hatch["permissions"])
		}
		for _, layer := range layers {
			perms, _ := layer.(map[string]any)
			for _, list := range spec.PermissionLists {
				for _, rule := range stringSliceFromAny(perms[list]) {
					if rule != "" {
						out[list] = appendUnique(out[list], rule)
					}
				}
			}
		}
	}
	return out
}

// globalPermissionMerge is the planned change to the rule lists of one
// user settings file.
type globalPermissionMerge struct {
	// data is the new content, nil when the lists need no change.
	data []byte
	// changes lists each rule added or removed, adopted each spec rule
	// the file already held.
	changes, adopted []string
	// owned is, per list, the rules sync owns after the write.
	owned map[string][]string
}

// mergeGlobalPermissionRules appends each wanted rule a list lacks,
// after the entries already there, and removes a rule an earlier sync
// recorded once the specs no longer name it. A rule sync never wrote
// stays where it is, so a rule the tool itself added survives. A rule
// already present is adopted, and sync owns it from then on.
func mergeGlobalPermissionRules(path string, data []byte, want, previous map[string][]string) (globalPermissionMerge, error) {
	m := globalPermissionMerge{owned: map[string][]string{}}
	values, err := settingsValues(path, "json", data)
	if err != nil {
		return m, err
	}
	out := data
	if len(bytes.TrimSpace(out)) == 0 {
		out = []byte("{}\n")
	}
	changed := false
	for _, list := range spec.PermissionLists {
		key := "permissions." + list
		if len(want[list]) == 0 && len(previous[list]) == 0 {
			continue
		}
		var native []any
		if raw, present := settingsLookup(values, key, "json"); present {
			items, ok := raw.([]any)
			if !ok {
				return m, fmt.Errorf("%s: %s is not a list of rules; fix it by hand, then sync", path, key)
			}
			native = items
		}
		holds := func(rule string) bool {
			return slices.ContainsFunc(native, func(item any) bool { return item == rule })
		}
		next := make([]any, 0, len(native)+len(want[list]))
		edited := false
		for _, item := range native {
			if rule, ok := item.(string); ok && slices.Contains(previous[list], rule) && !slices.Contains(want[list], rule) {
				m.changes = append(m.changes, fmt.Sprintf("remove %s from %s", jsonString(rule), key))
				edited = true
				continue
			}
			next = append(next, item)
		}
		for _, rule := range want[list] {
			if holds(rule) {
				if !slices.Contains(previous[list], rule) {
					m.adopted = append(m.adopted, key+" "+jsonString(rule))
				}
				continue
			}
			m.changes = append(m.changes, fmt.Sprintf("add %s to %s", jsonString(rule), key))
			next = append(next, rule)
			edited = true
		}
		if len(want[list]) > 0 {
			m.owned[list] = slices.Clone(want[list])
		}
		if !edited {
			continue
		}
		changed = true
		// A list the removals emptied goes, and its permissions object
		// with it once that is empty too, which undoes creating them.
		if len(next) == 0 {
			out, err = editJSONRoot(path, out, nil, nil, []string{key})
		} else {
			out, err = editJSONArray(path, out, key, next)
		}
		if err != nil {
			return m, err
		}
	}
	if changed {
		m.data = out
	}
	if len(m.owned) == 0 {
		m.owned = nil
	}
	return m, nil
}

// stageGlobalPermissions writes the permission rules of the Claude Code
// user settings file into one settings spec. A rule a home spec already
// gives Claude is left out, and so is one the ownership state records
// as written by sync: either would come back as a second copy. A rule
// holding a credential stays in the user file, with a warning, since
// the home is meant to be committed. Only ~/.claude/settings.json is
// read, never managed or project settings.
func stageGlobalPermissions(home, stage string, targets []string, have []spec.Entry, state globalState, warn io.Writer) error {
	const target = "claude"
	g := globalTargets[target]
	if !slices.Contains(targets, target) || !g.settings.permissionLists {
		return nil
	}
	path := g.path(home, g.settings.path)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	values, err := settingsValues(path, g.settings.format, data)
	if err != nil {
		return err
	}
	provided := globalPermissionRules(target, have)
	recorded := state.PermissionRules[target]
	lists := map[string][]string{}
	for _, list := range spec.PermissionLists {
		raw, _ := settingsLookup(values, "permissions."+list, g.settings.format)
		for _, rule := range stringSliceFromAny(raw) {
			if rule == "" || slices.Contains(provided[list], rule) || slices.Contains(recorded[list], rule) {
				continue
			}
			if mcpCredentialDetected(rule) {
				if _, err := fmt.Fprintf(warn, "warning: %s: left out a permissions.%s rule in %s: it holds a credential; add it under local/settings by hand\n", target, list, path); err != nil {
					return fmt.Errorf("write import warning: %w", err)
				}
				continue
			}
			lists[list] = appendUnique(lists[list], rule)
		}
	}
	_, err = writePermissionsSpec(filepath.Join(stage, "settings"), claudePermissionsSpec, lists)
	return err
}

// appendPermissionsSpec adds to an existing permissions spec each rule
// of the staged one it lacks, after the rules already there, and keeps
// everything else in it. It returns the new content and how many rules
// it added.
func appendPermissionsSpec(path string, existing, staged []byte) ([]byte, int, error) {
	var add struct {
		Permissions map[string][]string `yaml:"permissions"`
	}
	if err := yaml.Unmarshal(staged, &add); err != nil {
		return nil, 0, fmt.Errorf("parse staged %s: %w", path, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(existing, &doc); err != nil {
		return nil, 0, fmt.Errorf("parse %s: %w", path, err)
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	root := doc.Content[0]
	if doc.Kind != yaml.DocumentNode || root.Kind != yaml.MappingNode {
		return nil, 0, fmt.Errorf("%s: expected a mapping; add the rules by hand", path)
	}
	perms, err := yamlMappingChild(path, root, "permissions", yaml.MappingNode)
	if err != nil {
		return nil, 0, err
	}
	added := 0
	for _, list := range spec.PermissionLists {
		if len(add.Permissions[list]) == 0 {
			continue
		}
		rules, err := yamlMappingChild(path, perms, list, yaml.SequenceNode)
		if err != nil {
			return nil, 0, err
		}
		for _, rule := range add.Permissions[list] {
			if slices.ContainsFunc(rules.Content, func(n *yaml.Node) bool { return n.Kind == yaml.ScalarNode && n.Value == rule }) {
				continue
			}
			rules.Content = append(rules.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: rule})
			added++
		}
	}
	if added == 0 {
		return existing, 0, nil
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(yamlIndentOf(existing))
	if err := enc.Encode(&doc); err != nil {
		return nil, 0, fmt.Errorf("marshal %s: %w", path, err)
	}
	if err := enc.Close(); err != nil {
		return nil, 0, fmt.Errorf("marshal %s: %w", path, err)
	}
	return buf.Bytes(), added, nil
}

// yamlMappingChild returns the value under key in mapping, adding an
// empty one of kind when the key is missing or null.
func yamlMappingChild(path string, mapping *yaml.Node, key string, kind yaml.Kind) (*yaml.Node, error) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value != key {
			continue
		}
		value := mapping.Content[i+1]
		if value.Tag == "!!null" {
			*value = yaml.Node{Kind: kind}
		}
		if value.Kind != kind {
			return nil, fmt.Errorf("%s: %s has an unexpected shape; add the rules by hand", path, key)
		}
		// A flow list such as [Read] grows one line per rule instead.
		value.Style = 0
		return value, nil
	}
	value := &yaml.Node{Kind: kind}
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
	return value, nil
}

// yamlIndentOf is the indent width the first indented line of data
// uses, or the width yaml.Marshal writes.
func yamlIndentOf(data []byte) int {
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimLeft(line, " ")
		if n := len(line) - len(trimmed); n > 0 && trimmed != "" && !strings.HasPrefix(trimmed, "- ") {
			return n
		}
	}
	return 4
}

// withGlobalPermissionRules applies the rule list merge on top of the
// key merge m of the same file, starting from m's content, then base,
// then the disk. It returns the combined merge and the rules sync owns.
func withGlobalPermissionRules(path string, m globalSettingsMerge, base []byte, want, previous map[string][]string) (globalSettingsMerge, map[string][]string, error) {
	if len(want) == 0 && len(previous) == 0 {
		return m, nil, nil
	}
	data := m.data
	if data == nil {
		data = base
	}
	if data == nil {
		read, err := os.ReadFile(path)
		switch {
		case os.IsNotExist(err):
			if len(want) == 0 {
				return m, nil, nil
			}
		case err != nil:
			return m, nil, fmt.Errorf("read %s: %w", path, err)
		}
		data = read
	}
	rules, err := mergeGlobalPermissionRules(path, data, want, previous)
	if err != nil {
		return m, nil, err
	}
	if rules.data != nil {
		m.data = rules.data
	}
	m.changes = append(m.changes, rules.changes...)
	m.adopted = append(m.adopted, rules.adopted...)
	return m, rules.owned, nil
}
