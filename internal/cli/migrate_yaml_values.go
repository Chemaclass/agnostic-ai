package cli

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// yamlValueEdit changes the value of Key in the top-level mapping Field.
// A non-empty Tag goes in front of the value; otherwise Value replaces
// it, in the quote style the old value used.
type yamlValueEdit struct {
	Field, Key, Tag, Value string
}

type yamlSplice struct {
	line, start, end int
	text             string
}

// editNestedYAMLValues applies edits to the YAML document src at the
// positions yaml.v3 reports, so comments, key order, quoting, and every
// other byte stay as written. It fails, writing nothing, on a value that
// is not an untagged string of its own, a replaced value that is not on
// one line, and a result that does not decode to the same document and
// tags apart from the edits. Errors name the field and key, never the
// value.
func editNestedYAMLValues(src string, edits []yamlValueEdit) (string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		return "", err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return "", fmt.Errorf("not a mapping")
	}
	lines := strings.SplitAfter(src, "\n")
	var splices []yamlSplice
	for _, ed := range edits {
		values, err := onlyValue(doc.Content[0], ed.Field)
		if err != nil {
			return "", err
		}
		if values.Kind != yaml.MappingNode {
			return "", fmt.Errorf("%s: is not a mapping", ed.Field)
		}
		value, err := onlyValue(values, ed.Key)
		if err != nil {
			return "", fmt.Errorf("%s %w", ed.Field, err)
		}
		if value.Kind != yaml.ScalarNode || value.ShortTag() != "!!str" || value.Anchor != "" || value.Style&yaml.TaggedStyle != 0 {
			return "", fmt.Errorf("%s %s: is not an untagged string", ed.Field, ed.Key)
		}
		line := []rune(lines[value.Line-1])
		start := value.Column - 1
		if start > len(line) {
			return "", fmt.Errorf("%s %s: is not where the parser put it", ed.Field, ed.Key)
		}
		if ed.Tag != "" {
			splices = append(splices, yamlSplice{line: value.Line - 1, start: start, end: start, text: ed.Tag + " "})
			continue
		}
		end, ok := scalarEnd(line, start, value)
		if !ok {
			return "", fmt.Errorf("%s %s: is not a one-line value", ed.Field, ed.Key)
		}
		text := quoteLike(value.Style, ed.Value)
		// A plain scalar in a flow mapping cannot hold `{` or `}`.
		if value.Style == 0 && values.Style&yaml.FlowStyle != 0 {
			text = strconv.Quote(ed.Value)
		}
		splices = append(splices, yamlSplice{line: value.Line - 1, start: start, end: end, text: text})
	}
	slices.SortFunc(splices, func(a, b yamlSplice) int {
		if a.line != b.line {
			return b.line - a.line
		}
		return b.start - a.start
	})
	for _, s := range splices {
		line := []rune(lines[s.line])
		lines[s.line] = string(line[:s.start]) + s.text + string(line[s.end:])
	}
	out := strings.Join(lines, "")
	if err := sameApartFromValues(src, out, edits); err != nil {
		return "", err
	}
	return out, nil
}

// onlyValue returns the value of key in mapping, failing when key is
// missing or set more than once.
func onlyValue(mapping *yaml.Node, key string) (*yaml.Node, error) {
	var value *yaml.Node
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value != key {
			continue
		}
		if value != nil {
			return nil, fmt.Errorf("%s: is set twice", key)
		}
		value = mapping.Content[i+1]
	}
	if value == nil {
		return nil, fmt.Errorf("%s: is not set", key)
	}
	return value, nil
}

// sameApartFromValues checks that after decodes to before with only the
// edits applied, and that every nested value but the tagged ones keeps
// its tag.
func sameApartFromValues(before, after string, edits []yamlValueEdit) error {
	var old, next map[string]any
	if err := yaml.Unmarshal([]byte(before), &old); err != nil {
		return err
	}
	if err := yaml.Unmarshal([]byte(after), &next); err != nil {
		return fmt.Errorf("the rewrite does not parse: %w", err)
	}
	oldTags, err := nestedValueTags(before)
	if err != nil {
		return err
	}
	nextTags, err := nestedValueTags(after)
	if err != nil {
		return err
	}
	for _, ed := range edits {
		if ed.Tag != "" {
			oldTags[[2]string{ed.Field, ed.Key}] = ed.Tag
			continue
		}
		values, ok := old[ed.Field].(map[string]any)
		if !ok {
			return fmt.Errorf("%s: is not a mapping", ed.Field)
		}
		values[ed.Key] = ed.Value
	}
	if !reflect.DeepEqual(old, next) {
		return fmt.Errorf("the rewrite changes other values")
	}
	if !maps.Equal(oldTags, nextTags) {
		return fmt.Errorf("the rewrite changes other tags")
	}
	return nil
}

// nestedValueTags maps each value of a top-level mapping to its tag.
func nestedValueTags(src string) (map[[2]string]string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		return nil, err
	}
	tags := map[[2]string]string{}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return tags, nil
	}
	top := doc.Content[0].Content
	for i := 0; i+1 < len(top); i += 2 {
		if top[i+1].Kind != yaml.MappingNode {
			continue
		}
		nested := top[i+1].Content
		for j := 0; j+1 < len(nested); j += 2 {
			tags[[2]string{top[i].Value, nested[j].Value}] = nested[j+1].Tag
		}
	}
	return tags, nil
}
