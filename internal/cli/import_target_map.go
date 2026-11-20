package cli

import (
	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// perTargetValue returns the value a spec's key gives target: the
// scalar itself, or the map's entry for target, else its `default`.
func perTargetValue(value *yaml.Node, target string) *yaml.Node {
	if value == nil || value.Kind != yaml.MappingNode {
		return value
	}
	if entry := mappingValue(value, target); entry != nil {
		return entry
	}
	return mappingValue(value, "default")
}

// resolvedTargetValue is what sync writes for target from a spec's `key`
// value. A `model` map whose `default` names a tier gives the tier's
// model for target, the way ApplyModelTiers expands it.
func resolvedTargetValue(key string, value *yaml.Node, target string) *yaml.Node {
	resolved := perTargetValue(value, target)
	if key != "model" || resolved == nil || resolved.Kind != yaml.ScalarNode {
		return resolved
	}
	model := resolved.Value
	own := value.Kind == yaml.MappingNode && mappingValue(value, target) != nil
	var tiers map[string]config.ModelTier
	if importHookConfig != nil {
		tiers = importHookConfig.Models
	}
	if tier, named := tiers[model]; named && !own {
		var ok bool
		if model, ok = tier.Models[target]; !ok {
			if model, ok = tier.Models["default"]; !ok {
				return nil
			}
		}
	}
	if id, isAlias := adapters.ModelAlias(target, model); isAlias {
		model = id
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Value: model}
}

// importedTargetValue is the scalar an importer wrote under a key, and
// false when it wrote something else. Claude scopes a model as
// `{claude: <name>}`, which counts as that name.
func importedTargetValue(imported *yaml.Node, target string) (*yaml.Node, bool) {
	if imported != nil && imported.Kind == yaml.MappingNode && len(imported.Content) == 2 && imported.Content[0].Value == target {
		imported = imported.Content[1]
	}
	return imported, imported == nil || imported.Kind == yaml.ScalarNode
}

// importedHasKey reports whether the imported frontmatter sets key, which
// shows the native file holds it even when the target's field set does
// not list it.
func importedHasKey(data []byte, key string) bool {
	imported, _, ok := frontmatterNode(data)
	return ok && mappingValue(imported, key) != nil
}

// reconcileTargetMap keeps the other targets' values when the spec's
// `key` (`effort` or `model`) is a per-target map. data is the
// frontmatter an importer produced for target; existing is the spec file
// it replaces. It only acts on a map, so a scalar stays one.
//
// An imported value the map already gives target leaves the map alone.
// A different one sets only target's entry. A native file that no longer
// shows the key removes only target's entry, and the key when none is
// left. A target whose native format cannot show the key never reaches
// here with a changed value, so its entry is kept. A spec with
// `x-<target>.<key>` already says what target gets, so it is left alone;
// a `null` there is a delete marker, which a value shown again replaces.
// A `model` alias such as Codex's `sol` is compared by the id it resolves
// to, since that is what the native file holds.
func reconcileTargetMap(existing, data []byte, key string, targets []string, expresses bool) ([]byte, error) {
	if len(targets) == 0 || targets[0] == "" || !expresses {
		return data, nil
	}
	existingMap, _, ok := frontmatterNode(existing)
	if !ok {
		return data, nil
	}
	current := mappingValue(existingMap, key)
	if current == nil || current.Kind != yaml.MappingNode {
		return data, nil
	}
	importedMap, rest, ok := frontmatterNode(data)
	if !ok {
		return data, nil
	}
	imported, ok := importedTargetValue(mappingValue(importedMap, key), targets[0])
	if !ok {
		return data, nil
	}

	if allScoped(existingMap, key, targets) {
		return data, nil
	}
	merged := current
	var unmark, mark []string
	for _, target := range targets {
		scoped := mappingValue(mappingValue(existingMap, "x-"+target), key)
		deleted := scoped != nil && scoped.Tag == "!!null"
		if scoped != nil && !deleted {
			continue
		}
		given := resolvedTargetValue(key, merged, target)
		if deleted {
			given = nil
		}
		switch {
		case imported == nil:
			if mappingValue(merged, target) != nil {
				merged = withoutMappingKey(merged, target)
			}
		case given == nil || given.Value != imported.Value:
			merged = withMappingValue(merged, target, imported)
		}
		if deleted && imported != nil {
			unmark = append(unmark, target)
		}
		if imported == nil && resolvedTargetValue(key, merged, target) != nil {
			mark = append(mark, target)
		}
	}
	setOrDropKey(importedMap, key, merged)
	for _, target := range unmark {
		dropScopedKey(importedMap, key, target)
	}
	for _, target := range mark {
		markKeyDeleted(importedMap, key, target)
	}
	return encodeFrontmatter(importedMap, rest, targets[0])
}

// allScoped reports whether every target has a non-null x-<target>.<key>.
func allScoped(existingMap *yaml.Node, key string, targets []string) bool {
	for _, target := range targets {
		scoped := mappingValue(mappingValue(existingMap, "x-"+target), key)
		if scoped == nil || scoped.Tag == "!!null" {
			return false
		}
	}
	return true
}

// markKeyDeleted writes `x-<target>.<key>: null`, the documented way to
// stop one target from receiving a value the map's `default` gives.
func markKeyDeleted(mapping *yaml.Node, key, target string) {
	null := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
	scoped := mappingValue(mapping, "x-"+target)
	if scoped == nil || scoped.Kind != yaml.MappingNode {
		scoped = &yaml.Node{Kind: yaml.MappingNode}
		mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "x-" + target}, scoped)
	}
	for i := 0; i+1 < len(scoped.Content); i += 2 {
		if scoped.Content[i].Value == key {
			scoped.Content[i+1] = null
			return
		}
	}
	scoped.Content = append(scoped.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, null)
}

// dropScopedKey removes `x-<target>.<key>` from mapping.
func dropScopedKey(mapping *yaml.Node, key, target string) {
	scoped := mappingValue(mapping, "x-"+target)
	if scoped == nil || scoped.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(scoped.Content); i += 2 {
		if scoped.Content[i].Value == key {
			scoped.Content = append(scoped.Content[:i], scoped.Content[i+2:]...)
			return
		}
	}
}

// setOrDropKey writes value under key in mapping, or removes the key
// when value has no entries.
func setOrDropKey(mapping *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			if len(value.Content) == 0 {
				mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			} else {
				mapping.Content[i+1] = value
			}
			return
		}
	}
	if len(value.Content) > 0 {
		mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, value)
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
