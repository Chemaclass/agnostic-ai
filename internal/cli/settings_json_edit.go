package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

// editJSONRoot sets and removes keys in the text itself, so formatting,
// comments, and every other key stay byte for byte. A key is a dotted
// path into nested objects, such as "model.name"; a missing parent is
// created with it and removed again once empty. A new member goes after
// the last one, and removing it undoes that exactly.
func editJSONRoot(path string, data []byte, order []string, set map[string]any, remove []string) ([]byte, error) {
	text := strings.TrimRight(string(data), " \t\r\n")
	trailing := string(data)[len(text):]
	if text == "" {
		text, trailing = "{}", "\n"
	}
	unit := adapters.DetectJSONIndent(data)
	for _, key := range remove {
		open, err := jsonRootOpen(text)
		if err == nil {
			text, err = removeJSONPath(text, open, strings.Split(key, "."))
		}
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
	}
	for _, key := range order {
		open, err := jsonRootOpen(text)
		if err == nil {
			text, err = setJSONPath(text, open, strings.Split(key, "."), set[key], unit)
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %s: %w", path, key, err)
		}
	}
	return []byte(text + trailing), nil
}

// jsonRootOpen is the offset of the top-level object's opening brace.
func jsonRootOpen(text string) (int, error) {
	i := skipJSONSpace(text, 0)
	if i >= len(text) || text[i] != '{' {
		return 0, fmt.Errorf("expected a JSON object")
	}
	return i, nil
}

// skipJSONSpace returns the first offset from i that is neither
// whitespace nor a JSONC comment.
func skipJSONSpace(text string, i int) int {
	for i < len(text) {
		switch {
		case strings.ContainsRune(" \t\r\n", rune(text[i])):
			i++
		case strings.HasPrefix(text[i:], "//"):
			nl := strings.IndexByte(text[i:], '\n')
			if nl < 0 {
				return len(text)
			}
			i += nl + 1
		case strings.HasPrefix(text[i:], "/*"):
			end := strings.Index(text[i+2:], "*/")
			if end < 0 {
				return len(text)
			}
			i += end + 4
		default:
			return i
		}
	}
	return i
}

func setJSONPath(text string, open int, path []string, value any, unit string) (string, error) {
	members, closing, err := scanJSONObject(text, open)
	if err != nil {
		return "", err
	}
	// JSON decoding keeps the last of duplicate keys, so edit that one.
	i := lastJSONMember(members, path[0])
	if len(path) == 1 {
		if i >= 0 {
			m := members[i]
			return text[:m.valueStart] + jsonValueText(value, lineIndent(text, m.start), unit) + text[m.end:], nil
		}
		return insertJSONMember(text, open, closing, members, path[0], value, unit), nil
	}
	if i >= 0 {
		m := members[i]
		if text[m.valueStart] != '{' {
			return "", fmt.Errorf("%s holds a value that is not an object", path[0])
		}
		return setJSONPath(text, m.valueStart, path[1:], value, unit)
	}
	nested := value
	for j := len(path) - 1; j >= 1; j-- {
		nested = map[string]any{path[j]: nested}
	}
	return insertJSONMember(text, open, closing, members, path[0], nested, unit), nil
}

func removeJSONPath(text string, open int, path []string) (string, error) {
	members, closing, err := scanJSONObject(text, open)
	if err != nil {
		return "", err
	}
	i := lastJSONMember(members, path[0])
	if i < 0 {
		return text, nil
	}
	if len(path) > 1 {
		m := members[i]
		if text[m.valueStart] != '{' {
			return text, nil
		}
		if text, err = removeJSONPath(text, m.valueStart, path[1:]); err != nil {
			return "", err
		}
		// A parent the path left empty goes too, which undoes creating it.
		inner, innerClose, err := scanJSONObject(text, m.valueStart)
		if err != nil {
			return "", err
		}
		if len(inner) > 0 || strings.TrimSpace(text[m.valueStart+1:innerClose]) != "" {
			return text, nil
		}
		if members, closing, err = scanJSONObject(text, open); err != nil {
			return "", err
		}
	}
	return removeJSONMember(text, open, closing, members, i), nil
}

