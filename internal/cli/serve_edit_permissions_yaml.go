package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/preview"
)

func permissionListAt(data []byte, key string) ([]string, bool, error) {
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, false, fmt.Errorf("parse permission source: %w", err)
	}
	perms, _ := doc["permissions"].(map[string]any)
	value, present := perms[strings.TrimPrefix(key, "permissions.")]
	if value == nil {
		return nil, present, nil
	}
	list, ok := value.([]any)
	if !ok {
		return nil, false, fmt.Errorf("%s is not a list of rules", key)
	}
	rules := make([]string, 0, len(list))
	for _, item := range list {
		rule, ok := item.(string)
		if !ok {
			return nil, false, fmt.Errorf("%s contains a value that is not a rule", key)
		}
		if preview.CredentialInRule(rule) {
			return nil, false, fmt.Errorf("%s contains a credential; replace it with an environment reference in the source file", key)
		}
		rules = append(rules, rule)
	}
	return rules, true, nil
}

func setPermissionList(data []byte, key string, rules []string) ([]byte, error) {
	if rules == nil {
		return removeYAMLPath(data, key)
	}
	return setPermissionValue(data, key, rules)
}

// setPermissionValue writes one member of the permissions mapping: a rule
// list, or the default mode name.
func setPermissionValue(data []byte, key string, value any) ([]byte, error) {
	root, err := yamlDocument(data)
	if err != nil {
		return nil, err
	}
	if root.Anchor != "" || root.Style&yaml.FlowStyle != 0 {
		return nil, fmt.Errorf("%s: flow-style or anchored settings are not safely editable", key)
	}
	member := strings.TrimPrefix(key, "permissions.")
	mode, scalar := value.(string)
	_, mapping := yamlMapValue(root, "permissions")
	var out []byte
	if mapping == nil {
		out, err = editYAMLPair(data, root, "permissions", map[string]any{member: value})
	} else {
		if mapping.Kind != yaml.MappingNode || mapping.Anchor != "" {
			return nil, fmt.Errorf("%s is not in a plain permissions mapping", key)
		}
		at, node := yamlMapValue(mapping, member)
		switch {
		case node == nil:
			out, err = editYAMLPair(data, mapping, member, value)
		case scalar:
			if node.Anchor != "" || node.Kind != yaml.ScalarNode || node.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
				return nil, fmt.Errorf("%s is not a plain one-line value", key)
			}
			start, end, e := yamlScalarSpan(data, node)
			if e != nil {
				return nil, fmt.Errorf("%s is not safely editable: %w", key, e)
			}
			encoded, e := yaml.Marshal(mode)
			if e != nil {
				return nil, e
			}
			out = spliceEditYAML(data, start, end, strings.TrimSpace(string(encoded)))
		default:
			if node.Anchor != "" || node.Kind != yaml.SequenceNode && (node.Kind != yaml.ScalarNode || node.Tag != "!!null") {
				return nil, fmt.Errorf("%s is not a plain permission list", key)
			}
			start, e := yamlNodeOffset(data, node)
			if e != nil {
				return nil, e
			}
			end, e := yamlCollectionEnd(data, node)
			if e != nil {
				return nil, e
			}
			encoded, e := permissionListText(value, node, lineEnding(data))
			if e != nil {
				return nil, e
			}
			if node.Kind == yaml.SequenceNode && node.Style&yaml.FlowStyle == 0 && len(node.Content) > 0 && len(value.([]string)) == 0 {
				keyEnd, e := keyColonEnd(data, mapping.Content[at])
				if e != nil {
					return nil, fmt.Errorf("%s is not safely editable: %w", key, e)
				}
				start, encoded = keyEnd, " "+encoded
			}
			out = spliceEditYAML(data, start, end, encoded)
		}
	}
	if err != nil {
		return nil, err
	}
	var before, after map[string]any
	if err := yaml.Unmarshal(data, &before); err != nil {
		return nil, fmt.Errorf("parse permission source: %w", err)
	}
	if before == nil {
		before = map[string]any{}
	}
	if err := yaml.Unmarshal(out, &after); err != nil {
		return nil, fmt.Errorf("parse proposed permission source: %w", err)
	}
	perms, ok := before["permissions"].(map[string]any)
	if !ok {
		perms = map[string]any{}
		before["permissions"] = perms
	}
	if scalar {
		perms[member] = mode
	} else {
		rules, _ := value.([]string)
		values := make([]any, len(rules))
		for i, rule := range rules {
			values[i] = rule
		}
		perms[member] = values
	}
	if !reflect.DeepEqual(before, after) {
		return nil, fmt.Errorf("%s: the edit changes other YAML values", key)
	}
	return out, nil
}

// permissionListText writes rules in the style of the list it replaces: a
// block list keeps one `- rule` line per rule at the list's indent, and
// anything else, or an empty list, is a one-line flow list.
func permissionListText(value any, node *yaml.Node, newline string) (string, error) {
	rules, _ := value.([]string)
	if node.Kind != yaml.SequenceNode || node.Style&yaml.FlowStyle != 0 || len(node.Content) == 0 || len(rules) == 0 {
		encoded, err := json.Marshal(value)
		return string(encoded), err
	}
	lines := make([]string, len(rules))
	for i, rule := range rules {
		encoded, err := yaml.Marshal(rule)
		if err != nil {
			return "", err
		}
		lines[i] = "- " + strings.TrimSpace(string(encoded))
	}
	return strings.Join(lines, newline+strings.Repeat(" ", node.Column-1)), nil
}

// keyColonEnd is the offset just past a key's colon, where an emptied
// block list's `[]` parses whatever the list's indent was.
func keyColonEnd(data []byte, keyNode *yaml.Node) (int, error) {
	_, end, err := yamlScalarSpan(data, keyNode)
	if err != nil {
		return 0, err
	}
	for end < len(data) && (data[end] == ' ' || data[end] == '\t') {
		end++
	}
	if end >= len(data) || data[end] != ':' {
		return 0, errors.New("no colon after the key")
	}
	return end + 1, nil
}

// lineEnding is CRLF when data uses it, else LF.
func lineEnding(data []byte) string {
	if strings.Contains(string(data), "\r\n") {
		return "\r\n"
	}
	return "\n"
}
