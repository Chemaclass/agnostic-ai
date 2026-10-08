package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
