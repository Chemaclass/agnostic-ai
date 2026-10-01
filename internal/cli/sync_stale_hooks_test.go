package cli

import (
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// The hook the protect cleanup leaves is sync's and no spec produces it,
// so it leaves while another spec still writes the file (#1549, #1556).
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
