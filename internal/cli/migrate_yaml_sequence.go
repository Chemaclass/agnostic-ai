package cli

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// yamlSequenceRewrite renames the top-level key Key to NewKey, whose
// value is a list, and replaces the items Items holds by index, each in
// the quote style it had.
type yamlSequenceRewrite struct {
	Key, NewKey string
	Items       map[int]string
}

// rewriteTopLevelYAMLSequence applies rw to the YAML document src at the
// positions yaml.v3 reports, so comments, key order, quoting, flow or
// block style, and every other byte stay as written. It fails, writing
// nothing, on a list item that is not a one-line scalar, and when the
// result would not decode to the same document apart from the rewrite.
func rewriteTopLevelYAMLSequence(src string, rw yamlSequenceRewrite) (string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		return "", err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode || doc.Content[0].Style&yaml.FlowStyle != 0 {
		return "", fmt.Errorf("not a block mapping")
	}
	mapping := doc.Content[0]
	key, value := topLevelPair(mapping, rw.Key)
	if key == nil {
		return "", fmt.Errorf("no top-level key %q", rw.Key)
	}
	if newKey, _ := topLevelPair(mapping, rw.NewKey); newKey != nil && rw.NewKey != rw.Key {
		return "", fmt.Errorf("%s: is already set", rw.NewKey)
	}
	if key.Style != 0 || value.Kind != yaml.SequenceNode || value.Anchor != "" || value.Tag != "!!seq" {
		return "", fmt.Errorf("%s: is not a plain key with a list value", rw.Key)
	}
	lines := strings.SplitAfter(src, "\n")
	keyStart := key.Column - 1
	keyEnd := keyStart + len([]rune(key.Value))
	if line := []rune(lines[key.Line-1]); keyEnd > len(line) || string(line[keyStart:keyEnd]) != key.Value {
		return "", fmt.Errorf("%s: is not where the parser put it", rw.Key)
	}
	type edit struct {
		line, start, end int
		text             string
	}
	edits := []edit{{key.Line, keyStart, keyEnd, rw.NewKey}}
	for i, item := range value.Content {
		next, rewrite := rw.Items[i]
		if !rewrite {
			continue
		}
		if item.Kind != yaml.ScalarNode || item.Anchor != "" || item.Style&(yaml.LiteralStyle|yaml.FoldedStyle|yaml.TaggedStyle) != 0 {
			return "", fmt.Errorf("%s: item %d is not a one-line value", rw.Key, i+1)
		}
		end, ok := scalarEnd([]rune(lines[item.Line-1]), item.Column-1, item)
		if !ok {
			return "", fmt.Errorf("%s: item %d is not a one-line value", rw.Key, i+1)
		}
		edits = append(edits, edit{item.Line, item.Column - 1, end, quoteLike(item.Style, next)})
	}
	// Right to left, so an edit never moves the columns of the next one.
	slices.SortFunc(edits, func(a, b edit) int {
		if a.line != b.line {
			return b.line - a.line
		}
		return b.start - a.start
	})
	for _, e := range edits {
		line := []rune(lines[e.line-1])
		lines[e.line-1] = string(line[:e.start]) + e.text + string(line[e.end:])
	}
	out := strings.Join(lines, "")
	if err := sameSequenceApartFrom(src, out, rw); err != nil {
		return "", err
	}
	return out, nil
}

// sameSequenceApartFrom checks that after decodes to before with only
// rw applied.
func sameSequenceApartFrom(before, after string, rw yamlSequenceRewrite) error {
	var old, next map[string]any
	if err := yaml.Unmarshal([]byte(before), &old); err != nil {
		return err
	}
	if err := yaml.Unmarshal([]byte(after), &next); err != nil {
		return fmt.Errorf("the rewrite does not parse: %w", err)
	}
	want, _ := old[rw.Key].([]any)
	want = slices.Clone(want)
	for i, v := range rw.Items {
		if i < len(want) {
			want[i] = v
		}
	}
	if !reflect.DeepEqual(next[rw.NewKey], want) {
		return fmt.Errorf("the rewrite reads %s: as %v, not %v", rw.NewKey, next[rw.NewKey], want)
	}
	delete(old, rw.Key)
	delete(next, rw.NewKey)
	if !reflect.DeepEqual(old, next) {
		return fmt.Errorf("the rewrite changes other keys")
	}
	return nil
}

