package cli

import (
	"os"
	"path/filepath"
	"reflect"
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

func TestSync_ClaudePortableHookMovesToUnclaimedCustomHooks(t *testing.T) {
	const settings = ".claude/settings.json"
	for _, tc := range []struct {
		name, command string
		stop          bool
		priorUserStop bool
	}{
		{"same hook", "echo hi", false, false},
		{"changed hook", "echo changed", false, false},
		{"additional event", "echo hi", true, false},
		{"prior user hook", "echo hi", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cline]\n")
			mustWriteFile(t, settings, userSettings)
			mustWriteFile(t, ".agnostic-ai/settings/model.yaml", "model: example-model\n")
			mustWriteFile(t, ".agnostic-ai/hooks/fmt.yaml", "name: fmt\nevent: PostToolUse\ncommand: echo hi\n")
			runSyncOK(t)
			priorStop := []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "echo prior"}}}}
			if tc.priorUserStop {
				doc := readJSONMap(t, settings)
				priorHooks, ok := doc["hooks"].(map[string]any)
				if !ok {
					t.Fatalf("hooks = %#v, want the generated object", doc["hooks"])
				}
				priorHooks["Stop"] = priorStop
				writeJSONFile(t, settings, doc)
			}
			removeSpecs(t, ".agnostic-ai/hooks/fmt.yaml")
			body := "model: example-model\nx-claude:\n  hooks:\n    PostToolUse:\n      - matcher: ''\n        hooks:\n          - type: command\n            command: " + tc.command + "\n"
			if tc.stop {
				body += "    Stop:\n      - hooks:\n          - type: command\n            command: echo custom\n"
			}
			mustWriteFile(t, ".agnostic-ai/settings/model.yaml", body)
			runSyncOK(t)
			want := map[string]any{"PostToolUse": []any{map[string]any{"matcher": "", "hooks": []any{map[string]any{"type": "command", "command": tc.command}}}}}
			if tc.stop {
				want["Stop"] = []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "echo custom"}}}}
			}
			if tc.priorUserStop {
				want["Stop"] = priorStop
			}
			if got := readJSONMap(t, settings)["hooks"]; !reflect.DeepEqual(got, want) {
				t.Errorf("hooks = %#v, want %#v", got, want)
			}
			record, recorded := readStateFile(".").Merged[filepath.FromSlash(settings)]
			if !recorded {
				t.Error("settings.json has no merged ownership record")
			}
			for _, key := range record.Keys {
				if len(key.Path) > 0 && key.Path[0] == "hooks" {
					t.Errorf("custom hooks claimed as sync's: %#v", key)
				}
			}
			runSyncOK(t, "--check")

			doc := readJSONMap(t, settings)
			stop, _ := want["Stop"].([]any)
			want["Stop"] = append(stop, map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "echo mine"}}})
			doc["hooks"] = want
			writeJSONFile(t, settings, doc)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cline]\n")
			runSyncOK(t)
			if got := readJSONMap(t, settings); !reflect.DeepEqual(got["hooks"], want) || got["userKey"] != "mine" {
				t.Errorf("custom or user hooks lost on release: %#v", got)
			}
			runSyncOK(t, "--check")
		})
	}
}

// Sync writes `.cursor/hooks.json` whole, version key included, so once its
// last hook goes a later sync removes the file instead of leaving
// `{"version": 1}` behind.
func TestSync_CursorHooksFileLeavesWithItsLastHook(t *testing.T) {
	const hooksFile = ".cursor/hooks.json"
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\n")
	mustWriteFile(t, ".agnostic-ai/hooks/fmt.yaml", "name: fmt\nevent: afterFileEdit\ncommand: echo hi\n")
	runSyncOK(t)
	runSyncOK(t)
	removeSpecs(t, ".agnostic-ai/hooks/fmt.yaml")
	runSyncOK(t)
	if _, err := os.Stat(hooksFile); !os.IsNotExist(err) {
		t.Errorf("%s stayed after its last hook left: %v", hooksFile, err)
	}
	runSyncOK(t, "--check")
}

// A version key the user wrote stays when sync's hooks leave.
func TestSync_CursorHooksFileKeepsTheUsersVersionKey(t *testing.T) {
	const hooksFile = ".cursor/hooks.json"
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\n")
	mustWriteFile(t, hooksFile, `{"version": 1}`)
	mustWriteFile(t, ".agnostic-ai/hooks/fmt.yaml", "name: fmt\nevent: afterFileEdit\ncommand: echo hi\n")
	runSyncOK(t)
	runSyncOK(t)
	removeSpecs(t, ".agnostic-ai/hooks/fmt.yaml")
	runSyncOK(t)
	if got := readJSONMap(t, hooksFile); got["version"] != float64(1) || got["hooks"] != nil {
		t.Errorf("hooks file = %#v, want the user's version key alone", got)
	}
}
