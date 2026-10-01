package cli

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// staleKeyFiles lists targets whose shared settings file takes an
// x-<target> key, and the file and MCP map key, when it has one.
var staleKeyFiles = []struct {
	target, file, mcpKey string
}{
	{"amp", ".amp/settings.json", "amp.mcpServers"},
	{"augment", ".augment/settings.json", "mcpServers"},
	{"gemini", ".gemini/settings.json", "mcpServers"},
	{"kilo", "kilo.jsonc", "mcp"},
	{"opencode", "opencode.json", "mcp"},
	{"qoder", ".qoder/settings.json", "mcpServers"},
}

// A key sync stops setting leaves the file while another spec still
// writes it, and the user's keys and servers stay (#1549).
func TestSync_KeySyncStopsSettingLeavesStillWrittenFile(t *testing.T) {
	for _, tc := range staleKeyFiles {
		t.Run(tc.target, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+tc.target+"]\n")
			mine := map[string]any{"command": "my-server"}
			seed, _ := json.Marshal(map[string]any{"userKey": "kept", tc.mcpKey: map[string]any{"mine": mine}})
			mustWriteFile(t, tc.file, string(seed)+"\n")
			mustWriteFile(t, ".agnostic-ai/mcps/gh.yaml", "name: gh\ncommand: npx\n")
			mustWriteFile(t, ".agnostic-ai/settings/keep.yaml", "x-"+tc.target+":\n  keepAlive: true\n  stale: true\n")
			runSyncOK(t)
			if readJSONMap(t, tc.file)["stale"] != true {
				t.Fatalf("x-%s key not written: %#v", tc.target, readJSONMap(t, tc.file))
			}

			removeSpecs(t, ".agnostic-ai/mcps/gh.yaml")
			mustWriteFile(t, ".agnostic-ai/settings/keep.yaml", "x-"+tc.target+":\n  keepAlive: true\n")
			runSyncOK(t)
			got := readJSONMap(t, tc.file)
			want := map[string]any{"userKey": "kept", "keepAlive": true, tc.mcpKey: map[string]any{"mine": mine}}
			if tc.target == "opencode" {
				want["$schema"] = got["$schema"] // opencode sets it on every write
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s = %#v, want %#v", tc.file, got, want)
			}
			runSyncOK(t, "--check")
		})
	}
}

// A key sync stopped setting that the user edited since is theirs: it
// stays.
func TestSync_StaleKeyTheUserEditedStays(t *testing.T) {
	const settings = ".gemini/settings.json"
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [gemini]\n")
	mustWriteFile(t, ".agnostic-ai/settings/model.yaml", "model: example-model\n")
	mustWriteFile(t, geminiFmtHookSpec, "name: fmt\nevent: AfterTool\ncommand: echo hi\n")
	runSyncOK(t)
	doc := readJSONMap(t, settings)
	doc["model"] = map[string]any{"name": "my-model"}
	writeJSONFile(t, settings, doc)
	removeSpecs(t, ".agnostic-ai/settings/model.yaml")
	runSyncOK(t)
	if got := readJSONMap(t, settings)["model"]; !reflect.DeepEqual(got, map[string]any{"name": "my-model"}) {
		t.Errorf("model = %#v, want the user's edit", got)
	}
	runSyncOK(t, "--check")
}

// Claude Code's settings.json drops a key sync stopped setting, and a
// hook sync deleted is released, so a copy the user restores by hand
// stays when claude is dropped (#1549, #1555).
func TestSync_ClaudeDropsStaleKeysAndReleasesDeletedHooks(t *testing.T) {
	const settings = ".claude/settings.json"
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cline]\n")
	mustWriteFile(t, settings, "{\"userKey\": \"kept\"}\n")
	mustWriteFile(t, ".agnostic-ai/settings/model.yaml", "model: example-model\n")
	mustWriteFile(t, ".agnostic-ai/hooks/fmt.yaml", "name: fmt\nevent: PostToolUse\ncommand: echo hi\n")
	runSyncOK(t)
	hooks := readJSONMap(t, settings)["hooks"]
	removeSpecs(t, ".agnostic-ai/hooks/fmt.yaml")
	mustWriteFile(t, ".agnostic-ai/settings/model.yaml", "x-claude:\n  includeCoAuthoredBy: false\n")
	runSyncOK(t)
	got := readJSONMap(t, settings)
	if got["model"] != nil || got["hooks"] != nil {
		t.Errorf("stale keys stayed: %#v", got)
	}
	runSyncOK(t, "--check")

	got["hooks"] = hooks
	writeJSONFile(t, settings, got)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cline]\n")
	runSyncOK(t)
	if final := readJSONMap(t, settings); !reflect.DeepEqual(final["hooks"], hooks) || final["userKey"] != "kept" {
		t.Errorf("hand-restored hook or user key lost: %#v", final)
	}
}

func TestSync_ClaudeRetiredEnvParentDropsOnlyUneditedValues(t *testing.T) {
	const settings = ".claude/settings.json"
	for _, tc := range []struct {
		name string
		edit map[string]any
		want map[string]any
	}{
		{"unchanged parent", nil, map[string]any{"FOO": "current"}},
		{"edited sibling", map[string]any{"BAR": "mine"}, map[string]any{"FOO": "current", "BAR": "mine"}},
		{"added sibling", map[string]any{"BAZ": "mine"}, map[string]any{"FOO": "current", "BAR": "stale", "BAZ": "mine"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\noutputs:\n  claude:\n    settings:\n      env:\n        FOO: first\n        BAR: stale\n")
			mustWriteFile(t, ".agnostic-ai/settings/model.yaml", "model: example-model\n")
			runSyncOK(t)
			if tc.edit != nil {
				doc := readJSONMap(t, settings)
				env, ok := doc["env"].(map[string]any)
				if !ok {
					t.Fatalf("env = %#v, want the generated object", doc["env"])
				}
				for key, value := range tc.edit {
					env[key] = value
				}
				writeJSONFile(t, settings, doc)
			}
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
			mustWriteFile(t, ".agnostic-ai/settings/env.yaml", "x-claude:\n  env:\n    FOO: current\n")
			runSyncOK(t)
			if got := readJSONMap(t, settings)["env"]; !reflect.DeepEqual(got, tc.want) {
				t.Errorf("env = %#v, want %#v", got, tc.want)
			}
			record, recorded := readStateFile(".").Merged[filepath.FromSlash(settings)]
			if !recorded {
				t.Error("settings.json has no merged ownership record")
			}
			for _, key := range record.Keys {
				if slices.Equal(key.Path, []string{"env"}) {
					t.Errorf("retired env parent stayed claimed: %#v", key)
				}
			}
			runSyncOK(t, "--check")
		})
	}
}
