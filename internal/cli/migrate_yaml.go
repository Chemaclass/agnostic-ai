package cli

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// yamlKeyRewrite renames the top-level key Key to NewKey and sets its
// value to Value, in the quote style the old value used.
type yamlKeyRewrite struct {
	Key, NewKey, Value string
}

// rewriteTopLevelYAMLKeys applies rewrites to the YAML document src line
// by line, at the positions yaml.v3 reports, so comments, key order,
// quoting, and every other byte stay as written. It fails, writing
// nothing, on a key whose value is not a one-line scalar, and when the
// result would not decode to the same document apart from the rewrites.
func rewriteTopLevelYAMLKeys(src string, rewrites []yamlKeyRewrite) (string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		return "", err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode || doc.Content[0].Style&yaml.FlowStyle != 0 {
		return "", fmt.Errorf("not a block mapping")
	}
	mapping := doc.Content[0]
	lines := strings.SplitAfter(src, "\n")
	for _, rw := range rewrites {
		key, value := topLevelPair(mapping, rw.Key)
		if key == nil {
			return "", fmt.Errorf("no top-level key %q", rw.Key)
		}
		if newKey, _ := topLevelPair(mapping, rw.NewKey); newKey != nil {
			return "", fmt.Errorf("%s: is already set", rw.NewKey)
		}
		if key.Style != 0 || value.Kind != yaml.ScalarNode || value.Line != key.Line || value.Anchor != "" ||
			value.Style&(yaml.LiteralStyle|yaml.FoldedStyle|yaml.TaggedStyle) != 0 {
			return "", fmt.Errorf("%s: is not a plain key with a one-line value", rw.Key)
		}
		line := []rune(lines[key.Line-1])
		keyStart, valueStart := key.Column-1, value.Column-1
		keyEnd := keyStart + len([]rune(key.Value))
		if keyEnd > len(line) || string(line[keyStart:keyEnd]) != key.Value {
			return "", fmt.Errorf("%s: is not where the parser put it", rw.Key)
		}
		valueEnd, ok := scalarEnd(line, valueStart, value)
		if !ok {
			return "", fmt.Errorf("%s: is not a one-line value", rw.Key)
		}
		lines[key.Line-1] = string(line[:keyStart]) + rw.NewKey + string(line[keyEnd:valueStart]) + quoteLike(value.Style, rw.Value) + string(line[valueEnd:])
	}
	out := strings.Join(lines, "")
	if err := sameApartFrom(src, out, rewrites); err != nil {
		return "", err
	}
	return out, nil
}

func topLevelPair(mapping *yaml.Node, key string) (*yaml.Node, *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i], mapping.Content[i+1]
		}
	}
	return nil, nil
}

// scalarEnd returns the index just past the scalar that starts at start,
// or false when its text on the line does not spell its whole value.
func scalarEnd(line []rune, start int, value *yaml.Node) (int, bool) {
	var raw string
	switch value.Style {
	case 0:
		raw = value.Value
	case yaml.SingleQuotedStyle:
		raw = "'" + strings.ReplaceAll(value.Value, "'", "''") + "'"
	case yaml.DoubleQuotedStyle:
		for i := start + 1; i < len(line); i++ {
			switch line[i] {
			case '\\':
				i++
			case '"':
				return i + 1, true
			}
		}
		return 0, false
	default:
		return 0, false
	}
	end := start + len([]rune(raw))
	if end > len(line) || string(line[start:end]) != raw {
		return 0, false
	}
	return end, true
}

func quoteLike(style yaml.Style, value string) string {
	switch style {
	case yaml.SingleQuotedStyle:
		return "'" + strings.ReplaceAll(value, "'", "''") + "'"
	case yaml.DoubleQuotedStyle:
		return strconv.Quote(value)
	}
	return value
}

// sameApartFrom checks that after decodes to before with only the
// rewrites applied.
func sameApartFrom(before, after string, rewrites []yamlKeyRewrite) error {
	var old, next map[string]any
	if err := yaml.Unmarshal([]byte(before), &old); err != nil {
		return err
	}
	if err := yaml.Unmarshal([]byte(after), &next); err != nil {
		return fmt.Errorf("the rewrite does not parse: %w", err)
	}
	for _, rw := range rewrites {
		if next[rw.NewKey] != rw.Value {
			return fmt.Errorf("the rewrite reads %s: as %v, not %s", rw.NewKey, next[rw.NewKey], rw.Value)
		}
		delete(old, rw.Key)
		delete(next, rw.NewKey)
	}
	if !reflect.DeepEqual(old, next) {
		return fmt.Errorf("the rewrite changes other keys")
	}
	return nil
}
