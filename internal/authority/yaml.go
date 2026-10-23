package authority

import (
	"bytes"
	"cmp"
	"fmt"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// listOrder keeps permission lists in their canonical order when a
// proposal adds one the spec did not have.
var listOrder = map[string]int{"allow": 0, "ask": 1, "deny": 2}

// renderSpecYAML renders a YAML spec's fields in the order the spec
// writes them, nested objects included, so a before and after differ
// only where a change did.
func renderSpecYAML(e spec.Entry) string {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(mappingNode(e, e.Meta, nil)); err != nil {
		return ""
	}
	if err := enc.Close(); err != nil {
		return ""
	}
	return buf.String()
}

func mappingNode(e spec.Entry, m map[string]any, path []string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode}
	for _, key := range orderedKeys(e, m, path) {
		value := valueNode(e, m[key], append(slices.Clone(path), key))
		if style, ok := e.MetaStyles[key]; ok && len(path) == 0 && value.Kind == yaml.ScalarNode {
			value.Style = style
		}
		n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, value)
	}
	return n
}

func valueNode(e spec.Entry, v any, path []string) *yaml.Node {
	switch t := v.(type) {
	case map[string]any:
		return mappingNode(e, t, path)
	case []any:
		n := &yaml.Node{Kind: yaml.SequenceNode}
		for _, item := range t {
			n.Content = append(n.Content, valueNode(e, item, path))
		}
		return n
	}
	var n yaml.Node
	if err := n.Encode(v); err != nil {
		return &yaml.Node{Kind: yaml.ScalarNode, Value: fmt.Sprint(v)}
	}
	return &n
}

// orderedKeys lists m's keys in the order the spec recorded, then any
// new key: permission lists in canonical order, others by name.
func orderedKeys(e spec.Entry, m map[string]any, path []string) []string {
	recorded := e.MetaKeys
	if len(path) > 0 {
		recorded = e.KeyOrder(path...)
	}
	var out []string
	for _, k := range recorded {
		if _, ok := m[k]; ok && !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	var rest []string
	for k := range m {
		if !slices.Contains(out, k) {
			rest = append(rest, k)
		}
	}
	slices.SortFunc(rest, func(a, b string) int {
		ra, okA := listOrder[a]
		rb, okB := listOrder[b]
		if !okA {
			ra = len(listOrder)
		}
		if !okB {
			rb = len(listOrder)
		}
		return cmp.Or(cmp.Compare(ra, rb), cmp.Compare(a, b))
	})
	return append(out, rest...)
}
