package spec

import (
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCapabilityTools_MapsEachFormAndKeepsAliases(t *testing.T) {
	got, problem := CapabilityTools([]string{
		"read", "write", "edit", "shell", "shell(git diff *)", "web",
		"mcp:github", "mcp:github/get_issue", "Grep", "Bash(go test *)", "mcp__docs__search",
	})
	want := []string{
		"Read", "Write", "Edit", "Bash", "Bash(git diff *)", "WebFetch", "WebSearch",
		"mcp__github", "mcp__github__get_issue", "Grep", "Bash(go test *)", "mcp__docs__search",
	}
	if problem != "" || !slices.Equal(got, want) {
		t.Errorf("CapabilityTools = %v, %q; want %v", got, problem, want)
	}
}

// One vocabulary covers agents and hooks: every hook tool kind but any
// is a capability, and its Claude Code names are ones the Claude Code
// hook matcher for that kind lists.
func TestCapabilities_ShareTheHookToolKinds(t *testing.T) {
	for _, kind := range HookToolKinds {
		if kind == "any" {
			continue
		}
		if !slices.Contains(Capabilities, kind) {
			t.Errorf("hook tool kind %q is not a capability", kind)
			continue
		}
		matcher, _ := HookToolMatcher("claude", kind)
		names, _ := CapabilityTools([]string{kind})
		for _, n := range names {
			if !slices.Contains(strings.Split(matcher, "|"), n) {
				t.Errorf("can: %s gives %s, which the claude hook matcher %q for %s does not name", kind, n, matcher, kind)
			}
		}
	}
	tools, _ := CapabilityTools([]string{"mcp:github"})
	matcher, _ := HookToolMatcher("claude", "mcp:github")
	if !strings.HasPrefix(matcher, tools[0]+"__") {
		t.Errorf("can: mcp:github gives %s, which the claude hook matcher %q does not cover", tools[0], matcher)
	}
}

func TestNeutralCapability_RoundTripsThroughCapabilityTools(t *testing.T) {
	for tool, want := range map[string]string{
		"Read":                   "read",
		"Write":                  "write",
		"Edit":                   "edit",
		"Bash":                   "shell",
		"Bash(git diff *)":       "shell(git diff *)",
		"Bash(echo (a))":         "shell(echo (a))",
		"mcp__github":            "mcp:github",
		"mcp__github__get_issue": "mcp:github/get_issue",
		"mcp__a___b":             "mcp:a/_b",
	} {
		got, ok := NeutralCapability(tool)
		if !ok || got != want {
			t.Errorf("NeutralCapability(%q) = %q, %v; want %q", tool, got, ok, want)
			continue
		}
		if back, problem := CapabilityTools([]string{got}); problem != "" || len(back) != 1 || back[0] != tool {
			t.Errorf("CapabilityTools(%q) = %v, %q; want [%s]", got, back, problem, tool)
		}
	}
	for _, tool := range []string{"Grep", "WebFetch", "WebSearch", "Bash()", "Bash(x", "mcp__a__b__c", "mcp__a__", "Read(src/**)"} {
		if got, ok := NeutralCapability(tool); ok {
			t.Errorf("NeutralCapability(%q) = %q, want no capability", tool, got)
		}
	}
}

func TestAgentCapabilityProblem_NamesWhatIsWrong(t *testing.T) {
	cases := []struct {
		meta map[string]any
		want string
	}{
		{map[string]any{"can": []any{"read", "shell(git diff *)", "Grep"}}, ""},
		{map[string]any{"tools": []any{"Read"}}, ""},
		{map[string]any{"can": []any{"raed"}}, `unknown capability "raed" for can: (did you mean read?)`},
		{map[string]any{"can": []any{"delete"}}, `unknown capability "delete" for can:; use one of read, write, edit, shell, web`},
		{map[string]any{"can": []any{"read"}, "tools": []any{"Read"}}, "sets both can: and tools:; keep one"},
		{map[string]any{"can": "read"}, "can: must be a list"},
		{map[string]any{"can": []any{1}}, "can: entry 1 is not a capability name"},
		{map[string]any{"can": []any{"shell( )"}}, `can: "shell( )" needs a command pattern`},
		{map[string]any{"can": []any{"shell(git"}}, `can: "shell(git" is missing its closing parenthesis`},
		{map[string]any{"can": []any{"edit(src/**)"}}, `can: "edit(src/**)": only shell takes a pattern`},
		{map[string]any{"can": []any{"mcp:"}}, `can: "mcp:" needs an MCP server`},
		{map[string]any{"can": []any{"mcp:gh/a.b"}}, `can: "mcp:gh/a.b" needs an MCP server`},
		{map[string]any{"tools": []any{"Read"}, "x-kiro": map[string]any{"can": []any{"read"}}}, "x-kiro.can is not read"},
	}
	for _, c := range cases {
		got := AgentCapabilityProblem(c.meta)
		if (c.want == "") != (got == "") || !strings.HasPrefix(got, c.want) {
			t.Errorf("AgentCapabilityProblem(%v) = %q, want %q", c.meta, got, c.want)
		}
	}
}

// A can: value YAML cannot decode stays set, so the check rejects it
// instead of the loader dropping the restriction.
func TestParseMarkdown_KeepsAnUndecodableCan(t *testing.T) {
	e, err := ParseMarkdownBytes(KindAgent, []byte("---\nname: a\ncan: [read, !!float nope]\n---\n\nBody.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if problem := AgentCapabilityProblem(e.Meta); !strings.HasPrefix(problem, "can: must be a list") {
		t.Errorf("problem = %q", problem)
	}
}

func TestNativeTools_PutsToolsWhereCanWas(t *testing.T) {
	e := Entry{
		Kind:       KindAgent,
		Name:       "reviewer",
		Meta:       map[string]any{"name": "reviewer", "can": []any{"read", "web"}, "model": "sonnet"},
		MetaKeys:   []string{"name", "can", "model"},
		MetaStyles: map[string]yaml.Style{"model": yaml.DoubleQuotedStyle},
	}
	got, problem := e.NativeTools()
	if problem != "" {
		t.Fatal(problem)
	}
	if _, left := got.Meta["can"]; left || !slices.Equal(got.MetaKeys, []string{"name", "tools", "model"}) {
		t.Errorf("keys = %v, meta = %v", got.MetaKeys, got.Meta)
	}
	if tools, _ := got.Meta["tools"].([]any); len(tools) != 3 || tools[0] != "Read" || tools[1] != "WebFetch" || tools[2] != "WebSearch" {
		t.Errorf("tools = %v", got.Meta["tools"])
	}
	if _, kept := e.Meta["can"]; !kept || e.MetaKeys[1] != "can" {
		t.Error("NativeTools must not change the source entry")
	}
}

// An agent whose can: sync cannot read is left out, so a typo never
// writes an agent with every tool. That holds for a pack agent too,
// which the sync stop does not check.
func TestBundleFor_LeavesOutAnAgentWithAnUnreadableCan(t *testing.T) {
	b := NewBundle([]Entry{
		{Kind: KindAgent, Name: "ok", Meta: map[string]any{"can": []any{"read"}}},
		{Kind: KindAgent, Name: "typo", Meta: map[string]any{"can": []any{"raed"}}},
		{Kind: KindAgent, Name: "nested", Layer: "pack:p", Meta: map[string]any{"x-claude": map[string]any{"can": []any{"read"}}}},
	})
	agents := b.For("claude").Agents
	if len(agents) != 1 || agents[0].Name != "ok" {
		t.Fatalf("agents = %v", agents)
	}
	if tools, _ := agents[0].Meta["tools"].([]any); len(tools) != 1 || tools[0] != "Read" {
		t.Errorf("tools = %v", agents[0].Meta["tools"])
	}
}
