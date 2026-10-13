package cli

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// memoryHiddenShown caps the code points a hidden-character finding lists.
const memoryHiddenShown = 5

// zeroWidthJoiner glues the parts of an emoji sequence, such as a family or
// the rainbow flag, and zeroWidthNonJoiner shapes Persian and other
// scripts, so each counts as hidden only outside a run of visible
// non-ASCII characters.
const (
	zeroWidthNonJoiner = '\U0000200C'
	zeroWidthJoiner    = '\U0000200D'
)

func isJoiningControl(r rune) bool { return r == zeroWidthNonJoiner || r == zeroWidthJoiner }

// hiddenCharacters are the characters a reviewer cannot see in a diff and a
// model still reads: zero-width and joining controls, bidirectional
// controls, line and paragraph separators, invisible operators, the byte
// order mark, and the Unicode tags, which spell ASCII text.
var hiddenCharacters = &unicode.RangeTable{
	R16: []unicode.Range16{
		{Lo: 0x00AD, Hi: 0x00AD, Stride: 1},
		{Lo: 0x061C, Hi: 0x061C, Stride: 1},
		{Lo: 0x180E, Hi: 0x180E, Stride: 1},
		{Lo: 0x200B, Hi: 0x200F, Stride: 1},
		{Lo: 0x2028, Hi: 0x2029, Stride: 1},
		{Lo: 0x202A, Hi: 0x202E, Stride: 1},
		{Lo: 0x2060, Hi: 0x2064, Stride: 1},
		{Lo: 0x2066, Hi: 0x2069, Stride: 1},
		{Lo: 0xFEFF, Hi: 0xFEFF, Stride: 1},
	},
	R32: []unicode.Range32{
		{Lo: 0xE0000, Hi: 0xE007F, Stride: 1},
	},
}

func isHiddenCharacter(r rune) bool { return unicode.Is(hiddenCharacters, r) }

// joinsSymbols reports whether a joining control between before and after
// sits inside an emoji sequence or a word: both neighbors are visible, not
// ASCII.
func joinsSymbols(before, after rune) bool {
	symbol := func(r rune) bool { return r >= utf8.RuneSelf && !isHiddenCharacter(r) }
	return symbol(before) && symbol(after)
}

// eachHidden calls found with the byte offset and the character of each
// hidden character in text, in order.
func eachHidden(text string, found func(offset int, r rune)) {
	var before rune
	for offset := 0; offset < len(text); {
		r, size := utf8.DecodeRuneInString(text[offset:])
		if isHiddenCharacter(r) {
			var after rune
			if next := offset + size; next < len(text) {
				after, _ = utf8.DecodeRuneInString(text[next:])
			}
			if !isJoiningControl(r) || !joinsSymbols(before, after) {
				found(offset, r)
			}
		}
		before = r
		offset += size
	}
}

// hiddenIn returns each hidden character in text once, in code point order.
func hiddenIn(text string) []rune {
	var found []rune
	eachHidden(text, func(_ int, r rune) { found = append(found, r) })
	slices.Sort(found)
	return slices.Compact(found)
}

// stripHidden returns text without its hidden characters. Every other
// byte stays as it is.
func stripHidden(text string) string {
	if strings.IndexFunc(text, isHiddenCharacter) < 0 {
		return text
	}
	var out strings.Builder
	kept := 0
	eachHidden(text, func(offset int, r rune) {
		out.WriteString(text[kept:offset])
		kept = offset + utf8.RuneLen(r)
	})
	out.WriteString(text[kept:])
	return out.String()
}

// hiddenSeverity is how a hidden character counts in the store. Project
// memory is committed, so a pull request reviewer is the only check on
// what every tool reads; personal memory is the user's own file.
func (s memoryStore) hiddenSeverity() lintSeverity {
	if s.scope == "project" {
		return lintError
	}
	return lintWarn
}

// lintMemoryHidden flags each line that holds a hidden character
// (LINT044). The finding names the line and the code points, never the
// characters, since lint output lands in CI logs.
func lintMemoryHidden(path string, severity lintSeverity) []lintFinding {
	text, err := readMemoryFile(path)
	if err != nil {
		return nil
	}
	var out []lintFinding
	for i, line := range strings.Split(text, "\n") {
		found := hiddenIn(line)
		if len(found) == 0 {
			continue
		}
		out = append(out, lintFinding{
			Code:     "LINT044",
			Severity: severity,
			Path:     path,
			Message:  fmt.Sprintf("line %d holds hidden characters (%s) that a reviewer cannot see; delete them", i+1, codePoints(found)),
		})
	}
	return out
}

// codePoints lists the first memoryHiddenShown characters as U+XXXX and
// counts the rest.
func codePoints(found []rune) string {
	names := make([]string, 0, memoryHiddenShown+1)
	for _, r := range found[:min(len(found), memoryHiddenShown)] {
		names = append(names, fmt.Sprintf("U+%04X", r))
	}
	if more := len(found) - memoryHiddenShown; more > 0 {
		names = append(names, fmt.Sprintf("and %d more", more))
	}
	return strings.Join(names, ", ")
}
