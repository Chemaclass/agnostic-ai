package kiro

import (
	"slices"
	"testing"
)

func TestWiderTools_NamesTheAccessACategoryAdds(t *testing.T) {
	cases := []struct {
		names []string
		want  []string
	}{
		{[]string{"Edit"}, []string{"edit becomes Kiro's write category, which also allows delete_file"}},
		{[]string{"Edit", "Write", "Read"}, []string{"edit and write become Kiro's write category, which also allows delete_file"}},
		{[]string{"WebFetch"}, []string{"WebFetch becomes Kiro's web category, which also allows web_search"}},
		{[]string{"WebSearch", "WebFetch"}, nil},
		{[]string{"Read", "Grep", "Bash", "Bash(git diff *)"}, nil},
	}
	for _, c := range cases {
		if got := widerTools(c.names); !slices.Equal(got, c.want) {
			t.Errorf("widerTools(%v) = %q, want %q", c.names, got, c.want)
		}
	}
}
