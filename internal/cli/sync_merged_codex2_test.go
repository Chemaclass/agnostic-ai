package cli

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Once sync stops writing a rule, it no longer owns it: the same rule
// added by hand later stays when the target is dropped.
func TestSync_RuleSyncDroppedIsTheUsersWhenReadded(t *testing.T) {
	const file = ".cursor/cli.json"
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cursor, cline]\n")
	mustWriteFile(t, ".agnostic-ai/settings/perms.yaml", "permissions:\n  allow: [\"Bash(ls)\"]\n  deny: [\"Bash(rm:*)\"]\n")
	runSyncOK(t)
	before := readJSONMap(t, file)
	rule := before["permissions"].(map[string]any)["deny"].([]any)[0]
	mustWriteFile(t, ".agnostic-ai/settings/perms.yaml", "permissions:\n  allow: [\"Bash(ls)\"]\n")
	runSyncOK(t)
	doc := readJSONMap(t, file)
	perms := doc["permissions"].(map[string]any)
	perms["deny"] = []any{rule}
	writeJSONFile(t, file, doc)
	runSyncOK(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cline]\n")
	runSyncOK(t)
	got, _ := readJSONMap(t, file)["permissions"].(map[string]any)
	if !reflect.DeepEqual(got["deny"], []any{rule}) {
		t.Errorf("hand-added %v was removed: %#v", rule, got)
	}
}

// doctor --fix writes merged files as sync does, so it records the keys
// it set there too.
func TestDoctorFix_RecordsMergedKeysItWrites(t *testing.T) {
	const settings = ".gemini/settings.json"
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [gemini]\n")
	mustWriteFile(t, settings, userSettings)
	mustWriteFile(t, ".agnostic-ai/settings/model.yaml", "model: example-model\n")
	runSyncOK(t)
	mustWriteFile(t, geminiFmtHookSpec, "name: fmt\nevent: AfterTool\ncommand: echo hi\n")
	_, _ = runCLI(t, "doctor", "--fix")
	if readJSONMap(t, settings)["hooks"] == nil {
		t.Fatal("doctor --fix did not write the hook")
	}
	removeSpecs(t, geminiFmtHookSpec, ".agnostic-ai/settings/model.yaml")
	runSyncOK(t)
	assertOnlyUserSettings(t, settings)
}

// Dropping the protect hook moves sync's claim to the hooks it leaves,
// so once the last spec goes they leave too, and no retired hook stays
// active behind a kept orphan (#1556).
func TestSync_ProtectCleanupKeepsClaimOnGeneratedHooks(t *testing.T) {
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
	if strings.Contains(readFileString(t, settings), "agnostic-ai-protect") {
		t.Fatalf("protect hook stayed after its block left:\n%s", readFileString(t, settings))
	}
	removeSpecs(t, ".agnostic-ai/settings/model.yaml")
	runSyncOK(t)
	assertOnlyUserSettings(t, settings)
	if slices.Contains(readStateFile(".").Orphans, settings) {
		t.Errorf("%s kept as an orphan", settings)
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check: %v\n%s", err, out)
	}
}

// A hook the user adds to sync's hooks block makes the block theirs, so
// it stays through the protect cleanup and the release, and the protect
// hook, whose script is gone, does not.
func TestSync_ProtectCleanupKeepsAHandWrittenHook(t *testing.T) {
	const settings = ".gemini/settings.json"
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [gemini]\n")
	mustWriteFile(t, settings, userSettings)
	mustWriteFile(t, ".agnostic-ai/settings/model.yaml", "model: example-model\nprotected:\n  paths: [.env]\n")
	runSyncOK(t)
	doc := readJSONMap(t, settings)
	hooks := doc["hooks"].(map[string]any)
	hooks["AfterTool"] = []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "./mine.sh"}}}}
	writeJSONFile(t, settings, doc)
	mustWriteFile(t, ".agnostic-ai/settings/model.yaml", "model: example-model\n")
	runSyncOK(t)
	removeSpecs(t, ".agnostic-ai/settings/model.yaml")
	runSyncOK(t)
	body := readFileString(t, settings)
	if !strings.Contains(body, "./mine.sh") || strings.Contains(body, "agnostic-ai-protect") {
		t.Errorf("want the hand-written hook kept and the protect hook gone:\n%s", body)
	}
}
