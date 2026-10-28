package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const commentedGlobalSettings = "{\n  // mine\n  \"theme\": \"dark\"\n}\n"

func TestSyncGlobal_CommentedSettingsKeepsCommentsWithAndWithoutHooks(t *testing.T) {
	for _, target := range []string{"gemini", "claude"} {
		for _, withHook := range []bool{false, true} {
			home, source := globalAgentTestHome(t)
			settings := filepath.Join(home, "."+target, "settings.json")
			mustWriteGlobalTest(t, settings, commentedGlobalSettings)
			mustWriteGlobalTest(t, filepath.Join(source, "settings", "d.yaml"), "model:\n  claude: opus\n  gemini: gemini-2.5-pro\n")
			if withHook {
				mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\non: before-tool\ncommand: guard.sh\n")
			}
			if _, _, err := runGlobalAgentTest("--only", target); err != nil {
				t.Fatalf("%s, hook %v: %v", target, withHook, err)
			}
			first, err := os.ReadFile(settings)
			if err != nil {
				t.Fatal(err)
			}
			text := string(first)
			if !strings.Contains(text, "// mine") {
				t.Errorf("%s, hook %v: comment lost:\n%s", target, withHook, text)
			}
			if !strings.Contains(text, `"model"`) {
				t.Errorf("%s, hook %v: model not written:\n%s", target, withHook, text)
			}
			if withHook && !strings.Contains(text, "guard.sh") {
				t.Errorf("%s: hook not written:\n%s", target, text)
			}
			if _, _, err := runGlobalAgentTest("--only", target); err != nil {
				t.Fatalf("%s, hook %v: second sync: %v", target, withHook, err)
			}
			second, err := os.ReadFile(settings)
			if err != nil {
				t.Fatal(err)
			}
			if string(second) != text {
				t.Errorf("%s, hook %v: second sync changed the file:\n%s\nwant:\n%s", target, withHook, second, text)
			}
		}
	}
}

func TestSyncGlobal_CommentInsideHookEntryIsKeptAndRemovalRestoresFile(t *testing.T) {
	home, source := globalAgentTestHome(t)
	settings := filepath.Join(home, ".claude", "settings.json")
	original := "{\n  \"hooks\": {\n    \"PreToolUse\": [\n      { // my own guard\n        \"matcher\": \"Read\",\n        \"hooks\": [{\"type\": \"command\", \"command\": \"mine.sh\"},],\n      },\n    ]\n  }\n}\n"
	mustWriteGlobalTest(t, settings, original)
	spec := filepath.Join(source, "hooks", "guard.yaml")
	mustWriteGlobalTest(t, spec, "name: guard\non: before-tool\ncommand: guard.sh\n")
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	if text := string(data); !strings.Contains(text, "// my own guard") || !strings.Contains(text, "guard.sh") {
		t.Fatalf("want the comment kept and the hook added:\n%s", text)
	}
	if err := os.Remove(spec); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	if data, err = os.ReadFile(settings); err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Errorf("removing the hook must restore the file:\n%s\nwant:\n%s", data, original)
	}
}

func TestSyncGlobal_CommentKeepsHooksFileSyncCreated(t *testing.T) {
	home, source := globalAgentTestHome(t)
	settings := filepath.Join(home, ".gemini", "settings.json")
	spec := filepath.Join(source, "hooks", "guard.yaml")
	mustWriteGlobalTest(t, spec, "name: guard\non: before-tool\ncommand: guard.sh\n")
	if _, _, err := runGlobalAgentTest("--only", "gemini"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	mustWriteGlobalTest(t, settings, "// my note\n"+string(data))
	if err := os.Remove(spec); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runGlobalAgentTest("--only", "gemini"); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(settings)
	if err != nil {
		t.Fatalf("a file holding a comment must stay: %v", err)
	}
	if text := string(data); !strings.Contains(text, "// my note") || strings.Contains(text, "guard.sh") {
		t.Errorf("want the comment kept and the hook gone:\n%s", text)
	}
}
