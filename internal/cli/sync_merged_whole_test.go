package cli

import (
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// A file sync writes whole after merging into it before, here once an
// overlay holds the user's settings, drops the old claims: they would
// read sync's newer value as a hand edit and keep it.
func TestSync_WholeFileWriteDropsStaleMergedClaims(t *testing.T) {
	const settings = ".claude/settings.json"
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cline]\n")
	mustWriteFile(t, ".agnostic-ai/hooks/fmt.yaml", "name: fmt\nevent: PostToolUse\ncommand: echo a\n")
	runSyncOK(t)
	if _, ok := readStateFile(".").Merged[settings]; !ok {
		t.Fatal("merged write left no record")
	}
	mustWriteFile(t, ".agnostic-ai/overlays/claude.settings.json", "{\"theme\": \"dark\"}\n")
	mustWriteFile(t, ".agnostic-ai/hooks/fmt.yaml", "name: fmt\nevent: PostToolUse\ncommand: echo b\n")
	runSyncOK(t)
	if _, ok := readStateFile(".").Merged[settings]; ok {
		t.Error("whole-file write kept the stale merged record")
	}
}
