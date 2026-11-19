package spec

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

const maxMergeDepth = 32

// mergeCache holds the pairs each merge source expands to, so a source
// named many times is expanded once.
type mergeCache map[*yaml.Node][]*yaml.Node

// mergedContent returns a mapping's key and value nodes with merge keys
// spliced in, or its own content when a merge is invalid.
func mergedContent(n *yaml.Node) []*yaml.Node {
	content, err := expandMergeKeys(n.Content, mergeCache{}, 0)
	if err != nil {
		return n.Content
	}
	return content
}

func mergeError(line int, msg string) error {
	return &yaml.TypeError{Errors: []string{fmt.Sprintf("line %d: %s", line, msg)}}
}

// expandMergeKeys returns key and value nodes with each `<<` merge key
// replaced by the pairs it merges in. As in yaml.v3, an explicit key beats
// a merged one, and an earlier merge source beats a later one.
func expandMergeKeys(content []*yaml.Node, cache mergeCache, depth int) ([]*yaml.Node, error) {
	if depth > maxMergeDepth {
		return nil, mergeError(content[0].Line, "map merge nests too deeply")
	}
	explicit := make(map[string]bool, len(content)/2)
	var firstMerge *yaml.Node
	for i := 0; i+1 < len(content); i += 2 {
		if content[i].Tag == "!!merge" {
			if firstMerge != nil {
				return nil, mergeError(content[i].Line, fmt.Sprintf("mapping key \"<<\" already defined at line %d", firstMerge.Line))
			}
			firstMerge = content[i]
			continue
		}
		explicit[content[i].Value] = true
	}
	if firstMerge == nil {
		return content, nil
	}
	out := make([]*yaml.Node, 0, len(content))
	for i := 0; i+1 < len(content); i += 2 {
		if content[i].Tag != "!!merge" {
			out = append(out, content[i], content[i+1])
			continue
		}
		sources, err := mergeSources(content[i], content[i+1])
		if err != nil {
			return nil, err
		}
		for _, src := range sources {
			pairs, err := expandSource(src, cache, depth+1)
			if err != nil {
				return nil, err
			}
			for j := 0; j+1 < len(pairs); j += 2 {
				if explicit[pairs[j].Value] {
					continue
				}
				explicit[pairs[j].Value] = true
				out = append(out, pairs[j], pairs[j+1])
			}
		}
	}
	return out, nil
}

func expandSource(src *yaml.Node, cache mergeCache, depth int) ([]*yaml.Node, error) {
	if pairs, ok := cache[src]; ok {
		return pairs, nil
	}
	seen := make(map[string]int, len(src.Content)/2)
	for i := 0; i+1 < len(src.Content); i += 2 {
		key := src.Content[i]
		if key.Tag == "!!merge" {
			continue
		}
		if first, dup := seen[key.Value]; dup {
			return nil, mergeError(key.Line, fmt.Sprintf("mapping key %q already defined at line %d", key.Value, first))
		}
		seen[key.Value] = key.Line
	}
	pairs, err := expandMergeKeys(src.Content, cache, depth)
	if err != nil {
		return nil, err
	}
	cache[src] = pairs
	return pairs, nil
}

// mergeSources resolves a merge key's value to the mappings it names: a
// mapping, an alias to one, or a sequence of those.
func mergeSources(key, val *yaml.Node) ([]*yaml.Node, error) {
	const want = "map merge requires map or sequence of maps as the value"
	v := resolveAlias(val)
	switch v.Kind {
	case yaml.MappingNode:
		return []*yaml.Node{v}, nil
	case yaml.SequenceNode:
		out := make([]*yaml.Node, 0, len(v.Content))
		for _, item := range v.Content {
			item = resolveAlias(item)
			if item.Kind != yaml.MappingNode {
				return nil, mergeError(item.Line, want)
			}
			out = append(out, item)
		}
		return out, nil
	}
	return nil, mergeError(key.Line, want)
}
