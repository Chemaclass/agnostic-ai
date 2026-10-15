package cli

import (
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestLintClaudeRefusedNames_FlagsSkillAndCommandNames(t *testing.T) {
	b := spec.Bundle{
		Skills:   []spec.Entry{{Kind: spec.KindSkill, Name: "zero\U0000200Bwidth", Path: "skills/zw/SKILL.md"}},
		Commands: []spec.Entry{{Kind: spec.KindCommand, Name: "caf\U000000E9<", Path: "commands/cafe.md"}},
	}
	got := lintClaudeRefusedNames(b, []string{"claude", "codex"})
	if len(got) != 2 {
		t.Fatalf("want 2 findings, got %+v", got)
	}
	for i, want := range []string{
		"skill name \"zero\\u200bwidth\" holds U+200B, which Claude Code refuses, so the skill does not load there; rename it",
		"command name \"caf\U000000E9<\" holds `<`, which Claude Code refuses, so the command does not load there; rename it",
	} {
		if got[i].Code != "LINT047" || got[i].Severity != lintWarn || got[i].Message != want {
			t.Errorf("finding %d = %+v, want message %q", i, got[i], want)
		}
	}
}

func TestLintClaudeRefusedNames_SkipsWithoutClaude(t *testing.T) {
	bad := spec.Entry{Kind: spec.KindSkill, Name: "a<b", Path: "skills/a<b/SKILL.md"}
	if got := lintClaudeRefusedNames(spec.Bundle{Skills: []spec.Entry{bad}}, []string{"codex"}); len(got) != 0 {
		t.Errorf("flagged without claude enabled: %+v", got)
	}
	excluded := bad
	excluded.Meta = map[string]any{"target-exclude": "claude"}
	if got := lintClaudeRefusedNames(spec.Bundle{Skills: []spec.Entry{excluded}}, []string{"claude", "codex"}); len(got) != 0 {
		t.Errorf("flagged a skill excluded from claude: %+v", got)
	}
	good := spec.Entry{Kind: spec.KindSkill, Name: "deploy app", Path: "skills/deploy/SKILL.md"}
	if got := lintClaudeRefusedNames(spec.Bundle{Skills: []spec.Entry{good}}, []string{"claude"}); len(got) != 0 {
		t.Errorf("flagged a clean name: %+v", got)
	}
}
