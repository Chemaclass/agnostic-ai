package adapters

import "testing"

func TestHookMatcherCovers_FoldsOnlyWhereTheTargetRespellsTheMatcher(t *testing.T) {
	cases := []struct {
		target, native, spec string
		want                 bool
	}{
		{"codex", "Edit|Write", "Write | Edit|Write", true},
		{"codex", "Write|Edit", "Edit", true},
		{"codex", "Edit", "Edit|Write", false},
		{"codex", "", "", true},
		{"claude", "Edit|Write", "Write|Edit", false},
		{"claude", "Edit", "Edit", true},
		{"nonexistent", "Edit|Write", "Edit", false},
	}
	for _, tc := range cases {
		if got := HookMatcherCovers(tc.target, tc.native, tc.spec); got != tc.want {
			t.Errorf("HookMatcherCovers(%q, %q, %q) = %v, want %v", tc.target, tc.native, tc.spec, got, tc.want)
		}
	}
}
