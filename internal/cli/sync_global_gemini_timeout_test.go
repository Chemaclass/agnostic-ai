package cli

import (
	"os"
	"path/filepath"
	"regexp"
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

// v0.69.0 wrote no handler env and a timeout in seconds. A user who fixed
// the timeout by hand is accepted, and the entry gains the target env.
func TestSyncGlobal_UnsignalledGeminiHookFixedByHandIsAccepted(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), globalGeminiHook)
	if _, _, err := runGlobalAgentTest("--only", "gemini"); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(home, ".gemini", "settings.json")
	state := filepath.Join(source, "state", "global.json")
	for _, path := range []string{settings, state} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		old := stripGeminiTargetEnv(t, string(data))
		if path == state {
			old = strings.ReplaceAll(old, "30000", "30")
		}
		if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, warnings, err := runGlobalAgentTest("--only", "gemini"); err != nil || warnings != "" {
		t.Fatalf("err %v, warnings %q", err, warnings)
	}
	handler := firstGlobalHandler(t, readGlobalJSON(t, settings), "BeforeTool")
	if env, _ := handler["env"].(map[string]any); handler["timeout"] != float64(30000) || env["AGNOSTIC_AI_TARGET"] != "gemini" {
		t.Errorf("handler = %v", handler)
	}
}

// Dropping the target env from a managed hook that carried it is an
// edit, not an older version's output.
func TestSyncGlobal_RemovingTheTargetEnvFromAManagedHookStops(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), globalGeminiHook)
	if _, _, err := runGlobalAgentTest("--only", "gemini"); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(home, ".gemini", "settings.json")
	data, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte(stripGeminiTargetEnv(t, string(data))), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runGlobalAgentTest("--only", "gemini"); err == nil || !strings.Contains(err.Error(), "was edited") {
		t.Fatalf("want the edited-hook error, got %v", err)
	}
}

// stripGeminiTargetEnv removes the handler env sync writes, in any indent.
func stripGeminiTargetEnv(t *testing.T, data string) string {
	t.Helper()
	re := regexp.MustCompile(`"env":\s*\{\s*"AGNOSTIC_AI_TARGET":\s*"gemini"\s*\},?\s*`)
	if !re.MatchString(data) {
		t.Fatalf("no target env in:\n%s", data)
	}
	return re.ReplaceAllString(data, "")
}
