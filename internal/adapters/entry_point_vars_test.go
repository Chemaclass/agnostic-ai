package adapters

import (
	"slices"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestEntryPointRules_ExpandsWhatEveryReaderOfTheFileAgreesOn(t *testing.T) {
	body := "Skills in {{$SKILLS_DIR}}, MCP in {{$MCP_FILE}}."
	cases := []struct {
		name   string
		cfg    config.Config
		target string
		want   string
	}{
		{"one reader", config.Config{Targets: []string{"codex", "gemini"}}, "gemini",
			"Skills in .gemini/skills, MCP in .gemini/settings.json."},
		{"readers agree on one variable", config.Config{Targets: []string{"codex", "amp"}}, "amp",
			"Skills in .agents/skills, MCP in {{$MCP_FILE}}."},
		{"readers disagree", config.Config{Targets: []string{"codex", "opencode"}}, "codex",
			"Skills in {{$SKILLS_DIR}}, MCP in {{$MCP_FILE}}."},
		{"another spelling of the same file", config.Config{
			Targets: []string{"codex", "opencode"},
			Outputs: map[string]config.Output{"opencode": {File: "./AGENTS.md"}},
		}, "codex", "Skills in {{$SKILLS_DIR}}, MCP in {{$MCP_FILE}}."},
		{"an override makes readers agree", config.Config{
			Targets: []string{"codex", "opencode"},
			Outputs: map[string]config.Output{"opencode": {SkillsDir: ".agents/skills"}},
		}, "opencode", "Skills in .agents/skills, MCP in {{$MCP_FILE}}."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "r", Body: body}})
			got := EntryPointRules(b, c.target, &c.cfg).Rules
			if len(got) != 1 || got[0].Body != c.want {
				t.Errorf("got %+v, want body %q", got, c.want)
			}
			if b.Rules[0].Body != body {
				t.Errorf("source bundle mutated: %q", b.Rules[0].Body)
			}
		})
	}
}

// A reader with no surface for a variable cannot share another
// reader's path for it.
func TestEntryPointVars_AReaderWithoutTheSurfaceContestsIt(t *testing.T) {
	vals, contested := emit.EntryPointVars(&config.Config{Targets: []string{"codex", "kiro"}}, "codex")
	if _, ok := vals[emit.VarSkillsDir]; ok || !slices.Contains(contested, emit.VarSkillsDir) {
		t.Errorf("want SKILLS_DIR contested, got vals %v, contested %v", vals, contested)
	}
}

func TestEntryPointInlinedRules_ListsTheInlinersUnscopedRules(t *testing.T) {
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindRule, Name: "always", Body: "a"},
		{Kind: spec.KindRule, Name: "pkg", Scope: "pkg", Body: "p"},
		{Kind: spec.KindRule, Name: "cline-only", Meta: map[string]any{"targets": []any{"cline"}}, Body: "c"},
	})
	got := entryPointInlinedRules(&config.Config{Targets: []string{"codex", "cline"}}, b, "cline")
	if _, ok := got["always"]; !ok || len(got) != 1 {
		t.Errorf("want only the rule codex inlines, got %v", got)
	}
}
