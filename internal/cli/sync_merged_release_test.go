package cli

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// mergedSettingsFiles lists, per target, the files sync merges into
// beside keys it did not write, and the specs that make it write them.
var mergedSettingsFiles = []struct {
	target string
	files  []string
	specs  map[string]string
}{
	{"amp", []string{".amp/settings.json"}, nil},
	{"augment", []string{".augment/settings.json"}, nil},
	{"claude", []string{".claude/settings.json"}, nil},
	{"copilot", []string{".github/copilot/settings.json", ".vscode/mcp.json"}, nil},
	{"crush", []string{"crush.json"}, nil},
	{"factory", []string{".factory/settings.json"}, nil},
	{"gemini", []string{".gemini/settings.json"}, nil},
	{"junie", []string{".junie/config.json"}, nil},
	{"kilo", []string{"kilo.jsonc"}, nil},
	{"opencode", []string{"opencode.json"}, nil},
	{"qoder", []string{".qoder/settings.json"}, nil},
	{"windsurf", []string{".devin/config.json"}, map[string]string{
		".agnostic-ai/settings/perms.yaml": "permissions:\n  allow: [\"Bash(go test:*)\"]\n  deny: [\"Read(.env)\"]\n",
	}},
	{"zed", []string{".zed/settings.json"}, nil},
}

var mergedSettingsSpecs = map[string]string{
	".agnostic-ai/hooks/fmt.yaml":      "name: fmt\nevent: PostToolUse\ncommand: echo hi\n",
	".agnostic-ai/mcps/gh.yaml":        "name: gh\ncommand: npx\nargs: [gh-mcp]\n",
	".agnostic-ai/settings/model.yaml": "model: example-model\n",
}

const userSettings = "{\"userKey\": \"mine\", \"nested\": {\"keep\": true}}\n"

func writeMergedSettingsSpecs(t *testing.T, extra map[string]string) {
	t.Helper()
	for path, body := range mergedSettingsSpecs {
		mustWriteFile(t, path, body)
	}
	for path, body := range extra {
		mustWriteFile(t, path, body)
	}
}

func removeMergedSettingsSpecs(t *testing.T, extra map[string]string) {
	t.Helper()
	for path := range mergedSettingsSpecs {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	for path := range extra {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
}

func assertOnlyUserSettings(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	var got, want map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("%s: %v\n%s", path, err, data)
	}
	if err := json.Unmarshal([]byte(userSettings), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s holds keys other than the user's:\n%s", path, data)
	}
}

func runSyncOK(t *testing.T, args ...string) {
	t.Helper()
	out, err := runCLI(t, append([]string{"sync"}, args...)...)
	if err != nil {
		t.Fatalf("sync %v: %v\n%s", args, err, out)
	}
}

// Removing the last spec that writes a merged settings file takes out
// only the keys sync wrote and keeps the user's (#1541).
func TestSync_RemovingLastSpecKeepsUserKeysInMergedFile(t *testing.T) {
	for _, tc := range mergedSettingsFiles {
		for _, mode := range []string{"text", "json"} {
			t.Run(tc.target+"/"+mode, func(t *testing.T) {
				testutil.Chdir(t, t.TempDir())
				mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+tc.target+"]\n")
				for _, f := range tc.files {
					mustWriteFile(t, f, userSettings)
				}
				writeMergedSettingsSpecs(t, tc.specs)
				var args []string
				if mode == "json" {
					args = []string{"--json"}
				}
				runSyncOK(t, args...)
				removeMergedSettingsSpecs(t, tc.specs)
				runSyncOK(t, args...)
				for _, f := range tc.files {
					assertOnlyUserSettings(t, f)
				}
				runSyncOK(t, "--check")
			})
		}
	}
}

// Dropping the target from the config releases its merged files the
// same way: the user's keys stay (#1541).
func TestSync_RemovingTargetKeepsUserKeysInMergedFile(t *testing.T) {
	for _, tc := range mergedSettingsFiles {
		t.Run(tc.target, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+tc.target+", cline]\n")
			for _, f := range tc.files {
				mustWriteFile(t, f, userSettings)
			}
			writeMergedSettingsSpecs(t, tc.specs)
			runSyncOK(t)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cline]\n")
			runSyncOK(t)
			for _, f := range tc.files {
				assertOnlyUserSettings(t, f)
			}
		})
	}
}

// A merged file sync created goes away once nothing else is left in it.
// One that held the user's keys before sync stays, even when empty.
func TestSync_RemovingLastSpecDeletesMergedFileOnlyWhenSyncCreatedIt(t *testing.T) {
	const settings = ".gemini/settings.json"
	for _, tc := range []struct {
		name    string
		seed    string
		removed bool
	}{
		{"created by sync", "", true},
		{"user file", "{}\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [gemini]\n")
			if tc.seed != "" {
				mustWriteFile(t, settings, tc.seed)
			}
			mustWriteFile(t, ".agnostic-ai/hooks/fmt.yaml", "name: fmt\nevent: AfterTool\ncommand: echo hi\n")
			runSyncOK(t)
			runSyncOK(t)
			if err := os.Remove(".agnostic-ai/hooks/fmt.yaml"); err != nil {
				t.Fatal(err)
			}
			runSyncOK(t)
			if fileExists(settings) == tc.removed {
				data, _ := os.ReadFile(settings)
				t.Errorf("%s exists = %v, want %v:\n%s", settings, !tc.removed, !tc.removed, data)
			}
		})
	}
}

