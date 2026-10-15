package opencode

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
	"gopkg.in/yaml.v3"
)

// This file simulates OpenCode v1.19.0 without the adapter's helpers:
// packages/opencode/src/agent/agent.ts builds each agent's rules as
// Permission.merge(defaults, user, agent), merge is rulesets.flat(),
// and packages/opencode/src/permission/index.ts evaluate takes
// findLast(rule => Wildcard.match(permission, rule.permission) &&
// Wildcard.match(pattern, rule.pattern)), falling back to "ask".

type simRule struct{ key, pattern, action string }

// simDefaults is `defaults` in packages/opencode/src/agent/agent.ts.
var simDefaults = []simRule{
	{"*", "*", "allow"},
	{"doom_loop", "*", "ask"},
	{"external_directory", "*", "ask"},
	{"question", "*", "deny"},
	{"plan_enter", "*", "deny"},
	{"plan_exit", "*", "deny"},
	{"read", "*", "allow"},
	{"read", "*.env", "ask"},
	{"read", "*.env.*", "ask"},
	{"read", "*.env.example", "allow"},
}

// simMatch is packages/opencode/src/util/wildcard.ts match.
func simMatch(input, pattern string) bool {
	escaped := regexp.MustCompile(`[.+^${}()|\[\]\\]`).ReplaceAllString(pattern, `\$0`)
	escaped = strings.ReplaceAll(escaped, "*", ".*")
	escaped = strings.ReplaceAll(escaped, "?", ".")
	if strings.HasSuffix(escaped, " .*") {
		escaped = strings.TrimSuffix(escaped, " .*") + "( .*)?"
	}
	return regexp.MustCompile("(?s)^" + escaped + "$").MatchString(input)
}

func simEvaluate(rules []simRule, key, input string) string {
	for i := len(rules) - 1; i >= 0; i-- {
		if simMatch(key, rules[i].key) && simMatch(input, rules[i].pattern) {
			return rules[i].action
		}
	}
	return "ask"
}

var simSeverity = map[string]int{"allow": 1, "ask": 2, "deny": 3}

// simJSONPermission reads opencode.json's `permission` in file order,
// as fromConfig walks Object.entries.
func simJSONPermission(t *testing.T, path string) []simRule {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return simRulesUnder(doc.Content[0], "permission")
}

// simAgentPermission reads an agent file's frontmatter `permission`.
func simAgentPermission(t *testing.T, path string) []simRule {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(strings.Split(string(data), "---")[1]), &doc); err != nil {
		t.Fatal(err)
	}
	return simRulesUnder(doc.Content[0], "permission")
}

func simRulesUnder(root *yaml.Node, name string) []simRule {
	var out []simRule
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != name {
			continue
		}
		perm := root.Content[i+1]
		if perm.Kind == yaml.ScalarNode {
			return []simRule{{"*", "*", perm.Value}}
		}
		for j := 0; j+1 < len(perm.Content); j += 2 {
			key, value := perm.Content[j].Value, perm.Content[j+1]
			if value.Kind == yaml.ScalarNode {
				out = append(out, simRule{key, "*", value.Value})
				continue
			}
			for k := 0; k+1 < len(value.Content); k += 2 {
				out = append(out, simRule{key, value.Content[k].Value, value.Content[k+1].Value})
			}
		}
	}
	return out
}

