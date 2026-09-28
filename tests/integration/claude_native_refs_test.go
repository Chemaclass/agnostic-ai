package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// After import and sync, AGENTS.md names no Claude-native path and
// carries the imported rule once (#1326).
func TestImportClaude_NativePathsPointAtSources(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	files := map[string]string{
		"agnostic-ai.yaml":              "version: 1\ntargets: [claude, codex]\n",
		".claude/CLAUDE.md":             "# Project\n\n@.claude/rules/tone.md\n\nRead `.claude/skills/style/SKILL.md`.\n",
		".claude/rules/tone.md":         "# Tone\n\nBe plain.\n",
		".claude/skills/style/SKILL.md": "---\nname: style\ndescription: Style guide.\n---\n\nBody.\n",
	}
	for name, body := range files {
		must(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755))
		must(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}

	runCmd(t, "import", "claude")
	runCmd(t, "sync")
	runCmd(t, "sync", "--check")

	data, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	agents := string(data)
	if strings.Contains(agents, ".claude/") {
		t.Errorf("AGENTS.md names a Claude-native path:\n%s", agents)
	}
	if n := strings.Count(agents, "Be plain"); n != 1 {
		t.Errorf("AGENTS.md carries the rule %d times, want 1:\n%s", n, agents)
	}
	assertContains(t, filepath.Join(dir, "AGENTS.md"), "Read `.agnostic-ai/skills/style/SKILL.md`.")
}
