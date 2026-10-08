package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const handFormattedClaudeSettings = "{\n    \"model\": \"opus\",\n    \"permissions\": {\"allow\": [\"Bash(ls:*)\"]}\n}\n"

func TestSyncGlobal_SettingsWithNothingToWriteStayByteIdentical(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "Be terse.\n")
	settings := filepath.Join(home, ".claude", "settings.json")
	mustWriteGlobalTest(t, settings, handFormattedClaudeSettings)

	for run := 1; run <= 2; run++ {
		if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
			t.Fatalf("sync %d: %v", run, err)
		}
		data, err := os.ReadFile(settings)
		if err != nil {
			t.Fatalf("sync %d removed settings.json: %v", run, err)
		}
		if string(data) != handFormattedClaudeSettings {
			t.Errorf("sync %d reformatted settings.json with nothing to write:\n%s", run, data)
		}
	}
	if _, _, err := runGlobalAgentTest("--only", "claude", "--check"); err != nil {
		t.Errorf("check after an untouched settings file: %v", err)
	}
}

func TestSyncGlobal_SettingsHookChangeKeepsKeyOrderAndIndent(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "fmt.yaml"), "event: PostToolUse\nmatcher: Edit\ncommand: gofmt -w .\n")
	settings := filepath.Join(home, ".claude", "settings.json")
	mustWriteGlobalTest(t, settings, handFormattedClaudeSettings)

	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	model, permissions, hooks := strings.Index(body, `"model"`), strings.Index(body, `"permissions"`), strings.Index(body, `"hooks"`)
	if model < 0 || permissions < model || hooks < permissions {
		t.Errorf("existing keys must keep their order, with hooks appended:\n%s", body)
	}
	if !strings.HasPrefix(body, "{\n    \"model\"") {
		t.Errorf("the file's four-space indent must survive:\n%s", body)
	}
	if !strings.Contains(body, "gofmt -w .") {
		t.Errorf("the managed hook must land:\n%s", body)
	}
}

func TestSyncGlobal_UnchangedHookWithTimeoutIsNotRewritten(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "fmt.yaml"), "event: PostToolUse\nmatcher: Edit\ncommand: gofmt -w .\ntimeout: 30\n")
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(home, ".claude", "settings.json")
	data, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	reformatted := strings.ReplaceAll(string(data), "  ", "    ")
	mustWriteGlobalTest(t, settings, reformatted)

	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != reformatted {
		t.Errorf("an unchanged hook with a numeric field must not trigger a rewrite:\n%s", after)
	}
}

func TestSyncGlobal_HookAddThenRemoveRestoresSettingsBytes(t *testing.T) {
	cases := map[string]string{
		"odd spacing":               `{"model": "opus",   "theme": "dark"}`,
		"tabs and no final newline": "{\n\t\"theme\": \"dark\",\n\t\"model\": \"opus\"\n}",
		"hand-written hooks":        "{\"model\": \"opus\",   \"theme\": \"dark\",\n  \"hooks\":  {\n    \"PostToolUse\": [ {\"matcher\": \"Write\",  \"hooks\": [{\"type\": \"command\", \"command\": \"mine\"}]} ],\n    \"PreToolUse\": [{\"matcher\": \"\", \"hooks\": [{\"command\": \"pre\", \"type\": \"command\"}]}]\n  }\n}\n",
	}
	for name, original := range cases {
		t.Run(name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			settings := filepath.Join(home, ".claude", "settings.json")
			mustWriteGlobalTest(t, settings, original)
			fmtHook := filepath.Join(source, "hooks", "fmt.yaml")
			mustWriteGlobalTest(t, fmtHook, "event: PostToolUse\nmatcher: Edit\ncommand: gofmt -w .\n")
			startHook := filepath.Join(source, "hooks", "start.yaml")
			mustWriteGlobalTest(t, startHook, "event: SessionStart\ncommand: echo hi\n")

			if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
				t.Fatal(err)
			}
			added, err := os.ReadFile(settings)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(added), "gofmt -w .") || !strings.Contains(string(added), "echo hi") {
				t.Fatalf("the managed hooks must land:\n%s", added)
			}
			for _, path := range []string{fmtHook, startHook} {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(settings)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != original {
				t.Errorf("add then remove must restore the original bytes\nwant:\n%s\ngot:\n%s\nafter add:\n%s", original, data, added)
			}
		})
	}
}

