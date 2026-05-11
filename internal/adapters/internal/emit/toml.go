package emit

import (
	"fmt"
	"slices"
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

// EscapeTOMLBasic escapes the two characters that can break out of a
// basic (or basic-multiline) TOML string: backslash and double quote.
// Newlines pass through unchanged so multiline literals stay readable.
func EscapeTOMLBasic(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
