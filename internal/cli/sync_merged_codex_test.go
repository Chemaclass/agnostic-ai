package cli

import (
	"os"
	"reflect"
	"runtime"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// A partial sync that upgrades an older ledger must not forget that a
// skipped target's JSON file may hold the user's keys.
func TestSync_PartialUpgradeKeepsUnrecordedMergedFile(t *testing.T) {
	const settings = ".gemini/settings.json"
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [gemini, cline]\n")
	mustWriteFile(t, settings, userSettings)
	mustWriteFile(t, geminiFmtHookSpec, "name: fmt\nevent: AfterTool\ncommand: echo hi\n")
	runSyncOK(t)
	dropMergedRecords(t)
	runSyncOK(t, "--only", "cline")
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cline]\n")
	runSyncOK(t)
	if got := readJSONMap(t, settings); got["userKey"] != "mine" {
		t.Errorf("user key lost: %#v", got)
	}
}

// A confirmation covers the bytes shown at the prompt. An edit made
// while it was open stays.
func TestDoctorFix_MergedReleaseKeepsEditMadeDuringPrompt(t *testing.T) {
	const settings = ".gemini/settings.json"
	syncedGeminiWithHook(t, userSettings)
	doc := readJSONMap(t, settings)
	doc["later"] = true
	writeJSONFile(t, settings, doc)
	removeSpecs(t, geminiFmtHookSpec)
	runSyncOK(t, "--keep-edits")
	cfg, err := config.Load(".")
	if err != nil {
		t.Fatal(err)
	}
	reports := []driftReport{{Target: "agnostic-ai", Orphaned: []string{settings}}}
	if _, err := offerOrphanRemoval(cfg, reports, false, func(string) (bool, error) {
		edited := readJSONMap(t, settings)
		edited["hooks"] = map[string]any{"Mine": []any{}}
		writeJSONFile(t, settings, edited)
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := readJSONMap(t, settings); !reflect.DeepEqual(got["hooks"], map[string]any{"Mine": []any{}}) {
		t.Errorf("edit made during the prompt was removed: %#v", got)
	}
}

// A release that fails keeps the file in the ledger, so the next sync
// retries it.
func TestSync_FailedMergedReleaseKeepsLedgerRecord(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode 000 does not block reads on Windows")
	}
	const settings = ".gemini/settings.json"
	syncedGeminiWithHook(t, userSettings)
	removeSpecs(t, geminiFmtHookSpec)
	if err := os.Chmod(settings, 0); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(settings, 0o644) }()
	if _, err := os.ReadFile(settings); err == nil {
		t.Skip("filesystem permits reading a mode-000 file")
	}
	_, _ = runCLI(t, "sync")
	state := readStateFile(".")
	if _, ok := state.Merged[settings]; !ok {
		t.Error("failed release dropped the merged record")
	}
	if err := os.Chmod(settings, 0o644); err != nil {
		t.Fatal(err)
	}
	runSyncOK(t)
	assertOnlyUserSettings(t, settings)
}

// A skills path the user listed before sync is theirs, even when sync
// would add the same path.
func TestSync_KiloKeepsSkillPathTheUserListedFirst(t *testing.T) {
	const config = "kilo.jsonc"
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [kilo]\noutputs:\n  kilo:\n    skills-dir: custom-skills\n")
	mustWriteFile(t, config, "{\"skills\": {\"paths\": [\"custom-skills\"]}}\n")
	mustWriteFile(t, ".agnostic-ai/skills/demo/SKILL.md", "---\nname: demo\ndescription: Demo skill.\n---\n\nBody.\n")
	runSyncOK(t)
	removeSpecs(t, ".agnostic-ai/skills/demo/SKILL.md")
	runSyncOK(t)
	skills, _ := readJSONMap(t, config)["skills"].(map[string]any)
	if !reflect.DeepEqual(skills["paths"], []any{"custom-skills"}) {
		t.Errorf("%s skills = %#v", config, skills)
	}
}

// A scalar sync set under Claude Code's permissions is sync's to take
// out, beside the rules it added.
func TestSync_RemovingClaudeReleasesPermissionScalars(t *testing.T) {
	const settings = ".claude/settings.json"
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cline]\n")
	mustWriteFile(t, settings, "{\"userKey\": \"mine\"}\n")
	mustWriteFile(t, ".agnostic-ai/settings/mode.yaml", "x-claude:\n  permissions:\n    defaultMode: plan\n")
	runSyncOK(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cline]\n")
	runSyncOK(t)
	if got := readJSONMap(t, settings); !reflect.DeepEqual(got, map[string]any{"userKey": "mine"}) {
		t.Errorf("%s = %#v", settings, got)
	}
}
