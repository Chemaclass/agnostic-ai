package spec

import (
	"slices"
	"testing"
)

func TestSplitToolList_SplitsOnlyOutsideParentheses(t *testing.T) {
	for input, want := range map[string][]string{
		" Read, Edit ":                    {"Read", "Edit"},
		"read, shell(echo (a,b)), edit":   {"read", "shell(echo (a,b))", "edit"},
		"Bash(git log --format=%h,%s)":    {"Bash(git log --format=%h,%s)"},
		"Read(資料/a,b), shell(git status)": {"Read(資料/a,b)", "shell(git status)"},
		"shell(git status, read":          {"shell(git status, read"},
		"Read), Edit":                     {"Read)", "Edit"},
	} {
		if got := SplitToolList(input); !slices.Equal(got, want) {
			t.Errorf("split %q = %v; want %v", input, got, want)
		}
	}
}
