package cli

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// fieldTargetAll is the target of an edit that sets a field for every tool.
const fieldTargetAll = "all"

// fieldDefaultEntry is the per-tool map entry that applies to every tool
// the map leaves out.
const fieldDefaultEntry = "default"

// fieldAddress is where a field edit lands inside a settings file: the
// value for every tool, or one tool's entry in the field's per-tool map.
type fieldAddress struct {
	key string
	// target is a tool name, or empty for every tool.
	target string
}

func newFieldAddress(key, target string) fieldAddress {
	if target == fieldTargetAll {
		target = ""
	}
	return fieldAddress{key: key, target: target}
}

func (a fieldAddress) String() string {
	if a.target == "" {
		return a.key
	}
	return a.key + "." + a.target
}

// label names the address in titles and messages.
func (a fieldAddress) label() string {
	if a.target == "" {
		return a.key
	}
	return a.key + " for " + a.target
}

// entry is the per-tool map entry the address writes once the field is a map.
func (a fieldAddress) entry() string {
	return cmp.Or(a.target, fieldDefaultEntry)
}

// in is the value at the address inside v, the value a file gives key.
func (a fieldAddress) in(v any, set bool) (any, bool) {
	if m, ok := v.(map[string]any); ok {
		x, ok := m[a.entry()]
		return x, ok
	}
	if a.target != "" {
		return nil, false
	}
	return v, set
}

// personalKey is the dotted key whose personal value hides this address
// from a team edit of e: the whole field, unless the personal file sets
// it as a per-tool map, which merges entry by entry.
func (a fieldAddress) personalKey(e spec.Entry) string {
	own := e.LocalMeta
	if e.Base == nil {
		own = e.Meta
	}
	if _, isMap := own[a.key].(map[string]any); isMap {
		return a.key + "." + a.entry()
	}
	return a.key
}

// next is the value key holds after the edit, given own, the value the
// file sets, and base, the team value a personal file merges over. A
// personal map merges over the team value entry by entry, and a team
// single value counts as its default. A personal single value replaces a
// team map whole, so it never gains tool entries. A map left with only
// its default reads back as that single value, unless the team value is
// a map it must merge into. keep is false when the key goes away.
func (a fieldAddress) next(own any, ownSet bool, base, value any) (next any, keep bool, err error) {
	m, ownMap := own.(map[string]any)
	_, baseMap := base.(map[string]any)
	switch {
	case ownMap:
		m = maps.Clone(m)
	case a.target == "" && (ownSet || !baseMap):
		if value == nil && !ownSet {
			return nil, false, errFieldNotInFile
		}
		return value, value != nil, nil
	case value == nil:
		return nil, false, errFieldNotInFile
	case own != nil && baseMap:
		return nil, false, fmt.Errorf("your personal file sets %s for every tool over the shared per-tool values; reset it before setting one tool", a.key)
	default:
		m = map[string]any{}
		if own != nil {
			m[fieldDefaultEntry] = own
		}
	}
	if value == nil {
		if _, ok := m[a.entry()]; !ok {
			return nil, false, errFieldNotInFile
		}
		delete(m, a.entry())
	} else {
		m[a.entry()] = value
	}
	if len(m) == 0 {
		return nil, false, nil
	}
	if d, ok := m[fieldDefaultEntry]; ok && len(m) == 1 && !baseMap {
		return d, true, nil
	}
	return m, true, nil
}

// setFieldAt writes value at addr through byte splices, keeping every
// other line. base is the team value a personal file merges over.
func setFieldAt(data []byte, addr fieldAddress, value, base any) ([]byte, error) {
	_, mapping, err := yamlDocument(data)
	if err != nil {
		return nil, err
	}
	if voiceEditField(addr.key) {
		return setVoiceFieldAt(data, mapping, addr.key, value)
	}
	var old map[string]any
	if err := yaml.Unmarshal(data, &old); err != nil {
		return nil, err
	}
	if old == nil {
		old = map[string]any{}
	}
	own, ownSet := old[addr.key]
	next, keep, err := addr.next(own, ownSet, base, value)
	if err != nil {
		return nil, err
	}
	ownMap, _ := own.(map[string]any)
	nextMap, _ := next.(map[string]any)
	if ownMap == nil && nextMap == nil {
		if !keep {
			next = nil
		}
		return setScalarField(data, addr.key, next)
	}
	if mapping.Anchor != "" || mapping.Style&yaml.FlowStyle != 0 {
		return nil, errors.New("flow-style or anchored settings are not safely editable")
	}
	at, node := yamlMapValue(mapping, addr.key)
	if node == nil && ownSet {
		return nil, errors.New("inherited settings are not safely editable")
	}
	var out []byte
	switch {
	case !keep:
		out, err = removeYAMLPath(data, addr.key)
	case ownMap != nil && nextMap != nil:
		out, err = setYAMLMapEntries(data, addr.key, ownMap, nextMap)
	case ownMap != nil:
		out, next, err = collapseYAMLField(data, addr.key, ownMap, next)
	case node == nil:
		out, err = editYAMLPair(data, mapping, addr.key, orderedFieldMap(nextMap))
	default:
		out, err = expandYAMLScalar(data, mapping.Content[at], node, nextMap)
	}
	if err != nil {
		return nil, err
	}
	expected := maps.Clone(old)
	if keep {
		expected[addr.key] = next
	} else {
		delete(expected, addr.key)
	}
	var actual map[string]any
	if err := yaml.Unmarshal(out, &actual); err != nil {
		return nil, fmt.Errorf("the edit does not parse: %w", err)
	}
	if actual == nil {
		actual = map[string]any{}
	}
	if !reflect.DeepEqual(expected, actual) {
		return nil, errors.New("the edit changes other values")
	}
	return out, nil
}

