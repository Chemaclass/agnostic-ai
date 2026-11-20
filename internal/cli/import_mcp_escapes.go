package cli

import (
	"bytes"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// keepSyncedMCPEscapes returns now, an MCP spec import rewrote from
// sources, with each value that reads as before's escaped `$${NAME}` put
// back as written. A tool that expands `${NAME}` in a field cannot hold
// a literal `${`, so sync writes the escape there as `${NAME}` and the
// tool's file alone reads as a reference. A field no source expands
// that way keeps what import read, since its file tells the two apart.
// It reports false when nothing changed or the YAML does not parse.
func keepSyncedMCPEscapes(before, now []byte, sources []string) ([]byte, bool) {
	if !bytes.Contains(before, []byte("$${")) {
		return now, false
	}
	var beforeDoc, nowDoc yaml.Node
	if yaml.Unmarshal(before, &beforeDoc) != nil || yaml.Unmarshal(now, &nowDoc) != nil {
		return now, false
	}
	from, into := topMapping(&beforeDoc), topMapping(&nowDoc)
	if from == nil || into == nil || from.Kind != yaml.MappingNode || into.Kind != yaml.MappingNode {
		return now, false
	}
	expands := func(field string) bool {
		return slices.ContainsFunc(sources, func(s string) bool { return adapters.MCPFieldReadsDollarRefs(s, field) })
	}
	changed := keepMCPBlockEscapes(from, into, expands)
	for i := 0; i+1 < len(from.Content); i += 2 {
		if key := from.Content[i].Value; strings.HasPrefix(key, "x-") {
			if j := mappingIndex(into, key); j >= 0 {
				changed = keepMCPBlockEscapes(from.Content[i+1], into.Content[j+1], expands) || changed
			}
		}
	}
	if !changed {
		return now, false
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if enc.Encode(&nowDoc) != nil || enc.Close() != nil {
		return now, false
	}
	return buf.Bytes(), true
}

// keepMCPBlockEscapes restores escapes in each field of one server block
// that a source expands `${NAME}` in.
func keepMCPBlockEscapes(from, into *yaml.Node, expands func(field string) bool) bool {
	if from.Kind != yaml.MappingNode || into.Kind != yaml.MappingNode {
		return false
	}
	changed := false
	for _, field := range []string{"env", "headers", "url", "args"} {
		if expands(field) {
			changed = keepFieldEscapes(mappingValue(from, field), mappingValue(into, field)) || changed
		}
	}
	options, intoOptions := mappingValue(from, "requestOptions"), mappingValue(into, "requestOptions")
	if options != nil && intoOptions != nil && expands("headers") {
		changed = keepFieldEscapes(mappingValue(options, "headers"), mappingValue(intoOptions, "headers")) || changed
	}
	return changed
}

// keepFieldEscapes matches a scalar, a map by key, or a list item by the
// value sync wrote for it, so an argument added or removed elsewhere
// does not shift the match.
func keepFieldEscapes(from, into *yaml.Node) bool {
	if from == nil || into == nil || from.Kind != into.Kind {
		return false
	}
	switch from.Kind {
	case yaml.ScalarNode:
		if escapedAs(from, into) {
			into.Value, into.Style, into.Tag = from.Value, from.Style, from.Tag
			return true
		}
	case yaml.SequenceNode:
		changed := false
		claimed := make([]bool, len(into.Content))
		for _, item := range from.Content {
			for j, candidate := range into.Content {
				if !claimed[j] && escapedAs(item, candidate) {
					claimed[j] = true
					changed = keepFieldEscapes(item, candidate) || changed
					break
				}
			}
		}
		return changed
	case yaml.MappingNode:
		changed := false
		for i := 0; i+1 < len(from.Content); i += 2 {
			if j := mappingIndex(into, from.Content[i].Value); j >= 0 {
				changed = keepFieldEscapes(from.Content[i+1], into.Content[j+1]) || changed
			}
		}
		return changed
	}
	return false
}

// escapedAs reports whether into is the `${NAME}` text sync wrote for
// from's `$${NAME}` escape.
func escapedAs(from, into *yaml.Node) bool {
	return from.Kind == yaml.ScalarNode && into.Kind == yaml.ScalarNode &&
		strings.Contains(from.Value, "$${") && into.Value != from.Value &&
		spec.DecodeEnvRefEscapes(from.Value) == into.Value
}
