package spec

import (
	"reflect"
	"testing"
)

func TestGlobList(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want []string
	}{
		{"one string", "*.go", []string{"*.go"}},
		{"comma string", " *.go, *.mod ", []string{"*.go", "*.mod"}},
		{"brace set string", "src/**/*.{ts,tsx}", []string{"src/**/*.{ts,tsx}"}},
		{"brace set and another", "src/**/*.{ts,tsx},lib/*.js", []string{"src/**/*.{ts,tsx}", "lib/*.js"}},
		{"nested braces", "a/{b,{c,d}}/*.go,e", []string{"a/{b,{c,d}}/*.go", "e"}},
		{"unmatched open brace", "src/a{.ts,lib/*.js", []string{"src/a{.ts", "lib/*.js"}},
		{"list", []any{"src/**/*.{ts,tsx}", "lib/*.js"}, []string{"src/**/*.{ts,tsx}", "lib/*.js"}},
		{"string list", []string{"*.go"}, []string{"*.go"}},
		{"empty", "", nil},
		{"malformed list", []any{"*.go", 3}, nil},
		{"malformed value", map[string]any{"go": true}, nil},
	}
	for _, c := range cases {
		if got := GlobList(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: GlobList(%#v) = %#v, want %#v", c.name, c.in, got, c.want)
		}
	}
}