// orderedFieldMap is m as a YAML mapping with its default entry first.
func orderedFieldMap(m map[string]any) *yaml.Node {
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, k := range fieldMapKeys(m) {
		var v yaml.Node
		if err := v.Encode(m[k]); err != nil {
			panic(err)
		}
		node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k}, &v)
	}
	return node
}

func fieldMapKeys(m map[string]any) []string {
	keys := slices.Sorted(maps.Keys(m))
	if i := slices.Index(keys, fieldDefaultEntry); i > 0 {
		keys = append([]string{fieldDefaultEntry}, slices.Delete(keys, i, i+1)...)
	}
	return keys
}

// setYAMLMapEntries turns the per-tool map under key from into to, one
// entry at a time.
func setYAMLMapEntries(data []byte, key string, from, to map[string]any) ([]byte, error) {
	var err error
	for _, entry := range fieldMapKeys(from) {
		if _, kept := to[entry]; !kept {
			if data, err = removeYAMLPath(data, key+"."+entry); err != nil {
				return nil, err
			}
		}
	}
	for _, entry := range fieldMapKeys(to) {
		if old, ok := from[entry]; ok && reflect.DeepEqual(old, to[entry]) {
			continue
		}
		_, mapping, err := yamlDocument(data)
		if err != nil {
			return nil, err
		}
		_, parent := yamlMapValue(mapping, key)
		if parent == nil || parent.Kind != yaml.MappingNode || parent.Anchor != "" {
			return nil, fmt.Errorf("%s is not a plain mapping", key)
		}
		_, child := yamlMapValue(parent, entry)
		if child == nil {
			if data, err = editYAMLPair(data, parent, entry, to[entry]); err != nil {
				return nil, err
			}
			continue
		}
		if child.Kind != yaml.ScalarNode || child.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
			return nil, fmt.Errorf("%s.%s is not a one-line value", key, entry)
		}
		start, end, err := yamlScalarSpan(data, child)
		if err != nil {
			return nil, fmt.Errorf("%s.%s is not safely editable: %w", key, entry, err)
		}
		raw, err := yaml.Marshal(to[entry])
		if err != nil {
			return nil, err
		}
		data = spliceEditYAML(data, start, end, strings.TrimSpace(string(raw)))
	}
	return data, nil
}

// expandYAMLScalar replaces the one-line scalar under keyNode with the
// block mapping m, indented below the key. A comment after the scalar
// stays with it on the default line.
func expandYAMLScalar(data []byte, keyNode, value *yaml.Node, m map[string]any) ([]byte, error) {
	if value.Kind != yaml.ScalarNode || value.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
		return nil, errors.New("multi-line, anchored, or non-scalar settings are not safely editable")
	}
	_, keyEnd, err := yamlScalarSpan(data, keyNode)
	if err != nil {
		return nil, fmt.Errorf("field is not safely editable: %w", err)
	}
	start, end, err := yamlScalarSpan(data, value)
	if err != nil {
		return nil, fmt.Errorf("field is not safely editable: %w", err)
	}
	if strings.TrimSpace(string(data[keyEnd:start])) != ":" {
		return nil, errors.New("field is not safely editable")
	}
	doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{orderedFieldMap(m)}}
	raw, err := encodeYAML(doc)
	if err != nil {
		return nil, err
	}
	newline := "\n"
	if bytes.Contains(data, []byte("\r\n")) {
		newline = "\r\n"
	}
	lineEnd := end
	for lineEnd < len(data) && data[lineEnd] != '\n' {
		lineEnd++
	}
	if lineEnd > end && data[lineEnd-1] == '\r' {
		lineEnd--
	}
	tail := string(data[end:lineEnd])
	indent := strings.Repeat(" ", keyNode.Column-1+2)
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	for i, line := range lines {
		lines[i] = indent + line
	}
	head := ":"
	if _, ok := m[fieldDefaultEntry]; ok {
		lines[0] += tail
	} else {
		head += tail
	}
	return spliceEditYAML(data, keyEnd, lineEnd, head+newline+strings.Join(lines, newline)), nil
}

// collapseYAMLField turns the per-tool map under key into the single
// value v: it drops the other entries, then replaces the map left with
// only its default. A comment inside that map keeps the map form, and
// next is then the value the file holds.
func collapseYAMLField(data []byte, key string, own map[string]any, v any) (out []byte, next any, err error) {
	only := map[string]any{fieldDefaultEntry: v}
	if data, err = setYAMLMapEntries(data, key, own, only); err != nil {
		return nil, nil, err
	}
	_, mapping, err := yamlDocument(data)
	if err != nil {
		return nil, nil, err
	}
	at, node := yamlMapValue(mapping, key)
	if node == nil {
		return nil, nil, fmt.Errorf("%s is not in the file", key)
	}
	if out, err := collapseYAMLMap(data, mapping.Content[at], node, v); err == nil {
		return out, v, nil
	}
	return data, only, nil
}

// collapseYAMLMap replaces the mapping under keyNode with the scalar v,
// unless a comment inside it would be lost.
func collapseYAMLMap(data []byte, keyNode, value *yaml.Node, v any) ([]byte, error) {
	_, keyEnd, err := yamlScalarSpan(data, keyNode)
	if err != nil {
		return nil, err
	}
	end, err := yamlCollectionEnd(data, value)
	if err != nil {
		return nil, err
	}
	if bytes.ContainsRune(data[keyEnd:end], '#') {
		return nil, errors.New("the map has comments")
	}
	raw, err := yaml.Marshal(v)
	if err != nil {
		return nil, err
	}
	return spliceEditYAML(data, keyEnd, end, ": "+strings.TrimSpace(string(raw))), nil
}
