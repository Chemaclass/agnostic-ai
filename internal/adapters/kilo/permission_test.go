package kilo

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
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

// agentCase is one project for an agent: a settings spec, hand-written
// root and `.kilo/kilo.jsonc` files, configured MCP servers, and the
// agent's tools.
type agentCase struct {
	settings    map[string]any
	hand, local string
	servers     []string
	tools       []any
}

// write lays the case's hand-written files in the current directory and
// returns its bundle.
func (c agentCase) write(t *testing.T) spec.Bundle {
	t.Helper()
	if c.hand != "" {
		if err := os.WriteFile("kilo.jsonc", []byte(c.hand), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if c.local != "" {
		if err := os.MkdirAll(".kilo", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(".kilo", "kilo.jsonc"), []byte(c.local), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "a", Body: "body", Meta: map[string]any{"tools": c.tools}}}
	if c.settings != nil {
		entries = append(entries, spec.Entry{Kind: spec.KindSettings, Name: "s", Meta: c.settings})
	}
	for _, server := range c.servers {
		entries = append(entries, spec.Entry{Kind: spec.KindMCP, Name: server, Meta: map[string]any{"command": "npx"}})
	}
	return spec.NewBundle(entries)
}

// emitCase runs a whole Kilo emit for c and returns kilo.jsonc and the
// agent file.
func emitCase(t *testing.T, c agentCase) (jsonc, md string) {
	t.Helper()
	testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), c.write(t), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile("kilo.jsonc")
	agent, err := os.ReadFile(filepath.Join(".kilo", "agents", "a.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw), string(agent)
}

func emitAgent(t *testing.T, settings map[string]any, hand string, tools ...any) (jsonc, md string) {
	t.Helper()
	return emitCase(t, agentCase{settings: settings, hand: hand, tools: tools})
}

// A dry run and a check build the same agent as a real sync, hand-written
// keys in kilo.jsonc and .kilo/kilo.jsonc included.
func TestAgent_DryRunAndCheckSeeHandWrittenRules(t *testing.T) {
	c := agentCase{
		settings: map[string]any{"permissions": map[string]any{"ask": []any{"Read(docs/**)"}}},
		hand:     `{"permission": {"bash": {"git push *": "deny"}, "webfetch": "deny"}}`,
		local:    `{"permission": {"bash": {"git clean *": "deny"}}}`,
		tools:    []any{"Bash(git:*)", "Bash(npm:*)"},
	}
	_, synced := emitCase(t, c)

	testutil.TempCwd(t)
	bundle := c.write(t)
	sess := emit.NewSession()
	sess.StartCapture()
	if err := New().Emit(sess, bundle, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	var checked string
	for _, file := range sess.StopCapture() {
		if filepath.ToSlash(file.Path) == ".kilo/agents/a.md" {
			checked = file.Content
		}
	}
	if checked != synced {
		t.Errorf("check built\n%s\nsync built\n%s", checked, synced)
	}

	testutil.TempCwd(t)
	bundle = c.write(t)
	agentPath := filepath.Join(".kilo", "agents", "a.md")
	sess = emit.NewSession()
	sess.WatchPlannedFile(agentPath)
	if err := New().Emit(sess, bundle, &config.Config{}, true); err != nil {
		t.Fatal(err)
	}
	if previewed, _ := sess.PlannedFile(agentPath); previewed != synced {
		t.Errorf("dry run previewed\n%s\nsync built\n%s", previewed, synced)
	}
	for _, want := range []string{"git push *", "git clean *"} {
		if !strings.Contains(synced, want+": deny") {
			t.Errorf("want %q denied in:\n%s", want, synced)
		}
	}
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
		"github_list":         {"x"},
		"github_ent_create":   {"x"},
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
		local    string
		servers  []string
	}{
		{portable(map[string]any{"ask": []any{"Bash(npm:*)", "Bash(git push:*)"}, "deny": []any{"Bash(rm:*)"}}), "", []any{"Bash(git:*)"}, "", nil},
		{portable(map[string]any{"ask": []any{"Bash(git:*)"}}), "", []any{"Bash(git status:*)", "Read(src/**)"}, "", nil},
		{portable(map[string]any{"ask": []any{"Bash"}, "deny": []any{"Read(/secrets/**)"}}), "", []any{"Bash(git:*)", "Read"}, "", nil},
		{portable(map[string]any{"allow": []any{"Bash", "Read"}}), "", []any{"Bash(ls:*)", "Read(docs/**)"}, "", nil},
		{portable(map[string]any{"deny": []any{"Read(~)", "Read(yes)"}}), "", []any{"Read"}, "", nil},
		{native(map[string]any{"*": "ask", "read": map[string]any{"*": "allow", "~": "deny"}}), "", []any{"Bash", "Read"}, "", nil},
		{native(map[string]any{"bash": map[string]any{"*": "allow", "*push*": "ask"}}), "", []any{"Bash(git:*)"}, "", nil},
		{native(map[string]any{"*": "ask", "bash": map[string]any{"git *": "allow"}}), "", []any{"Bash"}, "", nil},
		{native(map[string]any{"github_*": "deny"}), "", []any{"mcp__github__create_issue"}, "", nil},
		{native(map[string]any{"bash": map[string]any{"* --force": "ask"}}), "", []any{"Bash(git:*)"}, "", nil},
		{nil, "", []any{"Read", "Bash", "Grep"}, "", nil},
		{nil, "", []any{"Read(src/**)"}, "", nil},
		{native(map[string]any{"doom_loop": "deny", "external_directory": map[string]any{"/etc/**": "deny"}}), "", []any{"Read"}, "", nil},
		{native(map[string]any{"read": "allow"}), "", []any{"Read"}, "", nil},
		{portable(map[string]any{"ask": []any{"Bash(* --force)"}, "deny": []any{"Bash(git:*)"}}), "", []any{"Bash(git:*)"}, "", nil},
		{native(map[string]any{"bash": map[string]any{"* --force": "ask", "git *": "deny"}}), "", []any{"Bash(git:*)"}, "", nil},
		{portable(map[string]any{"deny": []any{"Edit(.env)"}}), "", []any{"Write"}, "", nil},
		{portable(map[string]any{"deny": []any{"Edit(.env)"}}), "", []any{"Edit(docs/**)"}, "", nil},
		{nil, `{"permission": {"bash": {"git push *": "deny"}}}`, []any{"Bash(git:*)"}, "", nil},
		{nil, `{"permission": {"bash": {"git *": "ask", "git status *": "allow"}}}`, []any{"Bash(git:*)"}, `{"permission": {"bash": {"git *": "deny"}}}`, nil},
		{nil, "", []any{"Bash(git:*)"}, `{"permission": {"bash": "deny"}}`, nil},
		{nil, "", []any{"mcp__github__*"}, "", []string{"github", "github_ent"}},
		{native(map[string]any{"github_create_issue": "deny"}), "", []any{"mcp__github__*"}, "", []string{"github"}},
		{native(map[string]any{"github_ent_*": "allow"}), "", []any{"mcp__github__*", "mcp__github_ent__create"}, "", []string{"github", "github_ent"}},
	} {
		jsonc, md := emitCase(t, agentCase{settings: tc.settings, hand: tc.hand, local: tc.local, servers: tc.servers, tools: tc.tools})
		project := append(simDefaults(), simMergedRules(t, jsonc, tc.local)...)
		agent := append(slices.Clone(project), simAgentRules(t, md)...)
		own := simOwnRules(tc.tools, tc.servers)
		for key, inputs := range samples {
			for _, input := range inputs {
				want := simStricter(simEvaluate(project, key, input, false), simEvaluate(own, key, input, false))
				if got := simEvaluate(agent, key, input, false); got != want {
					t.Errorf("tools %v, settings %v, hand %q, %s %q: got %s, want %s\n%s", tc.tools, tc.settings, tc.hand, key, input, got, want, md)
				}
				want = simStricter(simEvaluate(project, key, input, true), simEvaluate(own, key, input, true))
				if got := simEvaluate(agent, key, input, true); simRank(got) < simRank(want) {
					t.Errorf("on Windows, tools %v, settings %v, hand %q, %s %q: got %s, want at least %s\n%s", tc.tools, tc.settings, tc.hand, key, input, got, want, md)
				}
			}
		}
	}
}

// Kilo ignores case on Windows only (Wildcard.match uses the "i" flag on
// win32), so a project rule whose case differs from the agent's pattern
// must still hold there, while staying never looser elsewhere.
func TestAgent_NeverLooserWhateverTheCase(t *testing.T) {
	samples := map[string][]string{
		"bash": {"git push origin", "GIT PUSH origin", "git status", "Git status"},
		"read": {"src/a.go", "SRC/secret/key", "src/secret/key", "a.env", "A.ENV", "src/x.Env"},
	}
	native := func(permission map[string]any) map[string]any {
		return map[string]any{"x-kilo": map[string]any{"permission": permission}}
	}
	for _, tc := range []agentCase{
		{settings: map[string]any{"permissions": map[string]any{"deny": []any{"Read(SRC/secret/**)"}}}, tools: []any{"Read(src/**)"}},
		{settings: native(map[string]any{"bash": map[string]any{"GIT PUSH *": "deny"}}), tools: []any{"Bash(git:*)"}},
		{settings: native(map[string]any{"bash": map[string]any{"Git *": "ask"}}), tools: []any{"Bash(git status:*)"}},
		{settings: native(map[string]any{"read": map[string]any{"*.ENV": "deny"}}), tools: []any{"Read"}},
		{settings: native(map[string]any{"read": map[string]any{"*.ENV": "deny"}}), tools: []any{"Read(src/**)"}},
	} {
		jsonc, md := emitCase(t, tc)
		project := append(simDefaults(), simMergedRules(t, jsonc, tc.local)...)
		agent := append(slices.Clone(project), simAgentRules(t, md)...)
		own := simOwnRules(tc.tools, tc.servers)
		for _, fold := range []bool{false, true} {
			for key, inputs := range samples {
				for _, input := range inputs {
					want := simStricter(simEvaluate(project, key, input, fold), simEvaluate(own, key, input, fold))
					if got := simEvaluate(agent, key, input, fold); simRank(got) < simRank(want) {
						t.Errorf("ignoring case %t, tools %v, settings %v, %s %q: got %s, want at least %s\n%s", fold, tc.tools, tc.settings, key, input, got, want, md)
					}
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
// listed tool, and ask for doom_loop and external_directory. A whole MCP
// server means that server's tools only, not those of another configured
// server whose name starts with it.
func simOwnRules(tools []any, servers []string) []simRule {
	rules := []simRule{{"*", "*", "deny"}, {"doom_loop", "*", "ask"}, {"external_directory", "*", "ask"}}
	keys := map[string]string{"Bash": "bash", "Read": "read", "Edit": "edit", "Write": "edit", "Grep": "grep"}
	for _, tool := range tools {
		name := tool.(string)
		if server, rest, ok := strings.Cut(strings.TrimPrefix(name, "mcp__"), "__"); ok {
			rules = append(rules, simRule{server + "_" + rest, "*", "allow"})
			if rest == "*" {
				for _, other := range servers {
					if other != server && strings.HasPrefix(other, server+"_") && !slices.Contains(tools, any("mcp__"+other+"__*")) {
						rules = append(rules, simRule{other + "_*", "*", "deny"})
					}
				}
			}
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

// simMergedRules reads the `permission` objects of kilo.jsonc and
// .kilo/kilo.jsonc and merges the second over the first the way Kilo's
// remeda mergeDeep does: a key keeps its first position, two objects
// merge, and anything else takes the later value.
func simMergedRules(t *testing.T, root, local string) []simRule {
	t.Helper()
	return simMapping(simMergeDeep(simPermission(t, root), simPermission(t, local)))
}

func simPermission(t *testing.T, jsonc string) *yaml.Node {
	t.Helper()
	if jsonc == "" {
		return nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(jsonc), &doc); err != nil {
		t.Fatal(err)
	}
	return simChild(doc.Content[0], "permission")
}

func simMergeDeep(base, over *yaml.Node) *yaml.Node {
	if base == nil || over == nil || base.Kind != yaml.MappingNode || over.Kind != yaml.MappingNode {
		if over != nil {
			return over
		}
		return base
	}
	out := &yaml.Node{Kind: yaml.MappingNode, Content: slices.Clone(base.Content)}
	for i := 0; i+1 < len(over.Content); i += 2 {
		key, value := over.Content[i], over.Content[i+1]
		found := false
		for j := 0; j+1 < len(out.Content); j += 2 {
			if out.Content[j].Value == key.Value {
				out.Content[j+1] = simMergeDeep(out.Content[j+1], value)
				found = true
			}
		}
		if !found {
			out.Content = append(out.Content, key, value)
		}
	}
	return out
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
// pattern both match wins, and "ask" when none does. fold ignores case,
// as Kilo does on Windows.
func simEvaluate(rules []simRule, key, input string, fold bool) string {
	action := "ask"
	for _, rule := range rules {
		if simMatch(key, rule.key, fold) && simMatch(input, rule.pattern, fold) {
			action = rule.action
		}
	}
	return action
}

func simRank(action string) int {
	return map[string]int{"allow": 0, "ask": 1, "deny": 2}[action]
}

func simStricter(a, b string) string {
	if simRank(b) > simRank(a) {
		return b
	}
	return a
}

// simMatch is Kilo's Wildcard.match (packages/opencode/src/util/wildcard.ts),
// with the "i" flag it adds on win32 when fold is set.
func simMatch(input, pattern string, fold bool) bool {
	input = strings.ReplaceAll(input, `\`, "/")
	pattern = strings.ReplaceAll(pattern, `\`, "/")
	escaped := regexp.QuoteMeta(pattern)
	escaped = strings.ReplaceAll(escaped, `\*`, ".*")
	escaped = strings.ReplaceAll(escaped, `\?`, ".")
	if strings.HasSuffix(escaped, " .*") {
		escaped = strings.TrimSuffix(escaped, " .*") + "( .*)?"
	}
	flags := "(?s)"
	if fold {
		flags = "(?si)"
	}
	return regexp.MustCompile(flags + "^" + escaped + "$").MatchString(input)
}
