package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncGlobal_KiroHookDirectory(t *testing.T) {
	home, source := globalAgentTestHome(t)
	path := filepath.Join(home, ".kiro", "hooks", "guard.json")
	manual := filepath.Join(home, ".kiro", "hooks", "manual.json")
	mustWriteGlobalTest(t, manual, `{"version":"v1","hooks":[]}`)
	specPath := filepath.Join(source, "hooks", "guard.yaml")
	mustWriteGlobalTest(t, specPath, "name: guard\nevent: PreToolUse\ncommand: [first, second]\ntimeout: 0\ndisabled: true\n")
	out, _, err := runGlobalAgentTest("--only", "kiro", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, path) {
		t.Errorf("preview missing hook: %s", out)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("preview wrote hook: %v", err)
	}
	if _, _, err := runGlobalAgentTest("--only", "kiro"); err != nil {
		t.Fatal(err)
	}
	doc := readGlobalJSON(t, path)
	if doc["version"] != "v1" {
		t.Errorf("version: %v", doc)
	}
	hooks, ok := doc["hooks"].([]any)
	if !ok || len(hooks) != 2 {
		t.Fatalf("hooks: %v", doc)
	}
	for _, raw := range hooks {
		h := raw.(map[string]any)
		if h["trigger"] != "PreToolUse" || h["timeout"] != float64(0) || h["enabled"] != false {
			t.Errorf("hook: %v", h)
		}
	}
	if _, _, err := runGlobalAgentTest("--only", "kiro", "--check"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(specPath); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("other target removed hook: %v", err)
	}
	if _, _, err := runGlobalAgentTest("--only", "kiro"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("removed source left hook: %v", err)
	}
	if _, err := os.Stat(manual); err != nil {
		t.Errorf("manual hook removed: %v", err)
	}
}

func TestSyncGlobal_KiroHookRootAndConflicts(t *testing.T) {
	_, source := globalAgentTestHome(t)
	root := t.TempDir()
	t.Setenv("KIRO_HOME", root)
	path := filepath.Join(root, "hooks", "guard.json")
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\nevent: Stop\nx-kiro:\n  action:\n    type: agent\n    prompt: Check the work.\n")
	mustWriteGlobalTest(t, path, `{"version":"v1","hooks":[]}`)
	if _, _, err := runGlobalAgentTest("--only", "kiro"); err == nil {
		t.Fatal("unmanaged collision accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runGlobalAgentTest("--only", "kiro"); err != nil {
		t.Fatal(err)
	}
	mustWriteGlobalTest(t, path, `{"version":"v1","hooks":[]}`)
	if _, _, err := runGlobalAgentTest("--only", "kiro"); err == nil {
		t.Fatal("managed edit overwritten")
	}
	if err := os.Remove(filepath.Join(source, "hooks", "guard.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runGlobalAgentTest("--only", "kiro"); err == nil {
		t.Fatal("managed edit removed")
	}
}

func TestSyncGlobal_KiroSharedScriptStaysOutsideHooks(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\nevent: PreToolUse\ncommand: .agnostic-ai/scripts/guard.sh\nargs: [checked]\n")
	script := filepath.Join(source, "scripts", "guard.sh")
	mustWriteGlobalTest(t, script, "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runGlobalAgentTest("--only", "kiro"); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(home, ".kiro", "scripts", "guard.sh")
	hook := readGlobalJSON(t, filepath.Join(home, ".kiro", "hooks", "guard.json"))["hooks"].([]any)[0].(map[string]any)
	command := hook["action"].(map[string]any)["command"].(string)
	if !strings.Contains(command, filepath.ToSlash(output)) || !strings.Contains(command, "checked") {
		t.Errorf("command: %s", command)
	}
	raw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "#!/bin/sh\nexit 0\n" {
		t.Errorf("script: %s", raw)
	}
	if _, _, err := runGlobalAgentTest("--only", "kiro", "--check"); err != nil {
		t.Fatal(err)
	}
}

func TestSyncGlobal_KiroRootMoveRemovesObsoleteHooksAndScripts(t *testing.T) {
	home, source := globalAgentTestHome(t)
	specPath := filepath.Join(source, "hooks", "guard.yaml")
	mustWriteGlobalTest(t, specPath, "name: guard\nevent: PreToolUse\ncommand: .agnostic-ai/scripts/guard.sh\n")
	mustWriteGlobalTest(t, filepath.Join(source, "scripts", "guard.sh"), "#!/bin/sh\nexit 0\n")
	if _, _, err := runGlobalAgentTest("--only", "kiro"); err != nil {
		t.Fatal(err)
	}
	oldHook := filepath.Join(home, ".kiro", "hooks", "guard.json")
	oldScript := filepath.Join(home, ".kiro", "scripts", "guard.sh")
	moved := filepath.Join(home, "kiro-moved")
	t.Setenv("KIRO_HOME", moved)
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{oldHook, oldScript} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("unselected Kiro path removed: %s: %v", path, err)
		}
	}
	if _, _, err := runGlobalAgentTest("--only", "kiro"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{oldHook, oldScript} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("obsolete Kiro path remains: %s: %v", path, err)
		}
	}
	newHook := filepath.Join(moved, "hooks", "guard.json")
	newScript := filepath.Join(moved, "scripts", "guard.sh")
	for _, path := range []string{newHook, newScript} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("moved Kiro path missing: %s: %v", path, err)
		}
	}
	if err := os.Remove(specPath); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runGlobalAgentTest("--only", "kiro"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{oldHook, oldScript, newHook, newScript} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("removed source left Kiro path: %s: %v", path, err)
		}
	}
	if _, _, err := runGlobalAgentTest("--only", "kiro", "--check"); err != nil {
		t.Fatal(err)
	}
}

func TestSyncGlobal_KiroRootMoveKeepsEditedOldHook(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\nevent: PreToolUse\ncommand: guard.sh\n")
	if _, _, err := runGlobalAgentTest("--only", "kiro"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".kiro", "hooks", "guard.json")
	edited := `{"version":"v1","hooks":[{"name":"guard","trigger":"PreToolUse","action":{"type":"command","command":"manual.sh"}}]}`
	mustWriteGlobalTest(t, path, edited)
	moved := filepath.Join(home, "kiro-moved")
	t.Setenv("KIRO_HOME", moved)
	if _, _, err := runGlobalAgentTest("--only", "kiro"); err == nil {
		t.Fatal("root move removed edited hook")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != edited {
		t.Errorf("edited hook changed: %s", raw)
	}
	if _, err := os.Stat(filepath.Join(moved, "hooks", "guard.json")); !os.IsNotExist(err) {
		t.Errorf("failed move created partial output: %v", err)
	}
}
