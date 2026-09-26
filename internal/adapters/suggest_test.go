package adapters

import (
	"strings"
	"testing"
)

func TestSuggestName(t *testing.T) {
	names := []string{"claude", "cline", "codex", "cursor", "kiro"}
	cases := map[string]string{
		"claud":  "claude",
		"cursr":  "cursor",
		"Claude": "claude",
		"codx":   "codex",
		"kilo":   "kiro",
		"zzzzzz": "",
		"":       "",
		"claude": "",
	}
	for in, want := range cases {
		if got := SuggestName(in, names); got != want {
			t.Errorf("SuggestName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolve_UnknownTargetSuggestsClosestBuiltIn(t *testing.T) {
	_, err := Resolve("claud")
	if err == nil {
		t.Fatal("expected an error for an unknown target")
	}
	if !strings.Contains(err.Error(), "did you mean claude?") {
		t.Errorf("error lacks a suggestion: %v", err)
	}
}