// rewriteFrontmatter applies rewrite to the YAML frontmatter of the
// Markdown document src, found the way the spec loader finds it, and
// keeps the delimiters and the body byte for byte.
func rewriteFrontmatter(src string, rewrite func(string) (string, error)) (string, error) {
	const delim = "---"
	rest, ok := strings.CutPrefix(src, delim+"\n")
	if !ok {
		return "", fmt.Errorf("no frontmatter")
	}
	end := strings.Index("\n"+rest, "\n"+delim)
	if end < 0 {
		return "", fmt.Errorf("no frontmatter")
	}
	front, err := rewrite(rest[:end])
	if err != nil {
		return "", err
	}
	return delim + "\n" + front + rest[end:], nil
}

// rewriteYAMLSequenceItems replaces the items of the list at path, a
// chain of block mapping keys from the top, that items holds by index,
// each in the quote style it had, and leaves every other byte as
// written. It fails, writing nothing, when the result would not decode
// to the same document apart from those items.
func rewriteYAMLSequenceItems(src string, path []string, items map[int]string) (string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		return "", err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
		return "", fmt.Errorf("not a block mapping")
	}
	node := doc.Content[0]
	for _, key := range path {
		if node.Kind != yaml.MappingNode || node.Style&yaml.FlowStyle != 0 || node.Anchor != "" {
			return "", fmt.Errorf("%s: is not in a block mapping", strings.Join(path, "."))
		}
		_, value := topLevelPair(node, key)
		if value == nil {
			return "", fmt.Errorf("no key %q", strings.Join(path, "."))
		}
		node = value
	}
	if node.Kind != yaml.SequenceNode || node.Anchor != "" || node.Tag != "!!seq" {
		return "", fmt.Errorf("%s: is not a list", strings.Join(path, "."))
	}
	lines := strings.SplitAfter(src, "\n")
	type edit struct {
		line, start, end int
		text             string
	}
	var edits []edit
	for i, item := range node.Content {
		next, rewrite := items[i]
		if !rewrite {
			continue
		}
		if item.Kind != yaml.ScalarNode || item.Anchor != "" || item.Style&(yaml.LiteralStyle|yaml.FoldedStyle|yaml.TaggedStyle) != 0 {
			return "", fmt.Errorf("%s: item %d is not a one-line value", strings.Join(path, "."), i+1)
		}
		end, ok := scalarEnd([]rune(lines[item.Line-1]), item.Column-1, item)
		if !ok {
			return "", fmt.Errorf("%s: item %d is not a one-line value", strings.Join(path, "."), i+1)
		}
		edits = append(edits, edit{item.Line, item.Column - 1, end, quoteLike(item.Style, next)})
	}
	slices.SortFunc(edits, func(a, b edit) int {
		if a.line != b.line {
			return b.line - a.line
		}
		return b.start - a.start
	})
	for _, e := range edits {
		line := []rune(lines[e.line-1])
		lines[e.line-1] = string(line[:e.start]) + e.text + string(line[e.end:])
	}
	out := strings.Join(lines, "")
	var before, after any
	if err := yaml.Unmarshal([]byte(src), &before); err != nil {
		return "", err
	}
	if err := yaml.Unmarshal([]byte(out), &after); err != nil {
		return "", fmt.Errorf("the rewrite does not parse: %w", err)
	}
	want, err := withSequenceItems(before, path, items)
	if err != nil {
		return "", err
	}
	if !reflect.DeepEqual(after, want) {
		return "", fmt.Errorf("the rewrite changes more than %s", strings.Join(path, "."))
	}
	return out, nil
}

// withSequenceItems returns doc with the items of the list at path
// replaced, without changing doc.
func withSequenceItems(doc any, path []string, items map[int]string) (any, error) {
	if len(path) == 0 {
		list, ok := doc.([]any)
		if !ok {
			return nil, fmt.Errorf("not a list")
		}
		list = slices.Clone(list)
		for i, v := range items {
			if i < len(list) {
				list[i] = v
			}
		}
		return list, nil
	}
	m, ok := doc.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("not a mapping")
	}
	inner, err := withSequenceItems(m[path[0]], path[1:], items)
	if err != nil {
		return nil, err
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	out[path[0]] = inner
	return out, nil
}