// A protected block as the last source behaves like any other (#1541).
func TestSync_RemovingProtectedBlockKeepsUserKeysInGeminiSettings(t *testing.T) {
	const settings = ".gemini/settings.json"
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [gemini]\n")
	mustWriteFile(t, settings, userSettings)
	mustWriteFile(t, ".agnostic-ai/settings/protect.yaml", "protected:\n  paths: [.env]\n")
	runSyncOK(t)
	if err := os.Remove(".agnostic-ai/settings/protect.yaml"); err != nil {
		t.Fatal(err)
	}
	runSyncOK(t)
	assertOnlyUserSettings(t, settings)
}

// doctor --fix removes what the next sync would sweep, so it must
// release a merged file the same way sync does (#1541).
func TestDoctorFix_KeepsUserKeysInMergedFile(t *testing.T) {
	const settings = ".gemini/settings.json"
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [gemini]\n")
	mustWriteFile(t, settings, userSettings)
	mustWriteFile(t, ".agnostic-ai/hooks/fmt.yaml", "name: fmt\nevent: AfterTool\ncommand: echo hi\n")
	runSyncOK(t)
	if err := os.Remove(".agnostic-ai/hooks/fmt.yaml"); err != nil {
		t.Fatal(err)
	}
	_, _ = runCLI(t, "doctor", "--fix")
	assertOnlyUserSettings(t, settings)
}

// Claude Code's settings.json keeps the user's own permission rules
// and keys once the last spec writing it goes (#1541).
func TestSync_RemovingLastSpecKeepsUserPermissionsInClaudeSettings(t *testing.T) {
	const settings = ".claude/settings.json"
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, settings, "{\"userKey\": \"mine\", \"permissions\": {\"allow\": [\"Mine\"]}}\n")
	mustWriteFile(t, ".agnostic-ai/settings/model.yaml", "model: example-model\npermissions:\n  allow: [\"Bash(go test:*)\"]\n")
	mustWriteFile(t, ".agnostic-ai/hooks/fmt.yaml", "name: fmt\nevent: PostToolUse\ncommand: echo hi\n")
	runSyncOK(t)
	for _, p := range []string{".agnostic-ai/settings/model.yaml", ".agnostic-ai/hooks/fmt.yaml"} {
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
	}
	runSyncOK(t)
	runSyncOK(t)
	data, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("%v\n%s", err, data)
	}
	if got["userKey"] != "mine" || !reflect.DeepEqual(got["permissions"], map[string]any{"allow": []any{"Mine"}}) || got["hooks"] != nil {
		t.Errorf("%s =\n%s", settings, data)
	}
	runSyncOK(t, "--check")
}

// A key sync stopped writing while other specs still wrote the file
// stays claimed, so the last removal takes it out too.
func TestSync_RemovingSpecsOneAtATimeReleasesEveryKeySyncSet(t *testing.T) {
	const settings = ".gemini/settings.json"
	for _, tc := range []struct {
		name string
		seed string
	}{
		{"user file", userSettings},
		{"created by sync", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [gemini]\n")
			if tc.seed != "" {
				mustWriteFile(t, settings, tc.seed)
			}
			writeMergedSettingsSpecs(t, nil)
			runSyncOK(t)
			for _, p := range []string{".agnostic-ai/hooks/fmt.yaml", ".agnostic-ai/mcps/gh.yaml", ".agnostic-ai/settings/model.yaml"} {
				if err := os.Remove(p); err != nil {
					t.Fatal(err)
				}
				runSyncOK(t)
			}
			if tc.seed == "" {
				if fileExists(settings) {
					data, _ := os.ReadFile(settings)
					t.Errorf("%s survived with nothing of the user's in it:\n%s", settings, data)
				}
				return
			}
			assertOnlyUserSettings(t, settings)
		})
	}
}

// An x-claude list merges with the user's entries in the file, so
// releasing it must not take the user's entries out.
func TestSync_RemovingXClaudeListKeepsUserEntries(t *testing.T) {
	const settings = ".claude/settings.json"
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, settings, "{\"sandbox\": {\"excludedCommands\": [\"mine\"]}}\n")
	mustWriteFile(t, ".agnostic-ai/settings/sandbox.yaml", "x-claude:\n  sandbox:\n    excludedCommands: [ours]\n")
	runSyncOK(t)
	if err := os.Remove(".agnostic-ai/settings/sandbox.yaml"); err != nil {
		t.Fatal(err)
	}
	runSyncOK(t)
	data, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("%v\n%s", err, data)
	}
	sandbox, _ := got["sandbox"].(map[string]any)
	list, _ := sandbox["excludedCommands"].([]any)
	if !slices.Contains(list, any("mine")) {
		t.Errorf("%s lost the user's entry:\n%s", settings, data)
	}
}
