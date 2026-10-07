package emit

import (
	"os"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// ExistingObjectAt returns the object at keyPath in the JSON or JSONC
// file at path in file order, or nil when the file or a key on the way
// is missing or holds something else.
func (s *Session) ExistingObjectAt(path string, keyPath []string, dryRun bool) *OrderedJSON {
	return s.existingObject(path, keyPath, dryRun)
}

// OrderedObject returns object with its keys in order, then any key order
// leaves out, sorted.
func OrderedObject(object map[string]any, order []string) (*OrderedJSON, error) {
	out := NewOrderedJSON()
	for _, key := range order {
		value, ok := object[key]
		if _, done := out.Get(key); !ok || done {
			continue
		}
		if err := out.Set(key, value); err != nil {
			return nil, err
		}
	}
	rest := make([]string, 0, len(object))
	for key := range object {
		if _, set := out.Get(key); !set {
			rest = append(rest, key)
		}
	}
	sort.Strings(rest)
	for _, key := range rest {
		if err := out.Set(key, object[key]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// SpecKeyOrder returns the keys of the mapping at keyPath in the YAML
// spec file entry was read from, in the order the file writes them. A
// loaded spec keeps the key order of its top level only. It returns nil
// when the file cannot be read or holds no mapping there.
func SpecKeyOrder(entry spec.Entry, keyPath ...string) []string {
	if entry.Path == "" {
		return nil
	}
	data, err := os.ReadFile(entry.Path)
	if err != nil {
		return nil
	}
	var doc yaml.Node
	if yaml.Unmarshal(data, &doc) != nil || len(doc.Content) == 0 {
		return nil
	}
	node := doc.Content[0]
	for _, key := range keyPath {
		node = mappingValue(node, key)
		if node == nil {
			return nil
		}
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}
	keys := make([]string, 0, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		keys = append(keys, node.Content[i].Value)
	}
	return keys
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}
