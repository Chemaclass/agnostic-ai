package kilo

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
	"gopkg.in/yaml.v3"
)

func TestSettingsPermission_WriteRulesLandOnEdit(t *testing.T) {
	settings := []spec.Entry{{Kind: spec.KindSettings, Meta: map[string]any{"permissions": map[string]any{"allow": []any{"Write"}, "deny": []any{"Edit(.env)"}}}}}
	got, dropped := settingsPermission(settings)
	if dropped != 0 {
		t.Errorf("dropped = %d", dropped)
	}
	if _, set := got["write"]; set {
		t.Errorf("Kilo's write tool checks edit, so no write key: %#v", got)
	}
	edits, ok := got["edit"].(*emit.OrderedJSON)
	if !ok {
		t.Fatalf("edit permissions = %#v", got["edit"])
	}
	allow, _ := edits.Get("*")
	deny, _ := edits.Get(".env")
	if string(allow) != `"allow"` || string(deny) != `"deny"` || !slices.Equal(edits.Keys(), []string{"*", ".env"}) {
		t.Errorf("edit permissions = %v", edits.Keys())
	}
}

func TestTranslatePermission_WriteAndEditShareEdit(t *testing.T) {
	for _, tc := range []struct {
		list, rule string
		want       []string
	}{
		{"deny", "Edit", []string{"edit"}},
		{"deny", "Edit(.env)", []string{"edit(.env)"}},
		{"allow", "Write", []string{"edit"}},
		{"ask", "Write(go.mod)", []string{"edit(go.mod)"}},
	} {
		t.Run(tc.list+"/"+tc.rule, func(t *testing.T) {
			got, ok := (Adapter{}).TranslatePermission(tc.list, tc.rule)
			if !ok || !slices.Equal(got, tc.want) {
				t.Errorf("translation = %v, %t, want %v", got, ok, tc.want)
			}
		})
	}
}

// emitAgent runs a whole Kilo emit with one agent and an optional
// settings spec and hand-written kilo.jsonc, and returns kilo.jsonc and
// the agent file.
func emitAgent(t *testing.T, settings map[string]any, hand string, tools ...any) (jsonc, md string) {
	t.Helper()
	dir := testutil.TempCwd(t)
	if hand != "" {
		if err := os.WriteFile(filepath.Join(dir, "kilo.jsonc"), []byte(hand), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "a", Body: "body", Meta: map[string]any{"tools": tools}}}
	if settings != nil {
		entries = append(entries, spec.Entry{Kind: spec.KindSettings, Name: "s", Meta: settings})
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "kilo.jsonc"))
	agent, err := os.ReadFile(filepath.Join(dir, ".kilo", "agents", "a.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw), string(agent)
}

func TestAgent_QuotesYAMLLookalikePatterns(t *testing.T) {
	_, md := emitAgent(t, map[string]any{"permissions": map[string]any{"deny": []any{"Read(~)", "Read(yes)"}}}, "", "Read")
	for _, want := range []string{`"~": deny`, `"yes": deny`} {
		if !strings.Contains(md, want) {
			t.Errorf("want %q in:\n%s", want, md)
		}
	}
}

func TestAgent_ExplicitShapes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings map[string]any
		tools    []any
		want     []string
	}{
		{"project ask on a listed tool stays ask", map[string]any{"permissions": map[string]any{"ask": []any{"Bash"}}}, []any{"Bash"}, []string{"\n  bash: ask\n"}},
		{"bare Bash asks by default like Kilo", nil, []any{"Bash"}, []string{"\n  bash:\n", "    cat *: allow\n", "    \"*\": ask\n"}},
		{"Read keeps Kilo's .env ask", nil, []any{"Read"}, []string{"    \"*.env\": ask\n", "    \"*.env.example\": allow\n"}},
		{"a project deny after an overlapping ask still wins", map[string]any{"permissions": map[string]any{"ask": []any{"Bash(* --force)"}, "deny": []any{"Bash(git:*)"}}}, []any{"Bash(git:*)"}, []string{"\n  bash: deny\n"}},
		{"a key pattern deny covers the agent's key", map[string]any{"x-kilo": map[string]any{"permission": map[string]any{"github_*": "deny"}}}, []any{"mcp__github__create_issue"}, []string{"\n  github_create_issue: deny\n"}},
		{"Write lands on edit", nil, []any{"Write"}, []string{"\n  edit: allow\n"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, md := emitAgent(t, tc.settings, "", tc.tools...)
			for _, want := range tc.want {
				if !strings.Contains(md, want) {
					t.Errorf("want %q in:\n%s", want, md)
				}
			}
		})
	}
}

