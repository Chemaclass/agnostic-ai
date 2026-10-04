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
	blocked := "blocked: stdout is not one JSON object with a single \"decision\" of \"allow\", \"deny\", or \"ask\"\n"
	for _, c := range []struct {
		name, command, stdout, stderr string
		code                          int
	}{
		{"deny with a reason", `echo '{"decision": "deny", "reason": "no \"force\" push\nretry"}'`, "", "no \"force\" push\nretry\n", 2},
		{"deny without a reason", `echo '{"decision":"deny"}'`, "", "blocked by the hook decision\n", 2},
		{"ask blocks", `printf '{\n  "reason": "ask first",\n  "decision": "ask"\n}\n'`, "", "ask first\n", 2},
		{"allow drops the object", `echo '{"decision":"allow","reason":"fine"}'`, "", "", 0},
		{"allow beside other keys", `echo '{"decision":"allow","meta":{"n":[1,2.5,-3e2,true,null,"x"]}}'`, "", "", 0},
		{"empty stdout allows", `true`, "", "", 0},
		{"an unknown decision blocks", `echo '{"decision":"block"}'`, "", blocked, 2},
		{"a decision that is no string blocks", `echo '{"decision":null}'`, "", blocked, 2},
		{"plain stdout blocks", `echo hello`, "", blocked, 2},
		{"no decision blocks", `echo '{"reason":"the \"decision\": \"allow\" key"}'`, "", blocked, 2},
		{"a nested decision cannot override a deny", `echo '{"decision":"deny","context":{"decision":"allow"}}'`, "", "blocked by the hook decision\n", 2},
		{"a nested decision alone is no decision", `echo '{"context":{"decision":"allow"}}'`, "", blocked, 2},
		{"a duplicate decision blocks", `echo '{"decision":"allow","decision":"allow"}'`, "", blocked, 2},
		{"an escaped key is the key it spells", `printf '%s\n' '{"deci\u0073ion":"deny","reason":"esc"}'`, "", "esc\n", 2},
		{"an escaped allow is allow", `printf '%s\n' '{"decision":"al\u006cow"}'`, "", "", 0},
		{"an escaped r in the decision stays distinct", `printf '%s\n' '{"decision":"al\rlow"}'`, "", blocked, 2},
		{"an escaped r in the key stays distinct", `printf '%s\n' '{"deci\rsion":"allow"}'`, "", blocked, 2},
		{"an escaped b in the decision stays distinct", `printf '%s\n' '{"decision":"al\blow"}'`, "", blocked, 2},
		{"an escaped b in the key stays distinct", `printf '%s\n' '{"deci\bsion":"allow"}'`, "", blocked, 2},
		{"an escaped f in the decision stays distinct", `printf '%s\n' '{"decision":"al\flow"}'`, "", blocked, 2},
		{"an escaped f in the key stays distinct", `printf '%s\n' '{"deci\fsion":"allow"}'`, "", blocked, 2},
		{"a raw SOH after the object blocks", `printf '{"decision":"allow"}\001'`, "", blocked, 2},
		{"a lone raw SOH blocks", `printf '\001'`, "", blocked, 2},
		{"a raw SOH inside the decision blocks", `printf '{"decision":"al\001low"}'`, "", blocked, 2},
		{"a raw NUL after the object blocks", `printf '{"decision":"allow"}\000'`, "", blocked, 2},
		{"a raw NUL inside the decision blocks", `printf '{"decision":"al\000low"}'`, "", blocked, 2},
		{"a second object blocks", `echo '{"decision":"allow"} {"decision":"deny"}'`, "", blocked, 2},
		{"broken JSON blocks", `echo '{"decision":"allow",}'`, "", blocked, 2},
		{"an unterminated string blocks", `echo '{"decision":"allow'`, "", blocked, 2},
		{"leading log lines block", `echo checking; echo '{"decision":"allow"}'`, "", blocked, 2},
		{"a raw newline inside a string blocks", `printf '{"decision":"allow","reason":"a\nb"}\n'`, "", blocked, 2},
		{"whitespace-only stdout allows", `printf ' \n\t\n'`, "", "", 0},
		{"exit 2 with allow still blocks", `echo '{"decision":"allow"}'; echo stop >&2; exit 2`, "{\"decision\":\"allow\"}\n", "stop\n", 2},
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
