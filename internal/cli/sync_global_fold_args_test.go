package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncGlobal_GeminiAndCursorFoldExecFormArgs(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "gemini.yaml"), "name: gemini-guard\nevent: BeforeTool\ncommand: node\nargs: [guard.js]\ntarget: gemini\n")
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "cursor.yaml"), "name: cursor-guard\nevent: beforeShellExecution\ncommand: node\nargs: [guard.js]\ntarget: cursor\n")
	only := []string{"--only", "gemini,cursor"}
	if _, _, err := runGlobalAgentTest(only...); err != nil {
		t.Fatal(err)
	}
	gemini := firstGlobalHandler(t, readGlobalJSON(t, filepath.Join(home, ".gemini", "settings.json")), "BeforeTool")
	if gemini["command"] != "node 'guard.js'" {
		t.Errorf("gemini handler = %v", gemini)
	}
	cursorEntries := readGlobalJSON(t, filepath.Join(home, ".cursor", "hooks.json"))["hooks"].(map[string]any)["beforeShellExecution"].([]any)
	if cursorEntries[0].(map[string]any)["command"] != "node 'guard.js'" {
		t.Errorf("cursor entries = %v", cursorEntries)
	}

	// A version before the fold recorded a bare `node`, and wrote it.
	for _, path := range []string{
		filepath.Join(source, "state", "global.json"),
		filepath.Join(home, ".gemini", "settings.json"),
		filepath.Join(home, ".cursor", "hooks.json"),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(strings.ReplaceAll(string(data), "node 'guard.js'", "node")), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, warnings, err := runGlobalAgentTest(only...); err != nil || warnings != "" {
		t.Fatalf("err %v, warnings %q", err, warnings)
	}
	groups := readGlobalJSON(t, filepath.Join(home, ".gemini", "settings.json"))["hooks"].(map[string]any)["BeforeTool"].([]any)
	if len(groups) != 1 || firstGlobalHandler(t, readGlobalJSON(t, filepath.Join(home, ".gemini", "settings.json")), "BeforeTool")["command"] != "node 'guard.js'" {
		t.Errorf("gemini after upgrade = %v", groups)
	}
	cursorEntries = readGlobalJSON(t, filepath.Join(home, ".cursor", "hooks.json"))["hooks"].(map[string]any)["beforeShellExecution"].([]any)
	if len(cursorEntries) != 1 || cursorEntries[0].(map[string]any)["command"] != "node 'guard.js'" {
		t.Errorf("cursor after upgrade = %v", cursorEntries)
	}
}

// Cursor also loads ~/.claude/settings.json hooks. The note follows the
// home config's targets, as project sync follows agnostic-ai.yaml, so
// --only does not hide it.
func TestSyncGlobal_NotesCursorMayRunClaudeExecFormHooksWithoutArgs(t *testing.T) {
	for _, tc := range []struct {
		config string
		only   string
		want   bool
	}{
		{"targets: [claude, cursor]\n", "claude,cursor", true},
		{"targets: [claude, cursor]\n", "claude", true},
		{"targets: [claude]\n", "claude", false},
	} {
		_, source := globalAgentTestHome(t)
		mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), tc.config)
		mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\nevent: PreToolUse\ncommand: node\nargs: [guard.js]\ntarget: claude\n")
		_, warnings, err := runGlobalAgentTest("--only", tc.only)
		if err != nil {
			t.Fatal(err)
		}
		note := "Cursor's third-party hooks docs do not list args, so 1 claude hook with args may run as a bare interpreter when Cursor loads ~/.claude/settings.json"
		if got := strings.Count(warnings, note); got != map[bool]int{true: 1, false: 0}[tc.want] {
			t.Errorf("config %q, --only %s: warnings:\n%s", tc.config, tc.only, warnings)
		}
	}
}