// TestAgent_MatchesProjectWithinItsList checks every sample against a
// simulation of Kilo's own evaluation, written apart from the adapter:
// the agent's decision is exactly the stricter of the project's (with
// Kilo's defaults) and the agent's bare allowlist.
func TestAgent_MatchesProjectWithinItsList(t *testing.T) {
	samples := map[string][]string{
		"bash":                {"git status", "git push origin", "git --force", "git push --force", "npm install", "rm -r tmp", "ls", "ls -la", "git", "cat a", "git diff"},
		"read":                {"src/a.go", "src/.env", "src/x.env.local", "src/a.env.example", `src\win.env`, "/secrets/key", "docs/x.md", "~", "yes", ".env", "x.env.local", "a.env.example"},
		"edit":                {".env", "docs/a.md", "src/a.go"},
		"github_create_issue": {"x"},
		"doom_loop":           {"x"},
		"external_directory":  {"/etc/passwd", "/tmp/x"},
		"question":            {"x"},
		"grep":                {"x"},
	}
	native := func(permission map[string]any) map[string]any {
		return map[string]any{"x-kilo": map[string]any{"permission": permission}}
	}
	portable := func(lists map[string]any) map[string]any {
		return map[string]any{"permissions": lists}
	}
	for _, tc := range []struct {
		settings map[string]any
		hand     string
		tools    []any
	}{
		{portable(map[string]any{"ask": []any{"Bash(npm:*)", "Bash(git push:*)"}, "deny": []any{"Bash(rm:*)"}}), "", []any{"Bash(git:*)"}},
		{portable(map[string]any{"ask": []any{"Bash(git:*)"}}), "", []any{"Bash(git status:*)", "Read(src/**)"}},
		{portable(map[string]any{"ask": []any{"Bash"}, "deny": []any{"Read(/secrets/**)"}}), "", []any{"Bash(git:*)", "Read"}},
		{portable(map[string]any{"allow": []any{"Bash", "Read"}}), "", []any{"Bash(ls:*)", "Read(docs/**)"}},
		{portable(map[string]any{"deny": []any{"Read(~)", "Read(yes)"}}), "", []any{"Read"}},
		{native(map[string]any{"*": "ask", "read": map[string]any{"*": "allow", "~": "deny"}}), "", []any{"Bash", "Read"}},
		{native(map[string]any{"bash": map[string]any{"*": "allow", "*push*": "ask"}}), "", []any{"Bash(git:*)"}},
		{native(map[string]any{"*": "ask", "bash": map[string]any{"git *": "allow"}}), "", []any{"Bash"}},
		{native(map[string]any{"github_*": "deny"}), "", []any{"mcp__github__create_issue"}},
		{native(map[string]any{"bash": map[string]any{"* --force": "ask"}}), "", []any{"Bash(git:*)"}},
		{nil, "", []any{"Read", "Bash", "Grep"}},
		{nil, "", []any{"Read(src/**)"}},
		{native(map[string]any{"doom_loop": "deny", "external_directory": map[string]any{"/etc/**": "deny"}}), "", []any{"Read"}},
		{native(map[string]any{"read": "allow"}), "", []any{"Read"}},
		{portable(map[string]any{"ask": []any{"Bash(* --force)"}, "deny": []any{"Bash(git:*)"}}), "", []any{"Bash(git:*)"}},
		{native(map[string]any{"bash": map[string]any{"* --force": "ask", "git *": "deny"}}), "", []any{"Bash(git:*)"}},
		{portable(map[string]any{"deny": []any{"Edit(.env)"}}), "", []any{"Write"}},
		{portable(map[string]any{"deny": []any{"Edit(.env)"}}), "", []any{"Edit(docs/**)"}},
		{nil, `{"permission": {"bash": {"git push *": "deny"}}}`, []any{"Bash(git:*)"}},
	} {
		jsonc, md := emitAgent(t, tc.settings, tc.hand, tc.tools...)
		project := append(simDefaults(), simJSONRules(t, jsonc)...)
		agent := append(slices.Clone(project), simAgentRules(t, md)...)
		own := simOwnRules(tc.tools)
		for key, inputs := range samples {
			for _, input := range inputs {
				want := simStricter(simEvaluate(project, key, input), simEvaluate(own, key, input))
				if got := simEvaluate(agent, key, input); got != want {
					t.Errorf("tools %v, settings %v, hand %q, %s %q: got %s, want %s\n%s", tc.tools, tc.settings, tc.hand, key, input, got, want, md)
				}
			}
		}
	}
}

// simRule is one entry of the flat list Kilo evaluates.
type simRule struct{ key, pattern, action string }

