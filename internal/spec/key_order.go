package spec

import (
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// keyPathSep joins the keys of a path in Entry.NestedKeys. YAML keys in
// a spec never hold it.
const keyPathSep = "\x00"

// KeyOrder returns the keys of the object at path in Meta in the order
// the spec writes them, base layer first, or nil when no order is
// recorded. Meta holds Go maps, which keep no order, and some tools
// read an object's order as meaning: OpenCode applies the last
// permission rule that matches.
func (e Entry) KeyOrder(path ...string) []string {
	return slices.Clone(e.NestedKeys[strings.Join(path, keyPathSep)])
}

// nestedKeyOrders records, by key path, the key order of every mapping
// below the top level of a YAML document. MetaKeys holds the top level.
// Only a top-level value that decoded into meta is walked, and an alias
// back to a node on the current path stops the walk there: such a value
// is a cycle, which decoding already rejected.
func nestedKeyOrders(n *yaml.Node, meta map[string]any) map[string][]string {
	if n == nil {
		return nil
	}
	if n.Kind == yaml.DocumentNode {
		if len(n.Content) == 0 {
			return nil
		}
		n = n.Content[0]
	}
	if n.Kind != yaml.MappingNode {
		return nil
	}
	out := map[string][]string{}
	active := map[*yaml.Node]bool{}
	var walk func(node *yaml.Node, path []string)
	walk = func(node *yaml.Node, path []string) {
		node = resolveAlias(node)
		if node.Kind != yaml.MappingNode || active[node] {
			return
		}
		active[node] = true
		defer delete(active, node)
		var keys []string
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i].Value
			if !slices.Contains(keys, key) {
				keys = append(keys, key)
			}
			walk(node.Content[i+1], append(slices.Clone(path), key))
		}
		if len(keys) > 0 {
			out[strings.Join(path, keyPathSep)] = keys
		}
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key := n.Content[i].Value
		if _, decoded := meta[key]; decoded {
			walk(n.Content[i+1], []string{key})
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// mergeNestedKeys gives each object of the merged meta base's key order,
// then over's new keys, as mergeMetaKeys does for the top level. A key
// over repeats keeps base's place.
func mergeNestedKeys(base, over map[string][]string, meta map[string]any) map[string][]string {
	if base == nil && over == nil {
		return nil
	}
	out := map[string][]string{}
	for _, orders := range []map[string][]string{base, over} {
		for joined, keys := range orders {
			object, ok := valueAt(meta, strings.Split(joined, keyPathSep)).(map[string]any)
			if !ok {
				continue
			}
			for _, key := range keys {
				if _, held := object[key]; held && !slices.Contains(out[joined], key) {
					out[joined] = append(out[joined], key)
				}
			}
		}
	}
	return out
}

func valueAt(meta map[string]any, path []string) any {
	var value any = meta
	for _, key := range path {
		object, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		value = object[key]
	}
	return value
}
