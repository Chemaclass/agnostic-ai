package emit

import (
	"fmt"
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

// EscapeTOMLBasic escapes the two characters that can break out of a
// basic (or basic-multiline) TOML string: backslash and double quote.
// Newlines pass through unchanged so multiline literals stay readable.
func EscapeTOMLBasic(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
