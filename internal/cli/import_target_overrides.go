package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"reflect"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// importScopedTargetKeys returns a key the spec at dst sets under
// `x-<target>`, and that sync therefore wrote at the top level of the
// native agent file, to `x-<target>`. Without it, import would make the
// key shared and every other tool would get it on the next sync. The
// spec's shared value for that key, when it has one, comes back in its
// place.
//
// The block is rebuilt from the spec's own, so its order and comments
// stay, and a value the native file did not change keeps the spec's
// node. A scoped key the native file no longer holds was deleted there
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
	changed := false
	for i := 0; i+1 < len(scoped.Content); i += 2 {
		k, was := scoped.Content[i].Value, scoped.Content[i+1]
		if slices.Contains(keep, k) || isNullNode(was) || mappingValue(native, k) != nil {
			continue
		}
		changed = true
	}
	if !changed {
		return data, nil
	}
	block := &yaml.Node{Kind: yaml.MappingNode, Style: scoped.Style &^ yaml.FlowStyle, HeadComment: scoped.HeadComment, LineComment: scoped.LineComment, FootComment: scoped.FootComment}
	for i := 0; i+1 < len(scoped.Content); i += 2 {
		k, was := scoped.Content[i], scoped.Content[i+1]
		switch top, inBlock := mappingValue(mapping, k.Value), mappingValue(native, k.Value); {
		case slices.Contains(keep, k.Value):
			block.Content = append(block.Content, k, was)
		case inBlock != nil:
			block.Content = append(block.Content, k, specOrNative(was, inBlock, k.Value))
		case top != nil && !isNullNode(was):
			block.Content = append(block.Content, k, specOrNative(was, top, k.Value))
			removeOrShare(mapping, k.Value, have)
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
	if len(block.Content) == 0 {
		block.Style = yaml.FlowStyle
	}
	setMappingNode(mapping, key, block)
	return encodeFrontmatter(mapping, rest, target)
}

// specOrNative returns the spec's node when the native value means the
// same, so its comments, order and style stay, and the native node,
// with the spec's comments, when it changed. A tool list the native
// file writes as one comma-separated string, as Qoder does, matches the
// spec's list.
func specOrNative(was, native *yaml.Node, key string) *yaml.Node {
	var a, b any
	if was.Decode(&a) == nil && native.Decode(&b) == nil {
		if reflect.DeepEqual(a, b) {
			return was
		}
		if text, ok := b.(string); ok && key == "tools" && reflect.DeepEqual(toStringSlice(a), spec.SplitToolList(text)) && len(toStringSlice(a)) > 0 {
			return was
		}
	}
	out := *native
	if out.LineComment == "" {
		out.LineComment = was.LineComment
	}
	if out.HeadComment == "" {
		out.HeadComment = was.HeadComment
	}
	return &out
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
// key when the spec shares none. A spec that shares `can` gets it back
// for a scoped `tools`.
func removeOrShare(mapping *yaml.Node, key string, have *yaml.Node) {
	sharedKey, shared := key, mappingValue(have, key)
	if shared == nil && key == "tools" {
		sharedKey, shared = "can", mappingValue(have, "can")
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value != key {
			continue
		}
		if shared != nil {
			mapping.Content[i].Value = sharedKey
			mapping.Content[i+1] = shared
		} else {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
		}
		return
	}
}

func isNullNode(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && n.Tag == "!!null"
}

// specScopesKey reports whether the spec at dst sets key under
// `x-<target>`, so the native value under that key came from there and
// a translation meant for a shared value must leave it alone.
func specScopesKey(dst, target, key string) bool {
	data, err := os.ReadFile(dst)
	if err != nil {
		return false
	}
	have, _, ok := frontmatterNode(data)
	return ok && mappingValue(mappingValue(have, "x-"+target), key) != nil
}
