package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// A fresh `import claude` needs no manual edit before lint and sync are
// quiet, and sync writes the Claude files the project started with (#1327).
func TestImportClaude_FreshImportLeavesLintAndSyncQuiet(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	skill := "---\nname: style\ndescription: Style guide.\nallowed-tools: Read, Grep\n---\n\nBody.\n"
	command := "---\ndescription: Fix a bug\nallowed-tools: [Read, Edit]\n---\n\nFix it.\n"
	files := map[string]string{
		"agnostic-ai.yaml":              "version: 1\ntargets: [claude, codex]\n",
		".claude/CLAUDE.md":             "# Project\n",
		".claude/skills/style/SKILL.md": skill,
		".claude/commands/fix.md":       command,
		".claude/templates/post.md":     "template\n",
		".claude/settings.json":         `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"exit 0"}]}]}}` + "\n",
	}
	for name, body := range files {
		must(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755))
		must(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}

	runCmd(t, "import", "claude")
	runCmd(t, "sync")
	runCmd(t, "lint", "--strict")
	runCmd(t, "sync", "--check")

	assertContains(t, filepath.Join(dir, ".claude/skills/style/SKILL.md"), "\nallowed-tools: Read, Grep\n")
	assertContains(t, filepath.Join(dir, ".claude/commands/fix.md"), "\nallowed-tools:\n  - Read\n  - Edit\n")
	assertContains(t, filepath.Join(dir, ".agnostic-ai/hooks/pretooluse-bash-exit-0.yaml"), "description: Runs `exit 0` on PreToolUse for Bash.")
	assertAbsent(t, filepath.Join(dir, ".agents/skills/style/SKILL.md"), "allowed-tools")
}
