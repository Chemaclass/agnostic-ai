package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const globalExecHook = "name: guard\nevent: SessionStart\ncommand: node\nargs: [guard.js, --strict]\ntargets: [claude, codex, qoder]\n"

func TestSyncGlobal_ExecFormHooksKeepArgs(t *testing.T) {
	home, source := globalAgentTestHome(t)
	spec := filepath.Join(source, "hooks", "guard.yaml")
	mustWriteGlobalTest(t, spec, globalExecHook)
	only := []string{"--only", "claude,codex,qoder"}
	if _, _, err := runGlobalAgentTest(only...); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"claude", "qoder"} {
		handler := firstGlobalHandler(t, readGlobalJSON(t, filepath.Join(home, "."+target, "settings.json")), "SessionStart")
		if handler["command"] != "node" || !reflect.DeepEqual(handler["args"], []any{"guard.js", "--strict"}) {
			t.Errorf("%s handler = %v", target, handler)
		}
	}
	// Codex has no exec form: no args field, and no shell prefix on a
	// command the spec meant to run without a shell.
	codex := firstGlobalHandler(t, readGlobalJSON(t, filepath.Join(home, ".codex", "hooks.json")), "SessionStart")
	if codex["command"] != "node" || codex["args"] != nil || codex["commandWindows"] != nil {
		t.Errorf("codex handler = %v", codex)
	}

	if _, _, err := runGlobalAgentTest(append(only, "--check")...); err != nil {
		t.Fatalf("check after sync: %v", err)
	}
	if err := os.Remove(spec); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runGlobalAgentTest(only...); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{".claude/settings.json", ".qoder/settings.json", ".codex/hooks.json"} {
		if data, err := os.ReadFile(filepath.Join(home, filepath.FromSlash(path))); err == nil {
			t.Errorf("%s left behind: %s", path, data)
		}
	}
}

func TestSyncGlobal_HandWrittenExecFormHookIsAdopted(t *testing.T) {
	home, source := globalAgentTestHome(t)
	settings := filepath.Join(home, ".claude", "settings.json")
	mustWriteGlobalTest(t, settings, `{"hooks":{"SessionStart":[{"matcher":"","hooks":[{"type":"command","command":"node","args":["guard.js","--strict"]}]}]}}`)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), strings.Replace(globalExecHook, "[claude, codex, qoder]", "[claude]", 1))
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	groups := readGlobalJSON(t, settings)["hooks"].(map[string]any)["SessionStart"].([]any)
	if len(groups) != 1 {
		t.Errorf("the hand-written exec-form hook must satisfy the spec, got %d groups: %v", len(groups), groups)
	}
	state, err := os.ReadFile(filepath.Join(source, "state", "global.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(state), "guard.js") {
		t.Errorf("the user's hook must not be recorded as managed:\n%s", state)
	}
}
