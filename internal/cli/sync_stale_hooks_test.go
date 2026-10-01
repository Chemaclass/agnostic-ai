package cli

import (
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// The hook the protect cleanup leaves is sync's and no spec produces it,
// so it leaves in the same sync while another spec still writes the file
// (#1549, #1556).
func TestSync_RetiredGeminiHookLeavesWhileModelStays(t *testing.T) {
	const settings = ".gemini/settings.json"
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [gemini]\n")
	mustWriteFile(t, settings, userSettings)
	mustWriteFile(t, ".agnostic-ai/settings/model.yaml", "model: example-model\nprotected:\n  paths: [.env]\n")
	mustWriteFile(t, geminiFmtHookSpec, "name: fmt\nevent: AfterTool\ncommand: echo hi\n")
	runSyncOK(t)
	mustWriteFile(t, ".agnostic-ai/settings/model.yaml", "model: example-model\n")
	removeSpecs(t, geminiFmtHookSpec)
	runSyncOK(t)
	got := readJSONMap(t, settings)
	if got["hooks"] != nil {
		t.Errorf("retired hook stayed: %#v", got["hooks"])
	}
	if got["model"] == nil || got["userKey"] != "mine" {
		t.Errorf("model or user key lost: %#v", got)
	}
	runSyncOK(t, "--check")
}

// A stale claim on a whole object must not take out a child this write
// still claims, or the next check finds drift.
func TestSync_StaleParentClaimKeepsCurrentChild(t *testing.T) {
	const settings = ".claude/settings.json"
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\noutputs:\n  claude:\n    settings:\n      env:\n        FOO: bar\n")
	mustWriteFile(t, ".agnostic-ai/settings/env.yaml", "x-claude:\n  env:\n    BAR: baz\n")
	runSyncOK(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	runSyncOK(t)
	env, _ := readJSONMap(t, settings)["env"].(map[string]any)
	if env["BAR"] != "baz" {
		t.Errorf("env = %#v, want BAR kept", env)
	}
	runSyncOK(t, "--check")
}
