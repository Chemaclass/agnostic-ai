package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

const globalSyntaxSkill = "---\nname: pr\ndescription: Open a PR\n---\nBranch:\n!`git branch --show-current`\nAsk: $ARGUMENTS\n"

func TestSyncGlobal_NotesClaudeSkillSyntaxOnTargetsThatReadPlainText(t *testing.T) {
	_, source := globalAgentTestHome(t)
	skill := filepath.Join(source, "skills", "pr", "SKILL.md")
	mustWriteGlobalTest(t, skill, globalSyntaxSkill)

	_, errOut, err := runGlobalAgentTest("--only", "claude,codex")
	if err != nil {
		t.Fatalf("sync --global: %v", err)
	}
	for _, want := range []string{
		"`!`command`` on 1 skill has no effect on codex",
		skill + ":6",
		"`$ARGUMENTS` on 1 skill has no effect on codex",
		skill + ":7",
	} {
		if !strings.Contains(errOut, want) {
			t.Errorf("want %q in:\n%s", want, errOut)
		}
	}
	if strings.Contains(errOut, "on claude") {
		t.Errorf("claude expands the syntax, got:\n%s", errOut)
	}
}

func TestSyncGlobal_FencedClaudeSkillSyntaxGetsNoNote(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "pr", "SKILL.md"),
		"---\nname: pr\ndescription: Open a PR\n---\n::target claude\n!`git branch --show-current`\nAsk: $ARGUMENTS\n::end\n")

	_, errOut, err := runGlobalAgentTest("--only", "claude,codex")
	if err != nil {
		t.Fatalf("sync --global: %v", err)
	}
	if strings.Contains(errOut, "has no effect") {
		t.Errorf("a ::target claude fence reaches only claude, got:\n%s", errOut)
	}
}

func TestSyncGlobal_OnUnsupportedErrorFailsOnClaudeSkillSyntax(t *testing.T) {
	_, source := globalAgentTestHome(t)
	skill := filepath.Join(source, "skills", "pr", "SKILL.md")
	mustWriteGlobalTest(t, skill, globalSyntaxSkill)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "on-unsupported: error\n")

	_, _, err := runGlobalAgentTest("--only", "codex")
	if err == nil {
		t.Fatal("want sync --global to fail under on-unsupported: error")
	}
	for _, want := range []string{skill + ":6", "codex reads", "as plain text"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("want %q in %v", want, err)
		}
	}
}

func TestSyncGlobal_LocalOnUnsupportedReplacesSharedValue(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "pr", "SKILL.md"), globalSyntaxSkill)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "on-unsupported: error\n")
	mustWriteGlobalTest(t, filepath.Join(source, "local", "agnostic-ai.yaml"), "on-unsupported: silent\n")

	_, errOut, err := runGlobalAgentTest("--only", "codex")
	if err != nil {
		t.Fatalf("sync --global: %v", err)
	}
	if strings.Contains(errOut, "has no effect") {
		t.Errorf("silent hides the note, got:\n%s", errOut)
	}
}

func TestSyncGlobal_OnUnsupportedSilentHidesClaudeSkillSyntaxNote(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "pr", "SKILL.md"), globalSyntaxSkill)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "on-unsupported: silent\n")

	_, errOut, err := runGlobalAgentTest("--only", "codex")
	if err != nil {
		t.Fatalf("sync --global: %v", err)
	}
	if strings.Contains(errOut, "has no effect") {
		t.Errorf("silent hides the note, got:\n%s", errOut)
	}
}
