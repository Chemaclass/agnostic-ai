package adapters

import "testing"

func TestHookMatcherKey_FoldsOnlyWhereTheTargetRespellsTheMatcher(t *testing.T) {
	cases := []struct {
		target, matcher, want string
	}{
		{"codex", "Write | Edit|Write", "Edit|Write"},
		{"codex", "", ""},
		{"claude", "Write|Edit", "Write|Edit"},
		{"nonexistent", "Write|Edit", "Write|Edit"},
	}
	for _, tc := range cases {
		if got := HookMatcherKey(tc.target, tc.matcher); got != tc.want {
			t.Errorf("HookMatcherKey(%q, %q) = %q, want %q", tc.target, tc.matcher, got, tc.want)
		}
	}
}
