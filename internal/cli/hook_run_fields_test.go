package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestHookRun_WarnsWhenTheSyncedMatcherTimeoutOrEnvDiffers(t *testing.T) {
	skipWithoutPOSIXShell(t)
	dir := testutil.TempCwd(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex, gemini]\n")
	spec := filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml")
	mustWrite(t, spec, "name: guard\nevent: PreToolUse\nmatcher: Bash\ntimeout: 5\ntargets: [claude, codex]\ncommand: 'cat >/dev/null'\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "gem.yaml"),
		"name: gem\nevent: BeforeTool\nmatcher: run_shell_command\ntarget: gemini\nx-gemini:\n  env:\n    MODE: strict\ncommand: 'cat >/dev/null'\n")
	mustSync(t)

	for _, hook := range []string{"guard", "gem"} {
		out, err := runHookRun(t, hook, "--bash", "ls")
		if err != nil || strings.Contains(out, "warning:") {
			t.Fatalf("%s after sync: err = %v\n%s", hook, err, out)
		}
	}

	mustWrite(t, spec, "name: guard\nevent: PreToolUse\nmatcher: Bash|Write\ntimeout: 9\ntargets: [claude, codex]\ncommand: 'cat >/dev/null'\n")
	out, err := runHookRun(t, "guard", "--bash", "ls")
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}
	for _, want := range []string{
		`warning: .claude/settings.json runs "cat >/dev/null" with matcher "Bash", not "Bash|Write"`,
		`warning: .codex/hooks.json runs "export AGNOSTIC_AI_TARGET=codex; cat >/dev/null" with matcher "Bash", not "Bash|Write"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}

	mustWrite(t, spec, "name: guard\nevent: PreToolUse\nmatcher: Bash\ntimeout: 9\ntargets: [claude, codex]\ncommand: 'cat >/dev/null'\n")
	out, _ = runHookRun(t, "guard", "--bash", "ls")
	if !strings.Contains(out, `.claude/settings.json runs "cat >/dev/null" with timeout 5s, not 9s`) {
		t.Errorf("output misses the timeout drift:\n%s", out)
	}

	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "gem.yaml"),
		"name: gem\nevent: BeforeTool\nmatcher: run_shell_command\ntarget: gemini\nx-gemini:\n  env:\n    MODE: loose\ncommand: 'cat >/dev/null'\n")
	out, _ = runHookRun(t, "gem", "--bash", "ls")
	if !strings.Contains(out, `.gemini/settings.json runs "cat >/dev/null" with different env MODE`) {
		t.Errorf("output misses the env drift:\n%s", out)
	}
}

func TestHookRun_ClaudeIfDecidesWhetherTheHookRuns(t *testing.T) {
	skipWithoutPOSIXShell(t)
	hookRunProject(t, "name: guard\nevent: PreToolUse\nmatcher: Bash\ntarget: claude\nif: Bash(git push *)\ncommand: 'echo blocked >&2; exit 2'\n")

	out, err := runHookRun(t, "guard", "--bash", "git status", "--expect", "allow")
	if err != nil || !strings.Contains(out, `if "Bash(git push *)" does not match this Bash call`) {
		t.Fatalf("git status: err = %v\n%s", err, out)
	}
	out, err = runHookRun(t, "guard", "--bash", "FOO=1 git push --force", "--expect", "block")
	if err != nil || !strings.Contains(out, "claude: block (exit 2") {
		t.Fatalf("git push: err = %v\n%s", err, out)
	}
}