func simEmit(t *testing.T, handWritten string, project map[string]any, tools []string) (projectRules, agentRules []simRule) {
	t.Helper()
	dir := testutil.TempCwd(t)
	if handWritten != "" {
		if err := os.WriteFile(filepath.Join(dir, "opencode.json"), []byte(handWritten), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var entries []spec.Entry
	if project != nil {
		entries = append(entries, spec.Entry{Kind: spec.KindSettings, Name: "s", Path: "s.yaml", Meta: map[string]any{
			"x-opencode": map[string]any{"permission": project},
		}})
	}
	toolList := make([]any, len(tools))
	for i, tool := range tools {
		toolList[i] = tool
	}
	entries = append(entries, spec.Entry{Kind: spec.KindAgent, Name: "a", Meta: map[string]any{"description": "d", "tools": toolList}, Body: "x"})
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	return simJSONPermission(t, filepath.Join(dir, "opencode.json")), simAgentPermission(t, filepath.Join(dir, ".opencode/agents/a.md"))
}

// An agent's decision equals the stricter of the project's decision and
// the agent's own list, for every case and input. The list allows its
// grants and denies every other tool; doom_loop and external_directory
// are no tools, so the project decides them alone.
func TestAgentPermission_EqualsProjectWithinTheList(t *testing.T) {
	type probe struct{ key, input string }
	probes := []probe{
		{"bash", "git diff HEAD"}, {"bash", "rm -rf x"}, {"bash", "ls"}, {"bash", "npm i"},
		{"bash", "git status x"}, {"bash", "git log"}, {"bash", "git push main"}, {"bash", "git push --force"},
		{"bash", "git --force"}, {"bash", "git"}, {"bash", "git status"},
		{"read", "src/a.go"}, {"read", "/secrets/k"}, {"read", ".env"}, {"read", "x.env.local"}, {"read", "x.env.example"},
		{"doom_loop", "x"}, {externalDirectoryKey, "/tmp/x"}, {externalDirectoryKey, "/secrets/k"},
		{"edit", "a.go"}, {"webfetch", "https://x"}, {"github_create_issue", "x"}, {"github_search", "x"}, {"question", "x"},
	}
	type grant struct{ key, pattern string }
	cases := []struct {
		name        string
		handWritten string
		project     map[string]any
		tools       []string
		grants      []grant
	}{
		{"no project, read", "", nil, []string{"Read"}, []grant{{"read", "*"}}},
		{"no project, scoped read", "", nil, []string{"Read(src/**)"}, []grant{{"read", "src/**"}}},
		{"tool ask, scoped agent", "", map[string]any{"bash": "ask"}, []string{"Bash(git diff *)"}, []grant{{"bash", "git diff *"}}},
		{"project ask covers agent pattern", "", map[string]any{"bash": map[string]any{"git *": "ask"}}, []string{"Bash(git status *)"}, []grant{{"bash", "git status *"}}},
		{"scoped asks outside and inside the agent", "", map[string]any{"bash": map[string]any{"npm *": "ask", "git push *": "ask"}}, []string{"Bash(git *)"}, []grant{{"bash", "git *"}}},
		{"partial overlap ask", "", map[string]any{"bash": map[string]any{"* --force": "ask"}}, []string{"Bash(git *)"}, []grant{{"bash", "git *"}}},
		{"global ask with a named key", "", map[string]any{"*": "ask", "bash": map[string]any{"rm *": "deny"}}, []string{"Bash(git *)", "Read"}, []grant{{"bash", "git *"}, {"read", "*"}}},
		{"global ask, scoped agent", "", map[string]any{"*": "ask"}, []string{"Bash(git diff *)", "Read"}, []grant{{"bash", "git diff *"}, {"read", "*"}}},
		{"global and doom_loop deny", "", map[string]any{"*": "deny", "doom_loop": "deny"}, []string{"Read"}, []grant{{"read", "*"}}},
		{"global deny, tool scoped deny", "", map[string]any{"*": "deny", "bash": map[string]any{"rm *": "deny"}}, []string{"Bash"}, []grant{{"bash", "*"}}},
		{"tool allow with scoped deny", "", map[string]any{"bash": map[string]any{"*": "allow", "rm *": "deny"}}, []string{"Bash"}, []grant{{"bash", "*"}}},
		{"scoped read deny", "", map[string]any{"read": map[string]any{"/secrets/**": "deny"}}, []string{"Read(src/**)", "Read"}, []grant{{"read", "*"}}},
		{"outside deny under global deny", "", map[string]any{"*": "deny", externalDirectoryKey: map[string]any{"/secrets/**": "deny"}}, []string{"Read"}, []grant{{"read", "*"}}},
		{"outside bare deny", "", map[string]any{externalDirectoryKey: "deny"}, []string{"Edit"}, []grant{{"edit", "*"}}},
		{"wildcard key deny", "", map[string]any{"github_*": "deny"}, []string{"mcp__github__create_issue"}, []grant{{"github_create_issue", "*"}}},
		{"one tool denied, whole server allowed", "", map[string]any{"github_create_issue": "deny"}, []string{"mcp__github"}, []grant{{"github_*", "*"}}},
		{"bare command deny", "", map[string]any{"bash": map[string]any{"git": "deny"}}, []string{"Bash(git *)"}, []grant{{"bash", "git *"}}},
		{"later project allows", "", map[string]any{"*": "ask", "read": "allow", "bash": map[string]any{"*": "ask", "git *": "allow"}}, []string{"Read", "Bash(git:*)"}, []grant{{"read", "*"}, {"bash", "git *"}}},
		{"hand-written opencode.json", `{"permission": {"bash": {"rm *": "deny"}, "webfetch": "deny"}}`, nil, []string{"Bash", "WebFetch"}, []grant{{"bash", "*"}, {"webfetch", "*"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			projectRules, agentRules := simEmit(t, tc.handWritten, tc.project, tc.tools)
			projectFlat := append(append([]simRule{}, simDefaults...), projectRules...)
			agentFlat := append(append([]simRule{}, projectFlat...), agentRules...)
			for _, p := range probes {
				list := "deny"
				if p.key == "doom_loop" || p.key == externalDirectoryKey {
					list = "allow"
				}
				for _, g := range tc.grants {
					if simMatch(p.key, g.key) && simMatch(p.input, g.pattern) {
						list = "allow"
					}
				}
				want := simEvaluate(projectFlat, p.key, p.input)
				if simSeverity[list] > simSeverity[want] {
					want = list
				}
				if got := simEvaluate(agentFlat, p.key, p.input); got != want {
					t.Errorf("%s %q: agent %s, want %s\nproject %v\nagent %v", p.key, p.input, got, want, projectRules, agentRules)
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
		{"git *", "git push x", true},
		{"git *", "git", true},
		{"git *", "npm i", false},
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

func TestWildcardContainsAndOverlap(t *testing.T) {
	for _, tc := range []struct {
		a, b              string
		contains, overlap bool
	}{
		{"git *", "git push *", true, true},
		{"git push *", "git *", false, true},
		{"* --force", "git *", false, true},
		{"npm *", "git *", false, false},
		{"g?t", "g*t", false, true},
		{"*", "*.env", true, true},
		{"github_*", "github_create_issue", true, true},
	} {
		if got := wildcardContains(tc.a, tc.b); got != tc.contains {
			t.Errorf("wildcardContains(%q, %q) = %t, want %t", tc.a, tc.b, got, tc.contains)
		}
		if got := wildcardOverlap(tc.a, tc.b); got != tc.overlap {
			t.Errorf("wildcardOverlap(%q, %q) = %t, want %t", tc.a, tc.b, got, tc.overlap)
		}
	}
}
