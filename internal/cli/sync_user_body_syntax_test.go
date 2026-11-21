package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

const userSyntaxSkill = "---\nname: pr\ndescription: Open a PR\n---\nBranch:\n!`git branch --show-current`\nAsk: $ARGUMENTS\n"

func TestSyncUser_NotesClaudeSkillSyntaxOnTargetsThatReadPlainText(t *testing.T) {
	_, source := userAgentTestHome(t)
	skill := filepath.Join(source, "skills", "pr", "SKILL.md")
	writeFile(t, skill, userSyntaxSkill)

	_, errOut, err := runUserAgentTest("--only", "claude,codex")
	if err != nil {
		t.Fatalf("sync --scope user: %v", err)
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

func TestSyncUser_FencedClaudeSkillSyntaxGetsNoNote(t *testing.T) {
	_, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(source, "skills", "pr", "SKILL.md"),
		"---\nname: pr\ndescription: Open a PR\n---\n::target claude\n!`git branch --show-current`\nAsk: $ARGUMENTS\n::end\n")

	_, errOut, err := runUserAgentTest("--only", "claude,codex")
	if err != nil {
		t.Fatalf("sync --scope user: %v", err)
	}
	if strings.Contains(errOut, "has no effect") {
		t.Errorf("a ::target claude fence reaches only claude, got:\n%s", errOut)
	}
}

func TestSyncUser_OnUnsupportedErrorFailsOnClaudeSkillSyntax(t *testing.T) {
	_, source := userAgentTestHome(t)
	skill := filepath.Join(source, "skills", "pr", "SKILL.md")
	writeFile(t, skill, userSyntaxSkill)
	writeFile(t, filepath.Join(source, "agnostic-ai.yaml"), "on-unsupported: error\n")

	_, _, err := runUserAgentTest("--only", "codex")
	if err == nil {
		t.Fatal("want sync --scope user to fail under on-unsupported: error")
	}
	for _, want := range []string{skill + ":6", "codex reads", "as plain text"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("want %q in %v", want, err)
		}
	}
}

func TestSyncUser_LocalOnUnsupportedReplacesSharedValue(t *testing.T) {
	_, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(source, "skills", "pr", "SKILL.md"), userSyntaxSkill)
	writeFile(t, filepath.Join(source, "agnostic-ai.yaml"), "on-unsupported: error\n")
	writeFile(t, filepath.Join(source, "local", "agnostic-ai.yaml"), "on-unsupported: silent\n")

	_, errOut, err := runUserAgentTest("--only", "codex")
	if err != nil {
		t.Fatalf("sync --scope user: %v", err)
	}
	if strings.Contains(errOut, "has no effect") {
		t.Errorf("silent hides the note, got:\n%s", errOut)
	}
}

func TestSyncUser_OnUnsupportedSilentHidesClaudeSkillSyntaxNote(t *testing.T) {
	_, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(source, "skills", "pr", "SKILL.md"), userSyntaxSkill)
	writeFile(t, filepath.Join(source, "agnostic-ai.yaml"), "on-unsupported: silent\n")

	_, errOut, err := runUserAgentTest("--only", "codex")
	if err != nil {
		t.Fatalf("sync --scope user: %v", err)
	}
	if strings.Contains(errOut, "has no effect") {
		t.Errorf("silent hides the note, got:\n%s", errOut)
	}
}
