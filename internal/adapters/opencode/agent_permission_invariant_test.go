package opencode

import (
	"encoding/json"
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
	"gopkg.in/yaml.v3"
)

type permissionRule struct{ key, pattern, action string }

// flattenAgent lists an agent permission map the way OpenCode reads it:
// the top-level catch-all first, then each key, patterns in written order.
func flattenAgent(t *testing.T, perms map[string]any) []permissionRule {
	t.Helper()
	keys := slices.Sorted(maps.Keys(perms))
	var out []permissionRule
	for _, key := range keys {
		switch v := perms[key].(type) {
		case string:
			out = append(out, permissionRule{key, "*", v})
		case *yaml.Node:
			for i := 0; i+1 < len(v.Content); i += 2 {
				out = append(out, permissionRule{key, v.Content[i].Value, v.Content[i+1].Value})
			}
		case map[string]any:
			for _, pattern := range slices.Sorted(maps.Keys(v)) {
				out = append(out, permissionRule{key, pattern, v[pattern].(string)})
			}
		default:
			t.Fatalf("unexpected value for %s: %#v", key, v)
		}
	}
	return out
}

func flattenProject(t *testing.T, project *agentProject) []permissionRule {
	t.Helper()
	keys := slices.Sorted(maps.Keys(project.rules))
	var out []permissionRule
	for _, key := range keys {
		rules := project.rules[key]
		for _, pattern := range rules.Keys() {
			raw, _ := rules.Get(pattern)
			var action string
			if err := json.Unmarshal(raw, &action); err != nil {
				t.Fatal(err)
			}
			out = append(out, permissionRule{key, pattern, action})
		}
	}
	return out
}

func globMatch(glob, s string) bool {
	re := "^" + strings.ReplaceAll(strings.ReplaceAll(regexp.QuoteMeta(glob), `\*`, ".*"), `\?`, ".") + "$"
	return regexp.MustCompile(re).MatchString(s)
}

func decide(rules []permissionRule, key, input string) string {
	action := "allow"
	if key == "doom_loop" || key == externalDirectoryKey {
		action = "ask"
	}
	for _, r := range rules {
		if globMatch(r.key, key) && globMatch(r.pattern, input) {
			action = r.action
		}
	}
	return action
}

// An agent's effective decision is never looser than the stricter of
// the project's decision and the agent's own tools list.
func TestAgentPermission_NeverLooserThanProject(t *testing.T) {
	type probe struct{ key, input string }
	probes := []probe{
		{"bash", "git diff HEAD"}, {"bash", "rm -rf x"}, {"bash", "ls"}, {"bash", "npm i"}, {"bash", "git push main"},
		{"read", "src/a.go"}, {"read", "/secrets/k"},
		{"doom_loop", "x"}, {externalDirectoryKey, "/tmp/x"}, {externalDirectoryKey, "/secrets/k"},
		{"edit", "a.go"},
	}
	cases := []struct {
		name    string
		project map[string]any
		tools   []string
	}{
		{"tool ask, scoped agent", map[string]any{"bash": "ask"}, []string{"Bash(git diff *)"}},
		{"scoped asks outside and inside the agent", map[string]any{"bash": map[string]any{"npm *": "ask", "git push *": "ask"}}, []string{"Bash(git *)"}},
		{"global ask, scoped agent", map[string]any{"*": "ask"}, []string{"Bash(git diff *)", "Read"}},
		{"global and doom_loop deny", map[string]any{"*": "deny", "doom_loop": "deny"}, []string{"Read"}},
		{"global deny, tool scoped deny", map[string]any{"*": "deny", "bash": map[string]any{"rm *": "deny"}}, []string{"Bash"}},
		{"tool allow with scoped deny", map[string]any{"bash": map[string]any{"*": "allow", "rm *": "deny"}}, []string{"Bash"}},
		{"scoped read deny", map[string]any{"read": map[string]any{"/secrets/**": "deny"}}, []string{"Read(src/**)", "Read"}},
		{"outside deny under global deny", map[string]any{"*": "deny", externalDirectoryKey: map[string]any{"/secrets/**": "deny"}}, []string{"Read"}},
		{"outside bare deny", map[string]any{externalDirectoryKey: "deny"}, []string{"Edit"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settings := []spec.Entry{{Kind: spec.KindSettings, Name: "s", Path: "s.yaml", Meta: map[string]any{
				"x-opencode": map[string]any{"permission": tc.project},
			}}}
			project, err := newAgentProject(settings, nil)
			if err != nil {
				t.Fatal(err)
			}
			perms, _ := agentPermission(tc.tools, project)
			alone, _ := agentPermission(tc.tools, nil)
			listRules := flattenAgent(t, alone)
			projectRules := flattenProject(t, project)
			agentRules := append(slices.Clone(projectRules), flattenAgent(t, perms)...)
			for _, p := range probes {
				want, got := decide(projectRules, p.key, p.input), decide(agentRules, p.key, p.input)
				if permissionSeverity[got] < permissionSeverity[want] {
					t.Errorf("%s %q: agent %s is looser than project %s (agent map %v)", p.key, p.input, got, want, agentRules)
				}
				if list := decide(listRules, p.key, p.input); permissionSeverity[got] < permissionSeverity[list] {
					t.Errorf("%s %q: agent %s is looser than its own list %s (agent map %v)", p.key, p.input, got, list, agentRules)
				}
			}
		})
	}
}

func TestWildcardMatch(t *testing.T) {
	for _, tc := range []struct {
		pattern, s string
		want       bool
	}{
		{"git *", "git push *", true},
		{"git *", "npm *", false},
		{"*", "", true},
		{"a?c", "abc", true},
		{"a?c", "ac", false},
		{"/secrets/**", "/secrets/k", true},
	} {
		if got := wildcardMatch(tc.pattern, tc.s); got != tc.want {
			t.Errorf("wildcardMatch(%q, %q) = %t, want %t", tc.pattern, tc.s, got, tc.want)
		}
	}
}
