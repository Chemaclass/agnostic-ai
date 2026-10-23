package cli

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

func yamlNodeOffset(data []byte, node *yaml.Node) (int, error) {
	line, column := 1, 1
	offset := 0
	if bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
		offset = 3
	}
	for i, r := range string(data[offset:]) {
		if line == node.Line && column == node.Column {
			return offset + i, nil
		}
		if r == '\n' {
			line++
			column = 1
		} else {
			column++
		}
	}
	return 0, errors.New("the value is not where the parser put it")
}

func yamlScalarSpan(data []byte, node *yaml.Node) (int, int, error) {
	if node.Kind != yaml.ScalarNode || node.Anchor != "" {
		return 0, 0, errors.New("the value is not a plain scalar")
	}
	start, err := yamlNodeOffset(data, node)
	if err != nil {
		return 0, 0, err
	}
	line := string(data[start:])
	if at := strings.IndexByte(line, '\n'); at >= 0 {
		line = line[:at]
	}
	end, ok := scalarEnd([]rune(line), 0, node)
	if !ok {
		return 0, 0, errors.New("the value is not a one-line scalar")
	}
	return start, start + len(string([]rune(line)[:end])), nil
}

func spliceEditYAML(data []byte, start, end int, text string) []byte {
	out := make([]byte, 0, len(data)+len(text)-(end-start))
	out = append(out, data[:start]...)
	out = append(out, text...)
	out = append(out, data[end:]...)
	return out
}

func yamlCollectionEnd(data []byte, node *yaml.Node) (int, error) {
	start, err := yamlNodeOffset(data, node)
	if err != nil {
		return 0, err
	}
	if node.Style&yaml.FlowStyle != 0 {
		var quote byte
		depth := 0
		comment := false
		for i := start; i < len(data); i++ {
			c := data[i]
			if comment {
				if c == '\n' {
					comment = false
				}
				continue
			}
			if quote != 0 {
				if quote == '"' && c == '\\' {
					i++
					continue
				}
				if c == quote {
					if quote == '\'' && i+1 < len(data) && data[i+1] == '\'' {
						i++
						continue
					}
					quote = 0
				}
				continue
			}
			switch c {
			case '\'', '"':
				quote = c
			case '#':
				comment = true
			case '[', '{':
				depth++
			case ']', '}':
				depth--
				if depth == 0 {
					return i + 1, nil
				}
			}
		}
		return 0, errors.New("the collection is not closed")
	}
	last := node
	for len(last.Content) > 0 {
		last = last.Content[len(last.Content)-1]
	}
	if last.Kind == yaml.ScalarNode {
		if last.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
			start, err := yamlNodeOffset(data, last)
			if err != nil {
				return 0, err
			}
			lineStart := bytes.LastIndexByte(data[:start], '\n') + 1
			indent := 0
			for lineStart+indent < len(data) && data[lineStart+indent] == ' ' {
				indent++
			}
			at := start
			for at < len(data) && data[at] != '\n' {
				at++
			}
			if at < len(data) {
				at++
			}
			for at < len(data) {
				end := at
				for end < len(data) && data[end] != '\n' {
					end++
				}
				line := strings.TrimSuffix(string(data[at:end]), "\r")
				spaces := len(line) - len(strings.TrimLeft(line, " "))
				if strings.TrimSpace(line) != "" && spaces <= indent {
					break
				}
				at = end
				if at < len(data) {
					at++
				}
			}
			return at, nil
		}
		_, end, err := yamlScalarSpan(data, last)
		if err == nil {
			return end, nil
		}
	}
	return 0, errors.New("the collection has no editable end")
}

