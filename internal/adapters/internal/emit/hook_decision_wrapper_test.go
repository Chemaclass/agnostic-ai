package emit

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Gemini CLI reads a JSON reply whatever the exit code, so its wrapper
// prints the deny itself: a reason that looks like JSON still blocks.
func TestPortableHookWrapper_GeminiPrintsTheDeny(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("runs the wrapper with bash")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is not on PATH")
	}
	wrapper := filepath.Join(t.TempDir(), DecisionWrapperName)
	if err := os.WriteFile(wrapper, []byte(PortableHookWrapper("gemini")), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(wrapper, "--decision", `echo '{"decision":"deny","reason":"{}"}'`).Output()
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 2 || string(out) != "{\"decision\":\"deny\",\"reason\":\"\\u007b\\u007d\"}\n" {
		t.Errorf("gemini deny = %q %v", out, err)
	}
}

// A large stdout neither slows the wrapper past a hook timeout nor slips
// past it: a long reason still denies, and long plain text still blocks.
func TestPortableHookWrapper_ReadsALargeStdoutQuickly(t *testing.T) {
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
	for name, command := range map[string]string{
		"a 900 KB reason":            `printf '{"decision":"deny","reason":"'; head -c 900000 /dev/zero | tr '\0' x; printf '"}\n'`,
		"a 2 MB allow":               `printf '{"decision":"allow","reason":"'; head -c 2000000 /dev/zero | tr '\0' x; printf '"}\n'`,
		"2 MB of plain text":         `head -c 2000000 /dev/zero | tr '\0' 'x\n'`,
		"2 MB of log lines":          `head -c 2000000 /dev/zero | tr '\0' '\n'; echo '{"decision":"allow"}' | tr -d '\n'; echo x`,
		"900 KB of tokens in arrays": `printf '{"decision":"allow","n":['; head -c 300000 /dev/zero | tr '\0' '1' | sed 's/1/1,/g'; printf '1]}\n'`,
	} {
		start := time.Now()
		cmd := exec.Command(wrapper, "--decision", command)
		var stderr strings.Builder
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		exit, ok := err.(*exec.ExitError)
		if !ok || exit.ExitCode() != 2 || len(out) != 0 {
			t.Errorf("%s: exit %v, stdout %d bytes; want a block", name, err, len(out))
		}
		if elapsed := time.Since(start); elapsed > 10*time.Second {
			t.Errorf("%s took %s", name, elapsed)
		}
		if name == "a 900 KB reason" && len(stderr.String()) < 900000 {
			t.Errorf("the reason must reach stderr whole, got %d bytes", len(stderr.String()))
		}
	}
}

// A hook imported from a synced file calls the wrapper itself; import
// leaves the wrapper behind as sync output, so sync writes it again.
func TestPortableHookWrapperScript_FollowsACommandThatCallsIt(t *testing.T) {
	native := spec.Entry{Kind: spec.KindHook, Meta: map[string]any{"event": "PreToolUse", "command": ".claude/hooks/agnostic-ai-portable-hook.sh --decision '.claude/hooks/guard.sh'"}}
	if _, ok := PortableHookWrapperScript([]spec.Entry{native}, "claude", ".claude/hooks"); !ok {
		t.Error("a command that calls the wrapper needs it")
	}
	plain := spec.Entry{Kind: spec.KindHook, Meta: map[string]any{"event": "PreToolUse", "command": "./guard.sh"}}
	if _, ok := PortableHookWrapperScript([]spec.Entry{plain}, "claude", ".claude/hooks"); ok {
		t.Error("a native hook that does not call the wrapper gets none")
	}
}
