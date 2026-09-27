package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type globalAdoptCase struct {
	target, root, hooksFile, event string
}

var globalAdoptCases = []globalAdoptCase{
	{"claude", ".claude", "settings.json", "PreToolUse"},
	{"cursor", ".cursor", "hooks.json", "beforeShellExecution"},
}

// syncedThenStateLost syncs an agent, a skill with a bundled file, a hook,
// and instructions to tc's target, then deletes the ownership state.
func syncedThenStateLost(t *testing.T, tc globalAdoptCase) (home, source string) {
	t.Helper()
	home, source = globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "# Agreements\n\n- Rule one.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "agents", "reviewer.md"), globalReviewer)
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "tidy", "SKILL.md"), "---\nname: tidy\ndescription: Tidy\n---\nTidy.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "tidy", "references", "notes.md"), "Notes.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "targets: ["+tc.target+"]\nevent: "+tc.event+"\nmatcher: Bash\ncommand: guard-command\ntimeout: 30\n")
	if _, _, err := runGlobalAgentTest("--only", tc.target); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(source, "state", "global.json")); err != nil {
		t.Fatal(err)
	}
	return home, source
}

func TestSyncGlobal_LostStateAdoptsFilesSyncWouldWrite(t *testing.T) {
	for _, tc := range globalAdoptCases {
		t.Run(tc.target, func(t *testing.T) {
			home, source := syncedThenStateLost(t, tc)

			if _, _, err := runGlobalAgentTest("--only", tc.target); err != nil {
				t.Fatalf("files identical to what sync writes must be adopted: %v", err)
			}
			data, err := os.ReadFile(filepath.Join(home, tc.root, tc.hooksFile))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(data), "guard-command") != 1 {
				t.Errorf("an identical hook must be adopted, not added twice:\n%s", data)
			}
			if _, _, err := runGlobalAgentTest("--only", tc.target, "--check"); err != nil {
				t.Errorf("check after adoption: %v", err)
			}

			// Adopted files are owned again, so a source removal sweeps them.
			for _, path := range []string{filepath.Join(source, "agents"), filepath.Join(source, "skills"), filepath.Join(source, "hooks")} {
				if err := os.RemoveAll(path); err != nil {
					t.Fatal(err)
				}
			}
			if _, _, err := runGlobalAgentTest("--only", tc.target); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{
				filepath.Join(home, tc.root, "agents", "reviewer.md"),
				filepath.Join(home, tc.root, "skills", "tidy"),
			} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Errorf("an adopted %s must be swept once its source goes, stat err = %v", path, err)
				}
			}
			if data, err := os.ReadFile(filepath.Join(home, tc.root, tc.hooksFile)); err == nil && strings.Contains(string(data), "guard-command") {
				t.Errorf("an adopted hook must be removed once its source goes:\n%s", data)
			}
		})
	}
}

func TestSyncGlobal_LostStateStillStopsOnADifferentFile(t *testing.T) {
	for _, tc := range globalAdoptCases {
		for _, edit := range []struct {
			name string
			do   func(t *testing.T, home string)
			want string
		}{
			{"agent", func(t *testing.T, home string) {
				editGlobalFile(t, filepath.Join(home, tc.root, "agents", "reviewer.md"), "Review carefully.", "Review carefully!")
			}, "unmanaged global spec collision"},
			{"skill file", func(t *testing.T, home string) {
				editGlobalFile(t, filepath.Join(home, tc.root, "skills", "tidy", "references", "notes.md"), "Notes.", "Notes!")
			}, "unmanaged global skill collision"},
			{"extra skill file", func(t *testing.T, home string) {
				mustWriteGlobalTest(t, filepath.Join(home, tc.root, "skills", "tidy", "mine.md"), "Mine.\n")
			}, "unmanaged global skill collision"},
			{"hook", func(t *testing.T, home string) {
				editGlobalFile(t, filepath.Join(home, tc.root, tc.hooksFile), `"timeout": 30`, `"timeout": 60`)
			}, tc.event + " hook"},
		} {
			t.Run(tc.target+"/"+edit.name, func(t *testing.T) {
				home, source := syncedThenStateLost(t, tc)
				edit.do(t, home)
				mustWriteGlobalTest(t, filepath.Join(source, "skills", "fresh", "SKILL.md"), "---\nname: fresh\ndescription: Fresh\n---\nFresh.\n")

				_, _, err := runGlobalAgentTest("--only", tc.target)
				if err == nil || !strings.Contains(err.Error(), edit.want) {
					t.Fatalf("a file that differs from what sync writes must stop the run with %q, got %v", edit.want, err)
				}
				if _, err := os.Stat(filepath.Join(home, tc.root, "skills", "fresh")); !os.IsNotExist(err) {
					t.Errorf("no write may land when one file differs, stat err = %v", err)
				}
				if _, err := os.Stat(filepath.Join(source, "state", "global.json")); !os.IsNotExist(err) {
					t.Errorf("the state must not be recorded when one file differs, stat err = %v", err)
				}
			})
		}
	}
}
