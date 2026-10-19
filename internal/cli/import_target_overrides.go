package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// importScopedTargetKeys returns a key the spec at dst sets under
// `x-<target>`, and that sync therefore wrote at the top level of the
// native agent file, to `x-<target>`. Without it, import would make the
// key shared and every other tool would get it on the next sync. The
// spec's shared value for that key, when it has one, comes back in its
// place.
//
// The block is rebuilt from the spec's own, so its order and comments
// stay. A scoped key the native file no longer holds was deleted there
// and is dropped, unless it is a delete marker (`null`). Keys in keep,
// which the target never writes or which its import scopes itself, stay
// as the spec has them. A file whose spec scopes none of its keys comes
// back unchanged.
func importScopedTargetKeys(data []byte, dst, target string, keep ...string) ([]byte, error) {
	existing, err := os.ReadFile(dst)
	if errors.Is(err, fs.ErrNotExist) {
		return data, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dst, err)
	}
	key := "x-" + target
	have, _, ok := frontmatterNode(existing)
	if !ok {
		return data, nil
	}
	scoped := mappingValue(have, key)
	if scoped == nil || scoped.Kind != yaml.MappingNode {
		return data, nil
	}
	doc, rest, ok := frontmatterNode(data)
	if !ok {
		return data, nil
	}
	mapping := doc
	native := mappingValue(mapping, key)
	if native != nil && native.Kind != yaml.MappingNode {
		return data, nil
	}
	moved := false
	for i := 0; i+1 < len(scoped.Content); i += 2 {
		k := scoped.Content[i].Value
		if !slices.Contains(keep, k) && mappingValue(mapping, k) != nil && mappingValue(native, k) == nil && !isNullNode(scoped.Content[i+1]) {
			moved = true
		}
	}
	if !moved {
		return data, nil
	}
	block := &yaml.Node{Kind: yaml.MappingNode, Style: scoped.Style &^ yaml.FlowStyle, HeadComment: scoped.HeadComment, LineComment: scoped.LineComment, FootComment: scoped.FootComment}
	for i := 0; i+1 < len(scoped.Content); i += 2 {
		k, was := scoped.Content[i], scoped.Content[i+1]
		switch top, inBlock := mappingValue(mapping, k.Value), mappingValue(native, k.Value); {
		case slices.Contains(keep, k.Value):
			block.Content = append(block.Content, k, was)
		case inBlock != nil:
			block.Content = append(block.Content, k, keepComments(inBlock, was))
		case top != nil && !isNullNode(was):
			block.Content = append(block.Content, k, keepComments(top, was))
			removeOrShare(mapping, k.Value, mappingValue(have, k.Value))
		case isNullNode(was):
			block.Content = append(block.Content, k, was)
		}
	}
	if native != nil {
		for i := 0; i+1 < len(native.Content); i += 2 {
			if mappingValue(block, native.Content[i].Value) == nil {
				block.Content = append(block.Content, native.Content[i], native.Content[i+1])
			}
		}
	}
	setMappingNode(mapping, key, block)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(mapping); err != nil {
		return nil, fmt.Errorf("encode %s agent frontmatter: %w", target, err)
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return []byte("---\n" + buf.String() + rest), nil
}

// frontmatterNode returns the frontmatter mapping of a Markdown spec and
// the text from its closing `---` on.
func frontmatterNode(data []byte) (*yaml.Node, string, bool) {
	rest, ok := strings.CutPrefix(string(data), "---")
	end := strings.Index(rest, "\n---")
	if !ok || end < 0 {
		return nil, "", false
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(rest[:end]), &doc); err != nil || doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, "", false
	}
	return doc.Content[0], strings.TrimPrefix(rest[end:], "\n"), true
}

func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// removeOrShare puts the spec's shared value back for key, or removes
// key when the spec shares none.
func removeOrShare(mapping *yaml.Node, key string, shared *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value != key {
			continue
		}
		if shared != nil {
			mapping.Content[i+1] = shared
		} else {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
		}
		return
	}
}

// keepComments returns value with the comments of was when value has
// none, so a comment written on the spec's override survives import.
func keepComments(value, was *yaml.Node) *yaml.Node {
	out := *value
	if out.LineComment == "" {
		out.LineComment = was.LineComment
	}
	if out.HeadComment == "" {
		out.HeadComment = was.HeadComment
	}
	return &out
}

func isNullNode(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && n.Tag == "!!null"
}