// simDefaults copies the rules Kilo puts before every config, in order:
// `baseDefaults` in packages/opencode/src/agent/agent.ts, then
// `defaultsPatch` in packages/opencode/src/kilocode/agent/index.ts
// (board off, no editor client).
func simDefaults() []simRule {
	rules := []simRule{
		{"*", "*", "allow"},
		{"doom_loop", "*", "ask"},
		{"external_directory", "*", "ask"},
		{"suggest", "*", "deny"},
		{"question", "*", "deny"},
		{"plan_enter", "*", "deny"},
		{"plan_exit", "*", "deny"},
		{"repo_clone", "*", "deny"},
		{"repo_overview", "*", "deny"},
		{"read", "*", "allow"},
		{"read", "*.env", "ask"},
		{"read", "*.env.*", "ask"},
		{"read", "*.env.example", "allow"},
		{"bash", "*", "ask"},
	}
	for _, cmd := range []string{"cat", "head", "tail", "less", "ls", "tree", "pwd", "echo", "wc", "which", "type", "file", "diff", "du", "df", "date", "uname", "whoami", "printenv", "man", "grep", "rg", "ag", "sort", "uniq", "cut", "tr", "jq", "touch", "mkdir", "cp", "mv", "tsc", "tsgo", "tar", "unzip", "gzip", "gunzip"} {
		rules = append(rules, simRule{"bash", cmd + " *", "allow"})
	}
	return append(rules,
		simRule{"recall", "*", "ask"},
		simRule{"kilo_memory_recall", "*", "ask"},
		simRule{"kilo_memory_save", "*", "ask"},
	)
}

// simOwnRules is the agent's bare allowlist: deny everything, allow each
// listed tool, and ask for doom_loop and external_directory.
func simOwnRules(tools []any) []simRule {
	rules := []simRule{{"*", "*", "deny"}, {"doom_loop", "*", "ask"}, {"external_directory", "*", "ask"}}
	keys := map[string]string{"Bash": "bash", "Read": "read", "Edit": "edit", "Write": "edit", "Grep": "grep"}
	for _, tool := range tools {
		name := tool.(string)
		if server, rest, ok := strings.Cut(strings.TrimPrefix(name, "mcp__"), "__"); ok {
			rules = append(rules, simRule{server + "_" + rest, "*", "allow"})
			continue
		}
		head, arg, scoped := strings.Cut(strings.TrimSuffix(name, ")"), "(")
		if !scoped {
			rules = append(rules, simRule{keys[head], "*", "allow"})
			continue
		}
		if prefix, ok := strings.CutSuffix(arg, ":*"); ok {
			arg = prefix + " *"
		}
		rules = append(rules, simRule{keys[head], arg, "allow"})
	}
	// A scoped tool denies the rest of its key; a bare one allows it all.
	var out []simRule
	for _, rule := range rules {
		if rule.pattern != "*" && !slices.Contains(out, simRule{rule.key, "*", "allow"}) && !slices.Contains(out, simRule{rule.key, "*", "deny"}) {
			out = append(out, simRule{rule.key, "*", "deny"})
		}
		out = append(out, rule)
	}
	return out
}

// simJSONRules reads the `permission` object of kilo.jsonc in file order.
func simJSONRules(t *testing.T, jsonc string) []simRule {
	t.Helper()
	if jsonc == "" {
		return nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(jsonc), &doc); err != nil {
		t.Fatal(err)
	}
	return simMapping(simChild(doc.Content[0], "permission"))
}

// simAgentRules reads the `permission` map from the agent frontmatter in
// its written order.
func simAgentRules(t *testing.T, md string) []simRule {
	t.Helper()
	parts := strings.SplitN(md, "---\n", 3)
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(parts[1]), &doc); err != nil {
		t.Fatal(err)
	}
	return simMapping(simChild(doc.Content[0], "permission"))
}

func simChild(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func simMapping(m *yaml.Node) []simRule {
	if m == nil {
		return nil
	}
	var rules []simRule
	for i := 0; i+1 < len(m.Content); i += 2 {
		key, value := m.Content[i].Value, m.Content[i+1]
		if value.Kind == yaml.ScalarNode {
			rules = append(rules, simRule{key, "*", value.Value})
			continue
		}
		for j := 0; j+1 < len(value.Content); j += 2 {
			rules = append(rules, simRule{key, value.Content[j].Value, value.Content[j+1].Value})
		}
	}
	return rules
}

// simEvaluate is Kilo's Permission.evaluate: the last rule whose key and
// pattern both match wins, and "ask" when none does.
func simEvaluate(rules []simRule, key, input string) string {
	action := "ask"
	for _, rule := range rules {
		if simMatch(key, rule.key) && simMatch(input, rule.pattern) {
			action = rule.action
		}
	}
	return action
}

func simStricter(a, b string) string {
	rank := map[string]int{"allow": 0, "ask": 1, "deny": 2}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

// simMatch is Kilo's Wildcard.match (packages/opencode/src/util/wildcard.ts).
func simMatch(input, pattern string) bool {
	input = strings.ReplaceAll(input, `\`, "/")
	pattern = strings.ReplaceAll(pattern, `\`, "/")
	escaped := regexp.QuoteMeta(pattern)
	escaped = strings.ReplaceAll(escaped, `\*`, ".*")
	escaped = strings.ReplaceAll(escaped, `\?`, ".")
	if strings.HasSuffix(escaped, " .*") {
		escaped = strings.TrimSuffix(escaped, " .*") + "( .*)?"
	}
	return regexp.MustCompile("(?s)^" + escaped + "$").MatchString(input)
}
