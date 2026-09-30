package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// Sync derives the worktree setup hooks and script from the environment
// spec, so import leaves them out instead of copying them into a hook
// spec that would run setup twice (#1498).
func TestImportClaude_SkipsGeneratedWorktreeSetup(t *testing.T) {
	root := t.TempDir()
	run := `sh \"$CLAUDE_PROJECT_DIR/.claude/hooks/agnostic-ai-worktree-setup.sh\"`
	writeFile(t, filepath.Join(root, ".claude", "settings.json"),
		`{"hooks":{"SessionStart":[{"matcher":"startup","hooks":[{"type":"command","command":"`+run+`"}]}],
		  "SubagentStart":[{"matcher":"","hooks":[{"type":"command","command":"`+run+`"}]}]}}`)
	writeFile(t, filepath.Join(root, ".claude", "hooks", "agnostic-ai-worktree-setup.sh"), "#!/bin/sh\n")
	writeFile(t, filepath.Join(root, ".claude", "hooks", "guard.sh"), "#!/bin/sh\n")
	dst := filepath.Join(root, "hooks")

	n, err := importClaudeHooks(root, dst)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("imported %d hooks, want none", n)
	}
	if err := captureHookScripts(root, "claude"); err != nil {
		t.Fatal(err)
	}
	scripts := filepath.Join(root, agnosticScriptsDir, "claude")
	if _, err := os.Stat(filepath.Join(scripts, "agnostic-ai-worktree-setup.sh")); !os.IsNotExist(err) {
		t.Errorf("generated setup script captured: %v", err)
	}
	if _, err := os.Stat(filepath.Join(scripts, "guard.sh")); err != nil {
		t.Errorf("hand-written script not captured: %v", err)
	}
}
