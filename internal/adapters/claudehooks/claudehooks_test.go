package claudehooks

import "testing"

func TestIsWorktreeSetupCommand(t *testing.T) {
	for command, want := range map[string]bool{
		`f="$CLAUDE_PROJECT_DIR/.claude/hooks/agnostic-ai-worktree-setup.sh"; [ ! -f "$f" ] || sh "$f"`: true,
		`f='C:\proj\.claude\hooks\agnostic-ai-worktree-setup.sh'; [ ! -f "$f" ] || sh "$f"`:             true,
		`sh .claude/hooks/guard.sh`:                         false,
		`sh .claude/hooks/my-agnostic-ai-worktree-setup.sh`: false,
	} {
		if got := IsWorktreeSetupCommand(command); got != want {
			t.Errorf("IsWorktreeSetupCommand(%q) = %v, want %v", command, got, want)
		}
	}
}
