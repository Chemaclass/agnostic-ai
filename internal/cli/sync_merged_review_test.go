package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func readJSONMap(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("%s: %v\n%s", path, err, data)
	}
	return got
}

func syncOutput(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	prev := logOut
	logOut = &out
	defer func() { logOut = prev }()
	printed, err := runCLI(t, append([]string{"sync"}, args...)...)
	if err != nil {
		t.Fatalf("sync %v: %v\n%s%s", args, err, printed, out.String())
	}
	return printed + out.String()
}

func removeSpecs(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
	}
}

func writeJSONFile(t *testing.T, path string, doc map[string]any) {
	t.Helper()
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, path, string(data)+"\n")
}

const geminiFmtHookSpec = ".agnostic-ai/hooks/fmt.yaml"

func syncedGeminiWithHook(t *testing.T, seed string) {
	t.Helper()
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [gemini]\n")
	if seed != "" {
		mustWriteFile(t, ".gemini/settings.json", seed)
	}
	mustWriteFile(t, geminiFmtHookSpec, "name: fmt\nevent: AfterTool\ncommand: echo hi\n")
	runSyncOK(t)
}

// Permission lists hold the user's rules beside sync's, so dropping the
// target takes out only the rules sync added (#1541).
func TestSync_RemovingTargetKeepsUserPermissionRules(t *testing.T) {
	for _, tc := range []struct {
		target, file string
	}{
		{"cursor", ".cursor/cli.json"},
		{"claude", ".claude/settings.json"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+tc.target+", cline]\n")
			mustWriteFile(t, tc.file, "{\"userKey\": \"mine\", \"permissions\": {\"allow\": [\"Bash(make)\"]}}\n")
			mustWriteFile(t, ".agnostic-ai/settings/perms.yaml", "permissions:\n  allow: [\"Bash(ls)\"]\n  deny: [\"Bash(rm:*)\"]\n")
			runSyncOK(t)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cline]\n")
			runSyncOK(t)
			got := readJSONMap(t, tc.file)
			want := map[string]any{"userKey": "mine", "permissions": map[string]any{"allow": []any{"Bash(make)"}}}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s = %#v, want %#v", tc.file, got, want)
			}
		})
	}
}

// After doctor --fix releases a merged file, check passes and the
// ledger no longer lists it.
func TestDoctorFix_ReleasedMergedFileLeavesTheLedger(t *testing.T) {
	const settings = ".gemini/settings.json"
	syncedGeminiWithHook(t, userSettings)
	removeSpecs(t, geminiFmtHookSpec)
	_, _ = runCLI(t, "doctor", "--fix")
	assertOnlyUserSettings(t, settings)
	if _, ok := readStateFile(".").Merged[settings]; ok {
		t.Error("released file still has a merged record")
	}
	runSyncOK(t, "--check")
}

// A value the user edited after sync wrote it is the user's now: the
// release keeps it and says so.
func TestSync_ReleaseKeepsEditedValueAndReportsIt(t *testing.T) {
	const settings = ".gemini/settings.json"
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [gemini]\n")
	mustWriteFile(t, settings, userSettings)
	mustWriteFile(t, ".agnostic-ai/mcps/gh.yaml", "name: gh\ncommand: npx\n")
	mustWriteFile(t, geminiFmtHookSpec, "name: fmt\nevent: AfterTool\ncommand: echo hi\n")
	runSyncOK(t)
	doc := readJSONMap(t, settings)
	doc["mcpServers"] = map[string]any{"gh": map[string]any{"command": "mine"}}
	writeJSONFile(t, settings, doc)
	removeSpecs(t, ".agnostic-ai/mcps/gh.yaml", geminiFmtHookSpec)
	out := syncOutput(t)
	got := readJSONMap(t, settings)
	if got["hooks"] != nil {
		t.Errorf("unedited hooks stayed: %#v", got)
	}
	if !reflect.DeepEqual(got["mcpServers"], doc["mcpServers"]) || got["userKey"] != "mine" {
		t.Errorf("edited value or user key lost: %#v", got)
	}
	if !strings.Contains(out, "kept orphan "+settings) {
		t.Errorf("release did not report the kept edit:\n%s", out)
	}
	runSyncOK(t)
	if !reflect.DeepEqual(readJSONMap(t, settings)["mcpServers"], doc["mcpServers"]) {
		t.Error("a later sync removed the edited value")
	}
}

