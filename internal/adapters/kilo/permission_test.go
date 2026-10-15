package kilo

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"gopkg.in/yaml.v3"
)

func TestSettingsPermission_EditDenyAlsoBlocksWrite(t *testing.T) {
	settings := []spec.Entry{{Kind: spec.KindSettings, Meta: map[string]any{"permissions": map[string]any{"allow": []any{"Write"}, "deny": []any{"Edit(.env)"}}}}}
	got, dropped := settingsPermission(settings)
	if dropped != 0 {
		t.Errorf("dropped = %d", dropped)
	}
	writes, ok := got["write"].(*emit.OrderedJSON)
	if !ok {
		t.Fatalf("write permissions = %#v", got["write"])
	}
	allow, _ := writes.Get("*")
	deny, _ := writes.Get(".env")
	if string(allow) != `"allow"` || string(deny) != `"deny"` {
		t.Errorf("write permissions = %#v", got["write"])
	}
}

func TestAgentPermission_PathRestriction(t *testing.T) {
	got, dropped := agentPermission([]string{"Read(src/**)", "Edit(docs/**)"}, nil)
	if dropped {
		t.Error("path-scoped tools dropped")
	}
	if read := permissionJSON(t, got["read"]); !strings.HasPrefix(read, `{"*":"deny","src/**":"allow"`) {
		t.Errorf("read permission = %s", read)
	}
}

func TestTranslatePermission_EditRestrictionsCoverWrite(t *testing.T) {
	for _, tc := range []struct {
		list, rule string
		want       []string
	}{
		{"deny", "Edit", []string{"edit", "write"}},
		{"ask", "Edit", []string{"edit", "write"}},
		{"deny", "Edit(.env)", []string{"edit(.env)", "write(.env)"}},
		{"ask", "Edit(go.mod)", []string{"edit(go.mod)", "write(go.mod)"}},
		{"allow", "Edit(docs/**)", []string{"edit(docs/**)"}},
	} {
		t.Run(tc.list+"/"+tc.rule, func(t *testing.T) {
			got, ok := (Adapter{}).TranslatePermission(tc.list, tc.rule)
			if !ok || !slices.Equal(got, tc.want) {
				t.Errorf("translation = %v, %t, want %v", got, ok, tc.want)
			}
		})
	}
}

func TestAgentPermission_KeepsAskDefaultsAfterCatchAll(t *testing.T) {
	got, _ := agentPermission([]string{"Read"}, nil)
	if got["*"] != "deny" {
		t.Errorf("permission = %#v", got)
	}
	if read := permissionJSON(t, got["read"]); read != `{"*":"allow","*.env":"ask","*.env.*":"ask"}` {
		t.Errorf("read = %s, want Kilo's .env asks after the allow", read)
	}
	for _, key := range []string{"doom_loop", "external_directory"} {
		if got[key] != "ask" {
			t.Errorf("%s = %#v, want ask", key, got[key])
		}
	}
}

func agentFrontmatter(t *testing.T, settings map[string]any, tools ...any) string {
	t.Helper()
	project, _ := settingsPermission([]spec.Entry{{Kind: spec.KindSettings, Meta: map[string]any{"permissions": settings}}})
	md, _ := agentMarkdown(spec.Entry{Kind: spec.KindAgent, Name: "a", Meta: map[string]any{"tools": tools}}, project)
	return md
}

func TestAgentMarkdown_ProjectAskWinsOverAgentAllow(t *testing.T) {
	got := agentFrontmatter(t, map[string]any{"ask": []any{"Bash"}}, "Bash")
	if !strings.Contains(got, "\n  bash: ask\n") {
		t.Errorf("agent Bash must stay ask:\n%s", got)
	}
}

func TestAgentMarkdown_ProjectDenyPatternSurvives(t *testing.T) {
	got := agentFrontmatter(t, map[string]any{"allow": []any{"Read(src/**)"}, "deny": []any{"Read(/secrets/**)"}}, "Read")
	if !strings.Contains(got, "\n    /secrets/**: deny\n") || strings.Contains(got, "src/**") {
		t.Errorf("read rules lost the project deny or copied its allow:\n%s", got)
	}
}

