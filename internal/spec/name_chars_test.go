package spec

import (
	"reflect"
	"testing"
)

func TestClaudeRefusedNameChars(t *testing.T) {
	cases := map[string][]string{
		"deploy":                     nil,
		"deploy app":                 nil,
		"café":                       nil,
		"a<b>":                       {"`<`", "`>`"},
		"zero\U0000200Bwidth":        {"U+200B"},
		"bell\U00000007":             {"U+0007"},
		"rtl\U0000202Eoverride":      {"U+202E"},
		"icon\U0000E000":             {"U+E000"},
		"no\U000000A0break":          {"U+00A0"},
		" lead":                      {"a leading space"},
		"trail ":                     {"a trailing space"},
		"two  spaces":                {"two spaces in a row"},
		"tab\U00000009x\U0000200Bx<": {"U+0009", "U+200B", "`<`"},
	}
	for name, want := range cases {
		if got := ClaudeRefusedNameChars(name); !reflect.DeepEqual(got, want) {
			t.Errorf("ClaudeRefusedNameChars(%+q) = %q, want %q", name, got, want)
		}
	}
}