func dropMergedRecords(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile(stateFilePath("."))
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]any
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	delete(state, "merged")
	state["version"] = 5
	writeJSONFile(t, stateFilePath("."), state)
}

// A ledger written before sync recorded its keys cannot tell the
// user's keys apart, so the first orphaning keeps the file (#1541).
func TestSync_LedgerWithoutKeyRecordKeepsMergedOrphan(t *testing.T) {
	const settings = ".gemini/settings.json"
	syncedGeminiWithHook(t, userSettings)
	dropMergedRecords(t)
	removeSpecs(t, geminiFmtHookSpec)
	for range 2 {
		out := syncOutput(t)
		if got := readJSONMap(t, settings); got["userKey"] != "mine" {
			t.Fatalf("user key lost: %#v", got)
		}
		if !strings.Contains(out, "kept orphan "+settings) {
			t.Errorf("kept file not reported:\n%s", out)
		}
	}
}

// A file the upgrade sync rewrites gets a record, and one with only
// sync's keys left goes away instead of staying as {}.
func TestSync_UpgradedLedgerTreatsSyncOnlyFileAsCreated(t *testing.T) {
	const settings = ".gemini/settings.json"
	syncedGeminiWithHook(t, "")
	dropMergedRecords(t)
	runSyncOK(t)
	removeSpecs(t, geminiFmtHookSpec)
	runSyncOK(t)
	if fileExists(settings) {
		data, _ := os.ReadFile(settings)
		t.Errorf("%s stayed:\n%s", settings, data)
	}
}

// A merged orphan --keep-edits kept still releases only sync's keys
// later, on a plain sync and on doctor --fix's confirmed removal.
func TestSync_KeptMergedOrphanReleasesKeysLater(t *testing.T) {
	const settings = ".gemini/settings.json"
	for _, via := range []string{"sync", "doctor"} {
		t.Run(via, func(t *testing.T) {
			syncedGeminiWithHook(t, userSettings)
			doc := readJSONMap(t, settings)
			doc["later"] = true
			writeJSONFile(t, settings, doc)
			removeSpecs(t, geminiFmtHookSpec)
			runSyncOK(t, "--keep-edits")
			if _, ok := readStateFile(".").Merged[settings]; !ok {
				t.Fatal("kept orphan lost its merged record")
			}
			if via == "sync" {
				runSyncOK(t)
			} else {
				cfg, err := config.Load(".")
				if err != nil {
					t.Fatal(err)
				}
				reports := []driftReport{{Target: "agnostic-ai", Orphaned: []string{settings}}}
				if _, err := offerOrphanRemoval(cfg, reports, false, func(string) (bool, error) { return true, nil }); err != nil {
					t.Fatal(err)
				}
			}
			got := readJSONMap(t, settings)
			if got["hooks"] != nil || got["userKey"] != "mine" || got["later"] != true {
				t.Errorf("%s = %#v", settings, got)
			}
		})
	}
}

// A merged orphan that no longer parses says so.
func TestSync_UnparseableMergedOrphanSaysWhy(t *testing.T) {
	const settings = ".gemini/settings.json"
	syncedGeminiWithHook(t, userSettings)
	mustWriteFile(t, settings, "{\"hooks\":")
	removeSpecs(t, geminiFmtHookSpec)
	out := syncOutput(t)
	if !strings.Contains(out, "does not parse") {
		t.Errorf("no parse reason:\n%s", out)
	}
	if data, _ := os.ReadFile(settings); string(data) != "{\"hooks\":" {
		t.Errorf("file changed: %s", data)
	}
}