func editYAMLPair(data []byte, mapping *yaml.Node, key string, value any) ([]byte, error) {
	if mapping.Kind != yaml.MappingNode || mapping.Anchor != "" {
		return nil, errors.New("the field is not in a plain mapping")
	}
	var fragment yaml.Node
	if err := fragment.Encode(map[string]any{key: value}); err != nil {
		return nil, err
	}
	raw, err := encodeYAML(&fragment)
	if err != nil {
		return nil, err
	}
	if mapping.Style&yaml.FlowStyle != 0 {
		end, err := yamlCollectionEnd(data, mapping)
		if err != nil {
			return nil, err
		}
		val, err := yaml.Marshal(value)
		if err != nil {
			return nil, err
		}
		var node yaml.Node
		if err := yaml.Unmarshal(val, &node); err != nil {
			return nil, err
		}
		node.Content[0].Style |= yaml.FlowStyle
		val, err = yaml.Marshal(node.Content[0])
		if err != nil {
			return nil, err
		}
		text := strconv.Quote(key) + ": " + strings.TrimSpace(string(val))
		if len(mapping.Content) > 0 {
			lastEnd, err := yamlCollectionEnd(data, mapping.Content[len(mapping.Content)-1])
			if err != nil {
				return nil, err
			}
			if yamlEditComma(data[lastEnd:end-1]) >= 0 {
				text += ","
				if data[end-2] == ',' {
					text = " " + text
				}
			} else {
				text = ", " + text
			}
		}
		return spliceEditYAML(data, end-1, end-1, text), nil
	}
	indent := 0
	at := len(data)
	if len(mapping.Content) > 0 {
		indent = mapping.Content[0].Column - 1
		last := mapping.Content[len(mapping.Content)-1]
		end, err := yamlCollectionEnd(data, last)

		if err != nil {
			return nil, err
		}
		at = end
		if at == 0 || data[at-1] != '\n' {
			for at < len(data) && data[at] != '\n' {
				at++
			}
			if at < len(data) {
				at++
			}
		}
	}
	newline := "\n"
	if bytes.Contains(data, []byte("\r\n")) {
		newline = "\r\n"
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	for i, line := range lines {
		lines[i] = strings.Repeat(" ", indent) + line
	}
	text := strings.Join(lines, newline) + newline
	if at > 0 && data[at-1] != '\n' {
		text = newline + text
	}
	return spliceEditYAML(data, at, at, text), nil
}

func boolAtYAML(node *yaml.Node, parts []string) (*yaml.Node, error) {
	for _, part := range parts {
		if node.Kind != yaml.MappingNode || node.Anchor != "" {
			return nil, fmt.Errorf("%s is not a field of the file", strings.Join(parts, "."))
		}
		var err error
		node, err = onlyValue(node, part)
		if err != nil {
			return nil, err
		}
	}
	if node.Kind != yaml.ScalarNode || node.ShortTag() != "!!bool" || node.Anchor != "" {
		return nil, fmt.Errorf("%s is not a boolean", strings.Join(parts, "."))
	}
	return node, nil
}

func setBool(data []byte, key string, value bool) ([]byte, error) {
	_, mapping, err := yamlDocument(data)
	if err != nil {
		return nil, err
	}
	node, err := boolAtYAML(mapping, strings.Split(key, "."))
	if err != nil {
		return nil, err
	}
	start, end, err := yamlScalarSpan(data, node)
	if err != nil {
		return nil, err
	}
	text := strconv.FormatBool(value)
	if node.Value == strings.ToUpper(node.Value) {
		text = strings.ToUpper(text)
	} else if len(node.Value) > 0 && unicode.IsUpper(rune(node.Value[0])) {
		text = strings.ToUpper(text[:1]) + text[1:]
	}
	out := spliceEditYAML(data, start, end, text)
	return checkBoolEdit(data, out, key, value, false)
}

func setBoolOverride(data []byte, key string, value bool) ([]byte, error) {
	_, node, err := yamlDocument(data)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(key, ".")
	for i, part := range parts {
		if node.Kind != yaml.MappingNode || node.Anchor != "" {
			return nil, fmt.Errorf("%s is not a field of the file", key)
		}
		_, next := yamlMapValue(node, part)
		if next == nil {
			var nested any = value
			for j := len(parts) - 1; j > i; j-- {
				nested = map[string]any{parts[j]: nested}
			}
			out, err := editYAMLPair(data, node, part, nested)
			if err != nil {
				return nil, err
			}
			return checkBoolEdit(data, out, key, value, true)
		}
		node = next
	}
	return setBool(data, key, value)
}

func checkBoolEdit(before, after []byte, key string, value bool, allowNew bool) ([]byte, error) {
	var old, next map[string]any
	if err := yaml.Unmarshal(before, &old); err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(after, &next); err != nil {
		return nil, fmt.Errorf("the edit does not parse: %w", err)
	}
	if old == nil {
		old = map[string]any{}
	}
	path := strings.Split(key, ".")
	current := old
	for _, part := range path[:len(path)-1] {
		child, ok := current[part].(map[string]any)
		if !ok {
			if !allowNew {
				return nil, fmt.Errorf("%s is not a field", key)
			}
			child = map[string]any{}
			current[part] = child
		}
		current = child
	}
	current[path[len(path)-1]] = value
	if !reflect.DeepEqual(old, next) {
		return nil, errors.New("the edit changes other values")
	}
	return after, nil
}

func setBuiltin(data []byte, name string, enabled bool) ([]byte, error) {
	_, mapping, err := yamlDocument(data)
	if err != nil {
		return nil, err
	}
	_, list := yamlMapValue(mapping, "builtins")
	if list == nil {
		var values map[string]any
		if err := yaml.Unmarshal(data, &values); err != nil {
			return nil, err
		}
		if _, inherited := values["builtins"]; inherited {
			return nil, errors.New("builtins is inherited from a YAML merge; write an explicit list before editing it")
		}
		if !enabled {
			return data, nil
		}
		out, err := editYAMLPair(data, mapping, "builtins", []string{name})
		if err != nil {
			return nil, err
		}
		return checkBuiltinEdit(data, out, []string{name})
	}
	if list.Kind != yaml.SequenceNode || list.Anchor != "" || list.Tag != "!!seq" {
		return nil, errors.New("builtins is not a plain list")
	}
	var names []string
	for _, item := range list.Content {
		if item.Kind != yaml.ScalarNode || item.Tag != "!!str" {
			return nil, errors.New("builtins contains a non-string")
		}
		names = append(names, item.Value)
	}
	index := slices.Index(names, name)
	if enabled == (index >= 0) {
		return data, nil
	}
	if list.Style&yaml.FlowStyle != 0 {
		end, err := yamlCollectionEnd(data, list)
		if err != nil {
			return nil, err
		}
		if enabled {
			at := end - 1
			text := name
			if len(list.Content) > 0 {
				_, at, err = yamlScalarSpan(data, list.Content[len(list.Content)-1])
				text = ", " + name
			}
			if err != nil {
				return nil, err
			}
			out := spliceEditYAML(data, at, at, text)
			return checkBuiltinEdit(data, out, append(names, name))
		}
		start, finish, err := yamlScalarSpan(data, list.Content[index])
		if err != nil {
			return nil, err
		}
		comma := -1
		left, right := finish, end-1
		if index > 0 {
			_, left, err = yamlScalarSpan(data, list.Content[index-1])
			right = start
		} else if len(list.Content) > 1 {
			right, _, err = yamlScalarSpan(data, list.Content[1])
		}
		if err != nil {
			return nil, err
		}
		if found := yamlEditComma(data[left:right]); found >= 0 {
			comma = left + found
		}
		if len(list.Content) > 1 && comma < 0 {
			return nil, errors.New("builtins: missing item separator")
		}
		if comma >= 0 && !bytes.ContainsAny(data[left:right], "#\r\n") {
			if index > 0 {
				start = left
			} else {
				finish = right
			}
			comma = -1
		}
		var out []byte
		if comma >= finish {
			out = spliceEditYAML(data, comma, comma+1, "")
			out = spliceEditYAML(out, start, finish, "")
		} else {
			out = spliceEditYAML(data, start, finish, "")
			if comma >= 0 {
				out = spliceEditYAML(out, comma, comma+1, "")
			}
		}
		return checkBuiltinEdit(data, out, slices.Delete(names, index, index+1))
	}
	if len(list.Content) == 0 {
		return nil, errors.New("builtins: an empty block list is not editable")
	}
	if enabled {
		last := list.Content[len(list.Content)-1]
		start, at, err := yamlScalarSpan(data, last)
		if err != nil {
			return nil, err
		}
		for at < len(data) && data[at] != '\n' {
			at++
		}
		if at < len(data) {
			at++
		}
		newline := "\n"
		if bytes.Contains(data, []byte("\r\n")) {
			newline = "\r\n"
		}
		lineStart := bytes.LastIndexByte(data[:start], '\n') + 1
		prefix := string(data[lineStart:start])
		if strings.TrimSpace(prefix) != "-" {
			return nil, errors.New("builtins: item is not on its own line")
		}
		text := prefix + name + newline
		if at > 0 && data[at-1] != '\n' {
			text = newline + text
		}
		return checkBuiltinEdit(data, spliceEditYAML(data, at, at, text), append(names, name))
	}
	item := list.Content[index]
	start, end, err := yamlScalarSpan(data, item)
	if err != nil {
		return nil, err
	}
	lineStart := bytes.LastIndexByte(data[:start], '\n') + 1
	if strings.TrimSpace(string(data[lineStart:start])) != "-" {
		return nil, errors.New("builtins: item is not on its own line")
	}
	lineEnd := end
	for lineEnd < len(data) && data[lineEnd] != '\n' {
		lineEnd++
	}
	tail := data[end:lineEnd]
	replacement := ""
	if comment := bytes.IndexByte(tail, '#'); comment >= 0 {
		newline := "\n"
		if bytes.HasSuffix(tail, []byte("\r")) {
			newline = "\r\n"
		}
		prefix := string(data[lineStart:start])
		replacement = prefix[:strings.IndexByte(prefix, '-')] + strings.TrimSuffix(string(tail[comment:]), "\r") + newline
	}
	if lineEnd < len(data) {
		lineEnd++
	}
	out := spliceEditYAML(data, lineStart, lineEnd, replacement)
	if len(list.Content) == 1 {
		key, _ := topLevelPair(mapping, "builtins")
		at, err := yamlNodeOffset(out, key)
		if err != nil {
			return nil, err
		}
		colon := bytes.IndexByte(out[at:], ':')
		if colon < 0 {
			return nil, errors.New("builtins: missing colon")
		}
		at += colon + 1
		out = spliceEditYAML(out, at, at, " []")
	}
	return checkBuiltinEdit(data, out, slices.Delete(names, index, index+1))
}

func checkBuiltinEdit(before, after []byte, names []string) ([]byte, error) {
	var old, next map[string]any
	if err := yaml.Unmarshal(before, &old); err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(after, &next); err != nil {
		return nil, fmt.Errorf("the edit does not parse: %w", err)
	}
	if old == nil {
		old = map[string]any{}
	}
	list := make([]any, len(names))
	for i, name := range names {
		list[i] = name
	}
	old["builtins"] = list
	if !reflect.DeepEqual(old, next) {
		return nil, errors.New("the edit changes other values")
	}
	return after, nil
}

func yamlEditComma(data []byte) int {
	comment := false
	for i, c := range data {
		if c == '\n' {
			comment = false
			continue
		}
		if comment {
			continue
		}
		if c == '#' {
			comment = true
			continue
		}
		if c == ',' {
			return i
		}
	}
	return -1
}
