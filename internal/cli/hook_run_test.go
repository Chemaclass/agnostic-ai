package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func skipWithoutPOSIXShell(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("runs POSIX shell hooks")
	}
}

// hookRunProject writes a claude+codex project with one hook spec.
func hookRunProject(t *testing.T, hook string) string {
	t.Helper()
	dir := testutil.TempCwd(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex, cursor]\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml"), hook)
	return dir
}

func runHookRun(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"hook", "run"}, args...))
	err := root.Execute()
	return out.String(), err
}

func TestHookRun_SharedEditHookAgreesAcrossTargets(t *testing.T) {
	skipWithoutPOSIXShell(t)
	hookRunProject(t, "name: guard\nevent: PreToolUse\nmatcher: Edit|Write\n"+
		`command: 'if grep -q "\.github/workflows/"; then echo "protected file" >&2; exit 2; fi'`+"\n")

	out, err := runHookRun(t, "guard", "--edit", ".github/workflows/tests.yml", "--expect", "block")
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}
	for _, want := range []string{"claude: block (exit 2", "codex: block (exit 2", "stderr: protected file", "cursor: not run"} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}
}

func TestHookRun_FailsWhenTargetsDecideDifferently(t *testing.T) {
	skipWithoutPOSIXShell(t)
	// Reads Claude Code's field, which a Codex patch never sends.
	hookRunProject(t, "name: guard\nevent: PreToolUse\nmatcher: Write\n"+
		`command: 'if grep -q "\"file_path\""; then exit 2; fi'`+"\n")

	out, err := runHookRun(t, "guard", "--edit", "src/app.go")
	if err == nil || !strings.Contains(err.Error(), "claude block, codex allow") {
		t.Fatalf("err = %v, want a divergence\n%s", err, out)
	}
}

func TestHookRun_ExpectFailsOnAnotherDecision(t *testing.T) {
	skipWithoutPOSIXShell(t)
	hookRunProject(t, "name: guard\nevent: PreToolUse\nmatcher: Bash\ncommand: 'cat >/dev/null'\n")

	out, err := runHookRun(t, "guard", "--bash", "git push --force", "--expect", "block")
	if err == nil || !strings.Contains(err.Error(), "expected block") {
		t.Fatalf("err = %v, want an expectation failure\n%s", err, out)
	}
}

func TestHookRun_GivesEachTargetTheEnvSyncGivesIt(t *testing.T) {
	skipWithoutPOSIXShell(t)
	dir := hookRunProject(t, "name: guard\nevent: UserPromptSubmit\n"+
		`command: 'test -f agnostic-ai.yaml && printf "target=%s root=%s\n" "$AGNOSTIC_AI_TARGET" "${CLAUDE_PROJECT_DIR:-none}"'`+"\n")
	// A parent session's values must not leak into the run.
	t.Setenv("AGNOSTIC_AI_TARGET", "claude")
	t.Setenv("CLAUDE_PROJECT_DIR", "/elsewhere")

	out, err := runHookRun(t, "guard", "--prompt", "hi")
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"target=claude root=" + real, "target=codex root=none"} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}
}

func TestHookRun_TimeoutFails(t *testing.T) {
	skipWithoutPOSIXShell(t)
	hookRunProject(t, "name: guard\nevent: SessionStart\nmatcher: compact\ntarget: claude\ntimeout: 1\ncommand: 'sleep 10'\n")

	out, err := runHookRun(t, "guard")
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want a timeout\n%s", err, out)
	}
	if !strings.Contains(out, "claude: timeout") {
		t.Errorf("output misses the timeout:\n%s", out)
	}
}

func TestHookRun_SessionStartUsesTheMatchedSource(t *testing.T) {
	skipWithoutPOSIXShell(t)
	hookRunProject(t, "name: guard\nevent: SessionStart\nmatcher: compact\n"+
		`command: 'grep -q "\"source\":\"compact\"" && echo context-added'`+"\n")

	out, err := runHookRun(t, "guard", "--target", "codex", "--expect", "allow")
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}
	if !strings.Contains(out, "stdout: context-added") || strings.Contains(out, "claude:") {
		t.Errorf("output = %s", out)
	}
}

func TestHookRun_RawPayloadFile(t *testing.T) {
	skipWithoutPOSIXShell(t)
	dir := hookRunProject(t, "name: guard\nevent: Stop\ntarget: codex\n"+
		`command: 'grep -q stop_hook_active || exit 2'`+"\n")
	payload := filepath.Join(dir, "stop.json")
	if err := os.WriteFile(payload, []byte(`{"hook_event_name":"Stop","stop_hook_active":false}`), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runHookRun(t, "guard", "--payload", payload, "--expect", "allow")
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}
}

func TestHookRun_UnknownHookAndTargetOutsideTheHook(t *testing.T) {
	hookRunProject(t, "name: guard\nevent: PreToolUse\ntarget: claude\ncommand: 'true'\n")

	if _, err := runHookRun(t, "missing", "--bash", "ls"); err == nil || !strings.Contains(err.Error(), "no hook named missing") {
		t.Errorf("err = %v", err)
	}
	if _, err := runHookRun(t, "guard", "--bash", "ls", "--target", "codex"); err == nil || !strings.Contains(err.Error(), "does not reach codex") {
		t.Errorf("err = %v", err)
	}
}
