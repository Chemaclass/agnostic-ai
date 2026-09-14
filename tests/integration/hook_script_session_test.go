package integration

import (
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const guardScript = ".claude/hooks/guard.sh"

// setupHookScriptProject writes a claude project whose hook runs a script
// stashed under .agnostic-ai/scripts, then moves into it.
func setupHookScriptProject(t *testing.T, unmanaged ...string) {
	t.Helper()
	testutil.Chdir(t, t.TempDir())
	config := "version: 1\ntargets: [claude]\n"
	if len(unmanaged) > 0 {
		config += "sync:\n  unmanaged:\n    - " + strings.Join(unmanaged, "\n    - ") + "\n"
	}
	mustWrite(t, "agnostic-ai.yaml", config)
	mustWrite(t, ".agnostic-ai/AGNOSTIC_AI.md", "# A\n")
	mustWrite(t, ".agnostic-ai/hooks/guard.yaml", "name: guard\ndescription: guard\nevent: PreToolUse\nmatcher: Bash\ncommand: "+guardScript+"\n")
	mustWrite(t, ".agnostic-ai/scripts/claude/guard.sh", "#!/bin/sh\necho stash\n")
}

// A hand-edited hook script listed under sync.unmanaged survives sync and
// never counts as drift (#789).
func TestSync_UnmanagedHookScriptSurvives(t *testing.T) {
	setupHookScriptProject(t, guardScript)
	mustWrite(t, guardScript, "hand-owned\n")

	runCmd(t, "sync")

	if got := readString(t, guardScript); got != "hand-owned\n" {
		t.Errorf("user-owned hook script overwritten: %q", got)
	}
	runCmd(t, "sync", "--check")
}

// A stashed script is synced output: hand edits show as drift, and it is
// swept once its hook spec is deleted (#789).
func TestSync_HookScriptIsTrackedAndSweptWithItsHook(t *testing.T) {
	setupHookScriptProject(t)
	runCmd(t, "sync")
	if got := readString(t, guardScript); got != "#!/bin/sh\necho stash\n" {
		t.Fatalf("script not materialized: %q", got)
	}
	mustWrite(t, guardScript, "edited\n")
	runCmdExpectErr(t, "sync", "--check")
	runCmd(t, "sync")

	must(t, os.Remove(".agnostic-ai/hooks/guard.yaml"))
	runCmd(t, "sync")

	if _, err := os.Stat(guardScript); !os.IsNotExist(err) {
		t.Errorf("script of a deleted hook still on disk: %v", err)
	}
}