func removeJSONMember(text string, open, closing int, members []jsonMember, i int) string {
	m := members[i]
	switch {
	case i > 0:
		return text[:m.comma] + text[m.comma+1:keptBefore(text, m.comma+1, m.start)] + text[m.end:]
	case len(members) > 1:
		// Drop the comma and the whitespace after it, keeping a comment
		// that sits above the next member.
		j := members[1].comma + 1
		for j < members[1].start && strings.ContainsRune(" \t\r\n", rune(text[j])) {
			j++
		}
		return text[:m.start] + text[j:]
	case strings.TrimSpace(text[open+1:m.start]) == "":
		return text[:open+1] + text[closing:]
	default:
		return text[:keptBefore(text, open+1, m.start)] + text[m.end:]
	}
}

func insertJSONMember(text string, open, closing int, members []jsonMember, key string, value any, unit string) string {
	return insertJSONEntry(text, open, closing, members, jsonString(key)+": ", value, unit)
}

// insertJSONEntry adds label and value after the last entry of the object
// or array opening at open. An array element has an empty label.
func insertJSONEntry(text string, open, closing int, members []jsonMember, label string, value any, unit string) string {
	if len(members) == 0 {
		outer := lineIndent(text, open)
		indent := outer + unit
		member := label + jsonValueText(value, indent, unit)
		return text[:open+1] + "\n" + indent + member + "\n" + outer + text[closing:]
	}
	last := members[len(members)-1].end
	first := members[0].start
	indent := text[strings.LastIndex(text[:first], "\n")+1 : first]
	if strings.TrimSpace(indent) != "" {
		// Members on the brace's own line: stay inline.
		return text[:last] + ", " + label + jsonValueText(value, "", "") + text[last:]
	}
	member := label + jsonValueText(value, indent, unit)
	return text[:last] + ",\n" + indent + member + text[last:]
}

// editJSONArray sets the array at a dotted key path to items, editing
// the text in place: an element that stays keeps its bytes, the ones
// gone are cut, and new ones go after the last. Items are matched in
// order, so appending and then removing undoes the append exactly. A
// key that holds no array yet is set whole.
func editJSONArray(path string, data []byte, key string, items []any) ([]byte, error) {
	text := string(data)
	open, err := jsonRootOpen(text)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	start, err := findJSONPath(text, open, strings.Split(key, "."))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if start < 0 || text[start] != '[' {
		return editJSONRoot(path, data, []string{key}, map[string]any{key: items}, nil)
	}
	want, _ := jsonRoundTrip(items).([]any)
	elements, _, err := scanJSONArray(text, start)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	var drop []int
	kept := 0
	for i, element := range elements {
		var have any
		if err := json.Unmarshal([]byte(text[element.start:element.end]), &have); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		if kept < len(want) && reflect.DeepEqual(have, want[kept]) {
			kept++
			continue
		}
		drop = append(drop, i)
	}
	for _, i := range slices.Backward(drop) {
		elements, closing, err := scanJSONArray(text, start)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		text = removeJSONMember(text, start, closing, elements, i)
	}
	unit := adapters.DetectJSONIndent(data)
	for _, item := range want[kept:] {
		elements, closing, err := scanJSONArray(text, start)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		text = insertJSONEntry(text, start, closing, elements, "", item, unit)
	}
	return []byte(text), nil
}

// findJSONPath is the offset where the value at path starts, or -1 when
// the path is missing or crosses a value that is not an object.
func findJSONPath(text string, open int, path []string) (int, error) {
	members, _, err := scanJSONObject(text, open)
	if err != nil {
		return 0, err
	}
	i := lastJSONMember(members, path[0])
	if i < 0 {
		return -1, nil
	}
	start := members[i].valueStart
	if len(path) == 1 {
		return start, nil
	}
	if text[start] != '{' {
		return -1, nil
	}
	return findJSONPath(text, start, path[1:])
}

// scanJSONArray lists the elements of the array whose opening bracket is
// at open, and the offset of its closing bracket. It skips JSONC
// comments.
func scanJSONArray(text string, open int) ([]jsonMember, int, error) {
	var elements []jsonMember
	comma := -1
	i := skipJSONSpace(text, open+1)
	for i < len(text) {
		if text[i] == ']' {
			return elements, i, nil
		}
		end, err := jsonValueEnd(text, i)
		if err != nil {
			return nil, 0, err
		}
		elements = append(elements, jsonMember{comma: comma, start: i, valueStart: i, end: end})
		i = skipJSONSpace(text, end)
		if i < len(text) && text[i] == ',' {
			comma = i
			i = skipJSONSpace(text, i+1)
		} else if i < len(text) && text[i] != ']' {
			return nil, 0, fmt.Errorf("expected , or ] in a JSON array")
		}
	}
	return nil, 0, fmt.Errorf("expected a JSON array")
}

