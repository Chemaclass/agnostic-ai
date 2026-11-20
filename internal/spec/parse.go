package spec

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/errs"
)

// ParseMarkdownBytes parses an in-memory spec document and returns the
// resulting Entry. Used by callers that have a spec body in hand and
// no on-disk path (e.g. the WASM playground) — Path is left empty so
// adapters that emit per-file outputs fall back to the entry's Name.
//
// The data must be UTF-8 with optional `---` frontmatter on top.
func ParseMarkdownBytes(kind Kind, data []byte) (Entry, error) {
	front, body, err := splitFrontmatter(normalizeLineEndings(data))
	if err != nil {
		return Entry{}, err
	}
	name, _ := front.meta["name"].(string)
	return Entry{
		Kind:             kind,
		Name:             name,
		Meta:             front.meta,
		MetaKeys:         front.keys,
		NestedKeys:       front.nestedKeys,
		MetaStyles:       front.styles,
		DuplicateKeys:    front.duplicates,
		Body:             body,
		OpeningBlockKept: front.bodyOnly,
	}, nil
}

// ParseYAMLBytes parses an in-memory YAML spec (hook, MCP, settings, or
// environment; no frontmatter) and returns the Entry.
func ParseYAMLBytes(kind Kind, data []byte) (Entry, error) {
	e, err := decodeYAMLEntry(data)
	if err != nil {
		return Entry{}, errs.YAMLLine(err, data)
	}
	e.Kind = kind
	return e, nil
}

func parseMarkdown(path string) (Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, fmt.Errorf("read: %w", err)
	}
	normalized := normalizeLineEndings(data)
	front, body, err := splitFrontmatter(normalized)
	if err != nil {
		// The decoded text keeps the rest of the opening `---` line, so its
		// lines already match the file.
		return Entry{}, formatYAMLError(path, err, 0, frontmatterYAML(normalized))
	}
	name, _ := front.meta["name"].(string)
	return Entry{
		Path:             path,
		Name:             name,
		Meta:             front.meta,
		MetaKeys:         front.keys,
		NestedKeys:       front.nestedKeys,
		MetaStyles:       front.styles,
		DuplicateKeys:    front.duplicates,
		Body:             body,
		OpeningBlockKept: front.bodyOnly,
		BodyLine:         bytes.Count(normalized[:len(normalized)-len(body)], []byte("\n")) + 1,
	}, nil
}

func parseYAML(path string) (Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, fmt.Errorf("read: %w", err)
	}
	return parseYAMLData(path, data)
}

func parseYAMLData(path string, data []byte) (Entry, error) {
	e, err := decodeYAMLEntry(data)
	if err != nil {
		return Entry{}, formatYAMLError(path, err, 0, data)
	}
	e.Path = path
	return e, nil
}

// decodeYAMLEntry parses a pure YAML spec into an Entry without Kind or
// Path.
func decodeYAMLEntry(data []byte) (Entry, error) {
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return Entry{}, err
	}
	front, err := nodeToOrderedMap(&node)
	if err != nil {
		return Entry{}, err
	}
	if front.meta == nil {
		front.meta = map[string]any{}
	}
	name, _ := front.meta["name"].(string)
	return Entry{
		Name:          name,
		Meta:          front.meta,
		MetaKeys:      front.keys,
		NestedKeys:    nestedKeyOrders(&node, front.meta),
		MetaStyles:    front.styles,
		DuplicateKeys: front.duplicates,
		Literals:      literalTags(&node),
	}, nil
}

// decodedMapping is a YAML mapping read for a spec: its values, the
// order of its keys, how the author quoted scalar values, and the keys
// written more than once.
type decodedMapping struct {
	meta       map[string]any
	bodyOnly   bool
	keys       []string
	nestedKeys map[string][]string
	styles     map[string]yaml.Style
	// duplicates lists each key a second occurrence repeats, in the order
	// the repeat first appears. The last value wins in meta.
	duplicates []string
}

// decodeYAMLOrdered parses a YAML document into a map, an ordered key
// slice, and per-key value styles so adapters can re-emit the
// frontmatter in source order with the author's original scalar style
// (plain vs. double-quoted, etc.). Empty input returns empty map and
// nil keys/styles.
func decodeYAMLOrdered(data []byte) (decodedMapping, error) {
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return decodedMapping{}, err
	}
	if len(node.Content) > 0 {
		value := node.Content[0]
		if value.Kind != yaml.MappingNode && (value.Tag != "!!null" || value.Value != "") {
			return decodedMapping{bodyOnly: true}, nil
		}
	}
	front, err := nodeToOrderedMap(&node)
	if err != nil {
		return decodedMapping{}, err
	}
	if front.meta == nil {
		front.meta = map[string]any{}
	}
	front.nestedKeys = nestedKeyOrders(&node, front.meta)
	return front, nil
}

