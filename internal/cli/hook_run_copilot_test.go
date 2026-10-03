package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// copilotGuardScript denies through Copilot's JSON reply, and checks the
// target and the camelCase toolArgs string on the way.
const copilotGuardScript = `#!/bin/sh
payload=$(cat)
[ "$AGNOSTIC_AI_TARGET" = copilot ] || exit 1
case "$payload" in
  *'rm -rf'*) echo '{"permissionDecision":"deny","permissionDecisionReason":"no recursive delete"}'; exit 0 ;;
esac
echo '{"permissionDecision":"allow"}'
`

func copilotProject(t *testing.T, hook, body string) {
	t.Helper()
	dir := testutil.TempCwd(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, copilot]\n")
	script := filepath.Join(dir, ".agnostic-ai", "scripts", "guard.sh")
	mustWrite(t, script, body)
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml"), hook)
	mustSync(t)
}

func TestHookRun_CopilotRunsAShellHookOnAssumptions(t *testing.T) {
	skipWithoutPOSIXShell(t)
	copilotProject(t, "name: guard\nevent: preToolUse\nmatcher: bash\ncommand: .agnostic-ai/scripts/guard.sh\n", copilotGuardScript)

	out, err := runHookRun(t, "guard", "--bash", "rm -rf /", "--expect", "block")
	for _, want := range []string{
		"copilot: block (exit 0", "(assumed: shell, working directory)",
		"event: preToolUse (bash)", "command: .github/hooks/scripts/guard.sh",
		"assumed shell: sh -c", "assumed working directory: project root",
		"docs: https://docs.github.com/en/copilot/reference/hooks-reference",
		"0 checked, 1 assumed (not counted; --include-assumed to count)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}
	if err == nil || !strings.Contains(err.Error(), "(copilot: block)") {
		t.Errorf("an uncounted run must not pass a check silently: %v", err)
	}
	if strings.Contains(out, "warning:") {
		t.Errorf("a fresh sync warns:\n%s", out)
	}
	if out, err := runHookRun(t, "guard", "--bash", "rm -rf /", "--expect", "block", "--include-assumed"); err != nil || !strings.Contains(out, "1 checked, 1 assumed (counted)") {
		t.Errorf("--include-assumed must count copilot: %v\n%s", err, out)
	}
	if out, err := runHookRun(t, "guard", "--bash", "ls", "--expect", "allow", "--include-assumed"); err != nil || !strings.Contains(out, "copilot: allow (exit 0") {
		t.Errorf("a harmless command must allow: %v\n%s", err, out)
	}
	out, _ = runHookRun(t, "guard", "--bash", "rm -rf /", "--expect", "allow")
	if !strings.Contains(out, "warning: assumed result block differs from allow and is not counted") {
		t.Errorf("an uncounted disagreement must warn:\n%s", out)
	}
}

func TestHookRun_CopilotRunsAnExecFormHookWithoutAShell(t *testing.T) {
	skipWithoutPOSIXShell(t)
	script := "#!/bin/sh\ncat >/dev/null\n[ \"$1\" = --strict ] && [ \"$2\" = 'a b' ] || exit 1\n[ \"$(basename \"$(pwd)\")\" = scripts ] || exit 1\nexit 2\n"
	copilotProject(t, "name: guard\nevent: PreToolUse\ncwd: .github/hooks/scripts\ncommand: .agnostic-ai/scripts/guard.sh\nargs: [--strict, 'a b']\n", script)
	if err := os.Chmod(filepath.Join(".github", "hooks", "scripts", "guard.sh"), 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := runHookRun(t, "guard", "--bash", "ls", "--target", "copilot", "--format", "json", "--include-assumed")
	var report struct {
		Targets []struct {
			Target      string `json:"target"`
			Decision    string `json:"decision"`
			Counted     bool   `json:"counted"`
			Assumptions []struct {
				Item string `json:"item"`
			} `json:"assumptions"`
		} `json:"targets"`
	}
	if jerr := json.Unmarshal([]byte(out), &report); jerr != nil || len(report.Targets) != 1 {
		t.Fatalf("invalid JSON: %v\n%s", jerr, out)
	}
	r := report.Targets[0]
	if err != nil || r.Decision != "block" || !r.Counted || len(r.Assumptions) != 1 || r.Assumptions[0].Item != "exec path" || strings.Contains(out, "start_error") {
		t.Errorf("exec form with cwd assumes only where its path resolves; the script runs and exit 2 denies: %+v, %v\n%s", r, err, out)
	}
}

func TestHookRun_CopilotListsWhatItCannotRunAsNotRun(t *testing.T) {
	skipWithoutPOSIXShell(t)
	copilotProject(t, "name: guard\nevent: PreToolUse\ncommand: 'cat | grep -q rm && exit 2'\n", copilotGuardScript)

	out, err := runHookRun(t, "guard", "--bash", "rm -rf /")
	if !strings.Contains(out, "copilot: not run (Copilot does not document its shell; use a script path)") || !strings.Contains(out, "claude: block") {
		t.Errorf("an inline shell command is not run on Copilot, and Claude Code still runs: %v\n%s", err, out)
	}
	out, err = runHookRun(t, "guard", "--edit", "a.go")
	if !strings.Contains(out, "copilot: not run (Copilot documents no toolArgs for edit, create, or apply_patch") {
		t.Errorf("--edit is refused on Copilot alone: %v\n%s", err, out)
	}
}

func TestHookRun_CopilotMissingExecutableFailsACountedCheck(t *testing.T) {
	skipWithoutPOSIXShell(t)
	copilotProject(t, "name: guard\nevent: PreToolUse\ncwd: .github/hooks/scripts\ncommand: agnostic-ai-missing-guard\nargs: [--strict]\n", copilotGuardScript)

	out, err := runHookRun(t, "guard", "--bash", "ls", "--target", "copilot", "--expect", "block")
	if err == nil || !strings.Contains(err.Error(), "failed on copilot") || !strings.Contains(out, "copilot: block (did not start") {
		t.Errorf("a hook that never started must not pass --expect block: %v\n%s", err, out)
	}
}

func TestHookRun_CopilotPermissionRequestMergesLaterOutputs(t *testing.T) {
	skipWithoutPOSIXShell(t)
	dir := testutil.TempCwd(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [copilot]\n")
	for name, reply := range map[string]string{"deny.sh": "deny", "allow.sh": "allow"} {
		script := filepath.Join(dir, ".agnostic-ai", "scripts", name)
		mustWrite(t, script, "#!/bin/sh\ncat >/dev/null\necho '{\"behavior\":\""+reply+"\"}'\n")
		if err := os.Chmod(script, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml"), "name: guard\nevent: permissionRequest\ncommand: [.agnostic-ai/scripts/deny.sh, .agnostic-ai/scripts/allow.sh]\n")
	mustSync(t)

	out, err := runHookRun(t, "guard", "--payload", writePayload(t, dir, `{"toolName":"bash"}`), "--expect", "allow", "--include-assumed")
	if err != nil || !strings.Contains(out, "copilot: block (exit 0") || !strings.Contains(out, "copilot: allow (exit 0") {
		t.Errorf("the later allow overrides the earlier deny: %v\n%s", err, out)
	}
}

func writePayload(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "payload.json")
	mustWrite(t, path, body)
	return path
}
