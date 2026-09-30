package copilot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func writeTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// VS Code reads any .md in .github/agents, so an agent that lives at
// data.md is written there instead of next to it as data.agent.md.
func TestAgentPath_KeepsAPlainMarkdownProfile(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	if got := agentPath(".github/agents", "data"); got != filepath.Join(".github/agents", "data.agent.md") {
		t.Errorf("new agent: got %s", got)
	}
	writeTestFile(t, filepath.Join(".github", "agents", "data.md"))
	if got := agentPath(".github/agents", "data"); got != filepath.Join(".github/agents", "data.md") {
		t.Errorf("existing data.md: got %s", got)
	}
}

func TestAgentMarkdown_WritesTheDisplayName(t *testing.T) {
	out, _ := agentMarkdown(spec.Entry{Name: "data", Meta: map[string]any{
		"description": "Queries.", "x-copilot": map[string]any{"name": "Data"},
	}})
	if want := "name: Data\n"; !strings.Contains(out, want) {
		t.Errorf("want %q in:\n%s", want, out)
	}
}

// A skill that lives in another directory Copilot reads is written there,
// not copied into .github/skills; a user-set skills-dir takes every skill.
func TestSkillsByDir_WritesASkillWhereItLives(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	writeTestFile(t, filepath.Join(".agents", "skills", "launch", "SKILL.md"))
	skills := []spec.Entry{{Name: "launch"}, {Name: "fresh"}}
	sess := emit.NewSession()
	got := skillsByDir(sess, skills, defaultSkillsDir, true)
	if len(got[".agents/skills"]) != 1 || got[".agents/skills"][0].Name != "launch" {
		t.Errorf("launch should stay in .agents/skills: %v", got)
	}
	if len(got[defaultSkillsDir]) != 1 || got[defaultSkillsDir][0].Name != "fresh" {
		t.Errorf("fresh should go to %s: %v", defaultSkillsDir, got)
	}
	if custom := skillsByDir(sess, skills, "tools/skills", false); len(custom["tools/skills"]) != 2 {
		t.Errorf("a custom skills-dir takes every skill: %v", custom)
	}
	// Codex writes .agents/skills in the same sync, so what is there can
	// change mid-run: the skill goes to .github/skills instead.
	sess.SetSkillsDirWriters(map[string][]string{filepath.Clean(".agents/skills"): {"codex", "copilot"}})
	if shared := skillsByDir(sess, skills, defaultSkillsDir, true); len(shared[defaultSkillsDir]) != 2 {
		t.Errorf("a dir another target writes is not reused: %v", shared)
	}
}
