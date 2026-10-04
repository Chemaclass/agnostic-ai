package spec

import "gopkg.in/yaml.v3"

// LiteralTag marks a value in a top-level map, such as an MCP `env` or
// `headers` value, as a plain setting rather than a secret:
// `NODE_ENV: !literal production`. It is YAML only: Meta holds the bare
// string, so sync writes the value without it.
const LiteralTag = "!literal"

// MarkedLiteral reports whether the spec tags the value of key in the
// top-level map field with LiteralTag.
func (e Entry) MarkedLiteral(field, key string) bool {
	return e.Literals[field][key]
}

// literalTags returns, by top-level key, the keys of each top-level
// mapping whose value carries LiteralTag. A repeated key counts as the
// decoder reads it: the last one wins.
func literalTags(n *yaml.Node) map[string]map[string]bool {
	if n.Kind == yaml.DocumentNode && len(n.Content) == 1 {
		n = n.Content[0]
	}
	if n.Kind != yaml.MappingNode {
		return nil
	}
	var out map[string]map[string]bool
	for i := 0; i+1 < len(n.Content); i += 2 {
		field, values := n.Content[i].Value, resolveAlias(n.Content[i+1])
		delete(out, field)
		if values.Kind != yaml.MappingNode {
			continue
		}
		marked := map[string]bool{}
		for j := 0; j+1 < len(values.Content); j += 2 {
			key := values.Content[j].Value
			if resolveAlias(values.Content[j+1]).Tag == LiteralTag {
				marked[key] = true
			} else {
				delete(marked, key)
			}
		}
		if len(marked) == 0 {
			continue
		}
		if out == nil {
			out = map[string]map[string]bool{}
		}
		out[field] = marked
	}
	return out
}

// resolveAlias returns the node an alias such as `*plain` stands for.
func resolveAlias(n *yaml.Node) *yaml.Node {
	if n.Kind == yaml.AliasNode && n.Alias != nil {
		return n.Alias
	}
	return n
}

// mergeLiterals keeps the LiteralTag mark of each merged map value from
// the layer whose value won.
func mergeLiterals(base, over Entry, meta map[string]any) map[string]map[string]bool {
	var out map[string]map[string]bool
	for field, raw := range meta {
		values, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		overValues, _ := over.Meta[field].(map[string]any)
		for key := range values {
			from := base
			if _, ok := overValues[key]; ok {
				from = over
			}
			if !from.MarkedLiteral(field, key) {
				continue
			}
			if out == nil {
				out = map[string]map[string]bool{}
			}
			if out[field] == nil {
				out[field] = map[string]bool{}
			}
			out[field][key] = true
		}
	}
	return out
}
