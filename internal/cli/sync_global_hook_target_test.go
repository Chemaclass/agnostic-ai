package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/cursor"
)

func readGlobalJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return doc
}

// firstGlobalHandler returns the first handler of a Claude-shaped event.
func firstGlobalHandler(t *testing.T, doc map[string]any, event string) map[string]any {
	t.Helper()
	groups, _ := doc["hooks"].(map[string]any)[event].([]any)
	if len(groups) == 0 {
		t.Fatalf("no %s hooks in %v", event, doc)
	}
	handlers, _ := groups[0].(map[string]any)["hooks"].([]any)
	handler, _ := handlers[0].(map[string]any)
	return handler
}

func TestSyncGlobal_HooksCarryTheTarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("AGNOSTIC_AI_HOME", filepath.Join(home, "source"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	for _, name := range []string{"CLAUDE_CONFIG_DIR", "CODEX_HOME", "GEMINI_CLI_HOME", "QODER_CONFIG_DIR"} {
		t.Setenv(name, "")
	}
	shared := filepath.Join(home, "source", "hooks", "guard.yaml")
	cursorHook := filepath.Join(home, "source", "hooks", "cursor-guard.yaml")
	mustWriteGlobalTest(t, shared, "name: guard\nevent: SessionStart\ncommand: guard.sh\ntargets: [claude, codex, gemini, qoder]\n")
	mustWriteGlobalTest(t, cursorHook, "name: cursor-guard\nevent: beforeShellExecution\ncommand: guard.sh\ntarget: cursor\n")
	only := []string{"sync", "--global", "--only", "claude,codex,cursor,gemini,qoder"}

	root := NewRootCmd("test")
	root.SetArgs(only)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	claude := readGlobalJSON(t, filepath.Join(home, ".claude", "settings.json"))
	if env, _ := claude["env"].(map[string]any); env["AGNOSTIC_AI_TARGET"] != "claude" {
		t.Errorf("claude env = %v", claude["env"])
	}
	if got := firstGlobalHandler(t, claude, "SessionStart")["command"]; got != "guard.sh" {
		t.Errorf("claude command = %v", got)
	}
	codex := firstGlobalHandler(t, readGlobalJSON(t, filepath.Join(home, ".codex", "hooks.json")), "SessionStart")
	if codex["command"] != "export AGNOSTIC_AI_TARGET=codex; guard.sh" || codex["commandWindows"] != "guard.sh" {
		t.Errorf("codex handler = %v", codex)
	}
	for target, path := range map[string]string{
		"gemini": filepath.Join(home, ".gemini", "settings.json"),
		"qoder":  filepath.Join(home, ".qoder", "settings.json"),
	} {
		handler := firstGlobalHandler(t, readGlobalJSON(t, path), "SessionStart")
		if env, _ := handler["env"].(map[string]any); env["AGNOSTIC_AI_TARGET"] != target || handler["command"] != "guard.sh" {
			t.Errorf("%s handler = %v", target, handler)
		}
	}
	cursorDoc := readGlobalJSON(t, filepath.Join(home, ".cursor", "hooks.json"))
	starts, _ := cursorDoc["hooks"].(map[string]any)["sessionStart"].([]any)
	var found bool
	for _, entry := range starts {
		found = found || entry.(map[string]any)["command"] == cursor.HookTargetCommand
	}
	if !found {
		t.Errorf("cursor sessionStart has no target hook: %v", starts)
	}

	root = NewRootCmd("test")
	root.SetArgs(append(only, "--check"))
	if err := root.Execute(); err != nil {
		t.Fatalf("check after sync: %v", err)
	}

	for _, path := range []string{shared, cursorHook} {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	root = NewRootCmd("test")
	root.SetArgs(only)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(home, ".claude", "settings.json"),
		filepath.Join(home, ".cursor", "hooks.json"),
		filepath.Join(home, ".codex", "hooks.json"),
	} {
		if data, err := os.ReadFile(path); err == nil {
			t.Errorf("%s left behind: %s", path, data)
		}
	}
}
