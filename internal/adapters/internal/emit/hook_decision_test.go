package emit

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// The exit code wrapper turns a stdout decision into exit 2 with the
// reason on stderr, which every exit code target reads as a block.
func TestPortableHookWrapper_ReadsTheStdoutDecision(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("runs the wrapper with bash")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is not on PATH")
	}
	wrapper := filepath.Join(t.TempDir(), DecisionWrapperName)
	if err := os.WriteFile(wrapper, []byte(PortableHookWrapper("claude")), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(command string) (string, string, int) {
		t.Helper()
		cmd := exec.Command(wrapper, "--decision", command)
		cmd.Stdin = strings.NewReader(`{"tool_input":{"command":"ls"}}`)
		var stderr strings.Builder
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		code := 0
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		return string(out), stderr.String(), code
	}
	for _, c := range []struct {
		name, command, stdout, stderr string
		code                          int
	}{
		{"deny with a reason", `echo '{"decision": "deny", "reason": "no \"force\" push\nretry"}'`, "", "no \"force\" push\nretry\n", 2},
		{"deny without a reason", `echo '{"decision":"deny"}'`, "", "blocked by the hook decision\n", 2},
		{"ask blocks", `printf '{\n  "reason": "ask first",\n  "decision": "ask"\n}\n'`, "", "ask first\n", 2},
		{"allow drops the object", `echo '{"decision":"allow","reason":"fine"}'`, "", "", 0},
		{"an unknown decision blocks", `echo '{"decision":"block"}'`, "", "blocked: the hook printed a decision that is not \"allow\", \"deny\", or \"ask\"\n", 2},
		{"a decision that is no string blocks", `echo '{"decision":null}'`, "", "blocked: the hook printed a decision that is not \"allow\", \"deny\", or \"ask\"\n", 2},
		{"plain stdout passes", `echo hello`, "hello\n", "", 0},
		{"a reason naming a decision is no decision", `echo '{"reason":"the \"decision\": \"allow\" key"}'`, "{\"reason\":\"the \\\"decision\\\": \\\"allow\\\" key\"}\n", "", 0},
		{"exit 2 stays", `echo '{"decision":"allow"}'; echo stop >&2; exit 2`, "{\"decision\":\"allow\"}\n", "stop\n", 2},
		{"exit 1 stays", `echo '{"decision":"deny"}'; exit 1`, "{\"decision\":\"deny\"}\n", "", 1},
		{"the command reads the payload", `grep -q '"ls"' && echo '{"decision":"deny","reason":"saw ls"}'`, "", "saw ls\n", 2},
	} {
		out, stderr, code := run(c.command)
		if out != c.stdout || stderr != c.stderr || code != c.code {
			t.Errorf("%s: got %d %q %q, want %d %q %q", c.name, code, out, stderr, c.code, c.stdout, c.stderr)
		}
	}
}

func TestPortableHookCommand_WrapsOnlyWhereTheHookNeedsIt(t *testing.T) {
	rewrite := func(path string) string { return RewriteHookPath(path, "claude") }
	plain, _ := spec.Entry{Kind: spec.KindHook, Meta: map[string]any{"on": "before-tool", "command": "x"}}.NativeHook("claude")
	if got := PortableHookCommand(plain, "claude", "./guard.sh", rewrite); got != "./guard.sh" {
		t.Errorf("a hook Claude Code reads as is stays as is: %q", got)
	}
	decided, _ := spec.Entry{Kind: spec.KindHook, Meta: map[string]any{"on": "before-tool", "decision": "stdout", "command": "x"}}.NativeHook("claude")
	if got := PortableHookCommand(decided, "claude", "./it's.sh", rewrite); got != `.claude/hooks/agnostic-ai-portable-hook.sh --decision './it'\''s.sh'` {
		t.Errorf("decision hook = %q", got)
	}
	_, options, inner, ok := UnwrapDecisionCommand(PortableHookCommand(decided, "claude", "./it's.sh", rewrite))
	if !ok || inner != "./it's.sh" || strings.Join(options, " ") != "--decision" {
		t.Errorf("unwrap = %v %q %v", options, inner, ok)
	}
}
