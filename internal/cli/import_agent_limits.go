package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// importPortableAgentKey renames a native agent key, such as Gemini's
// `max_turns` or Cursor's `is_background`, to the portable field when
// its value is valid there and the file does not set that field too.
// Any other file comes back unchanged, and sync still reads the native
// key.
//
// When the spec at dst already sets the field under `x-<target>`, the
// imported value goes back there and the spec's shared value is kept,
// so a value meant for one tool does not reach the others. With
// mergeExisting, for a target whose import merges into the spec, the
// spec's other `x-<target>` keys are kept too; otherwise the native file
// holds every key, so only the scoped one is written back.
func importPortableAgentKey(data []byte, dst, target, native, portable string, mergeExisting bool) ([]byte, error) {
	meta, _ := splitMdcFrontmatter(data)
	value, set := meta[native]
	if _, both := meta[portable]; !set || both {
		return data, nil
	}
	existing, err := os.ReadFile(dst)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read %s: %w", dst, err)
	}
	have, _ := splitMdcFrontmatter(existing)
	custom, _ := have["x-"+target].(map[string]any)
	for _, key := range []string{native, portable} {
		if _, scoped := custom[key]; scoped {
			keep := map[string]any{}
			if mergeExisting {
				keep = custom
			}
			return importScopedAgentValue(data, target, native, portable, key, have, keep)
		}
	}
	var raw string
	switch portable {
	case "maxTurns":
		n, ok := spec.AgentMaxTurns(value)
		if !ok {
			return data, nil
		}
		raw = strconv.Itoa(n)
	case "background":
		b, ok := spec.AgentBackground(value)
		if !ok {
			return data, nil
		}
		raw = strconv.FormatBool(b)
	default:
		return data, nil
	}
	out, err := rewriteFrontmatterKeys(string(data), []yamlKeyRewrite{{Key: native, NewKey: portable, Value: raw}})
	if err != nil {
		return data, nil
	}
	return []byte(out), nil
}

// importScopedAgentValue edits the frontmatter of data in place, keeping
// key order and comments: the native key becomes the spec's shared
// portable value, or goes when the spec has none, and its value moves
// under `x-<target>.<key>`, next to the keys in keep the file does not
// set there itself.
func importScopedAgentValue(data []byte, target, native, portable, key string, have, keep map[string]any) ([]byte, error) {
	text := string(data)
	rest, ok := strings.CutPrefix(text, "---")
	end := strings.Index(rest, "\n---")
	if !ok || end < 0 {
		return data, nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(rest[:end]), &doc); err != nil || doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return data, nil
	}
	mapping := doc.Content[0]
	at := -1
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == native {
			at = i
		}
	}
	if at < 0 {
		return data, nil
	}
	value := mapping.Content[at+1]
	if shared, set := have[portable]; set {
		var node yaml.Node
		if err := node.Encode(shared); err != nil {
			return nil, fmt.Errorf("encode %s: %w", portable, err)
		}
		mapping.Content[at].Value = portable
		mapping.Content[at+1] = &node
	} else {
		mapping.Content = append(mapping.Content[:at], mapping.Content[at+2:]...)
	}
	custom := xTargetMapping(mapping, "x-"+target)
	for _, k := range slices.Sorted(maps.Keys(keep)) {
		if k == key || mappingHasKey(custom, k) {
			continue
		}
		var node yaml.Node
		if err := node.Encode(keep[k]); err != nil {
			return nil, fmt.Errorf("encode x-%s.%s: %w", target, k, err)
		}
		custom.Content = append(custom.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: k}, &node)
	}
	setMappingNode(custom, key, value)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return nil, fmt.Errorf("encode %s agent frontmatter: %w", target, err)
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return []byte("---\n" + buf.String() + strings.TrimPrefix(rest[end:], "\n")), nil
}

// xTargetMapping returns the block mapping under key, adding it at the
// end when mapping has none.
func xTargetMapping(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key && mapping.Content[i+1].Kind == yaml.MappingNode {
			mapping.Content[i+1].Style &^= yaml.FlowStyle
			return mapping.Content[i+1]
		}
	}
	block := &yaml.Node{Kind: yaml.MappingNode}
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, block)
	return block
}

func setMappingNode(mapping *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = value
			return
		}
	}
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, value)
}
