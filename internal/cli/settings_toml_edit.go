package cli

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

// tomlRootKey is where one root-table key sits: its first and last line,
// and a trailing comment on a one-line value.
type tomlRootKey struct {
	start, end int
	comment    string
}

// editTOMLRoot sets and removes keys of the root table line by line, so
// comments, tables, and formatting elsewhere stay byte for byte. A set
// key replaces its line, keeping a trailing comment; a new key goes
// after the last root key, or at the top when there is none, which is
// always before the first table header.
func editTOMLRoot(path string, data []byte, order []string, set map[string]any, remove []string) ([]byte, error) {
	// Each line keeps its own "\r", so mixed line endings survive; new
	// lines follow the first line's ending. A BOM stays in front.
	text, bom := strings.CutPrefix(string(data), "\ufeff")
	cr := ""
	if first, _, _ := strings.Cut(text, "\n"); strings.HasSuffix(first, "\r") {
		cr = "\r"
	}
	trailing := strings.HasSuffix(text, "\n")
	text = strings.TrimSuffix(text, "\n")
	var lines []string
	if text != "" {
		lines = strings.Split(text, "\n")
	}
	keys, lastKey, err := scanTOMLRoot(lines)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	values := map[string]string{}
	for _, key := range order {
		text, err := tomlValueText(set[key])
		if err != nil {
			return nil, fmt.Errorf("%s: %s: %w", path, key, err)
		}
		values[key] = text
	}
	replaced := map[int][]string{}
	dropped := map[int]bool{}
	var added []string
	for _, key := range order {
		loc, ok := keys[key]
		if !ok {
			added = append(added, tomlKey(key)+" = "+values[key]+cr)
			continue
		}
		start := lines[loc.start]
		indent := start[:len(start)-len(strings.TrimLeft(start, " \t"))]
		next := indent + tomlKey(key) + " = " + values[key]
		if loc.comment != "" {
			next += " " + loc.comment
		}
		if strings.HasSuffix(start, "\r") {
			next += "\r"
		}
		replaced[loc.start] = []string{next}
		for i := loc.start + 1; i <= loc.end; i++ {
			dropped[i] = true
		}
	}
	for _, key := range remove {
		loc, ok := keys[key]
		if !ok {
			continue
		}
		for i := loc.start; i <= loc.end; i++ {
			dropped[i] = true
		}
	}
	// Removing every line up to the last root key also removes the blank
	// line an insert at the top put before the first table.
	if lastKey >= 0 && lastKey+1 < len(lines) && strings.TrimSpace(lines[lastKey+1]) == "" {
		all := true
		for i := 0; i <= lastKey && all; i++ {
			all = dropped[i]
		}
		if all && len(added) == 0 {
			dropped[lastKey+1] = true
		}
	}
	insertAt := 0
	if lastKey >= 0 {
		insertAt = lastKey + 1
	} else if len(added) > 0 && len(lines) > 0 && strings.TrimSpace(lines[0]) != "" {
		added = append(added, cr)
	}
	var out []string
	for i := 0; i <= len(lines); i++ {
		if i == insertAt {
			out = append(out, added...)
		}
		if i == len(lines) {
			break
		}
		if next, ok := replaced[i]; ok {
			out = append(out, next...)
			continue
		}
		if !dropped[i] {
			out = append(out, lines[i])
		}
	}
	result := strings.Join(out, "\n")
	if trailing || len(lines) == 0 {
		result += "\n"
	}
	if bom {
		result = "\ufeff" + result
	}
	return []byte(result), nil
}

// scanTOMLRoot finds the keys of the root table, the lines before the
// first table header, and the last line of the last root key (-1 when
// there is none). It tracks strings, comments, and nested arrays and
// inline tables, so a bracket inside a value is not read as a header.
func scanTOMLRoot(lines []string) (map[string]tomlRootKey, int, error) {
	keys := map[string]tomlRootKey{}
	lastKey := -1
	var multi string // the open multi-line string delimiter, if any
	depth := 0
	current := ""
	for i, raw := range lines {
		rest := raw
		if multi == "" && depth == 0 {
			trimmed := strings.TrimSpace(raw)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			if strings.HasPrefix(trimmed, "[") {
				return keys, lastKey, nil
			}
			eq := strings.Index(raw, "=")
			if eq < 0 {
				return nil, -1, fmt.Errorf("line %d: expected a key and value", i+1)
			}
			current = strings.Trim(strings.TrimSpace(raw[:eq]), `"'`)
			keys[current] = tomlRootKey{start: i, end: i}
			rest = raw[eq+1:]
		}
		comment := ""
		for j := 0; j < len(rest); j++ {
			c := rest[j]
			if multi != "" {
				if strings.HasPrefix(rest[j:], multi) {
					j += len(multi) - 1
					multi = ""
				} else if c == '\\' && multi == `"""` {
					j++
				}
				continue
			}
			switch {
			case strings.HasPrefix(rest[j:], `"""`), strings.HasPrefix(rest[j:], `'''`):
				multi = rest[j : j+3]
				j += 2
			case c == '"':
				for j++; j < len(rest) && rest[j] != '"'; j++ {
					if rest[j] == '\\' {
						j++
					}
				}
			case c == '\'':
				for j++; j < len(rest) && rest[j] != '\''; j++ {
				}
			case c == '[', c == '{':
				depth++
			case c == ']', c == '}':
				depth--
			case c == '#':
				comment = strings.TrimSpace(rest[j:])
				j = len(rest)
			}
		}
		loc := keys[current]
		loc.end = i
		if loc.start == i && multi == "" && depth == 0 {
			loc.comment = comment
		}
		keys[current] = loc
		if multi == "" && depth == 0 {
			lastKey = i
		}
	}
	if multi != "" || depth != 0 {
		return nil, -1, fmt.Errorf("unterminated value in the root table")
	}
	return keys, lastKey, nil
}

var bareTOMLKey = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// tomlKey spells key as a bare key when it can, else as a quoted one.
func tomlKey(key string) string {
	if bareTOMLKey.MatchString(key) {
		return key
	}
	return `"` + adapters.EscapeTOMLBasic(key) + `"`
}

// tomlValueText spells a scalar, or an array of scalars, as a TOML
// value. A table is refused: the root-table editor writes one line.
func tomlValueText(v any) (string, error) {
	switch value := v.(type) {
	case string:
		return `"` + adapters.EscapeTOMLBasic(value) + `"`, nil
	case bool:
		return strconv.FormatBool(value), nil
	case int:
		return strconv.Itoa(value), nil
	case int64:
		return strconv.FormatInt(value, 10), nil
	case uint64:
		return strconv.FormatUint(value, 10), nil
	case float64:
		if value == float64(int64(value)) {
			return strconv.FormatInt(int64(value), 10), nil
		}
		return strconv.FormatFloat(value, 'g', -1, 64), nil
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			if _, table := item.(map[string]any); table {
				return "", fmt.Errorf("an array of tables is not a root key")
			}
			text, err := tomlValueText(item)
			if err != nil {
				return "", err
			}
			parts = append(parts, text)
		}
		return "[" + strings.Join(parts, ", ") + "]", nil
	case []string:
		items := make([]any, len(value))
		for i, item := range value {
			items[i] = item
		}
		return tomlValueText(items)
	}
	return "", fmt.Errorf("%T is not a TOML scalar or array", v)
}
