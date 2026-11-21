package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncUser_GeminiAndCursorFoldExecFormArgs(t *testing.T) {
	home, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(source, "hooks", "gemini.yaml"), "name: gemini-guard\nevent: BeforeTool\ncommand: node\nargs: [guard.js]\ntarget: gemini\n")
	writeFile(t, filepath.Join(source, "hooks", "cursor.yaml"), "name: cursor-guard\nevent: beforeShellExecution\ncommand: node\nargs: [guard.js]\ntarget: cursor\n")
	only := []string{"--only", "gemini,cursor"}
	if _, _, err := runUserAgentTest(only...); err != nil {
		t.Fatal(err)
	}
	gemini := firstUserHandler(t, readUserJSON(t, filepath.Join(home, ".gemini", "settings.json")), "BeforeTool")
	if gemini["command"] != "node 'guard.js'" {
		t.Errorf("gemini handler = %v", gemini)
	}
	cursorEntries := readUserJSON(t, filepath.Join(home, ".cursor", "hooks.json"))["hooks"].(map[string]any)["beforeShellExecution"].([]any)
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
	if _, warnings, err := runUserAgentTest(only...); err != nil || warnings != "" {
		t.Fatalf("err %v, warnings %q", err, warnings)
	}
	groups := readUserJSON(t, filepath.Join(home, ".gemini", "settings.json"))["hooks"].(map[string]any)["BeforeTool"].([]any)
	if len(groups) != 1 || firstUserHandler(t, readUserJSON(t, filepath.Join(home, ".gemini", "settings.json")), "BeforeTool")["command"] != "node 'guard.js'" {
		t.Errorf("gemini after upgrade = %v", groups)
	}
	cursorEntries = readUserJSON(t, filepath.Join(home, ".cursor", "hooks.json"))["hooks"].(map[string]any)["beforeShellExecution"].([]any)
	if len(cursorEntries) != 1 || cursorEntries[0].(map[string]any)["command"] != "node 'guard.js'" {
		t.Errorf("cursor after upgrade = %v", cursorEntries)
	}
}

// Cursor also loads ~/.claude/settings.json hooks. The note follows the
// home config's targets, as project sync follows agnostic-ai.yaml, so
// --only does not hide it.
func TestSyncUser_NotesCursorMayRunClaudeExecFormHooksWithoutArgs(t *testing.T) {
	for _, tc := range []struct {
		config string
		only   string
		want   bool
	}{
		{"targets: [claude, cursor]\n", "claude,cursor", true},
		{"targets: [claude, cursor]\n", "claude", true},
		{"targets: [claude]\n", "claude", false},
	} {
		_, source := userAgentTestHome(t)
		writeFile(t, filepath.Join(source, "agnostic-ai.yaml"), tc.config)
		writeFile(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\nevent: PreToolUse\ncommand: node\nargs: [guard.js]\ntarget: claude\n")
		_, warnings, err := runUserAgentTest("--only", tc.only)
		if err != nil {
			t.Fatal(err)
		}
		note := "Cursor's third-party hooks docs do not list args, so 1 claude hook with args may run as a bare interpreter when Cursor loads ~/.claude/settings.json"
		if got := strings.Count(warnings, note); got != map[bool]int{true: 1, false: 0}[tc.want] {
			t.Errorf("config %q, --only %s: warnings:\n%s", tc.config, tc.only, warnings)
		}
	}
}

func TestSyncUser_ClaudeHookFailClosedWritesOnFailureBlock(t *testing.T) {
	home, source := userAgentTestHome(t)
	spec := filepath.Join(source, "hooks", "guard.yaml")
	writeFile(t, spec, "name: guard\nevent: PreToolUse\nmatcher: Bash\ncommand: guard.sh\nfailClosed: true\n")
	only := []string{"--only", "claude,codex"}
	if _, _, err := runUserAgentTest(only...); err != nil {
		t.Fatal(err)
	}
	claude := firstUserHandler(t, readUserJSON(t, filepath.Join(home, ".claude", "settings.json")), "PreToolUse")
	if claude["onFailure"] != "block" {
		t.Errorf("claude handler = %v, want onFailure block", claude)
	}
	codex := readUserJSON(t, filepath.Join(home, ".codex", "hooks.json"))
	if strings.Contains(fmt.Sprint(codex), "onFailure") {
		t.Errorf("codex hooks carry onFailure: %v", codex)
	}

	writeFile(t, spec, "name: guard\nevent: PreToolUse\nmatcher: Bash\ncommand: guard.sh\n")
	if _, _, err := runUserAgentTest(only...); err != nil {
		t.Fatal(err)
	}
	groups := readUserJSON(t, filepath.Join(home, ".claude", "settings.json"))["hooks"].(map[string]any)["PreToolUse"].([]any)
	if len(groups) != 1 {
		t.Fatalf("PreToolUse groups = %v, want the one entry updated in place", groups)
	}
	if _, set := firstUserHandler(t, readUserJSON(t, filepath.Join(home, ".claude", "settings.json")), "PreToolUse")["onFailure"]; set {
		t.Error("onFailure stayed after failClosed was removed")
	}
}

func TestSyncUser_ClaudeFailClosedNotesIgnoredFailures(t *testing.T) {
	cases := []struct {
		name     string
		event    string
		flags    string
		wantNote bool
	}{
		{"stop", "Stop", "", true},
		{"async", "PreToolUse", "async: true\n", true},
		{"async rewake", "PostToolUse", "asyncRewake: true\n", true},
		{"synchronous", "PreToolUse", "", false},
		{"permission request", "PermissionRequest", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home, source := userAgentTestHome(t)
			writeFile(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\nevent: "+c.event+"\ncommand: guard.sh\nfailClosed: true\n"+c.flags)
			_, warnings, err := runUserAgentTest("--only", "claude")
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(warnings, "`failClosed`"); got != c.wantNote {
				t.Errorf("failClosed note = %t, want %t: %s", got, c.wantNote, warnings)
			}
			if c.wantNote && !strings.Contains(warnings, "synchronous PreToolUse") {
				t.Errorf("note must suggest a synchronous failure gate: %s", warnings)
			}
			handler := firstUserHandler(t, readUserJSON(t, filepath.Join(home, ".claude", "settings.json")), c.event)
			if handler["onFailure"] != "block" {
				t.Errorf("onFailure must remain: %v", handler)
			}
		})
	}
}
