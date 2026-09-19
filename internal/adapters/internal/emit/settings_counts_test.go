package emit

import (
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestSpecsWithPermissions(t *testing.T) {
	entry := func(lists map[string]any) spec.Entry {
		return spec.Entry{Kind: spec.KindSettings, Meta: map[string]any{"permissions": lists}}
	}
	cases := []struct {
		name     string
		settings []spec.Entry
		want     int
	}{
		{name: "none", want: 0},
		{
			name:     "one list",
			settings: []spec.Entry{entry(map[string]any{"deny": []any{"Bash(rm:*)"}})},
			want:     1,
		},
		{
			// One spec, three lists, one note. The user is told their
			// policy did not land, not told it three times.
			name: "three lists on one spec still count once",
			settings: []spec.Entry{entry(map[string]any{
				"allow": []any{"Read"}, "deny": []any{"Write"}, "ask": []any{"Bash"},
			})},
			want: 1,
		},
		{
			name: "empty lists do not count",
			settings: []spec.Entry{entry(map[string]any{
				"allow": []any{}, "deny": nil,
			})},
			want: 0,
		},
		{
			name:     "a spec with no permissions key at all",
			settings: []spec.Entry{{Kind: spec.KindSettings, Meta: map[string]any{"model": "x"}}},
			want:     0,
		},
		{
			name: "counts each carrying spec",
			settings: []spec.Entry{
				entry(map[string]any{"allow": []any{"Read"}}),
				{Kind: spec.KindSettings, Meta: map[string]any{"model": "x"}},
				entry(map[string]any{"ask": []any{"Bash"}}),
			},
			want: 2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SpecsWithPermissions(tc.settings); got != tc.want {
				t.Errorf("SpecsWithPermissions() = %d, want %d", got, tc.want)
			}
		})
	}
}
