package kilo

import (
	"encoding/json"
	"maps"
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
	reads, ok := got["read"].(map[string]any)
	if !ok || reads["*"] != "deny" || reads["src/**"] != "allow" {
		t.Errorf("read permission = %#v", got["read"])
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
	if got["*"] != "deny" || got["read"] != "allow" {
		t.Errorf("permission = %#v", got)
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
	want := "\n  read:\n    \"*\": allow\n    /secrets/**: deny\n"
	if !strings.Contains(got, want) {
		t.Errorf("read rules lost the project deny or copied its allow, want %q in:\n%s", want, got)
	}
}

func TestAgentMarkdown_ScopedAllowKeepsProjectDenyLast(t *testing.T) {
	got := agentFrontmatter(t, map[string]any{"deny": []any{"Bash(rm:*)"}}, "Bash(git:*)")
	want := "\n  bash:\n    \"*\": deny\n    git *: allow\n    rm *: deny\n"
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
		{"pattern deny follows the allow", map[string]any{"deny": []any{"Read(/secrets/**)"}}, nil, []string{"Read"}, "read", `{"*":"allow","/secrets/**":"deny"}`},
		{"project allow is not copied", map[string]any{"allow": []any{"Read(docs/**)"}}, nil, []string{"Read(src/**)"}, "read", `{"*":"deny","src/**":"allow"}`},
		{"ask outside the agent's patterns stays denied", map[string]any{"ask": []any{"Bash(npm:*)"}}, nil, []string{"Bash(git:*)"}, "bash", `{"*":"deny","git *":"allow"}`},
		{"ask inside an agent pattern follows it", map[string]any{"ask": []any{"Bash(git push:*)"}}, nil, []string{"Bash(git:*)"}, "bash", `{"*":"deny","git *":"allow","git push *":"ask"}`},
		{"ask covering an agent pattern raises it", map[string]any{"ask": []any{"Bash(git:*)"}}, nil, []string{"Bash(git status:*)"}, "bash", `{"*":"deny","git status *":"ask"}`},
		{"deny outside the agent's patterns still follows", map[string]any{"deny": []any{"Bash(rm:*)"}}, nil, []string{"Bash(git:*)"}, "bash", `{"*":"deny","git *":"allow","rm *":"deny"}`},
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
		"bash": {"git status", "git push origin", "npm install", "rm -r tmp", "ls", "git"},
		"read": {"src/a.go", "/secrets/key", "docs/x.md", "~"},
	}
	for _, tc := range []struct {
		settings map[string]any
		native   map[string]any
		tools    []string
	}{
		{map[string]any{"ask": []any{"Bash(npm:*)", "Bash(git push:*)"}, "deny": []any{"Bash(rm:*)"}}, nil, []string{"Bash(git:*)"}},
		{map[string]any{"ask": []any{"Bash(git:*)"}}, nil, []string{"Bash(git status:*)", "Read(src/**)"}},
		{map[string]any{"ask": []any{"Bash"}, "deny": []any{"Read(/secrets/**)"}}, nil, []string{"Bash(git:*)", "Read"}},
		{map[string]any{"allow": []any{"Bash", "Read"}}, nil, []string{"Bash(ls:*)", "Read(docs/**)"}},
		{nil, map[string]any{"*": "ask", "read": map[string]any{"*": "allow", "~": "deny"}}, []string{"Bash", "Read"}},
		{nil, map[string]any{"bash": map[string]any{"*": "allow", "*push*": "ask"}}, []string{"Bash(git:*)"}},
	} {
		meta := map[string]any{}
		if tc.settings != nil {
			meta["permissions"] = tc.settings
		}
		if tc.native != nil {
			meta["x-kilo"] = map[string]any{"permission": tc.native}
		}
		project, _ := settingsPermission([]spec.Entry{{Kind: spec.KindSettings, Meta: meta}})
		own, _ := agentPermission(tc.tools, nil)
		merged, _ := agentPermission(tc.tools, project)
		for key, inputs := range samples {
			for _, input := range inputs {
				projectAction := evaluateRules(projectRules(project, key), input)
				ownAction := evaluateRules(agentRules(own, key), input)
				got := evaluateRules(append(projectRules(project, key), agentRules(merged, key)...), input)
				want := stricter(projectAction, ownAction)
				if actionStrictness[got] < actionStrictness[want] {
					t.Errorf("tools %v, %s %q: got %s, project %s, own %s", tc.tools, key, input, got, projectAction, ownAction)
				}
			}
		}
	}
}

// evaluateRules returns the last matching action, or "allow" when none
// matches: a key the project never names restricts nothing.
func evaluateRules(rules []patternRule, input string) string {
	action := "allow"
	for _, rule := range rules {
		if wildcardMatch(input, rule.pattern) {
			action = rule.action
		}
	}
	return action
}

func agentRules(perms map[string]any, key string) []patternRule {
	value, set := perms[key]
	if !set {
		value = perms[anyPattern]
	}
	switch v := value.(type) {
	case string:
		return []patternRule{{anyPattern, v}}
	case *yaml.Node:
		var rules []patternRule
		for i := 0; i+1 < len(v.Content); i += 2 {
			rules = append(rules, patternRule{v.Content[i].Value, v.Content[i+1].Value})
		}
		return rules
	case map[string]any:
		rules := []patternRule{{anyPattern, v[anyPattern].(string)}}
		for _, pattern := range slices.Sorted(maps.Keys(v)) {
			if pattern != anyPattern {
				rules = append(rules, patternRule{pattern, v[pattern].(string)})
			}
		}
		return rules
	}
	return nil
}
