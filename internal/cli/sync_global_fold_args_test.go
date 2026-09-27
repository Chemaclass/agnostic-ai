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

// Cursor also runs ~/.claude/settings.json hooks, and reads only `command`.
func TestSyncGlobal_NotesCursorRunsClaudeExecFormHooksWithoutArgs(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\nevent: PreToolUse\ncommand: node\nargs: [guard.js]\ntarget: claude\n")
	_, warnings, err := runGlobalAgentTest("--only", "claude,cursor")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(warnings, "Cursor runs .claude/settings.json hooks without args") != 1 {
		t.Errorf("warnings:\n%s", warnings)
	}
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil || strings.Contains(warnings, "without args") {
		t.Errorf("claude alone: err %v, warnings %q", err, warnings)
	}
}
