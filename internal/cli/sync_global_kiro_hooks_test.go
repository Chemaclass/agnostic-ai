package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const globalKiroGuard = "name: guard\nevent: PreToolUse\nmatcher: shell\ncommand: guard-hook\ntimeout: 10\n"

func readKiroHookFile(t *testing.T, path string) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(readGlobalTest(t, path)), &doc); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return doc
}

func TestSyncGlobal_KiroHooksReachUserHooksDir(t *testing.T) {
	home, source := globalAgentTestHome(t)
	hookSpec := filepath.Join(source, "hooks", "guard.yaml")
	mustWriteGlobalTest(t, hookSpec, globalKiroGuard)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "other.yaml"), "name: other\nevent: PreToolUse\ntarget: claude\ncommand: other-hook\n")
	mine := filepath.Join(home, ".kiro", "hooks", "mine.json")
	const mineBody = "{\"version\":\"v1\",\"hooks\":[]}\n"
	mustWriteGlobalTest(t, mine, mineBody)

	if _, w, err := runGlobalAgentTest("--only", "kiro"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	path := filepath.Join(home, ".kiro", "hooks", "guard.json")
	doc := readKiroHookFile(t, path)
	if doc["version"] != "v1" {
		t.Errorf("version = %v", doc["version"])
	}
	hooks, _ := doc["hooks"].([]any)
	if len(hooks) != 1 {
		t.Fatalf("hooks = %v", doc["hooks"])
	}
	hook, _ := hooks[0].(map[string]any)
	action, _ := hook["action"].(map[string]any)
	if hook["name"] != "guard" || hook["trigger"] != "PreToolUse" || hook["matcher"] != "shell" || hook["timeout"] != float64(10) || action["type"] != "command" || action["command"] != "guard-hook" {
		t.Errorf("hook = %v", hook)
	}
	if _, err := os.Stat(filepath.Join(home, ".kiro", "hooks", "other.json")); !os.IsNotExist(err) {
		t.Errorf("a hook scoped to another target must not reach Kiro (%v)", err)
	}
	if _, w, err := runGlobalAgentTest("--only", "kiro", "--check"); err != nil {
		t.Fatalf("check after sync: %v\n%s", err, w)
	}

	if err := os.Remove(hookSpec); err != nil {
		t.Fatal(err)
	}
	if _, w, err := runGlobalAgentTest("--only", "kiro"); err != nil {
		t.Fatalf("sync after removal: %v\n%s", err, w)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("removing the spec must remove its hook file (%v)", err)
	}
	if got := readGlobalTest(t, mine); got != mineBody {
		t.Errorf("a hook file the user wrote must stay:\n%q", got)
	}
}

func TestSyncGlobal_KiroHooksFollowKiroHome(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), globalKiroGuard)
	dir := filepath.Join(home, "kiro-home")
	t.Setenv("KIRO_HOME", dir)
	if _, w, err := runGlobalAgentTest("--only", "kiro"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	readKiroHookFile(t, filepath.Join(dir, "hooks", "guard.json"))
	if _, err := os.Stat(filepath.Join(home, ".kiro", "hooks")); !os.IsNotExist(err) {
		t.Errorf("hooks must leave the default root when KIRO_HOME is set (%v)", err)
	}
}

func TestSyncGlobal_KiroNeutralHookScriptsCopyToUserScripts(t *testing.T) {
	home, source := globalAgentTestHome(t)
	const body = "#!/bin/sh\necho kiro-guard\n"
	mustWriteGlobalTest(t, filepath.Join(source, "scripts", "guard.sh"), body)
	if err := os.Chmod(filepath.Join(source, "scripts", "guard.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\nevent: AgentSpawn\ncommand: .agnostic-ai/scripts/guard.sh\nargs: [--strict]\n")
	if _, w, err := runGlobalAgentTest("--only", "kiro"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	script := filepath.Join(home, ".kiro", "scripts", "guard.sh")
	if got := readGlobalTest(t, script); got != body {
		t.Errorf("script = %q", got)
	}
	hooks, _ := readKiroHookFile(t, filepath.Join(home, ".kiro", "hooks", "guard.json"))["hooks"].([]any)
	hook, _ := hooks[0].(map[string]any)
	action, _ := hook["action"].(map[string]any)
	if command, _ := action["command"].(string); !strings.Contains(command, script) || !strings.HasSuffix(command, " '--strict'") {
		t.Errorf("command must run the copied script with its args: %q", command)
	}

	if err := os.RemoveAll(filepath.Join(source, "hooks")); err != nil {
		t.Fatal(err)
	}
	if _, w, err := runGlobalAgentTest("--only", "kiro"); err != nil {
		t.Fatalf("sync after removal: %v\n%s", err, w)
	}
	if _, err := os.Stat(script); !os.IsNotExist(err) {
		t.Errorf("the copied script must go with its hook (%v)", err)
	}
}