func TestSyncGlobal_HookAddThenRemoveDropsSettingsSyncCreated(t *testing.T) {
	home, source := globalAgentTestHome(t)
	hook := filepath.Join(source, "hooks", "fmt.yaml")
	mustWriteGlobalTest(t, hook, "event: PostToolUse\nmatcher: Edit\ncommand: gofmt -w .\n")
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(home, ".claude", "settings.json")
	if _, err := os.Stat(settings); err != nil {
		t.Fatalf("sync must create settings.json: %v", err)
	}
	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(settings); !os.IsNotExist(err) {
		data, _ := os.ReadFile(settings)
		t.Errorf("a settings file sync created must go once its hooks do:\n%s", data)
	}
}

func TestSyncGlobal_HookRemoveKeepsKeyAddedToSettingsSyncCreated(t *testing.T) {
	home, source := globalAgentTestHome(t)
	hook := filepath.Join(source, "hooks", "fmt.yaml")
	mustWriteGlobalTest(t, hook, "event: PostToolUse\nmatcher: Edit\ncommand: gofmt -w .\n")
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(home, ".claude", "settings.json")
	data, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(data), "{", "{\n  \"model\":   \"opus\",", 1)
	mustWriteGlobalTest(t, settings, edited)
	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	if want := "{\n  \"model\":   \"opus\"\n}\n"; string(after) != want {
		t.Errorf("removing the hook must keep the user's key as written\nwant:\n%s\ngot:\n%s", want, after)
	}
}

func TestSyncGlobal_HookAddThenRemoveKeepsUserContainers(t *testing.T) {
	targets := []struct{ name, file, event string }{
		{"claude", ".claude/settings.json", "SessionStart"},
		{"codex", ".codex/hooks.json", "SessionStart"},
		{"gemini", ".gemini/settings.json", "SessionStart"},
		{"cursor", ".cursor/hooks.json", "sessionStart"},
	}
	for _, target := range targets {
		cases := map[string]string{
			"empty object":         "{ }",
			"empty hooks":          "{\"model\": \"x\",  \"hooks\": {}}\n",
			"null hooks":           "{\"hooks\": null}",
			"empty event":          "{\n  \"hooks\": {\"" + target.event + "\": [ ]}\n}\n",
			"empty env":            "{\"env\": { },   \"model\": \"x\"}",
			"crlf":                 "{\r\n  \"model\": \"x\"\r\n}\r\n",
			"crlf with hooks":      "{\r\n  \"hooks\": {\r\n    \"" + target.event + "\": []\r\n  }\r\n}\r\n",
			"cursor template":      "{\"version\": 1, \"hooks\": {}}",
			"user version":         "{\n  \"version\": 1\n}\n",
			"one-line hooks event": "{\"hooks\": {\"" + target.event + "\": [{\"matcher\": \"\", \"hooks\": [{\"type\": \"command\", \"command\": \"mine\"}]}]}}",
		}
		for name, original := range cases {
			t.Run(target.name+"/"+name, func(t *testing.T) {
				home, source := globalAgentTestHome(t)
				native := filepath.Join(home, filepath.FromSlash(target.file))
				mustWriteGlobalTest(t, native, original)
				hook := filepath.Join(source, "hooks", "start.yaml")
				mustWriteGlobalTest(t, hook, "event: SessionStart\ncommand: echo hi\n")
				if _, _, err := runGlobalAgentTest("--only", target.name); err != nil {
					t.Fatal(err)
				}
				added, err := os.ReadFile(native)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(added), "echo hi") {
					t.Fatalf("the managed hook must land:\n%s", added)
				}
				if strings.Contains(original, "\r\n") && strings.Contains(strings.ReplaceAll(string(added), "\r\n", ""), "\n") {
					t.Errorf("a CRLF file must get CRLF lines:\n%q", added)
				}
				if err := os.Remove(hook); err != nil {
					t.Fatal(err)
				}
				if _, _, err := runGlobalAgentTest("--only", target.name); err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(native)
				if err != nil {
					t.Fatalf("a file the user wrote must stay: %v\nafter add:\n%s", err, added)
				}
				if string(data) != original {
					t.Errorf("add then remove must restore the original bytes\nwant: %q\ngot:  %q\nafter add: %q", original, data, added)
				}
			})
		}
	}
}

