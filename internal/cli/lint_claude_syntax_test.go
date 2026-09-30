package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestLintClaudeBodySyntax_FlagsUnfencedShapesForNonClaudeTargets(t *testing.T) {
	skill := spec.Entry{
		Kind: spec.KindSkill, Name: "pr", Path: "skills/pr/SKILL.md", BodyLine: 5,
		Body: "## Context\n\n!`git log main..HEAD --oneline`\n\nUse $ARGUMENTS as the issue number.\n",
	}
	got := lintClaudeBodySyntax(spec.Bundle{Skills: []spec.Entry{skill}}, []string{"claude", "codex", "cursor"}, targetsSupportingKind)
	if len(got) != 2 {
		t.Fatalf("want 2 findings, got %+v", got)
	}
	for i, want := range []string{
		"line 7: codex, cursor read !`command` as plain text, so the command does not run; put the line in a ::target claude fence",
		"line 9: codex, cursor read $ARGUMENTS as plain text, so it stays literal text; put the line in a ::target claude fence",
	} {
		if got[i].Code != "LINT019" || got[i].Severity != lintWarn || got[i].Path != skill.Path || got[i].Message != want {
			t.Errorf("finding %d = %+v, want message %q", i, got[i], want)
		}
	}
}

func TestLintClaudeBodySyntax_FencedOrClaudeOnlyStaysClean(t *testing.T) {
	fenced := spec.Entry{
		Kind: spec.KindSkill, Name: "pr", Path: "skills/pr/SKILL.md", BodyLine: 1,
		Body: "::target claude\n!`git status`\nUse $ARGUMENTS.\n::end\n::target codex\nRun `git status` first.\n::end\n",
	}
	if got := lintClaudeBodySyntax(spec.Bundle{Skills: []spec.Entry{fenced}}, []string{"claude", "codex"}, targetsSupportingKind); len(got) != 0 {
		t.Errorf("fenced body flagged: %+v", got)
	}
	bare := spec.Entry{Kind: spec.KindSkill, Name: "pr", Path: "skills/pr/SKILL.md", Body: "Use $ARGUMENTS.\n"}
	if got := lintClaudeBodySyntax(spec.Bundle{Skills: []spec.Entry{bare}}, []string{"claude"}, targetsSupportingKind); len(got) != 0 {
		t.Errorf("claude-only project flagged: %+v", got)
	}
}

func TestLintClaudeBodySyntax_CommandSkipsTargetsThatSubstitute(t *testing.T) {
	command := spec.Entry{Kind: spec.KindCommand, Name: "fix", Path: "commands/fix.md", BodyLine: 1, Body: "Fix $ARGUMENTS.\n"}
	if got := lintClaudeBodySyntax(spec.Bundle{Commands: []spec.Entry{command}}, []string{"claude", "opencode", "factory"}, targetsSupportingKind); len(got) != 0 {
		t.Errorf("targets that substitute $ARGUMENTS flagged: %+v", got)
	}
}

// A Claude project imported with Codex enabled keeps its Claude body,
// then sync notes and lint flags each Claude-only line for Codex (#1436).
func TestImportClaude_ClaudeSyntaxKeepsTheBodyAndWarnsForCodex(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	skill := "---\nname: pr\ndescription: Open a PR.\n---\n\n## Context\n\n!`git log main..HEAD --oneline`\n\nUse $ARGUMENTS as the issue number.\n"
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex]\n")
	writeFile(t, filepath.Join(dir, ".claude", "skills", "pr", "SKILL.md"), skill)

	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	source, err := os.ReadFile(filepath.Join(dir, ".agnostic-ai", "skills", "pr", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(source), "!`git log main..HEAD --oneline`\n\nUse $ARGUMENTS as the issue number.\n") {
		t.Errorf("import changed the Claude body:\n%s", source)
	}

	notes := &strings.Builder{}
	adapters.SetWarner(notes)
	t.Cleanup(func() { adapters.SetWarner(os.Stderr) })
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	out := notes.String()
	for _, want := range []string{
		"`!`command`` on 1 skill has no effect on codex (the command does not run at .agnostic-ai/skills/pr/SKILL.md:8;",
		"`$ARGUMENTS` on 1 skill has no effect on codex (stays literal text at .agnostic-ai/skills/pr/SKILL.md:10;",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("sync output missing %q:\n%s", want, out)
		}
	}
	claude, err := os.ReadFile(filepath.Join(dir, ".claude", "skills", "pr", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(claude), "\n!`git log main..HEAD --oneline`\n") {
		t.Errorf("Claude skill lost its dynamic context:\n%s", claude)
	}

	out, _ = runCLI(t, "lint")
	if !strings.Contains(out, "LINT019 [warn] .agnostic-ai/skills/pr/SKILL.md: line 8: codex reads !`command` as plain text") {
		t.Errorf("lint output missing LINT019:\n%s", out)
	}

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex]\non-unsupported: error\n")
	if out, err := runCLI(t, "sync"); err == nil {
		t.Errorf("on-unsupported: error must fail the sync:\n%s", out)
	}
}
