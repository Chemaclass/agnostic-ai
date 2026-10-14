package emit

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
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
// out silently. An empty map writes `key = {}`.
func WriteTOMLInlineTable(sb *strings.Builder, key string, m map[string]any) (skipped []string) {
	lit, skipped := inlineTable(m)
	sb.WriteString(key + " = " + lit + "\n")
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
		lit, ok := TOMLLiteral(m[k])
		if !ok {
			skipped = append(skipped, k)
			continue
		}
		parts = append(parts, TOMLKey(k)+" = "+lit)
	}
	if len(parts) == 0 {
		return "{}", skipped
	}
	return "{ " + strings.Join(parts, ", ") + " }", skipped
}

// TOMLLiteral renders v as a single-line TOML value: strings, bools,
// integers, finite floats (a whole float keeps its `.0`), time.Time,
// string and mixed arrays, and nested maps as inline tables. It returns
// false for anything else, including NaN and infinities, which TOML
// spells differently from Go.
func TOMLLiteral(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return `"` + EscapeTOMLSingleLine(t) + `"`, true
	case bool:
		return strconv.FormatBool(t), true
	case int:
		return strconv.Itoa(t), true
	case int64:
		return strconv.FormatInt(t, 10), true
	case uint64:
		return strconv.FormatUint(t, 10), true
	case float64:
		return tomlFloat(t)
	case time.Time:
		return t.Format(time.RFC3339Nano), true
	case []string:
		items := make([]string, len(t))
		for i, s := range t {
			items[i] = `"` + EscapeTOMLSingleLine(s) + `"`
		}
		return "[" + strings.Join(items, ", ") + "]", true
	case []any:
		items := make([]string, len(t))
		for i, x := range t {
			lit, ok := TOMLLiteral(x)
			if !ok {
				return "", false
			}
			items[i] = lit
		}
		return "[" + strings.Join(items, ", ") + "]", true
	case map[string]string:
		m := make(map[string]any, len(t))
		for k, s := range t {
			m[k] = s
		}
		lit, _ := inlineTable(m)
		return lit, true
	case map[string]any:
		lit, skipped := inlineTable(t)
		return lit, len(skipped) == 0
	}
	return "", false
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
	case string:
		WriteTOMLString(sb, key, t)
	case bool:
		fmt.Fprintf(sb, "%s = %t\n", key, t)
	case int:
		fmt.Fprintf(sb, "%s = %d\n", key, t)
	case int64:
		fmt.Fprintf(sb, "%s = %d\n", key, t)
	case float64:
		lit, ok := tomlFloat(t)
		if !ok {
			return false
		}
		sb.WriteString(key + " = " + lit + "\n")
	case []string:
		WriteTOMLStringArray(sb, key, t)
	case []any:
		ss := StringSlice(t)
		if len(ss) != len(t) {
			return false // a non-string element: skip the whole array
		}
		WriteTOMLStringArray(sb, key, ss)
	default:
		return false
	}
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
