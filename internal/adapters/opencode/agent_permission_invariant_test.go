package opencode

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
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

// simMatch is packages/opencode/src/util/wildcard.ts match: backslashes
// become slashes, and windows ignores case as OpenCode does there.
func simMatch(input, pattern string, windows bool) bool {
	input = strings.ReplaceAll(input, `\`, "/")
	pattern = strings.ReplaceAll(pattern, `\`, "/")
	flags := "(?s)"
	if windows {
		flags = "(?si)"
	}
	escaped := regexp.MustCompile(`[.+^${}()|\[\]\\]`).ReplaceAllString(pattern, `\$0`)
	escaped = strings.ReplaceAll(escaped, "*", ".*")
	escaped = strings.ReplaceAll(escaped, "?", ".")
	if strings.HasSuffix(escaped, " .*") {
		escaped = strings.TrimSuffix(escaped, " .*") + "( .*)?"
	}
	return regexp.MustCompile(flags + "^" + escaped + "$").MatchString(input)
}

func simEvaluate(rules []simRule, key, input string, windows bool) string {
	for i := len(rules) - 1; i >= 0; i-- {
		if simMatch(key, rules[i].key, windows) && simMatch(input, rules[i].pattern, windows) {
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

func simEmit(t *testing.T, handWritten string, project map[string]any, mcps, tools []string) (projectRules, agentRules []simRule) {
	t.Helper()
	dir := testutil.TempCwd(t)
	if handWritten != "" {
		if err := os.WriteFile(filepath.Join(dir, "opencode.json"), []byte(handWritten), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var entries []spec.Entry
	for _, server := range mcps {
		entries = append(entries, spec.Entry{Kind: spec.KindMCP, Name: server, Meta: map[string]any{"command": "x"}})
	}
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
// the agent's own list, for every case and input, on Windows and on
// case-sensitive systems alike, except that where only case decides, it
// may be stricter on one of them. It is never looser. The list allows its
// grants and denies every other tool; doom_loop and external_directory
// are no tools, so the project decides them alone.
func TestAgentPermission_EqualsProjectWithinTheList(t *testing.T) {
	type probe struct{ key, input string }
	probes := []probe{
		{"bash", "git diff HEAD"}, {"bash", "rm -rf x"}, {"bash", "ls"}, {"bash", "npm i"},
		{"bash", "git status x"}, {"bash", "git log"}, {"bash", "git push main"}, {"bash", "git push --force"},
		{"bash", "git --force"}, {"bash", "git"}, {"bash", "git status"},
		{"read", "src/a.go"}, {"read", "SRC/a.go"}, {"read", `src\a.go`}, {"bash", "GIT push"}, {"read", "/secrets/k"}, {"read", ".env"}, {"read", "x.env.local"}, {"read", "x.env.example"},
		{"doom_loop", "x"}, {externalDirectoryKey, "/tmp/x"}, {externalDirectoryKey, "/secrets/k"},
		{"edit", "a.go"}, {"webfetch", "https://x"}, {"github_create_issue", "x"}, {"github_search", "x"}, {"github_ent_search", "x"}, {"question", "x"},
	}
	type grant struct{ key, pattern string }
	cases := []struct {
		name        string
		handWritten string
		project     map[string]any
		tools       []string
		grants      []grant
		mcps        []string
		notGranted  []grant
	}{
		{"no project, read", "", nil, []string{"Read"}, []grant{{"read", "*"}}, nil, nil},
		{"no project, scoped read", "", nil, []string{"Read(src/**)"}, []grant{{"read", "src/**"}}, nil, nil},
		{"tool ask, scoped agent", "", map[string]any{"bash": "ask"}, []string{"Bash(git diff *)"}, []grant{{"bash", "git diff *"}}, nil, nil},
		{"project ask covers agent pattern", "", map[string]any{"bash": map[string]any{"git *": "ask"}}, []string{"Bash(git status *)"}, []grant{{"bash", "git status *"}}, nil, nil},
		{"scoped asks outside and inside the agent", "", map[string]any{"bash": map[string]any{"npm *": "ask", "git push *": "ask"}}, []string{"Bash(git *)"}, []grant{{"bash", "git *"}}, nil, nil},
		{"partial overlap ask", "", map[string]any{"bash": map[string]any{"* --force": "ask"}}, []string{"Bash(git *)"}, []grant{{"bash", "git *"}}, nil, nil},
		{"global ask with a named key", "", map[string]any{"*": "ask", "bash": map[string]any{"rm *": "deny"}}, []string{"Bash(git *)", "Read"}, []grant{{"bash", "git *"}, {"read", "*"}}, nil, nil},
		{"global ask, scoped agent", "", map[string]any{"*": "ask"}, []string{"Bash(git diff *)", "Read"}, []grant{{"bash", "git diff *"}, {"read", "*"}}, nil, nil},
		{"global and doom_loop deny", "", map[string]any{"*": "deny", "doom_loop": "deny"}, []string{"Read"}, []grant{{"read", "*"}}, nil, nil},
		{"global deny, tool scoped deny", "", map[string]any{"*": "deny", "bash": map[string]any{"rm *": "deny"}}, []string{"Bash"}, []grant{{"bash", "*"}}, nil, nil},
		{"tool allow with scoped deny", "", map[string]any{"bash": map[string]any{"*": "allow", "rm *": "deny"}}, []string{"Bash"}, []grant{{"bash", "*"}}, nil, nil},
		{"scoped read deny", "", map[string]any{"read": map[string]any{"/secrets/**": "deny"}}, []string{"Read(src/**)", "Read"}, []grant{{"read", "*"}}, nil, nil},
		{"outside deny under global deny", "", map[string]any{"*": "deny", externalDirectoryKey: map[string]any{"/secrets/**": "deny"}}, []string{"Read"}, []grant{{"read", "*"}}, nil, nil},
		{"outside bare deny", "", map[string]any{externalDirectoryKey: "deny"}, []string{"Edit"}, []grant{{"edit", "*"}}, nil, nil},
		{"wildcard key deny", "", map[string]any{"github_*": "deny"}, []string{"mcp__github__create_issue"}, []grant{{"github_create_issue", "*"}}, nil, nil},
		{"one tool denied, whole server allowed", "", map[string]any{"github_create_issue": "deny"}, []string{"mcp__github"}, []grant{{"github_*", "*"}}, nil, nil},
		{"bare command deny", "", map[string]any{"bash": map[string]any{"git": "deny"}}, []string{"Bash(git *)"}, []grant{{"bash", "git *"}}, nil, nil},
		{"later project allows", "", map[string]any{"*": "ask", "read": "allow", "bash": map[string]any{"*": "ask", "git *": "allow"}}, []string{"Read", "Bash(git:*)"}, []grant{{"read", "*"}, {"bash", "git *"}}, nil, nil},
		{"whole server leaves a prefixed server out", "", nil, []string{"mcp__github"}, []grant{{"github_*", "*"}}, []string{"github", "github_ent"}, []grant{{"github_ent_*", "*"}}},
		{"deny differs only in case", "", map[string]any{"read": map[string]any{"SRC/**": "deny"}}, []string{"Read(src/**)"}, []grant{{"read", "src/**"}}, nil, nil},
		{"allow differs only in case", "", map[string]any{"read": map[string]any{"*": "deny", "SRC/**": "allow"}}, []string{"Read(src/**)"}, []grant{{"read", "src/**"}}, nil, nil},
		{"ask differs only in case", "", map[string]any{"bash": map[string]any{"GIT *": "ask"}}, []string{"Bash(git *)"}, []grant{{"bash", "git *"}}, nil, nil},
		{"hand-written opencode.json", `{"permission": {"bash": {"rm *": "deny"}, "webfetch": "deny"}}`, nil, []string{"Bash", "WebFetch"}, []grant{{"bash", "*"}, {"webfetch", "*"}}, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			projectRules, agentRules := simEmit(t, tc.handWritten, tc.project, tc.mcps, tc.tools)
			projectFlat := append(append([]simRule{}, simDefaults...), projectRules...)
			agentFlat := append(append([]simRule{}, projectFlat...), agentRules...)
			for _, p := range probes {
				want := func(windows bool) string {
					list := "deny"
					if p.key == "doom_loop" || p.key == externalDirectoryKey {
						list = "allow"
					}
					for _, g := range tc.grants {
						if simMatch(p.key, g.key, windows) && simMatch(p.input, g.pattern, windows) {
							list = "allow"
						}
					}
					for _, g := range tc.notGranted {
						if simMatch(p.key, g.key, windows) && simMatch(p.input, g.pattern, windows) {
							list = "deny"
						}
					}
					project := simEvaluate(projectFlat, p.key, p.input, windows)
					if simSeverity[list] > simSeverity[project] {
						return list
					}
					return project
				}
				// Where case decides, the agent may be stricter on one OS.
				caseDecides := want(true) != want(false)
				for _, windows := range []bool{false, true} {
					w, got := want(windows), simEvaluate(agentFlat, p.key, p.input, windows)
					if simSeverity[got] < simSeverity[w] || (!caseDecides && got != w) {
						t.Errorf("%s %q (windows %t): agent %s, want %s\nproject %v\nagent %v", p.key, p.input, windows, got, w, projectRules, agentRules)
					}
				}
			}
		})
	}
}

func TestWildcardContains_Literals(t *testing.T) {
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
		if got := wildcardContains(tc.pattern, tc.s, false); got != tc.want {
			t.Errorf("wildcardContains(%q, %q) = %t, want %t", tc.pattern, tc.s, got, tc.want)
		}
	}
}

func TestWildcardContainsAndOverlap(t *testing.T) {
	for _, tc := range []struct {
		a, b              string
		fold              bool
		contains, overlap bool
	}{
		{"git *", "git push *", false, true, true},
		{"git push *", "git *", false, false, true},
		{"* --force", "git *", false, false, true},
		{"npm *", "git *", false, false, false},
		{"g?t", "g*t", false, false, true},
		{"*", "*.env", false, true, true},
		{"github_*", "github_create_issue", false, true, true},
		{"SRC/*", "src/*", false, false, false},
		{"SRC/*", "src/*", true, true, true},
	} {
		if got := wildcardContains(tc.a, tc.b, tc.fold); got != tc.contains {
			t.Errorf("wildcardContains(%q, %q, %t) = %t, want %t", tc.a, tc.b, tc.fold, got, tc.contains)
		}
		if got := wildcardOverlap(tc.a, tc.b, tc.fold); got != tc.overlap {
			t.Errorf("wildcardOverlap(%q, %q, %t) = %t, want %t", tc.a, tc.b, tc.fold, got, tc.overlap)
		}
	}
}

func TestWildcardByteEqual_FoldsCaseAndSlashes(t *testing.T) {
	for _, tc := range []struct {
		a, b     byte
		fold     bool
		expected bool
	}{
		{'\\', '/', false, true},
		{'A', 'a', false, false},
		{'A', 'a', true, true},
		{'a', 'b', true, false},
	} {
		if got := wildcardByteEqual(tc.a, tc.b, tc.fold); got != tc.expected {
			t.Errorf("wildcardByteEqual(%q, %q, %t) = %t, want %t", tc.a, tc.b, tc.fold, got, tc.expected)
		}
	}
	if !wildcardContains("SRC/a.go", `src\a.go`, true) {
		t.Error("folded, SRC/a.go must cover src\\a.go")
	}
	if got := globIntersection("SRC/**", "src/**", true); len(got) != 1 || got[0] != "src/**" {
		t.Errorf("a folded intersection keeps the grant's case, got %v", got)
	}
}
