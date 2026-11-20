package cli

import (
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

// sharedAgentFileTargets names the target that writes the same native
// agent file as an import source. Both must get the same model, or the
// shared file differs between them.
var sharedAgentFileTargets = map[string]string{
	"goose":     "openhands",
	"openhands": "goose",
}

// commandFormatsWithModel are the import sources whose command files can
// hold a `model` key. A command from any other source never changes the
// spec's model.
var commandFormatsWithModel = map[string]bool{"claude": true, "opencode": true, "junie": true}

// importModelTargets lists the targets an import from source sets a
// model entry for: source and the target sharing its native file.
func importModelTargets(source string) []string {
	if peer, ok := sharedAgentFileTargets[source]; ok {
		return []string{source, peer}
	}
	return []string{source}
}

// portableModel reports whether model is `inherit` or a tier name, which
// every tool reads the same way.
func portableModel(model string) bool {
	if model == "inherit" {
		return true
	}
	if importHookConfig == nil {
		return false
	}
	_, tier := importHookConfig.Models[model]
	return tier
}

// scopableModel reports whether a model imported into a new spec is a
// name only source can run, so it gets a per-target entry. Claude scopes
// only Claude model names.
func scopableModel(source, model string) bool {
	if source == "claude" {
		return adapters.ClaudeModel(model)
	}
	return !portableModel(model)
}

// reconcileImportedModel sets the model of an imported agent or command
// spec by one rule: a native model changes only the importing tool's
// entry, and only when it differs from what sync gives that tool. A
// native file without a model changes nothing when its format cannot hold
// one (holdsModel false); otherwise it removes that tool's entry.
//
// existing is the spec on disk, nil when there is none. A new spec gets
// `model: {<tool>: <name>}`. A shared scalar stays one when only the
// importing tool reads it; otherwise it becomes the map's `default`.
// Skills keep their model as written, since most tools write none.
func reconcileImportedModel(existing, data []byte, path string, holdsModel bool) ([]byte, error) {
	source := importScopeTarget
	if source == "" || filepath.Base(path) == "SKILL.md" {
		return data, nil
	}
	importedMap, rest, ok := frontmatterNode(data)
	if !ok {
		return data, nil
	}
	native, ok := importedTargetValue(mappingValue(importedMap, "model"), source)
	if !ok {
		return data, nil
	}
	var existingMap, current *yaml.Node
	if existing != nil {
		if m, _, ok := frontmatterNode(existing); ok {
			existingMap, current = m, mappingValue(m, "model")
		}
	}
	targets := importModelTargets(source)
	agent := slices.Contains(strings.Split(filepath.ToSlash(path), "/"), "agents")
	var unmark, mark []string
	var result *yaml.Node
	switch {
	case current == nil && native == nil:
		return data, nil
	case current == nil:
		result = native
		if scopableModel(source, native.Value) {
			result = perTargetModel(nil, targets, native)
		}
	case scopedModel(existingMap, source) != nil && !scopedModelDeleted(existingMap, source):
		result = current
	case native == nil && (!holdsModel || syncedModel(current, source, agent) == nil):
		result = current
	case native != nil && sameResolvedModel(current, source, native, agent) && !scopedModelDeleted(existingMap, source):
		result = current
	case current.Kind == yaml.ScalarNode && native == nil:
		result = current
		if onlyTargetsRead(targets) {
			result = nil
			break
		}
		mark = targets
	case current.Kind == yaml.ScalarNode:
		result = native
		if !portableModel(native.Value) && !onlyTargetsRead(targets) {
			result = perTargetModel(current, targets, native)
		}
		for _, target := range targets {
			if scopedModelDeleted(existingMap, target) {
				unmark = append(unmark, target)
			}
		}
	case current.Kind == yaml.MappingNode:
		result = current
		for _, target := range targets {
			if scopedModel(existingMap, target) != nil && !scopedModelDeleted(existingMap, target) {
				continue
			}
			if native == nil {
				if mappingValue(result, target) != nil {
					result = withoutMappingKey(result, target)
				}
				if syncedModel(result, target, agent) != nil {
					mark = append(mark, target)
				}
				continue
			}
			if scopedModelDeleted(existingMap, target) {
				unmark = append(unmark, target)
			}
			result = withMappingValue(result, target, native)
		}
	default:
		return data, nil
	}
	if sameModelNode(result, mappingValue(importedMap, "model")) && len(mark)+len(unmark) == 0 && !hasEmptyScopedBlock(importedMap, targets) {
		return data, nil
	}
	if result == nil || result.Kind == yaml.MappingNode && len(result.Content) == 0 {
		dropMappingKey(importedMap, "model")
	} else {
		setMappingKey(importedMap, "model", result)
	}
	for _, target := range unmark {
		dropScopedKey(importedMap, "model", target)
	}
	for _, target := range mark {
		markKeyDeleted(importedMap, "model", target)
	}
	for _, target := range targets {
		if block := mappingValue(importedMap, "x-"+target); block != nil && block.Kind == yaml.MappingNode && len(block.Content) == 0 {
			dropMappingKey(importedMap, "x-"+target)
		}
	}
	return encodeFrontmatter(importedMap, rest, source)
}

// syncedModel is the model sync writes into target's file from value, or
// nil when it writes none: no value reaches target, or target's adapter
// drops a shared Claude model name its agents cannot load.
func syncedModel(value *yaml.Node, target string, agent bool) *yaml.Node {
	given := resolvedTargetValue("model", value, target)
	if given == nil {
		return nil
	}
	own := value.Kind == yaml.MappingNode && mappingValue(value, target) != nil
	if agent && !own && adapters.ForeignClaudeModel(target, perTargetValue(value, target).Value) {
		return nil
	}
	return given
}

// sameResolvedModel reports whether value gives target the model native
// names, after tiers and aliases.
func sameResolvedModel(value *yaml.Node, target string, native *yaml.Node, agent bool) bool {
	given := syncedModel(value, target, agent)
	if given == nil {
		return false
	}
	want := native.Value
	if id, isAlias := adapters.ModelAlias(target, want); isAlias {
		want = id
	}
	return given.Value == want
}

// onlyTargetsRead reports whether every configured target is in targets,
// so a shared value reaches no other tool.
func onlyTargetsRead(targets []string) bool {
	if importHookConfig == nil {
		return true
	}
	for _, target := range importHookConfig.Targets {
		if !slices.Contains(targets, target) {
			return false
		}
	}
	return true
}

// perTargetModel returns `{default: shared, <target>: native, ...}`, or
// the entries alone when shared is nil.
func perTargetModel(shared *yaml.Node, targets []string, native *yaml.Node) *yaml.Node {
	out := &yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle}
	if shared != nil {
		out.Content = append(out.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "default"}, shared)
	}
	for _, target := range targets {
		out.Content = append(out.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: target}, &yaml.Node{Kind: yaml.ScalarNode, Value: native.Value})
	}
	return out
}

