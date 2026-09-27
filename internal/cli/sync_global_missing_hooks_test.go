package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncGlobal_MissingManagedHookIsWrittenAgain(t *testing.T) {
	for _, tc := range []struct {
		name string
		lose func(t *testing.T, settings string)
		keep string
	}{
		{"file removed", func(t *testing.T, settings string) {
			if err := os.Remove(settings); err != nil {
				t.Fatal(err)
			}
		}, ""},
		{"file emptied", func(t *testing.T, settings string) {
			mustWriteGlobalTest(t, settings, "{}\n")
		}, ""},
		{"entry removed", func(t *testing.T, settings string) {
			mustWriteGlobalTest(t, settings, `{"model":"sonnet","hooks":{"PreToolUse":[{"matcher":"Edit","hooks":[{"type":"command","command":"user-command"}]}]}}`)
		}, "user-command"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "event: PreToolUse\nmatcher: Bash\ncommand: guard-command\n")
			if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
				t.Fatal(err)
			}
			settings := filepath.Join(home, ".claude", "settings.json")
			tc.lose(t, settings)

			_, warnings, err := runGlobalAgentTest("--only", "claude")
			if err != nil {
				t.Fatalf("a lost managed hook must not stop the run: %v", err)
			}
			if !strings.Contains(warnings, settings) || !strings.Contains(warnings, "PreToolUse") || !strings.Contains(warnings, "missing") {
				t.Errorf("the run must name the missing hook, got %q", warnings)
			}
			data, err := os.ReadFile(settings)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(data), "guard-command") != 1 {
				t.Errorf("the managed hook must be written again, once:\n%s", data)
			}
			if tc.keep != "" && !strings.Contains(string(data), tc.keep) {
				t.Errorf("the user's own hook must survive:\n%s", data)
			}
			if _, _, err := runGlobalAgentTest("--only", "claude", "--check"); err != nil {
				t.Errorf("check after the rebuild: %v", err)
			}
		})
	}
}

func TestSyncGlobal_OneSourceSyncedUnderTwoHomes(t *testing.T) {
	first, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "event: PreToolUse\nmatcher: Bash\ncommand: guard-command\n")
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	second := t.TempDir()
	t.Setenv("HOME", second)
	t.Setenv("USERPROFILE", second)

	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("a second home must sync from the same source: %v", err)
	}
	for _, home := range []string{first, second} {
		data, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "guard-command") {
			t.Errorf("%s lost its managed hook:\n%s", home, data)
		}
	}
}
