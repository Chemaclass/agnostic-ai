package spec

import (
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func TestIncludeRefs_WritesNativeSeparatorsWithSlashes(t *testing.T) {
	body := "intro\n@" + filepath.FromSlash("apps/web/README.md") + "\n@docs/./guide.md\n"

	got := IncludeRefs(body)

	want := []string{"apps/web/README.md", "docs/guide.md"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("IncludeRefs = %q, want %q", got, want)
	}
}

func TestIncludeRefs_SkipsOnlyRealFences(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []string
	}{
		{"inline code span", "```example```\n@docs/guide.md\n", []string{"docs/guide.md"}},
		{"fenced line", "```md\n@docs/example.md\n```\n@docs/guide.md\n", []string{"docs/guide.md"}},
		{"closer with text", "```\n@docs/a.md\n``` not a closer\n@docs/b.md\n```\n@docs/guide.md\n", []string{"docs/guide.md"}},
		{"tilde info with backtick", "~~~ `sh`\n@docs/example.md\n~~~\n@docs/guide.md\n", []string{"docs/guide.md"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IncludeRefs(tc.body)

			if !slices.Equal(got, tc.want) {
				t.Errorf("IncludeRefs = %v, want %v", got, tc.want)
			}
		})
	}
}