// setMappingKey sets key to value in mapping, appending it when absent.
func setMappingKey(mapping *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = value
			return
		}
	}
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, value)
}

// dropMappingKey removes key from mapping.
func dropMappingKey(mapping *yaml.Node, key string) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			return
		}
	}
}

// scopedModel is target's `x-<target>.model` in spec, or nil.
func scopedModel(spec *yaml.Node, target string) *yaml.Node {
	return mappingValue(mappingValue(spec, "x-"+target), "model")
}

// sameModelNode reports whether a and b hold the same model value.
func sameModelNode(a, b *yaml.Node) bool {
	if a == nil || b == nil {
		return a == b
	}
	var av, bv any
	return a.Decode(&av) == nil && b.Decode(&bv) == nil && reflect.DeepEqual(av, bv)
}

// hasEmptyScopedBlock reports whether spec holds an empty x-<target> block
// for one of targets.
func hasEmptyScopedBlock(spec *yaml.Node, targets []string) bool {
	for _, target := range targets {
		if block := mappingValue(spec, "x-"+target); block != nil && block.Kind == yaml.MappingNode && len(block.Content) == 0 {
			return true
		}
	}
	return false
}

// scopedModelDeleted reports whether the spec marks target's model
// deleted with `x-<target>: {model: null}`.
func scopedModelDeleted(spec *yaml.Node, target string) bool {
	scoped := scopedModel(spec, target)
	return scoped != nil && scoped.Tag == "!!null"
}
