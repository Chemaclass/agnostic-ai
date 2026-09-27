package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type globalHookFormatCase struct {
	target, file, event string
	// withUserHook is a hooks file holding only a user's own entry.
	withUserHook string
}

var globalHookFormatCases = []globalHookFormatCase{
	{"claude", ".claude/settings.json", "PreToolUse", `{"model":"sonnet","hooks":{"PreToolUse":[{"matcher":"Edit","hooks":[{"type":"command","command":"user-command"}]}]}}`},
	{"cursor", ".cursor/hooks.json", "beforeShellExecution", `{"version":1,"hooks":{"beforeShellExecution":[{"command":"user-command"}]}}`},
}

// syncedGlobalHook syncs one guard-command hook to tc's target and
// returns the hooks file.
func syncedGlobalHook(t *testing.T, tc globalHookFormatCase) string {
	t.Helper()
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "targets: ["+tc.target+"]\nevent: "+tc.event+"\nmatcher: Bash\ncommand: guard-command\n")
	if _, _, err := runGlobalAgentTest("--only", tc.target); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(home, filepath.FromSlash(tc.file))
}

func TestSyncGlobal_MissingManagedHookIsWrittenAgain(t *testing.T) {
	for _, tc := range globalHookFormatCases {
		for _, loss := range []struct {
			name, content string
		}{
			{"file removed", ""},
			{"file emptied", "{}\n"},
			{"entry removed", tc.withUserHook},
		} {
			t.Run(tc.target+"/"+loss.name, func(t *testing.T) {
				hooksFile := syncedGlobalHook(t, tc)
				if loss.content == "" {
					if err := os.Remove(hooksFile); err != nil {
						t.Fatal(err)
					}
				} else {
					mustWriteGlobalTest(t, hooksFile, loss.content)
				}

				_, warnings, err := runGlobalAgentTest("--only", tc.target)
				if err != nil {
					t.Fatalf("a lost managed hook must not stop the run: %v", err)
				}
				if !strings.Contains(warnings, hooksFile) || !strings.Contains(warnings, tc.event) || !strings.Contains(warnings, "missing") {
					t.Errorf("the run must name the missing hook, got %q", warnings)
				}
				data, err := os.ReadFile(hooksFile)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Count(string(data), "guard-command") != 1 {
					t.Errorf("the managed hook must be written again, once:\n%s", data)
				}
				if loss.content == tc.withUserHook && !strings.Contains(string(data), "user-command") {
					t.Errorf("the user's own hook must survive:\n%s", data)
				}
				if _, _, err := runGlobalAgentTest("--only", tc.target, "--check"); err != nil {
					t.Errorf("check after the rebuild: %v", err)
				}
			})
		}
	}
}

func TestSyncGlobal_EditedManagedHookStopsTheRun(t *testing.T) {
	for _, tc := range globalHookFormatCases {
		t.Run(tc.target, func(t *testing.T) {
			hooksFile := syncedGlobalHook(t, tc)
			edited := editGlobalFile(t, hooksFile, `"command": "guard-command"`, `"command": "guard-command", "timeout": 30`)

			_, _, err := runGlobalAgentTest("--only", tc.target)
			if err == nil || !strings.Contains(err.Error(), hooksFile) || !strings.Contains(err.Error(), tc.event+" hook was edited") {
				t.Fatalf("an edited managed hook must stop the run and name it, got %v", err)
			}
			data, err := os.ReadFile(hooksFile)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != edited {
				t.Errorf("the edited hook must stay as the user left it, with no second copy:\n%s", data)
			}
		})
	}
}

func TestSyncGlobal_StateFromAnotherHomeStopsBeforeWrites(t *testing.T) {
	first, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "event: PreToolUse\nmatcher: Bash\ncommand: guard-command\n")
	mustWriteGlobalTest(t, filepath.Join(source, "agents", "reviewer.md"), globalReviewer)
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	second := t.TempDir()
	t.Setenv("HOME", second)
	t.Setenv("USERPROFILE", second)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(second, ".config"))

	_, _, err := runGlobalAgentTest("--only", "claude")
	if err == nil || !strings.Contains(err.Error(), "another home") || !strings.Contains(err.Error(), first) || !strings.Contains(err.Error(), second) {
		t.Fatalf("a state recorded under another home must stop the run and name both homes, got %v", err)
	}
	for _, path := range []string{
		filepath.Join(first, ".claude", "settings.json"),
		filepath.Join(first, ".claude", "agents", "reviewer.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("the first home's %s must survive: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(second, ".claude")); !os.IsNotExist(err) {
		t.Errorf("nothing may be written under the second home, stat err = %v", err)
	}
}
