package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

const userExecHook = "name: guard\nevent: SessionStart\ncommand: node\nargs: [guard.js, --strict]\ntargets: [claude, codex, qoder]\n"

func TestSyncUser_ExecFormHooksKeepArgs(t *testing.T) {
	home, source := userAgentTestHome(t)
	spec := filepath.Join(source, "hooks", "guard.yaml")
	writeFile(t, spec, userExecHook)
	only := []string{"--only", "claude,codex,qoder"}
	if _, _, err := runUserAgentTest(only...); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"claude", "qoder"} {
		handler := firstUserHandler(t, readUserJSON(t, filepath.Join(home, "."+target, "settings.json")), "SessionStart")
		if handler["command"] != "node" || !reflect.DeepEqual(handler["args"], []any{"guard.js", "--strict"}) {
			t.Errorf("%s handler = %v", target, handler)
		}
	}
	// Codex has no exec form, so the args fold into the command, quoted.
	codex := firstUserHandler(t, readUserJSON(t, filepath.Join(home, ".codex", "hooks.json")), "SessionStart")
	if codex["command"] != "export AGNOSTIC_AI_TARGET=codex; node 'guard.js' '--strict'" || codex["commandWindows"] != "node 'guard.js' '--strict'" || codex["args"] != nil {
		t.Errorf("codex handler = %v", codex)
	}

	if _, _, err := runUserAgentTest(append(only, "--check")...); err != nil {
		t.Fatalf("check after sync: %v", err)
	}
	if err := os.Remove(spec); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runUserAgentTest(only...); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{".claude/settings.json", ".qoder/settings.json", ".codex/hooks.json"} {
		if data, err := os.ReadFile(filepath.Join(home, filepath.FromSlash(path))); err == nil {
			t.Errorf("%s left behind: %s", path, data)
		}
	}
}

func TestSyncUser_HandWrittenExecFormHookIsAdopted(t *testing.T) {
	home, source := userAgentTestHome(t)
	settings := filepath.Join(home, ".claude", "settings.json")
	writeFile(t, settings, `{"hooks":{"SessionStart":[{"matcher":"","hooks":[{"type":"command","command":"node","args":["guard.js","--strict"]}]}]}}`)
	writeFile(t, filepath.Join(source, "hooks", "guard.yaml"), strings.Replace(userExecHook, "[claude, codex, qoder]", "[claude]", 1))
	if _, _, err := runUserAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	groups := readUserJSON(t, settings)["hooks"].(map[string]any)["SessionStart"].([]any)
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

func TestSyncUser_CodexExecFormHookKeepsItsCommandWindows(t *testing.T) {
	home, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\nevent: SessionStart\ncommand: node\nargs: [guard.js]\ncommandWindows: node.exe guard.js\ntarget: codex\n")
	if _, _, err := runUserAgentTest("--only", "codex"); err != nil {
		t.Fatal(err)
	}
	codex := firstUserHandler(t, readUserJSON(t, filepath.Join(home, ".codex", "hooks.json")), "SessionStart")
	if codex["command"] != "export AGNOSTIC_AI_TARGET=codex; node 'guard.js'" || codex["commandWindows"] != "node.exe guard.js" {
		t.Errorf("codex handler = %v", codex)
	}
}

// An older version dropped args, and a user may have added them back to
// the managed entry by hand. That edit is what sync writes now.
func TestSyncUser_ArgsAddedByHandToAManagedHookAreAccepted(t *testing.T) {
	for _, target := range []string{"claude", "qoder"} {
		home, source := userAgentTestHome(t)
		writeFile(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: ["+target+"]\n")
		writeFile(t, filepath.Join(source, "hooks", "guard.yaml"), strings.Replace(userExecHook, "[claude, codex, qoder]", "["+target+"]", 1))
		if _, _, err := runUserAgentTest("--only", target); err != nil {
			t.Fatal(err)
		}
		// Rewrite file and state as the old version left them: no args.
		settings := filepath.Join(home, "."+target, "settings.json")
		state := filepath.Join(source, "state", "global.json")
		argsJSON := regexp.MustCompile(`"args":\s*\[[^\]]*\],?\s*`)
		data, err := os.ReadFile(state)
		if err != nil {
			t.Fatal(err)
		}
		if !argsJSON.Match(data) {
			t.Fatalf("%s: no args recorded:\n%s", target, data)
		}
		if err := os.WriteFile(state, argsJSON.ReplaceAll(data, nil), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, warnings, err := runUserAgentTest("--only", target); err != nil || warnings != "" {
			t.Fatalf("%s: err %v, warnings %q", target, err, warnings)
		}
		handler := firstUserHandler(t, readUserJSON(t, settings), "SessionStart")
		if !reflect.DeepEqual(handler["args"], []any{"guard.js", "--strict"}) {
			t.Errorf("%s handler = %v", target, handler)
		}
	}

	// Codex recorded a bare `node`; a user who fixed it by hand to what
	// sync writes now keeps one entry, managed again.
	home, source := userAgentTestHome(t)
	spec := filepath.Join(source, "hooks", "guard.yaml")
	writeFile(t, spec, codexExecHook)
	if _, _, err := runUserAgentTest("--only", "codex"); err != nil {
		t.Fatal(err)
	}
	recordBareCodexCommand(t, source)
	if _, warnings, err := runUserAgentTest("--only", "codex"); err != nil || warnings != "" {
		t.Fatalf("codex: err %v, warnings %q", err, warnings)
	}
	hooksFile := filepath.Join(home, ".codex", "hooks.json")
	if groups := readUserJSON(t, hooksFile)["hooks"].(map[string]any)["PreToolUse"].([]any); len(groups) != 1 {
		t.Errorf("codex: want one entry, got %v", groups)
	}
	if err := os.Remove(spec); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runUserAgentTest("--only", "codex"); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(hooksFile); err == nil {
		t.Errorf("codex: the hook is still managed, so it goes with its source: %s", data)
	}
}

const codexExecHook = "name: guard\nevent: PreToolUse\nmatcher: Bash\ncommand: node\nargs: [guard.js]\ntarget: codex\n"

// recordBareCodexCommand rewrites the state as a version before the fold
// recorded it: `node` with no args.
func recordBareCodexCommand(t *testing.T, source string) {
	t.Helper()
	state := filepath.Join(source, "state", "global.json")
	data, err := os.ReadFile(state)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "node 'guard.js'") {
		t.Fatalf("no folded command recorded:\n%s", data)
	}
	if err := os.WriteFile(state, []byte(strings.ReplaceAll(string(data), "node 'guard.js'", "node")), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A hand fix that differs from what sync writes now is an edit: the run
// stops rather than adding a second copy of the guard.
func TestSyncUser_CodexBareCommandFixedByHandOtherwiseStops(t *testing.T) {
	home, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(source, "hooks", "guard.yaml"), codexExecHook)
	if _, _, err := runUserAgentTest("--only", "codex"); err != nil {
		t.Fatal(err)
	}
	recordBareCodexCommand(t, source)
	hooksFile := filepath.Join(home, ".codex", "hooks.json")
	data, err := os.ReadFile(hooksFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hooksFile, []byte(strings.ReplaceAll(string(data), "node 'guard.js'", "node guard.js")), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runUserAgentTest("--only", "codex"); err == nil || !strings.Contains(err.Error(), "was edited") {
		t.Fatalf("want the edited-hook error, got %v", err)
	}
	if groups := readUserJSON(t, hooksFile)["hooks"].(map[string]any)["PreToolUse"].([]any); len(groups) != 1 {
		t.Errorf("want one entry, got %v", groups)
	}
}
