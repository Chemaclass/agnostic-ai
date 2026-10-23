package cli

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestQwen_GlobalOrdinaryPortableHooksProduceNativeBlockingReplies(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executes POSIX hook commands")
	}
	for _, on := range []string{"before-tool", "prompt-submit", "after-tool", "after-edit", "stop"} {
		for _, command := range []string{`printf '{}' >&2; exit 2`, `exit 2`} {
			t.Run(on+"/"+command, func(t *testing.T) {
				home, source := globalAgentTestHome(t)
				mustWriteGlobalTest(t, filepath.Join(source, "hooks/guard.yaml"), "name: guard\non: "+on+"\ncommand: "+jsonString(command)+"\n")
				if _, warnings, err := runGlobalAgentTest("--only", "qwen"); err != nil {
					t.Fatalf("sync: %v\n%s", err, warnings)
				}
				event := map[string]string{"before-tool": "PreToolUse", "prompt-submit": "UserPromptSubmit", "after-tool": "PostToolUse", "after-edit": "PostToolUse", "stop": "Stop"}[on]
				handler := firstGlobalHandler(t, readGlobalJSON(t, filepath.Join(home, ".qwen/settings.json")), event)
				cmd := exec.Command("bash", "-c", handler["command"].(string))
				var stderr strings.Builder
				cmd.Stderr = &stderr
				stdout, err := cmd.Output()
				exit, ok := err.(*exec.ExitError)
				if !ok || exit.ExitCode() != 2 || len(stdout) != 0 {
					t.Fatalf("exit=%v stdout=%q", err, stdout)
				}
				var reply map[string]any
				if err := json.Unmarshal([]byte(stderr.String()), &reply); err != nil {
					t.Fatalf("no native blocking reply: %v, stderr=%q", err, stderr.String())
				}
				if on == "after-tool" || on == "after-edit" {
					if reply["continue"] != false {
						t.Errorf("PostToolUse did not stop: %#v", reply)
					}
				} else if reply["decision"] != "deny" {
					t.Errorf("native denial missing: %#v", reply)
				}
			})
		}
	}
}

