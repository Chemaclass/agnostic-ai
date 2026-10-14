package emit

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
)

// WriteTOMLString writes `key = "value"` with backslashes and double
// quotes escaped per the TOML basic-string rules.
func WriteTOMLString(sb *strings.Builder, key, value string) {
	fmt.Fprintf(sb, "%s = \"%s\"\n", key, EscapeTOMLSingleLine(value))
}

// WriteTOMLMultiline writes `key = """\n<value>\n"""`. Backslashes and
// double quotes inside value are escaped so the closing delimiter cannot
// be confused with content.
func WriteTOMLMultiline(sb *strings.Builder, key, value string) {
	escaped := EscapeTOMLBasic(value)
	sb.WriteString(key + " = \"\"\"\n")
	sb.WriteString(escaped)
	if !strings.HasSuffix(escaped, "\n") {
		sb.WriteByte('\n')
	}
	sb.WriteString("\"\"\"\n")
}

// WriteTOMLStringArray writes `key = ["a", "b", ...]` with each value
// escaped per the basic-string rules.
func WriteTOMLStringArray(sb *strings.Builder, key string, values []string) {
	sb.WriteString(key + " = [")
	for i, v := range values {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("\"" + EscapeTOMLSingleLine(v) + "\"")
	}
	sb.WriteString("]\n")
}

// WriteTOMLInlineStringTable writes an inline-table value:
//
//	key = { k1 = "v1", k2 = "v2" }
//
// Keys are sorted alphabetically for deterministic output. Empty maps
// emit nothing. Used by adapters that pass env vars / headers as a
// nested TOML structure (Codex `[mcp_servers.<name>] env = {...}`).
func WriteTOMLInlineStringTable(sb *strings.Builder, key string, m map[string]string) {
	if len(m) == 0 {
		return
	}
	sb.WriteString(key + " = { ")
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for i, k := range keys {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(TOMLKey(k) + " = \"" + EscapeTOMLSingleLine(m[k]) + "\"")
	}
	sb.WriteString(" }\n")
}

// WriteTOMLInlineTable writes `key = { k1 = v1, ... }` with each value
// encoded by TOMLLiteral, so booleans, numbers, dates, arrays, and
// nested tables keep their TOML type. Keys are sorted and quoted when
// they are not bare-safe. A value TOMLLiteral cannot encode is left out
// and its key is returned in skipped; nil values (YAML null) are left
// out silently. A map with nothing left to write writes nothing.
func WriteTOMLInlineTable(sb *strings.Builder, key string, m map[string]any) (skipped []string) {
	lit, skipped := inlineTable(m)
	if lit != "{}" {
		sb.WriteString(key + " = " + lit + "\n")
	}
	return skipped
}

func inlineTable(m map[string]any) (string, []string) {
	if len(m) == 0 {
		return "{}", nil
	}
	var skipped []string
	parts := make([]string, 0, len(m))
	for _, k := range slices.Sorted(maps.Keys(m)) {
		if m[k] == nil {
			continue
		}
		lit, inner, ok := literal(m[k])
		if !ok {
			skipped = append(skipped, k)
			continue
		}
		for _, p := range inner {
			skipped = append(skipped, k+"."+p)
		}
		if lit == "{}" && len(inner) > 0 {
			continue // every leaf of this nested table was dropped
		}
		parts = append(parts, TOMLKey(k)+" = "+lit)
	}
	if len(parts) == 0 {
		return "{}", skipped
	}
	return "{ " + strings.Join(parts, ", ") + " }", skipped
}

// TOMLLiteral renders v as a single-line TOML value: strings, bools,
// integers, finite floats (a whole float keeps its `.0`), string and
// mixed arrays, and nested maps as inline tables. It returns false for
// anything else, including NaN, infinities, and dates (whose offset
// would depend on the machine's timezone), and for a map or array that
// holds such a value.
func TOMLLiteral(v any) (string, bool) {
	lit, skipped, ok := literal(v)
	return lit, ok && len(skipped) == 0
}