// jsonValueEnd is the offset just past the JSON value starting at i.
func jsonValueEnd(text string, i int) (int, error) {
	switch text[i] {
	case '{':
		_, closing, err := scanJSONObject(text, i)
		return closing + 1, err
	case '[':
		_, closing, err := scanJSONArray(text, i)
		return closing + 1, err
	case '"':
		for j := i + 1; j < len(text); j++ {
			switch text[j] {
			case '\\':
				j++
			case '"':
				return j + 1, nil
			}
		}
		return 0, fmt.Errorf("unterminated string")
	}
	j := i
	for j < len(text) && !strings.ContainsRune(" \t\r\n,]}/", rune(text[j])) {
		j++
	}
	if j == i {
		return 0, fmt.Errorf("expected a JSON value")
	}
	return j, nil
}

// keptBefore returns where the cut for a removed member starts: at the
// member itself when only whitespace precedes it from from, which
// undoes an insert exactly, else after the last line break before it,
// so a comment there stays on its own line.
func keptBefore(text string, from, start int) int {
	between := text[from:start]
	if strings.TrimSpace(between) == "" {
		return from
	}
	if nl := strings.LastIndex(between, "\n"); nl >= 0 {
		return from + nl + 1
	}
	return start
}

// lineIndent is the leading whitespace of the line holding pos.
func lineIndent(text string, pos int) string {
	line := text[strings.LastIndex(text[:pos], "\n")+1:]
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

// jsonValueText spells v as JSON, indenting nested lines under prefix.
// An empty unit keeps it on one line.
func jsonValueText(v any, prefix, unit string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if unit != "" {
		enc.SetIndent(prefix, unit)
	}
	if err := enc.Encode(v); err != nil {
		return "null"
	}
	return strings.TrimSuffix(buf.String(), "\n")
}

// jsonMember is one member of a JSON object: the comma before it (-1
// for the first), where its key starts, where its value starts, and
// where the value ends.
type jsonMember struct {
	key                           string
	comma, start, valueStart, end int
}

func lastJSONMember(members []jsonMember, key string) int {
	for i := len(members) - 1; i >= 0; i-- {
		if members[i].key == key {
			return i
		}
	}
	return -1
}

// scanJSONObject lists the members of the object whose opening brace is
// at open, and the offset of its closing brace. It skips strings and
// JSONC comments.
func scanJSONObject(text string, open int) ([]jsonMember, int, error) {
	var members []jsonMember
	depth := 0
	var current *jsonMember
	expectKey := false
	comma := -1
	for i := open; i < len(text); i++ {
		c := text[i]
		switch {
		case strings.HasPrefix(text[i:], "//"):
			if nl := strings.IndexByte(text[i:], '\n'); nl >= 0 {
				i += nl
			} else {
				i = len(text)
			}
		case strings.HasPrefix(text[i:], "/*"):
			end := strings.Index(text[i+2:], "*/")
			if end < 0 {
				return nil, 0, fmt.Errorf("unterminated comment")
			}
			i += end + 3
		case c == '"':
			j := i + 1
			for ; j < len(text) && text[j] != '"'; j++ {
				if text[j] == '\\' {
					j++
				}
			}
			if j >= len(text) {
				return nil, 0, fmt.Errorf("unterminated string")
			}
			if depth == 1 && expectKey {
				var key string
				if err := json.Unmarshal([]byte(text[i:j+1]), &key); err != nil {
					return nil, 0, err
				}
				members = append(members, jsonMember{key: key, comma: comma, start: i})
				current = &members[len(members)-1]
				expectKey = false
			} else if depth == 1 && current != nil {
				current.end = j + 1
			}
			i = j
		case c == ':' && depth == 1 && current != nil && current.valueStart == 0:
			current.valueStart = skipJSONSpace(text, i+1)
		case c == '{' || c == '[':
			depth++
			if depth == 1 {
				expectKey = c == '{'
			}
		case c == '}' || c == ']':
			depth--
			if depth == 1 && current != nil {
				current.end = i + 1
			}
			if depth == 0 {
				return members, i, nil
			}
		case c == ',' && depth == 1:
			expectKey = true
			current = nil
			comma = i
		case depth == 1 && current != nil && current.valueStart != 0 && !strings.ContainsRune(" \t\r\n", rune(c)):
			current.end = i + 1
		}
	}
	return nil, 0, fmt.Errorf("expected a JSON object")
}
