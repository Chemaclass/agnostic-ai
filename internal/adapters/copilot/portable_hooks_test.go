package copilot

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// repoWrapperPath is where sync writes the portable hook wrapper.
const repoWrapperPath = ".github/hooks/scripts/agnostic-ai-portable-hook.sh"

func TestEmit_PortableBeforeToolHookRunsThroughTheWrapper(t *testing.T) {
	emitTargetHooks(t, &config.Config{},
		spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
			"on": "before-tool", "match": "shell", "command": ".github/hooks/scripts/guard.sh", "args": []any{"--strict"}, "cwd": "app",
		}},
		spec.Entry{Kind: spec.KindHook, Name: "status", Meta: map[string]any{"on": "session-start", "command": "./status.sh"}},
	)
	got := readTargetFile(t, ".github/hooks/agnostic-ai.json")
	assertContainsAll(t, got,
		`"PreToolUse": [`,
		`"matcher": "Bash"`,
		`"command": "../.github/hooks/scripts/agnostic-ai-portable-hook.sh '../.github/hooks/scripts/guard.sh '\\''--strict'\\'''"`,
		`"command": "./status.sh"`,
	)
	if strings.Contains(got, `"exec"`) {
		t.Errorf("a wrapped hook runs a command line, not the exec form:\n%s", got)
	}

	emitTargetHooks(t, &config.Config{}, spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
		"on": "before-tool", "command": "bash", "args": []any{".github/hooks/scripts/guard.sh"}, "cwd": "app",
	}})
	assertContainsAll(t, readTargetFile(t, ".github/hooks/agnostic-ai.json"),
		`"command": "../.github/hooks/scripts/agnostic-ai-portable-hook.sh 'bash '\\''../.github/hooks/scripts/guard.sh'\\'''"`)
	info, err := os.Stat(repoWrapperPath)
	if err != nil || runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("wrapper = %v, %v; want an executable file", info, err)
	}
}

func TestEmit_NativeHookSyncsAsWrittenWithNoWrapper(t *testing.T) {
	emitTargetHooks(t, &config.Config{}, spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
		"event": "PreToolUse", "matcher": "Bash", "command": "./guard.sh",
	}})
	if got := readTargetFile(t, ".github/hooks/agnostic-ai.json"); !strings.Contains(got, `"command": "./guard.sh"`) || strings.Contains(got, emit.DecisionWrapperName) {
		t.Errorf("native hook must keep its command:\n%s", got)
	}
	if _, err := os.Stat(repoWrapperPath); !os.IsNotExist(err) {
		t.Errorf("no portable hook, so no wrapper: %v", err)
	}
}

func TestUnwrapPortableCommand_RestoresTheCommandSyncWrapped(t *testing.T) {
	for _, c := range []struct {
		inner, cwd string
		options    []string
	}{
		{".github/hooks/scripts/guard.sh", "", nil},
		{`../.github/hooks/scripts/guard.sh 'it'\''s'`, "app", []string{"--decision"}},
	} {
		wrapped := emit.DecisionWrapperCommand(ScriptForCwd(repoWrapperPath, c.cwd), c.options, c.inner)
		options, got, ok := UnwrapPortableCommand(wrapped, c.cwd)
		if !ok || got != c.inner || strings.Join(options, " ") != strings.Join(c.options, " ") {
			t.Errorf("unwrap %q = %q %v %v, want %q", wrapped, got, options, ok, c.inner)
		}
	}
	for _, command := range []string{"./guard.sh", repoWrapperPath + " unquoted", repoWrapperPath + " 'a' 'b'", "other/agnostic-ai-portable-hook.sh 'x'"} {
		if _, got, ok := UnwrapPortableCommand(command, ""); ok {
			t.Errorf("%q is no wrapped command, got %q", command, got)
		}
	}
}

// The wrapper gives the reply Copilot's preToolUse reads for each exit
// code a portable hook uses.
func TestDecisionWrapper_RepliesAsACopilotPreToolUseHook(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("runs the wrapper with bash")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is not on PATH")
	}
	wrapper := filepath.Join(t.TempDir(), emit.DecisionWrapperName)
	if err := os.WriteFile(wrapper, []byte(emit.PortableHookWrapper(target)), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(command string) (string, int) {
		t.Helper()
		cmd := exec.Command(wrapper, command)
		cmd.Stdin = strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"git push --force"}}`)
		out, err := cmd.Output()
		code := 0
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		return string(out), code
	}

	out, code := run(`if grep -q "push --force"; then echo "no force push" >&2; exit 2; fi`)
	var reply map[string]string
	if err := json.Unmarshal([]byte(out), &reply); err != nil || code != 2 || reply["permissionDecision"] != "deny" || reply["permissionDecisionReason"] != "no force push" {
		t.Errorf("exit 2 = %d %q (%v)", code, out, err)
	}
	if out, code = run(`echo '{"permissionDecision":"ask"}'`); code != 0 || out != "{\"permissionDecision\":\"ask\"}\n" {
		t.Errorf("exit 0 must pass stdout = %d %q", code, out)
	}
	if out, code = run(`echo '{"permissionDecision":"allow"}'; exit 1`); code != 1 || out != "" {
		t.Errorf("exit 1 = %d %q; it must stay, so Copilot denies the call when a guard breaks", code, out)
	}
}
