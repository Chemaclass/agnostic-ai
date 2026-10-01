package emit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
)

// IsYAMLPath reports whether path names a YAML file.
func IsYAMLPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yml", ".yaml":
		return true
	}
	return false
}

// WriteMergedYAML is WriteMergedJSON for a YAML file, such as Aider's
// config, that also holds keys sync did not write.
func (s *Session) WriteMergedYAML(path, content string, keys []MergedKey, released [][]string, dryRun bool) error {
	return s.writeMerged(path, content, withYAMLValueSums(content, keys), released, dryRun)
}

// ReleaseMerged releases a merged file by its format: YAML or JSON.
func (s *Session) ReleaseMerged(path string, keys []MergedKey, created, force, dryRun bool) (MergedRelease, []MergedKey, error) {
	if IsYAMLPath(path) {
		return s.ReleaseMergedYAML(path, keys, created, force, dryRun)
	}
	return s.ReleaseMergedJSON(path, keys, created, force, dryRun)
}

// ParsesMerged reports whether the merged file at path still parses in
// its format, so its keys can be told apart.
func ParsesMerged(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	if IsYAMLPath(path) {
		_, err := parseYAMLMapping(string(data))
		return err == nil
	}
	stripped, _ := StripJSONC(data)
	return json.Valid(stripped)
}

func withYAMLValueSums(content string, keys []MergedKey) []MergedKey {
	doc, err := parseYAMLMapping(content)
	out := make([]MergedKey, 0, len(keys))
	for _, key := range keys {
		if key.Items != nil {
			key.Items = itemSums(key.Items)
		} else if err == nil {
			if _, value, found := yamlValueAt(doc, key.Path); found {
				key.Sum = yamlValueSum(value)
			}
		}
		out = append(out, key)
	}
	return out
}

// parseYAMLMapping parses content, provenance header aside, into a
// document whose root is a mapping. An empty document is an empty
// mapping.
func parseYAMLMapping(content string) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(header.Strip(content)), &doc); err != nil {
		return nil, err
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("not a mapping")
	}
	return &doc, nil
}

// yamlValueAt returns the mapping that holds the value at path in doc,
// and the value.
func yamlValueAt(doc *yaml.Node, path []string) (parent, value *yaml.Node, found bool) {
	node := doc.Content[0]
	for i, key := range path {
		if node.Kind != yaml.MappingNode {
			return nil, nil, false
		}
		at := yamlKeyIndex(node, key)
		if at < 0 {
			return nil, nil, false
		}
		if i == len(path)-1 {
			return node, node.Content[at+1], true
		}
		node = node.Content[at+1]
	}
	return nil, nil, false
}

func yamlKeyIndex(mapping *yaml.Node, key string) int {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return i
		}
	}
	return -1
}

// yamlValueSum fingerprints a YAML value regardless of its style, the
// way jsonValueSum does for JSON.
func yamlValueSum(value *yaml.Node) string {
	var decoded any
	if err := value.Decode(&decoded); err != nil {
		return ContentSum(value.Value)
	}
	raw, _ := yaml.Marshal(value)
	return canonicalValueSum(decoded, string(raw))
}

// ReleaseMergedYAML is ReleaseMergedJSON for a YAML file. It edits the
// parsed document, so the user's keys keep their order and comments;
// comments on the keys it takes out go with them. The provenance header
// goes too, since sync no longer writes the file.
func (s *Session) ReleaseMergedYAML(path string, keys []MergedKey, created, force, dryRun bool) (result MergedRelease, edited []MergedKey, err error) {
	if s.skipUnmanaged(path) {
		return MergedKept, nil, nil
	}
	existing, err := os.ReadFile(path)
	if IsAbsent(err) {
		return MergedUnchanged, nil, nil
	}
	if err != nil {
		return MergedKept, nil, fmt.Errorf("read %s: %w", path, err)
	}
	doc, err := parseYAMLMapping(string(existing))
	if err != nil {
		return MergedKept, nil, nil
	}
	changed := false
	for _, key := range keys {
		parent, value, found := yamlValueAt(doc, key.Path)
		switch {
		case !found:
		case key.Items != nil:
			if keep, dropped := withoutYAMLItems(value, key.Items); dropped {
				changed = true
				if !keep {
					deleteYAMLPath(doc, key.Path, parent)
				}
			}
		case !force && key.Sum != "" && yamlValueSum(value) != key.Sum:
			edited = append(edited, key)
		default:
			deleteYAMLPath(doc, key.Path, parent)
			changed = true
		}
	}
	root := doc.Content[0]
	if len(edited) == 0 && len(root.Content) == 0 && created {
		if _, err := s.remove(path, "", existing, force, dryRun); err != nil {
			return MergedKept, nil, err
		}
		return MergedRemoved, nil, nil
	}
	if changed || header.Has(string(existing)) {
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(detectYAMLIndent(header.Strip(string(existing))))
		if err := enc.Encode(doc); err != nil {
			return MergedKept, nil, fmt.Errorf("marshal %s: %w", path, err)
		}
		if err := enc.Close(); err != nil {
			return MergedKept, nil, fmt.Errorf("marshal %s: %w", path, err)
		}
		if err := s.WriteFile(path, buf.String(), dryRun); err != nil {
			return MergedKept, nil, err
		}
		changed = true
	}
	switch {
	case len(edited) > 0:
		return MergedEdited, edited, nil
	case changed:
		return MergedStripped, nil, nil
	}
	return MergedUnchanged, nil, nil
}

// withoutYAMLItems drops the string entries whose sums are in items
// from the list value, or the value itself when it is one such string.
// keep is false when nothing is left.
func withoutYAMLItems(value *yaml.Node, items []string) (keep, dropped bool) {
	switch value.Kind {
	case yaml.ScalarNode:
		if slices.Contains(items, ContentSum(value.Value)) {
			return false, true
		}
	case yaml.SequenceNode:
		kept := slices.DeleteFunc(slices.Clone(value.Content), func(entry *yaml.Node) bool {
			return entry.Kind == yaml.ScalarNode && slices.Contains(items, ContentSum(entry.Value))
		})
		if len(kept) == len(value.Content) {
			return true, false
		}
		value.Content = kept
		return len(kept) > 0, true
	}
	return true, false
}

// deleteYAMLPath removes the key at path from parent, then every
// mapping on the way that the removal left empty.
func deleteYAMLPath(doc *yaml.Node, path []string, parent *yaml.Node) {
	at := yamlKeyIndex(parent, path[len(path)-1])
	if at < 0 {
		return
	}
	parent.Content = slices.Delete(parent.Content, at, at+2)
	if len(parent.Content) > 0 || len(path) == 1 {
		return
	}
	grand, _, found := yamlValueAt(doc, path[:len(path)-1])
	if found {
		deleteYAMLPath(doc, path[:len(path)-1], grand)
	}
}

// detectYAMLIndent returns the smallest indent content uses, 2 to 8
// spaces, or 4, yaml.Marshal's indent, which sync writes with.
func detectYAMLIndent(content string) int {
	indent := 0
	for line := range strings.SplitSeq(content, "\n") {
		trimmed := strings.TrimLeft(line, " ")
		n := len(line) - len(trimmed)
		if n == 0 || trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if indent == 0 || n < indent {
			indent = n
		}
	}
	if indent < 2 || indent > 8 {
		return 4
	}
	return indent
}