func TestAgentMarkdown_ScopedAllowKeepsProjectDenyLast(t *testing.T) {
	got := agentFrontmatter(t, map[string]any{"deny": []any{"Bash(git clean:*)"}}, "Bash(git:*)")
	want := "\n  bash:\n    \"*\": deny\n    git *: allow\n    git clean *: deny\n"
	if !strings.Contains(got, want) {
		t.Errorf("want %q in:\n%s", want, got)
	}
}

func TestAgentMarkdown_QuotesYAMLLookalikePatterns(t *testing.T) {
	got := agentFrontmatter(t, map[string]any{"deny": []any{"Read(~)", "Read(yes)"}}, "Read")
	for _, want := range []string{`"~": deny`, `"yes": deny`} {
		if !strings.Contains(got, want) {
			t.Errorf("want %q in:\n%s", want, got)
		}
	}
}

func TestAgentMarkdown_NoProjectRuleKeepsBareAllow(t *testing.T) {
	got := agentFrontmatter(t, map[string]any{"deny": []any{"WebFetch"}}, "Bash")
	if !strings.Contains(got, "\n  bash: allow\n") {
		t.Errorf("bash should stay a bare allow:\n%s", got)
	}
}

func TestAgentPermission_NeverLooserThanProject(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings map[string]any
		native   map[string]any
		tools    []string
		key      string
		want     string
	}{
		{"tool ask stays ask", map[string]any{"ask": []any{"Bash"}}, nil, []string{"Bash"}, "bash", `"ask"`},
		{"tool catch-all ask applies in place on a scoped allow", map[string]any{"ask": []any{"Bash"}}, nil, []string{"Bash(git diff:*)"}, "bash", `{"*":"deny","git diff *":"ask"}`},
		{"project-wide ask applies in place", nil, map[string]any{"*": "ask"}, []string{"Bash(git diff:*)"}, "bash", `{"*":"deny","git diff *":"ask"}`},
		{"project-wide ask on a bare allow", nil, map[string]any{"*": "ask"}, []string{"Bash"}, "bash", `"ask"`},
		{"pattern deny follows the allow", map[string]any{"deny": []any{"Read(/secrets/**)"}}, nil, []string{"Read"}, "read", `{"*":"allow","*.env":"ask","*.env.*":"ask","/secrets/**":"deny"}`},
		{"project allow is not copied", map[string]any{"allow": []any{"Read(docs/**)"}}, nil, []string{"Read(src/**)"}, "read", `{"*":"deny","src/**":"allow","src/*.env":"ask","src/*.env.*":"ask"}`},
		{"ask outside the agent's patterns stays denied", map[string]any{"ask": []any{"Bash(npm:*)"}}, nil, []string{"Bash(git:*)"}, "bash", `{"*":"deny","git *":"allow"}`},
		{"ask inside an agent pattern follows it", map[string]any{"ask": []any{"Bash(git push:*)"}}, nil, []string{"Bash(git:*)"}, "bash", `{"*":"deny","git *":"allow","git push *":"ask"}`},
		{"ask covering an agent pattern raises it", map[string]any{"ask": []any{"Bash(git:*)"}}, nil, []string{"Bash(git status:*)"}, "bash", `{"*":"deny","git status *":"ask"}`},
		{"deny outside the agent's patterns still follows", map[string]any{"deny": []any{"Bash(rm:*)"}}, nil, []string{"Bash(git:*)"}, "bash", `{"*":"deny","git *":"allow"}`},
		{"project-wide ask still applies to a key the project names", nil, map[string]any{"*": "ask", "bash": map[string]any{"git *": "allow"}}, []string{"Bash"}, "bash", `"ask"`},
		{"project deny on a key pattern covers a matching key", nil, map[string]any{"github_*": "deny"}, []string{"mcp__github__create_issue"}, "github_create_issue", `"deny"`},
		{"partial overlap keeps only the intersection", nil, map[string]any{"bash": map[string]any{"* --force": "ask"}}, []string{"Bash(git:*)"}, "bash", `{"*":"deny","git *":"allow","git * --force":"ask","git --force":"ask"}`},
		{"Kilo asks before reading .env", nil, nil, []string{"Read"}, "read", `{"*":"allow","*.env":"ask","*.env.*":"ask"}`},
		{"a project .env allow overrides Kilo's ask", nil, map[string]any{"read": map[string]any{"*.env": "allow", "*.env.*": "allow"}}, []string{"Read"}, "read", `"allow"`},
		{"doom_loop deny wins over the ask default", nil, map[string]any{"doom_loop": "deny"}, []string{"Read"}, "doom_loop", `"deny"`},
		{"external_directory deny pattern survives", nil, map[string]any{"external_directory": map[string]any{"/etc/**": "deny"}}, []string{"Read"}, "external_directory", `{"*":"ask","/etc/**":"deny"}`},
		{"external_directory allow does not loosen", nil, map[string]any{"external_directory": "allow"}, []string{"Read"}, "external_directory", `"ask"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			meta := map[string]any{}
			if tc.settings != nil {
				meta["permissions"] = tc.settings
			}
			if tc.native != nil {
				meta["x-kilo"] = map[string]any{"permission": tc.native}
			}
			project, _ := settingsPermission([]spec.Entry{{Kind: spec.KindSettings, Meta: meta}})
			perms, _ := agentPermission(tc.tools, project)
			if got := permissionJSON(t, perms[tc.key]); got != tc.want {
				t.Errorf("%s = %s, want %s", tc.key, got, tc.want)
			}
		})
	}
}

func permissionJSON(t *testing.T, value any) string {
	t.Helper()
	node, ok := value.(*yaml.Node)
	if !ok {
		out, _ := json.Marshal(value)
		return string(out)
	}
	ordered := emit.NewOrderedJSON()
	for i := 0; i+1 < len(node.Content); i += 2 {
		_ = ordered.Set(node.Content[i].Value, node.Content[i+1].Value)
	}
	out, _ := json.Marshal(ordered)
	return string(out)
}

func TestAgentPermission_NeverLooserThanProjectOrOwnList(t *testing.T) {
	samples := map[string][]string{
		"bash":                {"git status", "git push origin", "git --force", "git push --force", "npm install", "rm -r tmp", "ls", "git"},
		"read":                {"src/a.go", "src/.env", "src/x.env.local", "/secrets/key", "docs/x.md", "~", ".env", "x.env.local", "a.env.example"},
		"github_create_issue": {"x"},
		"doom_loop":           {"x"},
		"external_directory":  {"/etc/passwd", "/tmp/x"},
		"question":            {"x"},
	}
	for _, tc := range []struct {
		settings map[string]any
		native   map[string]any
		tools    []any
	}{
		{map[string]any{"ask": []any{"Bash(npm:*)", "Bash(git push:*)"}, "deny": []any{"Bash(rm:*)"}}, nil, []any{"Bash(git:*)"}},
		{map[string]any{"ask": []any{"Bash(git:*)"}}, nil, []any{"Bash(git status:*)", "Read(src/**)"}},
		{map[string]any{"ask": []any{"Bash"}, "deny": []any{"Read(/secrets/**)"}}, nil, []any{"Bash(git:*)", "Read"}},
		{map[string]any{"allow": []any{"Bash", "Read"}}, nil, []any{"Bash(ls:*)", "Read(docs/**)"}},
		{nil, map[string]any{"*": "ask", "read": map[string]any{"*": "allow", "~": "deny"}}, []any{"Bash", "Read"}},
		{nil, map[string]any{"bash": map[string]any{"*": "allow", "*push*": "ask"}}, []any{"Bash(git:*)"}},
		{nil, map[string]any{"*": "ask", "bash": map[string]any{"git *": "allow"}}, []any{"Bash"}},
		{nil, map[string]any{"github_*": "deny"}, []any{"mcp__github__create_issue"}},
		{nil, map[string]any{"bash": map[string]any{"* --force": "ask"}}, []any{"Bash(git:*)"}},
		{nil, nil, []any{"Read", "Bash"}},
		{nil, nil, []any{"Read(src/**)"}},
		{nil, map[string]any{"doom_loop": "deny", "external_directory": map[string]any{"/etc/**": "deny"}}, []any{"Read"}},
		{nil, map[string]any{"read": "allow"}, []any{"Read"}},
	} {
		meta := map[string]any{}
		if tc.settings != nil {
			meta["permissions"] = tc.settings
		}
		if tc.native != nil {
			meta["x-kilo"] = map[string]any{"permission": tc.native}
		}
		project, _ := settingsPermission([]spec.Entry{{Kind: spec.KindSettings, Meta: meta}})
		agent := spec.Entry{Kind: spec.KindAgent, Name: "a", Meta: map[string]any{"tools": tc.tools}}
		merged, _ := agentMarkdown(agent, project)
		projectFlat := append(simDefaults(), simProjectRules(t, project)...)
		ownFlat := simOwnRules(tc.tools)
		mergedFlat := append(append([]simRule{}, projectFlat...), simAgentRules(t, merged)...)
		for key, inputs := range samples {
			for _, input := range inputs {
				want := stricter(simEvaluate(projectFlat, key, input), simEvaluate(ownFlat, key, input))
				if got := simEvaluate(mergedFlat, key, input); actionStrictness[got] < actionStrictness[want] {
					t.Errorf("tools %v, project %v %v, %s %q: got %s, want at least %s", tc.tools, tc.settings, tc.native, key, input, got, want)
				}
			}
		}
	}
}

// simOwnRules is the agent's bare allowlist: deny everything, allow
// each listed tool, and ask for doom_loop and external_directory.
func simOwnRules(tools []any) []simRule {
	rules := []simRule{{"*", "*", "deny"}, {"doom_loop", "*", "ask"}, {"external_directory", "*", "ask"}}
	scoped := map[string]bool{}
	for _, name := range tools {
		key, pattern, ok := permissionRule(name.(string))
		if !ok {
			continue
		}
		if pattern != "*" && !scoped[key] {
			scoped[key] = true
			rules = append(rules, simRule{key, "*", "deny"})
		}
		rules = append(rules, simRule{key, pattern, "allow"})
	}
	return rules
}

// simDefaults is Kilo's `baseDefaults` and `defaultsPatch` rule list
// (packages/opencode/src/agent/agent.ts, src/kilocode/agent/index.ts),
// allows included, for the simulation.
func simDefaults() []simRule {
	return []simRule{
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
		{"recall", "*", "ask"},
		{"kilo_memory_recall", "*", "ask"},
		{"kilo_memory_save", "*", "ask"},
	}
}

// simProjectRules reads the project map back from the JSON kilo.jsonc
// gets, in its written order.
func simProjectRules(t *testing.T, project map[string]any) []simRule {
	t.Helper()
	raw, err := json.Marshal(project)
	if err != nil {
		t.Fatal(err)
	}
	var node yaml.Node
	if err := yaml.Unmarshal(raw, &node); err != nil {
		t.Fatal(err)
	}
	if len(node.Content) == 0 {
		return nil
	}
	return simMapping(node.Content[0])
}

// simAgentRules reads the `permission` map from rendered frontmatter in
// its written order.
func simAgentRules(t *testing.T, md string) []simRule {
	t.Helper()
	front := strings.SplitN(md, "---\n", 3)[1]
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(front), &node); err != nil {
		t.Fatal(err)
	}
	root := node.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "permission" {
			return simMapping(root.Content[i+1])
		}
	}
	return nil
}

func simMapping(m *yaml.Node) []simRule {
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

// simMatch is Kilo's Wildcard.match, written apart from wildcardMatch.
func simMatch(input, pattern string) bool {
	escaped := regexp.QuoteMeta(pattern)
	escaped = strings.ReplaceAll(escaped, `\*`, ".*")
	escaped = strings.ReplaceAll(escaped, `\?`, ".")
	if strings.HasSuffix(escaped, " .*") {
		escaped = strings.TrimSuffix(escaped, " .*") + "( .*)?"
	}
	return regexp.MustCompile("(?s)^" + escaped + "$").MatchString(input)
}

// simRule is one entry of the flat list Kilo evaluates.
type simRule struct{ key, pattern, action string }
