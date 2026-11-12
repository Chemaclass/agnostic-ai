package cli

import (
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func qwenSessionStartTimeouts(t *testing.T, path string) []any {
	t.Helper()
	var out []any
	groups := readGlobalJSON(t, path)["hooks"].(map[string]any)["SessionStart"].([]any)
	for _, group := range groups {
		for _, handler := range group.(map[string]any)["hooks"].([]any) {
			out = append(out, handler.(map[string]any)["timeout"])
		}
	}
	return out
}

func TestQwen_CommandHookTimeoutRoundTripsThroughImport(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [qwen]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai/hooks/long.yaml"), "name: long\non: session-start\ncommand: sleep 2\ntimeout: 1200\n")
	native := filepath.Join(dir, ".qwen/settings.json")
	for _, args := range [][]string{{"sync"}, {"sync", "--check"}} {
		if out, err := runCLI(t, args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	if got := qwenSessionStartTimeouts(t, native); len(got) != 1 || got[0] != float64(1200000) {
		t.Fatalf("native timeouts = %v, want [1200000]", got)
	}

	// Import keeps native handlers under x-qwen as written, so a second
	// sync must not multiply the value again.
	writeFile(t, filepath.Join(dir, ".agnostic-ai/hooks/long.yaml"), "")
	if out, err := runCLI(t, "import", "qwen"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	for _, args := range [][]string{{"sync"}, {"sync", "--check"}} {
		if out, err := runCLI(t, args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	if got := qwenSessionStartTimeouts(t, native); len(got) == 0 || got[len(got)-1] != float64(1200000) {
		t.Errorf("timeout after import and sync = %v, want 1200000", got)
	}
}

func TestSyncGlobal_QwenCommandHookTimeoutUnit(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "short.yaml"), "name: short\non: session-start\ncommand: echo short\ntimeout: 999\n")
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "edge.yaml"), "name: edge\non: session-start\ncommand: echo edge\ntimeout: 1000\n")
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "long.yaml"), "name: long\non: session-start\ncommand: echo long\ntimeout: 1200\n")

	if _, warnings, err := runGlobalAgentTest("--only", "qwen"); err != nil {
		t.Fatalf("sync: %v\n%s", err, warnings)
	}
	got := map[float64]bool{}
	for _, timeout := range qwenSessionStartTimeouts(t, filepath.Join(home, ".qwen", "settings.json")) {
		got[timeout.(float64)] = true
	}
	for _, want := range []float64{999, 1000000, 1200000} {
		if !got[want] {
			t.Errorf("global timeouts = %v, missing %v", got, want)
		}
	}
	if _, _, err := runGlobalAgentTest("--only", "qwen", "--check"); err != nil {
		t.Fatalf("check after sync: %v", err)
	}
}
