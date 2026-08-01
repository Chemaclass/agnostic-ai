package emit

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// WriteTOMLString writes `key = "value"` with backslashes and double
// quotes escaped per the TOML basic-string rules.
func WriteTOMLString(sb *strings.Builder, key, value string) {
	fmt.Fprintf(sb, "%s = \"%s\"\n", key, EscapeTOMLBasic(value))
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
		sb.WriteString("\"" + EscapeTOMLBasic(v) + "\"")
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
		sb.WriteString(k + " = \"" + EscapeTOMLBasic(m[k]) + "\"")
	}
	sb.WriteString(" }\n")
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
		fmt.Fprintf(sb, "%s = %s\n", key, strconv.FormatFloat(t, 'f', -1, 64))
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

// EscapeTOMLBasic escapes the two characters that can break out of a
// basic (or basic-multiline) TOML string: backslash and double quote.
// Newlines pass through unchanged so multiline literals stay readable.
func EscapeTOMLBasic(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
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