func TestQwen_NativeSkillAndSettingsImportIntoFreshOutputs(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [qwen]\n")
	writeFile(t, filepath.Join(dir, ".qwen/settings.json"), `{"disableAllHooks":true,"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"native-guard"}]}]}}`)
	writeFile(t, filepath.Join(dir, ".qwen/skills/check/SKILL.md"), "---\nname: check\ndescription: Check\nmodel: custom-model\n---\nCheck carefully.\n")
	if out, err := runCLI(t, "import", "qwen"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if err := os.Remove(filepath.Join(dir, ".qwen/settings.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, ".qwen/skills/check/SKILL.md")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, ".agnostic-ai/hooks/generated.yaml"), "event: PreToolUse\ncommand: echo generated\n")
	for _, args := range [][]string{{"sync"}, {"sync", "--check"}, {"lint"}} {
		if out, err := runCLI(t, args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	native := readGlobalJSON(t, filepath.Join(dir, ".qwen/settings.json"))
	if native["disableAllHooks"] != true {
		t.Errorf("top-level disableAllHooks lost: %#v", native)
	}
	groups := native["hooks"].(map[string]any)["PreToolUse"].([]any)
	if len(groups) != 2 {
		t.Errorf("native and generated guards must both survive fresh sync: %#v", groups)
	}
	skill := readGlobalTest(t, filepath.Join(dir, ".qwen/skills/check/SKILL.md"))
	if !strings.Contains(skill, "model: custom-model") {
		t.Errorf("native skill model lost: %s", skill)
	}
}

func TestQwen_GlobalNativeSkillModelIsPreserved(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "skills/check/SKILL.md"), "---\nname: check\ndescription: Check\nx-qwen:\n  model: custom-model\n---\nCheck carefully.\n")
	out, warnings, err := runGlobalAgentTest("--only", "qwen")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	native := readGlobalTest(t, filepath.Join(home, ".qwen/skills/check/SKILL.md"))
	if !strings.Contains(native, "model: custom-model") {
		t.Errorf("native skill model lost: %s", native)
	}
	if strings.Contains(out+warnings, "`model` on") {
		t.Errorf("preserved native model reported unsupported: %s\n%s", out, warnings)
	}
}

func TestQwen_GlobalDisabledMCPIsRemovedWithoutDroppingEnabledPeer(t *testing.T) {
	home, source := globalAgentTestHome(t)
	disabled := filepath.Join(source, "mcps/disabled.yaml")
	mustWriteGlobalTest(t, disabled, "name: disabled\ncommand: must-not-start\n")
	mustWriteGlobalTest(t, filepath.Join(source, "mcps/active.yaml"), "name: active\ncommand: keep-active\n")
	if _, warnings, err := runGlobalAgentTest("--only", "qwen"); err != nil {
		t.Fatalf("initial sync: %v\n%s", err, warnings)
	}
	mustWriteGlobalTest(t, disabled, "name: disabled\ncommand: must-not-start\ndisabled: true\n")
	if _, warnings, err := runGlobalAgentTest("--only", "qwen"); err != nil {
		t.Fatalf("disable sync: %v\n%s", err, warnings)
	}
	servers := readGlobalJSON(t, filepath.Join(home, ".qwen/settings.json"))["mcpServers"].(map[string]any)
	if _, present := servers["disabled"]; present {
		t.Errorf("disabled server remains live: %#v", servers["disabled"])
	}
	if servers["active"].(map[string]any)["command"] != "keep-active" {
		t.Errorf("enabled peer lost: %#v", servers)
	}
}

func TestQwen_GlobalNativeMCPExclusionsPreserveUserSettings(t *testing.T) {
	home, source := globalAgentTestHome(t)
	settings := filepath.Join(home, ".qwen/settings.json")
	original := "{\n  \"mcp\": {\n    \"excluded\": [\"user-block\"],\n    \"toolIdleTimeoutMs\": 1234\n  },\n  \"mcpServers\": {\"inherited\": {\"command\": \"external-server\"}}\n}\n"
	mustWriteGlobalTest(t, settings, original)
	if err := os.Chmod(settings, 0o600); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(source, "settings/exclude.yaml")
	for _, name := range []string{"inherited", "replacement"} {
		mustWriteGlobalTest(t, input, "model: custom-model\nx-qwen:\n  mcp:\n    excluded: ["+name+"]\n")
		beforeCheck := readGlobalTest(t, settings)
		if _, _, err := runGlobalAgentTest("--only", "qwen", "--check"); err == nil {
			t.Error("check missed exclusion drift")
		}
		if got := readGlobalTest(t, settings); got != beforeCheck {
			t.Errorf("check changed native bytes: %s", got)
		}
		for _, args := range [][]string{{"--only", "qwen"}, {"--only", "qwen", "--check"}} {
			if _, warnings, err := runGlobalAgentTest(args...); err != nil {
				t.Fatalf("%v: %v\n%s", args, err, warnings)
			}
		}
		doc := readGlobalJSON(t, settings)
		mcp := doc["mcp"].(map[string]any)
		if !reflect.DeepEqual(mcp["excluded"], []any{"user-block", name}) || mcp["toolIdleTimeoutMs"] != float64(1234) {
			t.Errorf("native exclusion or sibling lost: %#v", mcp)
		}
		if doc["mcpServers"].(map[string]any)["inherited"] == nil {
			t.Errorf("unowned native server removed: %#v", doc)
		}
		var state globalState
		if err := json.Unmarshal([]byte(readGlobalTest(t, filepath.Join(source, "state/global.json"))), &state); err != nil {
			t.Fatal(err)
		}
		if !sameSetting(state.Settings["qwen"]["mcp.excluded"], []string{name}) {
			t.Errorf("unrelated exclusion claimed: %#v", state.Settings["qwen"])
		}
		before := readGlobalTest(t, settings)
		if _, warnings, err := runGlobalAgentTest("--only", "qwen"); err != nil {
			t.Fatalf("repeat: %v\n%s", err, warnings)
		}
		if got := readGlobalTest(t, settings); got != before {
			t.Errorf("repeat changed native bytes: %s", got)
		}
	}
	if err := os.Remove(input); err != nil {
		t.Fatal(err)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "qwen"); err != nil {
		t.Fatalf("remove: %v\n%s", err, warnings)
	}
	if got := readGlobalTest(t, settings); got != original {
		t.Errorf("removal changed original bytes:\n%s", got)
	}
	if info, err := os.Stat(settings); err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("private mode lost: %v, %v", info, err)
	}
}

func TestQwen_GlobalNativeMCPExclusionsKeepUserAdditions(t *testing.T) {
	home, source := globalAgentTestHome(t)
	settings := filepath.Join(home, ".qwen/settings.json")
	mustWriteGlobalTest(t, settings, `{"mcp":{"excluded":["user-block"]}}`)
	input := filepath.Join(source, "settings/exclude.yaml")
	mustWriteGlobalTest(t, input, "x-qwen:\n  mcp:\n    excluded: [inherited]\n")
	mustWriteGlobalTest(t, filepath.Join(source, "mcps/active.yaml"), "name: active\ncommand: keep-active\n")
	mustWriteGlobalTest(t, filepath.Join(source, "hooks/guard.yaml"), "on: before-tool\ncommand: exit 2\n")
	if _, warnings, err := runGlobalAgentTest("--only", "qwen"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	synced := readGlobalTest(t, settings)
	mustWriteGlobalTest(t, settings, strings.Replace(synced, `"user-block"`, `"user-block","user-added"`, 1))
	if err := os.Remove(input); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--only", "qwen"}, {"--only", "qwen", "--check"}} {
		if _, warnings, err := runGlobalAgentTest(args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, warnings)
		}
	}
	doc := readGlobalJSON(t, settings)
	if got := doc["mcp"].(map[string]any)["excluded"]; !reflect.DeepEqual(got, []any{"user-block", "user-added"}) {
		t.Errorf("user exclusion lost: %#v", got)
	}
	if doc["mcpServers"].(map[string]any)["active"].(map[string]any)["command"] != "keep-active" {
		t.Errorf("managed MCP lost: %#v", doc)
	}
	if firstGlobalHandler(t, doc, "PreToolUse")["command"] == nil {
		t.Errorf("managed hook lost: %#v", doc)
	}
}

func TestQwen_GlobalNativeMCPExclusionsCreateAndRemoveFile(t *testing.T) {
	home, source := globalAgentTestHome(t)
	settings := filepath.Join(home, ".qwen/settings.json")
	input := filepath.Join(source, "settings/exclude.yaml")
	mustWriteGlobalTest(t, input, "x-qwen:\n  mcp:\n    excluded: [inherited]\n")
	if _, warnings, err := runGlobalAgentTest("--only", "qwen"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	if got := readGlobalJSON(t, settings)["mcp"].(map[string]any)["excluded"]; !reflect.DeepEqual(got, []any{"inherited"}) {
		t.Errorf("exclusion missing: %#v", got)
	}
	if err := os.Remove(input); err != nil {
		t.Fatal(err)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "qwen"); err != nil {
		t.Fatalf("remove: %v\n%s", err, warnings)
	}
	if _, err := os.Stat(settings); !os.IsNotExist(err) {
		t.Errorf("created settings file remains: %v", err)
	}
}

func TestQwen_GlobalNativeMCPExclusionsShareOneGuardedSettingsWrite(t *testing.T) {
	home, source := globalAgentTestHome(t)
	settings := filepath.Join(home, ".qwen/settings.json")
	original := `{"mcp":{"excluded":["user-block"]}}`
	mustWriteGlobalTest(t, settings, original)
	bundle := spec.Bundle{
		Settings: []spec.Entry{{Kind: spec.KindSettings, Meta: map[string]any{"model": "custom-model", "x-qwen": map[string]any{"mcp": map[string]any{"excluded": []any{"inherited"}}}}}},
		Hooks:    []spec.Entry{{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "PreToolUse", "command": "exit 2"}}},
		MCPs:     []spec.Entry{{Kind: spec.KindMCP, Name: "active", Meta: map[string]any{"command": "keep-active"}}},
	}
	writes, _, err := buildGlobalWrites(home, source, []string{"qwen"}, nil, bundle, globalState{Version: globalStateVersion}, nil, io.Discard, "")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, write := range writes {
		if write.path != settings {
			continue
		}
		count++
		if write.planned == nil || string(write.planned.data) != original {
			t.Errorf("original snapshot missing: %+v", write.planned)
		}
		var doc map[string]any
		if err := json.Unmarshal(write.data, &doc); err != nil {
			t.Fatal(err)
		}
		if doc["model"].(map[string]any)["name"] != "custom-model" || doc["mcpServers"] == nil || doc["hooks"] == nil || !reflect.DeepEqual(doc["mcp"].(map[string]any)["excluded"], []any{"user-block", "inherited"}) {
			t.Errorf("planned settings lost a surface: %#v", doc)
		}
	}
	if count != 1 {
		t.Errorf("settings has %d writes, want one", count)
	}
	changed := `{"mcp":{"excluded":["user-block","new-block"]}}`
	mustWriteGlobalTest(t, settings, changed)
	if _, err := applyGlobalChanges(writes, nil, false, nil); err == nil || !strings.Contains(err.Error(), "changed while sync ran") {
		t.Errorf("stale write was accepted: %v", err)
	}
	if got := readGlobalTest(t, settings); got != changed {
		t.Errorf("stale write lost user change: %s", got)
	}
}

func TestQwen_ProjectNativeMCPExclusionsReleaseOnlyOwnedEntries(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [qwen]\n")
	settings := filepath.Join(dir, ".qwen/settings.json")
	writeFile(t, settings, `{"mcp":{"excluded":["user-block"],"toolIdleTimeoutMs":1234}}`)
	source := filepath.Join(dir, ".agnostic-ai/settings/exclude.yaml")
	writeFile(t, source, "x-qwen:\n  mcp:\n    excluded: [inherited]\n")
	for _, args := range [][]string{{"sync"}, {"sync", "--check"}} {
		if out, err := runCLI(t, args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("remove: %v\n%s", err, out)
	}
	mcp := readGlobalJSON(t, settings)["mcp"].(map[string]any)
	excluded := mcp["excluded"].([]any)
	if len(excluded) != 1 || excluded[0] != "user-block" || mcp["toolIdleTimeoutMs"] != float64(1234) {
		t.Errorf("removing source changed user MCP settings: %#v", mcp)
	}
}

func TestQwen_ProjectDisabledMCPIsRemovedWithoutDroppingEnabledPeer(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [qwen]\n")
	disabled := filepath.Join(dir, ".agnostic-ai/mcps/disabled.yaml")
	writeFile(t, disabled, "name: disabled\ncommand: must-not-start\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai/mcps/active.yaml"), "name: active\ncommand: keep-active\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("initial sync: %v\n%s", err, out)
	}
	writeFile(t, disabled, "name: disabled\ncommand: must-not-start\ndisabled: true\n")
	for _, args := range [][]string{{"sync"}, {"sync", "--check"}} {
		if out, err := runCLI(t, args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	servers := readGlobalJSON(t, filepath.Join(dir, ".qwen/settings.json"))["mcpServers"].(map[string]any)
	if _, present := servers["disabled"]; present {
		t.Errorf("disabled server remains live: %#v", servers["disabled"])
	}
	if servers["active"].(map[string]any)["command"] != "keep-active" {
		t.Errorf("enabled peer lost: %#v", servers)
	}
}
