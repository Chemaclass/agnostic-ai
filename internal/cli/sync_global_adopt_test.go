package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/cursor"
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

			_, notes, err := runGlobalAgentTest("--only", tc.target)
			if err != nil {
				t.Fatalf("files identical to what sync writes must be adopted: %v", err)
			}
			for _, adopted := range []string{
				filepath.Join(home, tc.root, "agents", "reviewer.md"),
				filepath.Join(home, tc.root, "skills", "tidy"),
			} {
				if !strings.Contains(notes, "adopted "+adopted+":") {
					t.Errorf("the run must name adopted %s, got %q", adopted, notes)
				}
			}
			if strings.Contains(notes, filepath.Join(home, tc.root, "skills", "tidy", "SKILL.md")) {
				t.Errorf("an adopted skill folder is named once, not per file, got %q", notes)
			}
			data, err := os.ReadFile(filepath.Join(home, tc.root, tc.hooksFile))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(data), "guard-command") != 1 {
				t.Errorf("an identical hook must satisfy the source, not be added twice:\n%s", data)
			}
			if _, _, err := runGlobalAgentTest("--only", tc.target, "--check"); err != nil {
				t.Errorf("check after adoption: %v", err)
			}

			// Adopted files are owned again, so a source removal sweeps them.
			// An identical hook entry stays the user's, since nothing in it
			// says sync wrote it.
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
			if data, err := os.ReadFile(filepath.Join(home, tc.root, tc.hooksFile)); err != nil || !strings.Contains(string(data), "guard-command") {
				t.Errorf("an unrecorded hook entry must survive its source going, err = %v:\n%s", err, data)
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
			}, "not recorded as managed"},
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

// The user's hook satisfies the spec as written, and the target signal
// that sits outside it (Claude's settings env, Cursor's sessionStart
// hook) is still added, then removed with the source.
func TestSyncGlobal_UserHookMatchingASpecStaysTheUsers(t *testing.T) {
	for _, tc := range []struct {
		target, file, content string
		signal                func(doc map[string]any) bool
	}{
		{"claude", ".claude/settings.json", `{"model":"sonnet","hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"guard-command"}]}]}}`,
			func(doc map[string]any) bool {
				env, _ := doc["env"].(map[string]any)
				return env["AGNOSTIC_AI_TARGET"] == "claude"
			}},
		{"cursor", ".cursor/hooks.json", `{"version":1,"hooks":{"PreToolUse":[{"command":"guard-command","matcher":"Bash"}]}}`,
			func(doc map[string]any) bool {
				starts, _ := doc["hooks"].(map[string]any)["sessionStart"].([]any)
				return len(starts) == 1 && starts[0].(map[string]any)["command"] == cursor.HookTargetCommand
			}},
	} {
		t.Run(tc.target, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			hooksFile := filepath.Join(home, filepath.FromSlash(tc.file))
			mustWriteGlobalTest(t, hooksFile, tc.content)
			sourceHook := filepath.Join(source, "hooks", "guard.yaml")
			mustWriteGlobalTest(t, sourceHook, "targets: ["+tc.target+"]\nevent: PreToolUse\nmatcher: Bash\ncommand: guard-command\n")
			var want map[string]any
			if err := json.Unmarshal([]byte(tc.content), &want); err != nil {
				t.Fatal(err)
			}

			if _, _, err := runGlobalAgentTest("--only", tc.target); err != nil {
				t.Fatal(err)
			}
			got := readGlobalJSON(t, hooksFile)
			if !tc.signal(got) {
				t.Errorf("an adopted hook must still get the target signal:\n%v", got)
			}
			if !reflect.DeepEqual(got["hooks"].(map[string]any)["PreToolUse"], want["hooks"].(map[string]any)["PreToolUse"]) {
				t.Errorf("a user hook identical to the source must satisfy it as it is:\n%v", got)
			}
			state, err := os.ReadFile(filepath.Join(source, "state", "global.json"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(state), "guard-command") {
				t.Errorf("the user's hook must not be recorded as managed:\n%s", state)
			}

			if err := os.Remove(sourceHook); err != nil {
				t.Fatal(err)
			}
			if _, _, err := runGlobalAgentTest("--only", tc.target); err != nil {
				t.Fatal(err)
			}
			if got := readGlobalJSON(t, hooksFile); !reflect.DeepEqual(got, want) {
				t.Errorf("the user's hook must survive the source going, and the signal must go:\n%v", got)
			}
		})
	}
}

func TestSyncGlobal_UnrecordedGroupSharingASpecCommandStops(t *testing.T) {
	home, source := globalAgentTestHome(t)
	settings := filepath.Join(home, ".claude", "settings.json")
	group := `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"guard-command"},{"type":"command","command":"other-command"}]}]}}`
	mustWriteGlobalTest(t, settings, group)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "event: PreToolUse\nmatcher: Bash\ncommand: guard-command\n")

	_, _, err := runGlobalAgentTest("--only", "claude")
	if err == nil || !strings.Contains(err.Error(), "not recorded as managed") || !strings.Contains(err.Error(), "own entry") {
		t.Fatalf("a group already running the spec's command must stop the run and say how to split it, got %v", err)
	}
	data, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != group {
		t.Errorf("the user's group must stay as it is:\n%s", data)
	}
}
