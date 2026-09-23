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
