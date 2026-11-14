package cli

import (
	"gopkg.in/yaml.v3"
)

// perTargetEffort returns the effort value a spec gives target: the
// scalar itself, or the map's entry for target, else its `default`.
func perTargetEffort(effort *yaml.Node, target string) *yaml.Node {
	if effort == nil || effort.Kind != yaml.MappingNode {
		return effort
	}
	if value := mappingValue(effort, target); value != nil {
		return value
	}
	return mappingValue(effort, "default")
}

// reconcileEffortMap keeps the other targets' values when the spec's
// `effort` is a per-target map. data is the frontmatter an importer
// produced for target; existing is the spec file it replaces. It only
// acts on a map, so a scalar `effort` stays one.
//
// An imported value the map already gives target leaves the map alone.
// A different one sets only target's entry. A native file that no longer
// shows effort removes only target's entry, and the key when none is left.
// A target whose native format cannot show effort never reaches here with
// a changed value, so its entry is kept.
func reconcileEffortMap(existing, data []byte, target string, expressesEffort bool) ([]byte, error) {
	if target == "" || !expressesEffort {
		return data, nil
	}
	existingMap, _, ok := frontmatterNode(existing)
	if !ok {
		return data, nil
	}
	effort := mappingValue(existingMap, "effort")
	if effort == nil || effort.Kind != yaml.MappingNode {
		return data, nil
	}
	importedMap, rest, ok := frontmatterNode(data)
	if !ok {
		return data, nil
	}
	imported := mappingValue(importedMap, "effort")
	if imported != nil && imported.Kind != yaml.ScalarNode {
		return data, nil
	}

	merged := effort
	switch {
	case imported == nil:
		if mappingValue(effort, target) != nil {
			merged = withoutMappingKey(effort, target)
		}
	case perTargetEffort(effort, target) == nil || perTargetEffort(effort, target).Value != imported.Value:
		merged = withMappingValue(effort, target, imported)
	}
	setOrDropEffort(importedMap, merged)
	return encodeFrontmatter(importedMap, rest, target)
}

// setOrDropEffort writes effort under the `effort` key of mapping, or
// removes the key when effort has no entries.
func setOrDropEffort(mapping, effort *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == "effort" {
			if len(effort.Content) == 0 {
				mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			} else {
				mapping.Content[i+1] = effort
			}
			return
		}
	}
	if len(effort.Content) > 0 {
		mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "effort"}, effort)
	}
}

func withoutMappingKey(mapping *yaml.Node, key string) *yaml.Node {
	out := &yaml.Node{Kind: yaml.MappingNode, Style: mapping.Style}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value != key {
			out.Content = append(out.Content, mapping.Content[i], mapping.Content[i+1])
		}
	}
	return out
}

func withMappingValue(mapping *yaml.Node, key string, value *yaml.Node) *yaml.Node {
	out := &yaml.Node{Kind: yaml.MappingNode, Style: mapping.Style, Content: append([]*yaml.Node(nil), mapping.Content...)}
	for i := 0; i+1 < len(out.Content); i += 2 {
		if out.Content[i].Value == key {
			out.Content[i+1] = value
			return out
		}
	}
	out.Content = append(out.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, value)
	return out
}
