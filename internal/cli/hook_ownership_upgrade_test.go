package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// editLedger rewrites the sync ledger as an older version left it.
func editLedger(t *testing.T, edit func(state map[string]any)) {
	t.Helper()
	raw, err := os.ReadFile(stateFilePath("."))
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]any
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	edit(state)
	writeJSONFile(t, stateFilePath("."), state)
}

// An older version wrote .codex/hooks.json whole. A changed hook command
// after upgrading replaces the old one instead of keeping it as the
// user's (#1858).
func TestSync_UpgradeReplacesHooksAnOlderSyncWroteWhole(t *testing.T) {
	const hooks = ".codex/hooks.json"
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n")
	mustWriteFile(t, ".agnostic-ai/hooks/fmt.yaml", "name: fmt\nevent: PostToolUse\ncommand: echo old\n")
	runSyncOK(t)
	data, err := os.ReadFile(hooks)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	editLedger(t, func(state map[string]any) {
		if merged, ok := state["merged"].(map[string]any); ok {
			delete(merged, hooks)
		}
		sums, _ := state["output_sums"].(map[string]any)
		if sums == nil {
			sums = map[string]any{}
		}
		sums[hooks] = hex.EncodeToString(sum[:])
		state["output_sums"] = sums
	})
	mustWriteFile(t, ".agnostic-ai/hooks/fmt.yaml", "name: fmt\nevent: PostToolUse\ncommand: echo new\n")
	runSyncOK(t)
	got, _ := os.ReadFile(hooks)
	if strings.Contains(string(got), "echo old") || !strings.Contains(string(got), "echo new") {
		t.Errorf("old generated hook kept after upgrade:\n%s", got)
	}
}

// An older version claimed the whole Gemini `hooks` object. Hooks the
// user adds after upgrading survive the last spec's removal and a
// confirmed doctor --fix (#1858).
func TestDoctorFix_UpgradeKeepsHooksAddedAfterAWholeClaim(t *testing.T) {
	const settings = ".gemini/settings.json"
	syncedGeminiWithHook(t, "")
	runSyncOK(t)
	editLedger(t, func(state map[string]any) {
		merged := state["merged"].(map[string]any)
		record := merged[settings].(map[string]any)
		record["keys"] = []any{map[string]any{"path": []any{"hooks"}, "sum": "older"}}
	})
	doc := readJSONMap(t, settings)
	hooks := doc["hooks"].(map[string]any)
	hooks["BeforeTool"] = []any{map[string]any{"matcher": "run_shell_command", "hooks": []any{map[string]any{"type": "command", "command": "./guard.sh"}}}}
	writeJSONFile(t, settings, doc)
	runSyncOK(t)
	removeSpecs(t, geminiFmtHookSpec)
	runSyncOK(t)
	_, _ = runCLI(t, "doctor", "--fix", "--yes")
	if got, _ := os.ReadFile(settings); !strings.Contains(string(got), "./guard.sh") {
		t.Errorf("hand-written hook lost after upgrade:\n%s", got)
	}
}
