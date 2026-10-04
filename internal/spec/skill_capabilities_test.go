package spec

import (
	"slices"
	"testing"
)

func TestSkillCapabilities_ExpandsWithoutChangingSource(t *testing.T) {
	e := Entry{Kind: KindSkill, Meta: map[string]any{"allowed-tools": []any{"read(src/**)", "shell(git diff *)", "delete"}}}
	got, problem := e.NativeAllowedTools()
	if problem != "" {
		t.Fatal(problem)
	}
	if !slices.Equal(got.Meta["allowed-tools"].([]any), []any{"Read(src/**)", "Bash(git diff *)", "Delete"}) {
		t.Errorf("tools = %v", got.Meta["allowed-tools"])
	}
	if e.Meta["allowed-tools"].([]any)[0] != "read(src/**)" {
		t.Error("source changed")
	}
}

func TestAgentCapabilities_AcceptsPathsAndDelete(t *testing.T) {
	got, problem := CapabilityTools([]string{"read(src/**)", "edit(.env)", "delete"})
	if problem != "" || !slices.Equal(got, []string{"Read(src/**)", "Edit(.env)", "Delete"}) {
		t.Errorf("tools = %v, %q", got, problem)
	}
}

func TestSkillCapabilityProblem_RejectsMalformedRestrictions(t *testing.T) {
	for _, value := range []any{[]any{"raed"}, []any{1}, []any{"edit()"}} {
		if problem := SkillCapabilityProblem(map[string]any{"allowed-tools": value}); problem == "" {
			t.Errorf("accepted %v", value)
		}
	}
}

func TestSkillCapabilities_ScalarKeepsCommandCommas(t *testing.T) {
	for input, want := range map[string]string{
		"Bash(git log --format=%h,%s)":      "Bash(git log --format=%h,%s)",
		"read, shell(git log --format=a,b)": "Read, Bash(git log --format=a,b)",
		"read, shell(echo (a,b)), edit":     "Read, Bash(echo (a,b)), Edit",
	} {
		entry := Entry{Kind: KindSkill, Meta: map[string]any{"allowed-tools": input}}
		native, problem := entry.NativeAllowedTools()
		if problem != "" || native.Meta["allowed-tools"] != want {
			t.Errorf("scalar %q = %v, %q; want %q", input, native.Meta["allowed-tools"], problem, want)
		}
	}
}
