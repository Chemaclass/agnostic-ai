package emit

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func claudeSyntaxSkill() spec.Entry {
	return spec.Entry{
		Kind: spec.KindSkill, Name: "pr", Path: "skills/pr/SKILL.md", BodyLine: 5,
		Body: "## Context\n\n!`git log main..HEAD --oneline`\n\nUse $ARGUMENTS as the issue number.\n",
	}
}

func TestReportClaudeBodySyntax_NotesEachShapeWithItsFileLine(t *testing.T) {
	buf := swapWarnerForNotes(t)
	if err := ReportClaudeBodySyntax("codex", spec.KindSkill, []spec.Entry{claudeSyntaxSkill()}, OnUnsupportedWarn); err != nil {
		t.Fatal(err)
	}
	FlushCoverageNotes()
	got := buf.String()
	for _, want := range []string{
		"note: `!`command`` on 1 skill has no effect on codex (the command does not run at skills/pr/SKILL.md:7; put the line in a ::target claude fence)",
		"note: `$ARGUMENTS` on 1 skill has no effect on codex (stays literal text at skills/pr/SKILL.md:9; put the line in a ::target claude fence)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestReportClaudeBodySyntax_ClaudeGetsNoNote(t *testing.T) {
	buf := swapWarnerForNotes(t)
	if err := ReportClaudeBodySyntax("claude", spec.KindSkill, []spec.Entry{claudeSyntaxSkill()}, OnUnsupportedWarn); err != nil {
		t.Fatal(err)
	}
	FlushCoverageNotes()
	if buf.Len() != 0 {
		t.Errorf("claude expands its own syntax, got %q", buf)
	}
}

func TestReportClaudeBodySyntax_CommandTargetsThatSubstituteArgumentsGetNoArgumentsNote(t *testing.T) {
	buf := swapWarnerForNotes(t)
	command := spec.Entry{Kind: spec.KindCommand, Name: "fix", Path: "commands/fix.md", BodyLine: 1,
		Body: "Fix issue $ARGUMENTS, starting with $1.\n!`git status`\n"}
	for _, target := range []string{"opencode", "codex", "factory"} {
		if err := ReportClaudeBodySyntax(target, spec.KindCommand, []spec.Entry{command}, OnUnsupportedWarn); err != nil {
			t.Fatal(err)
		}
	}
	FlushCoverageNotes()
	got := buf.String()
	if strings.Contains(got, "`$ARGUMENTS`") || strings.Contains(got, "opencode") {
		t.Errorf("a target that substitutes the syntax got a note:\n%s", got)
	}
	for _, want := range []string{
		"`$N` on 1 command has no effect on factory (",
		"`!`command`` on 1 command has no effect on codex, factory (",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestReportClaudeBodySyntax_ErrorModeFailsNamingTheLine(t *testing.T) {
	swapWarnerForNotes(t)
	err := ReportClaudeBodySyntax("codex", spec.KindSkill, []spec.Entry{claudeSyntaxSkill()}, OnUnsupportedError)
	if err == nil || !strings.Contains(err.Error(), "skills/pr/SKILL.md:7") || !strings.Contains(err.Error(), "codex") {
		t.Fatalf("want an error naming the line and target, got %v", err)
	}
}

func TestReportClaudeBodySyntax_SilentModeNotesNothing(t *testing.T) {
	swapWarnerForNotes(t)
	if err := ReportClaudeBodySyntax("codex", spec.KindSkill, []spec.Entry{claudeSyntaxSkill()}, OnUnsupportedSilent); err != nil {
		t.Fatal(err)
	}
	if n := PendingCoverageNotesCount(); n != 0 {
		t.Errorf("silent mode buffered %d notes", n)
	}
}

func TestReportUnsupported_NotesClaudeSyntaxInSkillsAndCommands(t *testing.T) {
	buf := swapWarnerForNotes(t)
	command := spec.Entry{Kind: spec.KindCommand, Name: "fix", Path: "commands/fix.md", BodyLine: 1, Body: "Fix $ARGUMENTS.\n"}
	caps := Capabilities{Target: "cursor", Supports: []spec.Kind{spec.KindSkill, spec.KindCommand}}
	b := spec.Bundle{Skills: []spec.Entry{claudeSyntaxSkill()}, Commands: []spec.Entry{command}}
	if err := ReportUnsupported(caps, b, OnUnsupportedWarn); err != nil {
		t.Fatal(err)
	}
	FlushCoverageNotes()
	got := buf.String()
	for _, want := range []string{"on 1 skill has no effect on cursor", "`$ARGUMENTS` on 1 command has no effect on cursor"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if err := ReportUnsupported(caps, b, OnUnsupportedError); err == nil {
		t.Error("on-unsupported: error must fail on Claude-only body syntax")
	}
}
