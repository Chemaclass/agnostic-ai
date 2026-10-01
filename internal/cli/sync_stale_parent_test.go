package cli

import (
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// A value that moves from a child claim to a current claim on its
// parent stays, though the child claim goes.
func TestSync_CurrentParentClaimKeepsMovedChild(t *testing.T) {
	const settings = ".claude/settings.json"
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".agnostic-ai/settings/env.yaml", "x-claude:\n  env:\n    FOO: bar\n")
	runSyncOK(t)
	removeSpecs(t, ".agnostic-ai/settings/env.yaml")
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\noutputs:\n  claude:\n    settings:\n      env:\n        FOO: bar\n")
	runSyncOK(t)
	env, _ := readJSONMap(t, settings)["env"].(map[string]any)
	if env["FOO"] != "bar" {
		t.Errorf("env = %#v, want FOO kept", env)
	}
	runSyncOK(t, "--check")
}
