package spec

import (
	"reflect"
	"testing"
)

func TestClaudeRefusedNameChars(t *testing.T) {
	cases := map[string][]string{
		"deploy":                            nil,
		"deploy app":                        nil,
		"caf\U000000E9":                     nil,
		"a<b>":                              {"`<`", "`>`"},
		"zero\U0000200Bwidth":               {"U+200B"},
		"bell\U00000007":                    {"U+0007"},
		"rtl\U0000202Eoverride":             {"U+202E"},
		"icon\U0000E000":                    {"U+E000"},
		"no\U000000A0break":                 {"U+00A0"},
		" lead":                             {"a leading space"},
		"trail ":                            {"a trailing space"},
		"two  spaces":                       {"two spaces in a row"},
		"dev\U0001F468\U0000200D\U0001F4BB": nil,
		"\U0000062E\U0000200C\U00000627":    nil,
		"flag\U0001F3F4\U000E0067\U000E0062\U000E0073\U000E0063\U000E0074\U000E007F": nil,
		"a\U0000200Db":                   {"U+200D"},
		"\U0001F468\U0000200B\U0001F4BB": {"U+200B"},
		"tag\U000E0067":                  {"U+E0067"},
		"bad\xffutf8\xfe":                {"invalid UTF-8"},
		"tab\U00000009x\U0000200Bx<":     {"U+0009", "U+200B", "`<`"},
	}
	for name, want := range cases {
		if got := ClaudeRefusedNameChars(name); !reflect.DeepEqual(got, want) {
			t.Errorf("ClaudeRefusedNameChars(%+q) = %q, want %q", name, got, want)
		}
	}
}
