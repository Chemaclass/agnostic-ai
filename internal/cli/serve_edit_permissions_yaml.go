package cli

import (
	"encoding/json"
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
	_, root, err := yamlDocument(data)
	if err != nil {
		return nil, err
	}
	if root.Anchor != "" || root.Style&yaml.FlowStyle != 0 {
		return nil, fmt.Errorf("%s: flow-style or anchored settings are not safely editable", key)
	}
	list := strings.TrimPrefix(key, "permissions.")
	_, mapping := yamlMapValue(root, "permissions")
	var out []byte
	if mapping == nil {
		out, err = editYAMLPair(data, root, "permissions", map[string]any{list: rules})
	} else {
		if mapping.Kind != yaml.MappingNode || mapping.Anchor != "" {
			return nil, fmt.Errorf("%s is not in a plain permissions mapping", key)
		}
		_, value := yamlMapValue(mapping, list)
		if value == nil {
			out, err = editYAMLPair(data, mapping, list, rules)
		} else {
			if value.Anchor != "" || value.Kind != yaml.SequenceNode && (value.Kind != yaml.ScalarNode || value.Tag != "!!null") {
				return nil, fmt.Errorf("%s is not a plain permission list", key)
			}
			start, e := yamlNodeOffset(data, value)
			if e != nil {
				return nil, e
			}
			end, e := yamlCollectionEnd(data, value)
			if e != nil {
				return nil, e
			}
			encoded, e := json.Marshal(rules)
			if e != nil {
				return nil, e
			}
			out = spliceEditYAML(data, start, end, string(encoded))
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
	values := make([]any, len(rules))
	for i, rule := range rules {
		values[i] = rule
	}
	perms[list] = values
	if !reflect.DeepEqual(before, after) {
		return nil, fmt.Errorf("%s: the edit changes other YAML values", key)
	}
	return out, nil
}
