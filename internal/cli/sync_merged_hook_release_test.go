package cli

import (
	"reflect"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// When sync deletes the hooks it wrote, it gives up its claim on them.
// A hook the user then writes back by hand, with the same value, is the
// user's, so dropping the target keeps it (#1555).
func TestSync_HandRestoredHookSurvivesDroppingClaude(t *testing.T) {
	const settings = ".claude/settings.json"
	const hookSpec = ".agnostic-ai/hooks/fmt.yaml"
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cline]\n")
	mustWriteFile(t, ".agnostic-ai/settings/model.yaml", "model: opus\n")
	mustWriteFile(t, hookSpec, "name: fmt\nevent: PostToolUse\nmatcher: Edit\ncommand: echo hi\n")
	runSyncOK(t)
	written := readJSONMap(t, settings)
	if written["hooks"] == nil || written["env"] == nil {
		t.Fatalf("sync wrote no hooks or hook env: %#v", written)
	}

	removeSpecs(t, hookSpec)
	runSyncOK(t)
	doc := readJSONMap(t, settings)
	if doc["hooks"] != nil || doc["env"] != nil {
		t.Fatalf("removing the hook spec kept hooks or env: %#v", doc)
	}
	doc["hooks"] = written["hooks"]
	doc["env"] = written["env"]
	writeJSONFile(t, settings, doc)

	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cline]\n")
	runSyncOK(t)

	got := readJSONMap(t, settings)
	want := map[string]any{"hooks": written["hooks"], "env": written["env"]}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %#v, want the hand-restored %#v", settings, got, want)
	}
}

// An x-claude key that sets hooks or the hook env again keeps them
// claimed, so dropping the target still takes out what sync wrote.
func TestSync_XClaudeHooksStayClaimedWithoutHookSpecs(t *testing.T) {
	const settings = ".claude/settings.json"
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cline]\n")
	mustWriteFile(t, settings, "{\"userKey\": \"mine\"}\n")
	mustWriteFile(t, ".agnostic-ai/settings/model.yaml", "model: opus\nx-claude:\n  env:\n    AGNOSTIC_AI_TARGET: claude\n")
	runSyncOK(t)

	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cline]\n")
	runSyncOK(t)

	if got, want := readJSONMap(t, settings), map[string]any{"userKey": "mine"}; !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %#v, want %#v", settings, got, want)
	}
}
