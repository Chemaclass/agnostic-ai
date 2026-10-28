package emit

import (
	"slices"
	"testing"
)

func TestOrderByLifecycle_PutsLifecycleFirstThenFirstSeen(t *testing.T) {
	seen := []string{"custom", "PostToolUse", "other", "PreToolUse", "custom", "PostToolUse"}

	got := OrderByLifecycle(seen, []string{"SessionStart", "PreToolUse", "PostToolUse"})

	want := []string{"PreToolUse", "PostToolUse", "custom", "other"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
