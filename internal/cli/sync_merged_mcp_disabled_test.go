package cli

import (
	"reflect"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Sync's MCP rejections leave with the Claude Code target, and the
// user's own stay, so re-adding the server enabled is not still rejected.
func TestSync_RemovingClaudeReleasesItsMCPRejections(t *testing.T) {
	const settings = ".claude/settings.json"
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cline]\n")
	mustWriteFile(t, settings, "{\"disabledMcpjsonServers\": [\"mine\"]}\n")
	mustWriteFile(t, ".agnostic-ai/mcps/gh.yaml", "name: gh\ncommand: npx\ndisabled: true\n")
	runSyncOK(t)
	if got := readJSONMap(t, settings)["disabledMcpjsonServers"]; !reflect.DeepEqual(got, []any{"mine", "gh"}) {
		t.Fatalf("rejections after sync = %#v", got)
	}
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cline]\n")
	runSyncOK(t)
	if got := readJSONMap(t, settings)["disabledMcpjsonServers"]; !reflect.DeepEqual(got, []any{"mine"}) {
		t.Errorf("rejections after dropping claude = %#v, want [mine]", got)
	}
}
