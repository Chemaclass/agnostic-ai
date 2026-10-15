package spec

import (
	"fmt"
	"strings"
	"unicode"
)

// ClaudeRefusedNameChars lists, in order, what in a skill or command name
// Claude Code refuses to load: control, format (zero-width and bidi),
// private-use characters, blanks other than one plain
// space between words, and `<` or `>`. Code points are named, never
// printed, so invisible ones show in the output.
// See https://code.claude.com/docs/en/skills#characters-a-skill-name-cant-hold.
func ClaudeRefusedNameChars(name string) []string {
	var out []string
	if strings.HasPrefix(name, " ") {
		out = append(out, "a leading space")
	}
	if strings.HasSuffix(name, " ") && strings.TrimSpace(name) != "" {
		out = append(out, "a trailing space")
	}
	if strings.Contains(name, "  ") {
		out = append(out, "two spaces in a row")
	}
	seen := map[rune]bool{}
	for _, r := range name {
		if seen[r] || !claudeRefusesRune(r) {
			continue
		}
		seen[r] = true
		if r == '<' || r == '>' {
			out = append(out, "`"+string(r)+"`")
		} else {
			out = append(out, fmt.Sprintf("U+%04X", r))
		}
	}
	return out
}

func claudeRefusesRune(r rune) bool {
	switch {
	case r == '<' || r == '>':
		return true
	case r == ' ':
		return false
	}
	return unicode.In(r, unicode.Cc, unicode.Cf, unicode.Co, unicode.Cs, unicode.Z)
}
