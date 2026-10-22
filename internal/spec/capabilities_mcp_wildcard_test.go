package spec

import (
	"slices"
	"testing"
)

func TestCapabilityToolsFor_WholeServerWildcardOnlyForClaude(t *testing.T) {
	cases := []struct {
		rule, claude, other string
	}{
		{"mcp:github", "mcp__github", "mcp__github"},
		{"mcp:my-server", "mcp__my-server", "mcp__my-server"},
		{"mcp:reports__v2", "mcp__reports__v2__*", "mcp__reports__v2"},
		{"mcp:api_", "mcp__api___*", "mcp__api_"},
		{"mcp:reports__v2/run", "mcp__reports__v2__run", "mcp__reports__v2__run"},
	}
	for _, c := range cases {
		got, problem := CapabilityToolsFor("claude", []string{c.rule})
		if problem != "" || !slices.Equal(got, []string{c.claude}) {
			t.Errorf("claude %s = %v %q, want %s", c.rule, got, problem, c.claude)
		}
		for _, target := range []string{"", "opencode", "qoder"} {
			got, _ := CapabilityToolsFor(target, []string{c.rule})
			if !slices.Equal(got, []string{c.other}) {
				t.Errorf("%q %s = %v, want %s", target, c.rule, got, c.other)
			}
		}
	}
}

func TestNativeToolsFor_ClaudeWritesWildcardForDoubleUnderscoreServer(t *testing.T) {
	agent := Entry{Kind: KindAgent, Meta: map[string]any{"can": []any{"read", "mcp:github", "mcp:reports__v2"}}}
	native, problem := agent.NativeToolsFor("claude")
	want := []any{"Read", "mcp__github", "mcp__reports__v2__*"}
	if problem != "" || !slices.Equal(native.Meta["tools"].([]any), want) {
		t.Errorf("tools = %v %q, want %v", native.Meta["tools"], problem, want)
	}
}

func TestNativePermissionsFor_ClaudeWritesWildcardForDoubleUnderscoreServer(t *testing.T) {
	settings := Entry{Kind: KindSettings, Meta: map[string]any{"permissions": map[string]any{
		"allow": []any{"mcp:reports__v2", "mcp:github", "mcp__reports__v2__*"},
	}}}
	perms := func(target string) []any {
		return settings.NativePermissionsFor(target).Meta["permissions"].(map[string]any)["allow"].([]any)
	}
	if got, want := perms("claude"), []any{"mcp__reports__v2__*", "mcp__github", "mcp__reports__v2__*"}; !slices.Equal(got, want) {
		t.Errorf("claude allow = %v, want %v", got, want)
	}
	if got, want := perms("opencode"), []any{"mcp__reports__v2", "mcp__github", "mcp__reports__v2__*"}; !slices.Equal(got, want) {
		t.Errorf("opencode allow = %v, want %v", got, want)
	}
}