// literal is TOMLLiteral that keeps going inside maps: a map drops only
// its unencodable leaves and lists their dotted paths in skipped. An
// array cannot drop an element without shifting the rest, so an array
// with an unencodable element is unencodable as a whole (ok false).
func literal(v any) (lit string, skipped []string, ok bool) {
	switch t := v.(type) {
	case string:
		return `"` + EscapeTOMLSingleLine(t) + `"`, nil, true
	case bool:
		return strconv.FormatBool(t), nil, true
	case int:
		return strconv.Itoa(t), nil, true
	case int64:
		return strconv.FormatInt(t, 10), nil, true
	case uint64:
		if t > math.MaxInt64 {
			return "", nil, false
		}
		return strconv.FormatUint(t, 10), nil, true
	case float64:
		lit, ok := tomlFloat(t)
		return lit, nil, ok
	case []string:
		items := make([]string, len(t))
		for i, s := range t {
			items[i] = `"` + EscapeTOMLSingleLine(s) + `"`
		}
		return "[" + strings.Join(items, ", ") + "]", nil, true
	case []any:
		items := make([]string, len(t))
		for i, x := range t {
			lit, inner, ok := literal(x)
			if !ok || len(inner) > 0 {
				return "", nil, false
			}
			items[i] = lit
		}
		return "[" + strings.Join(items, ", ") + "]", nil, true
	case map[string]string:
		m := make(map[string]any, len(t))
		for k, s := range t {
			m[k] = s
		}
		lit, _ := inlineTable(m)
		return lit, nil, true
	case map[string]any:
		lit, skipped := inlineTable(t)
		return lit, skipped, true
	}
	return "", nil, false
}

func tomlFloat(f float64) (string, bool) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return "", false
	}
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s, true
}

// TOMLKey returns s as a TOML key: bare when it only has letters,
// digits, `_`, and `-`, otherwise a quoted key so a space or dot cannot
// break the line or split the key into a path.
func TOMLKey(s string) string {
	if s == "" {
		return `""`
	}
	for _, r := range s {
		bare := r == '-' || r == '_' ||
			(r >= '0' && r <= '9') ||
			(r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z')
		if !bare {
			return `"` + EscapeTOMLSingleLine(s) + `"`
		}
	}
	return s
}

// WriteTOMLValue writes `key = <value>`, picking the TOML form from v's
// dynamic type: string, bool, integers, float64, and string slices
// ([]string or a []any whose elements are all strings). Unsupported
// types (nested tables, mixed-type arrays) are skipped and the function
// returns false, so a caller passing arbitrary author metadata never
// emits malformed TOML.
func WriteTOMLValue(sb *strings.Builder, key string, v any) bool {
	switch t := v.(type) {
	case string, bool, int, int64, []string:
	case float64:
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return false
		}
		// Whole floats keep the bare-integer bytes config.toml has always had.
		sb.WriteString(TOMLKey(key) + " = " + strconv.FormatFloat(t, 'f', -1, 64) + "\n")
		return true
	case []any:
		if len(StringSlice(t)) != len(t) {
			return false // a non-string element: skip the whole array
		}
	default:
		return false
	}
	lit, ok := TOMLLiteral(v)
	if !ok {
		return false
	}
	sb.WriteString(TOMLKey(key) + " = " + lit + "\n")
	return true
}

// EscapeTOMLBasic escapes what can break out of a basic or
// basic-multiline TOML string: backslash, double quote, and control
// characters. Newlines, tabs, and CRLF pass through unchanged so
// multiline literals stay readable.
func EscapeTOMLBasic(s string) string {
	return escapeTOML(s, false)
}

// EscapeTOMLSingleLine escapes s for a one-line basic string: everything
// EscapeTOMLBasic escapes, plus newlines and tabs.
func EscapeTOMLSingleLine(s string) string {
	return escapeTOML(s, true)
}

func escapeTOML(s string, singleLine bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\':
			b.WriteString(`\\`)
		case c == '"':
			b.WriteString(`\"`)
		case c == '\n' && !singleLine, c == '\t' && !singleLine:
			b.WriteByte(c)
		case c == '\r' && !singleLine && i+1 < len(s) && s[i+1] == '\n':
			b.WriteByte(c)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\t':
			b.WriteString(`\t`)
		case c == '\r':
			b.WriteString(`\r`)
		case c < 0x20 || c == 0x7f:
			fmt.Fprintf(&b, `\u%04X`, c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// WriteTOMLMCPStdioFields writes the `command`, `args`, and `env`
// fields for a stdio-transport MCP server into the current TOML table.
// Shared by every adapter that renders MCP servers as TOML (Codex's
// `[mcp_servers.<name>]` block tables, OpenHands' `[[mcp.stdio_servers]]`
// array-of-tables) so the field set and string escaping stay identical
// across both rather than each hand-rolling its own copy.
func WriteTOMLMCPStdioFields(sb *strings.Builder, meta map[string]any) {
	if cmd, _ := meta["command"].(string); cmd != "" {
		WriteTOMLString(sb, "command", cmd)
	}
	if args := StringSlice(meta["args"]); len(args) > 0 {
		WriteTOMLStringArray(sb, "args", args)
	}
	WriteTOMLInlineStringTable(sb, "env", StringMap(meta["env"]))
}

// WriteTOMLMCPURLField writes the `url` field shared by every remote-
// transport MCP server (http, sse, ...) into the current TOML table.
func WriteTOMLMCPURLField(sb *strings.Builder, meta map[string]any) {
	if url, _ := meta["url"].(string); url != "" {
		WriteTOMLString(sb, "url", url)
	}
}
