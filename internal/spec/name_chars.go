package spec

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
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
	invalid := false
	var before rune
	for offset := 0; offset < len(name); {
		r, size := utf8.DecodeRuneInString(name[offset:])
		var after rune
		if offset+size < len(name) {
			after, _ = utf8.DecodeRuneInString(name[offset+size:])
		}
		switch {
		case r == utf8.RuneError && size == 1:
			if !invalid {
				out = append(out, "invalid UTF-8")
			}
			invalid = true
		case seen[r] || !claudeRefusesRune(r) || partOfEmoji(before, r, after):
		case r == '<' || r == '>':
			seen[r] = true
			out = append(out, "`"+string(r)+"`")
		default:
			seen[r] = true
			out = append(out, fmt.Sprintf("U+%04X", r))
		}
		before = r
		offset += size
	}
	return out
}

// partOfEmoji reports whether r is a joiner inside an emoji sequence or
// word, such as a family emoji or a Persian word, or a tag character that
// spells a subdivision flag after its emoji. LINT044 allows the same.
func partOfEmoji(before, r, after rune) bool {
	switch {
	case r == '\U0000200C' || r == '\U0000200D':
		return visibleNonASCII(before) && visibleNonASCII(after)
	case isTag(r):
		return isTag(before) || unicode.Is(unicode.So, before)
	}
	return false
}

func isTag(r rune) bool { return r >= '\U000E0020' && r <= '\U000E007F' }

func visibleNonASCII(r rune) bool {
	return r >= utf8.RuneSelf && r != utf8.RuneError && !claudeRefusesRune(r)
}

func claudeRefusesRune(r rune) bool {
	switch r {
	case '<', '>':
		return true
	case ' ':
		return false
	}
	return unicode.In(r, unicode.Cc, unicode.Cf, unicode.Co, unicode.Cs, unicode.Z)
}