// nodeToOrderedMap converts a YAML mapping node to its values, ordered
// keys, per-key value styles, and repeated keys. Non-mapping inputs
// return the zero value; the DocumentNode wrapper is peeled when
// present. A value that fails to decode, such as a nested repeated key,
// returns its error; only the capability restrictions are kept as nodes
// for validation to report. The styles map only carries entries
// for scalar values whose Style was non-default; missing keys mean "let
// the encoder choose" at re-emit time.
func nodeToOrderedMap(n *yaml.Node) (decodedMapping, error) {
	if n == nil {
		return decodedMapping{}, nil
	}
	if n.Kind == yaml.DocumentNode {
		if len(n.Content) == 0 {
			return decodedMapping{}, nil
		}
		return nodeToOrderedMap(n.Content[0])
	}
	if n.Kind != yaml.MappingNode {
		return decodedMapping{}, nil
	}
	out := decodedMapping{
		meta: make(map[string]any, len(n.Content)/2),
		keys: make([]string, 0, len(n.Content)/2),
	}
	seen := make(map[string]bool, len(n.Content)/2)
	content, err := expandMergeKeys(n.Content, mergeCache{}, 0)
	if err != nil {
		return decodedMapping{}, err
	}
	for i := 0; i+1 < len(content); i += 2 {
		keyNode := content[i]
		valNode := content[i+1]
		if seen[keyNode.Value] && !slices.Contains(out.duplicates, keyNode.Value) {
			out.duplicates = append(out.duplicates, keyNode.Value)
		}
		seen[keyNode.Value] = true
		var v any
		if err := valNode.Decode(&v); err != nil {
			// Keep malformed restrictions for capability validation.
			if keyNode.Value != capabilityKey && keyNode.Value != "allowed-tools" {
				return decodedMapping{}, err
			}
			v = valNode
		}
		if _, dup := out.meta[keyNode.Value]; !dup {
			out.keys = append(out.keys, keyNode.Value)
		}
		out.meta[keyNode.Value] = v
		if valNode.Kind == yaml.ScalarNode && valNode.Style != 0 {
			if out.styles == nil {
				out.styles = make(map[string]yaml.Style)
			}
			out.styles[keyNode.Value] = valNode.Style
		}
	}
	return out, nil
}

// normalizeLineEndings converts CRLF to LF so the frontmatter splitter
// works on Windows-authored files.
func normalizeLineEndings(b []byte) []byte {
	if !bytes.Contains(b, []byte("\r\n")) {
		return b
	}
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
}

// markdownHeadingRe matches a line such as `## Setup` or `# Setup`. A block
// holding one is Markdown text, not a YAML comment, so it stays in the body.
// A bare `#` or `#note` stays a comment, and so does a commented-out key such
// as `# paths: src/**`, which is frontmatter someone turned off.
var markdownHeadingRe = regexp.MustCompile(`^#{1,6}[ \t]+\S`)
var commentedKeyRe = regexp.MustCompile(`^#\s*[\w-]+\s*:`)

func hasMarkdownHeading(block []byte) bool {
	for _, line := range strings.Split(string(block), "\n") {
		if markdownHeadingRe.MatchString(line) && !commentedKeyRe.MatchString(line) {
			return true
		}
	}
	return false
}

// splitFrontmatter parses a leading `---` block as YAML and returns the
// ordered map of frontmatter keys along with the remaining body. A file
// that does not start with `---`, or whose closing `---` is missing,
// is treated as body-only with empty meta. Malformed YAML inside a
// fully delimited block is surfaced as an error rather than silently
// swallowed.
// frontmatterYAML is the text splitFrontmatter decodes: from after the
// opening `---` to the line before the closing one.
func frontmatterYAML(data []byte) []byte {
	before, _, _ := bytes.Cut(bytes.TrimPrefix(data, []byte("---")), []byte("\n---"))
	return before
}

func splitFrontmatter(data []byte) (decodedMapping, string, error) {
	const delim = "---"
	empty := decodedMapping{meta: map[string]any{}}

	if !bytes.HasPrefix(data, []byte(delim)) {
		return empty, string(data), nil
	}
	rest := data[len(delim):]
	before, after, ok := bytes.Cut(rest, []byte("\n"+delim))
	if !ok {
		return empty, string(data), nil
	}
	yamlPart := before
	body := bytes.TrimLeft(after, "\n")

	front, err := decodeYAMLOrdered(yamlPart)
	if err != nil {
		return decodedMapping{}, "", err
	}
	if len(front.meta) == 0 && hasMarkdownHeading(yamlPart) {
		front.bodyOnly = true
	}
	if front.bodyOnly {
		empty.bodyOnly = true
		return empty, string(data), nil
	}
	return front, string(body), nil
}