func TestSyncGlobal_HookRemoveAfterUpgradeDropsContainersOlderStateMade(t *testing.T) {
	home, source := globalAgentTestHome(t)
	settings := filepath.Join(home, ".claude", "settings.json")
	original := "{\"model\": \"x\"}\n"
	mustWriteGlobalTest(t, settings, original)
	hook := filepath.Join(source, "hooks", "start.yaml")
	mustWriteGlobalTest(t, hook, "event: SessionStart\ncommand: echo hi\n")
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(source, "state", "global.json")
	var state map[string]any
	if err := json.Unmarshal([]byte(readGlobalTest(t, statePath)), &state); err != nil {
		t.Fatal(err)
	}
	state["version"] = 5
	delete(state, "hookContainers")
	older, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	mustWriteGlobalTest(t, statePath, string(older))
	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	if got := readGlobalTest(t, settings); got != original {
		t.Errorf("state from before the container record must still drop the hooks and env sync added\nwant: %q\ngot:  %q", original, got)
	}
}

func TestSyncGlobal_HooksWriteStopsWhenFileChangedAfterRead(t *testing.T) {
	for _, target := range []struct{ name, file string }{
		{"claude", ".claude/settings.json"},
		{"codex", ".codex/hooks.json"},
		{"cursor", ".cursor/hooks.json"},
	} {
		t.Run(target.name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			native := filepath.Join(home, filepath.FromSlash(target.file))
			original := "{\"model\": \"x\"}\n"
			mustWriteGlobalTest(t, native, original)
			bundle := spec.Bundle{Hooks: []spec.Entry{{Kind: spec.KindHook, Name: "start", Meta: map[string]any{"event": "SessionStart", "command": "echo hi"}}}}
			writes, _, err := buildGlobalWrites(home, source, []string{target.name}, nil, bundle, globalState{Version: globalStateVersion}, nil, io.Discard, "")
			if err != nil {
				t.Fatal(err)
			}
			i := slices.IndexFunc(writes, func(w globalWrite) bool { return w.path == native })
			if i < 0 {
				t.Fatalf("no write for %s", native)
			}
			if p := writes[i].planned; p == nil || p.absent || string(p.data) != original {
				t.Fatalf("the hooks write must carry the bytes the merge read, got %+v", p)
			}
			changed := "{\"model\": \"y\"}\n"
			mustWriteGlobalTest(t, native, changed)
			if _, err := applyGlobalChanges(writes, nil, false, nil); err == nil || !strings.Contains(err.Error(), "changed while sync ran") {
				t.Fatalf("err = %v", err)
			}
			if got := readGlobalTest(t, native); got != changed {
				t.Errorf("the tool's write must survive: %q", got)
			}
		})
	}
}

func TestSyncGlobal_ReplacingEveryEntryKeepsOneLineArray(t *testing.T) {
	home, source := globalAgentTestHome(t)
	hook := filepath.Join(source, "hooks", "start.yaml")
	mustWriteGlobalTest(t, hook, "event: SessionStart\ncommand: echo one\n")
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(home, ".claude", "settings.json")
	data, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, data); err != nil {
		t.Fatal(err)
	}
	oneLine := strings.Replace(compact.String(), "{", "{\"model\": \"x\", ", 1)
	mustWriteGlobalTest(t, settings, oneLine)
	mustWriteGlobalTest(t, hook, "event: SessionStart\ncommand: echo two\n")
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.Replace(oneLine, "echo one", "echo two", 1); string(after) != want {
		t.Errorf("replacing every entry of a one-line array must keep it on one line\nwant: %s\ngot:  %s", want, after)
	}
}
