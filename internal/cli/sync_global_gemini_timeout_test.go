package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const globalGeminiHook = "name: guard\nevent: BeforeTool\ncommand: guard.sh\ntimeout: 30\ntarget: gemini\n"

func geminiGlobalTimeout(t *testing.T, home string) any {
	t.Helper()
	return firstGlobalHandler(t, readGlobalJSON(t, filepath.Join(home, ".gemini", "settings.json")), "BeforeTool")["timeout"]
}

// Gemini reads a hook timeout in milliseconds; a spec states seconds.
func TestSyncGlobal_GeminiTimeoutIsMilliseconds(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), globalGeminiHook)
	if _, _, err := runGlobalAgentTest("--only", "gemini"); err != nil {
		t.Fatal(err)
	}
	if got := geminiGlobalTimeout(t, home); got != float64(30000) {
		t.Errorf("timeout = %v, want 30000", got)
	}
}

func TestSyncGlobal_HandWrittenGeminiHookInMillisecondsIsAdopted(t *testing.T) {
	home, source := globalAgentTestHome(t)
	settings := filepath.Join(home, ".gemini", "settings.json")
	mustWriteGlobalTest(t, settings, `{"hooks":{"BeforeTool":[{"matcher":"","hooks":[{"type":"command","command":"guard.sh","timeout":30000}]}]}}`)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), globalGeminiHook)
	if _, _, err := runGlobalAgentTest("--only", "gemini"); err != nil {
		t.Fatal(err)
	}
	if groups := readGlobalJSON(t, settings)["hooks"].(map[string]any)["BeforeTool"].([]any); len(groups) != 1 {
		t.Errorf("the hand-written hook must satisfy the spec, got %v", groups)
	}
}

// An older version wrote and recorded the timeout in seconds. Sync
// replaces that entry, and accepts one the user already corrected.
func TestSyncGlobal_GeminiTimeoutRecordedInSecondsIsReplaced(t *testing.T) {
	for _, fixedByHand := range []bool{false, true} {
		home, source := globalAgentTestHome(t)
		mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), globalGeminiHook)
		if _, _, err := runGlobalAgentTest("--only", "gemini"); err != nil {
			t.Fatal(err)
		}
		files := []string{filepath.Join(source, "state", "global.json")}
		if !fixedByHand {
			files = append(files, filepath.Join(home, ".gemini", "settings.json"))
		}
		for _, path := range files {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), "30000") {
				t.Fatalf("%s holds no 30000:\n%s", path, data)
			}
			if err := os.WriteFile(path, []byte(strings.ReplaceAll(string(data), "30000", "30")), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if _, warnings, err := runGlobalAgentTest("--only", "gemini"); err != nil || warnings != "" {
			t.Fatalf("fixed by hand %v: err %v, warnings %q", fixedByHand, err, warnings)
		}
		if got := geminiGlobalTimeout(t, home); got != float64(30000) {
			t.Errorf("fixed by hand %v: timeout = %v, want 30000", fixedByHand, got)
		}
		groups := readGlobalJSON(t, filepath.Join(home, ".gemini", "settings.json"))["hooks"].(map[string]any)["BeforeTool"].([]any)
		if len(groups) != 1 {
			t.Errorf("fixed by hand %v: want one entry, got %v", fixedByHand, groups)
		}
	}
}
