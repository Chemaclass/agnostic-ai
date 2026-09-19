package spec

import "testing"

func TestSplitPermissionRule(t *testing.T) {
	cases := []struct {
		rule  string
		scope string
		arg   string
		ok    bool
	}{
		{rule: "Bash(go test:*)", scope: "Bash", arg: "go test:*", ok: true},
		{rule: "Read(**)", scope: "Read", arg: "**", ok: true},
		{rule: "Write(.env*)", scope: "Write", arg: ".env*", ok: true},
		// A nested paren belongs to the argument: the scope ends at the
		// first one and the match is on the trailing ")".
		{rule: "Bash(sh -c (x))", scope: "Bash", arg: "sh -c (x)", ok: true},
		// A bare tool name is a whole-tool rule, not a scoped one.
		{rule: "Bash"},
		// An empty argument is not a rule. `Bash()` would widen to every
		// command if it were read as a bare `Bash`.
		{rule: "Bash()"},
		{rule: "(x)"},
		{rule: "Bash(x"},
		{rule: ""},
	}
	for _, tc := range cases {
		scope, arg, ok := SplitPermissionRule(tc.rule)
		if ok != tc.ok || scope != tc.scope || arg != tc.arg {
			t.Errorf("SplitPermissionRule(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tc.rule, scope, arg, ok, tc.scope, tc.arg, tc.ok)
		}
	}
}

func TestSplitMCPPermissionRule(t *testing.T) {
	cases := []struct {
		rule   string
		server string
		tool   string
		ok     bool
	}{
		{rule: "mcp__github__list_issues", server: "github", tool: "list_issues", ok: true},
		// The tool half keeps any further separator, so only the first
		// one divides server from tool.
		{rule: "mcp__srv__a__b", server: "srv", tool: "a__b", ok: true},
		{rule: "mcp__srv"},
		{rule: "mcp__"},
		{rule: "mcp____tool"},
		{rule: "mcp__srv__"},
		{rule: "github__list_issues"},
		{rule: ""},
	}
	for _, tc := range cases {
		server, tool, ok := SplitMCPPermissionRule(tc.rule)
		if ok != tc.ok || server != tc.server || tool != tc.tool {
			t.Errorf("SplitMCPPermissionRule(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tc.rule, server, tool, ok, tc.server, tc.tool, tc.ok)
		}
	}
}
