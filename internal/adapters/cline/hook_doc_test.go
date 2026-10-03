package cline

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestHookScript_IsTheEventScriptSyncWritesForOneSpec(t *testing.T) {
	h := spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "PreToolUse", "command": []any{"./a.sh", "./b.sh"}}}
	emitTargetHooks(t, &config.Config{}, h)

	got := HookScript(h)
	if want := "set -e\nexport AGNOSTIC_AI_TARGET=cline\n" + clineBlockPrelude + "\nset +e\n(\nset -e\n./a.sh\n)" + clineBlockOnExit2 + "\nset +e\n(\nset -e\n./b.sh\n)" + clineBlockOnExit2; got != want {
		t.Errorf("HookScript = %q", got)
	}
	if synced := readTargetFile(t, ".clinerules/hooks/PreToolUse"); !strings.HasSuffix(synced, got) {
		t.Errorf("the synced script ends with a different body:\n%s", synced)
	}
	if HookScript(spec.Entry{Meta: map[string]any{"event": "PreToolUse"}}) != "" {
		t.Error("a spec with no command has no script")
	}
}

func TestHookScriptPath_FoldsTheEventAndFollowsHooksDir(t *testing.T) {
	if got := HookScriptPath(&config.Config{}, "pretooluse"); got != filepath.Join(".clinerules", "hooks", "PreToolUse") {
		t.Errorf("default path = %q", got)
	}
	cfg := &config.Config{Outputs: map[string]config.Output{"cline": {HooksDir: ".cline/hooks"}}}
	if got := HookScriptPath(cfg, "PostToolUse"); got != filepath.Join(".cline/hooks", "PostToolUse") {
		t.Errorf("hooks-dir path = %q", got)
	}
	if got := HookScriptPath(&config.Config{}, "Stop"); got != "" {
		t.Errorf("Cline reads no Stop script, got %q", got)
	}
}

// The script turns exit 2 into a cancel reply both Cline runtimes read,
// stops on any other failure, and passes a reply a command prints itself.
func TestHookScript_TurnsExit2IntoACancelReply(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("runs the script with bash")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is not on PATH")
	}
	run := func(commands ...string) (string, int) {
		t.Helper()
		cmd := exec.Command("bash", "-c", hookScript(commands))
		cmd.Stdin = strings.NewReader("{}")
		out, err := cmd.Output()
		code := 0
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		return string(out), code
	}

	out, code := run(`printf 'line "one"\n\tline two\n' >&2; exit 2`, "echo never")
	if code != 0 || out != "HOOK_CONTROL\t{\"cancel\": true, \"errorMessage\": \"line \\\"one\\\"\\n\\tline two\"}\n" {
		t.Errorf("exit 2 = %d %q", code, out)
	}
	var reply map[string]any
	if err := json.Unmarshal([]byte(strings.TrimPrefix(strings.TrimSpace(out), "HOOK_CONTROL\t")), &reply); err != nil || reply["cancel"] != true {
		t.Errorf("reply is not a cancel: %v %v", err, reply)
	}

	if out, code = run("exit 2"); !strings.Contains(out, `"errorMessage": "blocked by a hook that exited 2"`) || code != 0 {
		t.Errorf("exit 2 without stderr = %d %q", code, out)
	}
	if out, code = run("exit 1", "echo never"); code != 1 || out != "" {
		t.Errorf("exit 1 = %d %q; it must stop the script with its own code", code, out)
	}
	if out, code = run(`echo '{"cancel": true}'`); code != 0 || out != "{\"cancel\": true}\n" {
		t.Errorf("a reply the command prints = %d %q", code, out)
	}
	if out, code = run("sh -c 'exit 2'; echo checked"); code != 0 || !strings.HasPrefix(out, "HOOK_CONTROL\t") {
		t.Errorf("exit 2 from the first of two commands in a spec = %d %q", code, out)
	}
	if out, code = run("false\necho after"); code != 1 || out != "" {
		t.Errorf("a failure inside a spec must stop it = %d %q", code, out)
	}
	for _, stderr := range []string{`\033[31mred\033[0m`, `bad \377\376`, `missing } here`, `use { x`, `\b\f\001`} {
		out, code = run(`printf '` + stderr + `\n' >&2; exit 2`)
		var reply map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(strings.TrimSpace(out), "HOOK_CONTROL\t")), &reply); err != nil || reply["cancel"] != true || code != 0 {
			t.Errorf("stderr %q gives %d %q: %v", stderr, code, out, err)
		}
		if strings.Count(out, "{") != 1 || strings.Count(out, "}") != 1 {
			t.Errorf("stderr %q leaves unbalanced braces: %q", stderr, out)
		}
	}
}
