package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// antigravityGuardScript blocks a recursive delete: through Antigravity's
// documented `decision: "deny"` reply on its camelCase toolCall payload,
// and through exit 2 elsewhere. Antigravity gets a JSON decision either
// way, since it documents no exit codes.
const antigravityGuardScript = `#!/bin/sh
payload=$(cat)
case "$payload" in
  *'"toolCall"'*)
    case "$payload" in
      *'"name":"run_command"'*'rm -rf'*|*'rm -rf'*'"name":"run_command"'*) echo '{"decision":"deny","reason":"no recursive delete"}' ;;
      *) echo '{"decision":"allow"}' ;;
    esac
    exit 0 ;;
esac
case "$payload" in
  *'rm -rf'*) echo "no recursive delete" >&2; exit 2 ;;
esac
exit 0
`

// antigravityProject syncs a claude+antigravity project with one hook
// spec whose command runs body.
func antigravityProject(t *testing.T, hook, body string) {
	t.Helper()
	dir := testutil.TempCwd(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, antigravity]\n")
	script := filepath.Join(dir, ".agnostic-ai", "scripts", "protect-files.sh")
	mustWrite(t, script, body)
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "protect-files.yaml"), hook)
	mustSync(t)
}

const antigravityHookSpec = "name: protect-files\nevent: PreToolUse\ncommand: .agnostic-ai/scripts/protect-files.sh\n"

func TestHookRun_AntigravityReadsItsDecisionOnAnAssumedShell(t *testing.T) {
	skipWithoutPOSIXShell(t)
	antigravityProject(t, antigravityHookSpec, antigravityGuardScript)

	out, err := runHookRun(t, "protect-files", "--bash", "rm -rf /", "--expect", "block")
	for _, want := range []string{
		"claude: block (exit 2",
		"antigravity: block (exit 0", "(assumed: shell, working directory)", "event: PreToolUse (run_command)",
		"assumed shell: sh -c (Antigravity does not document the shell that runs a hook command)",
		"assumed working directory: project root (Antigravity does not document the directory a hook command runs in)",
		"docs: https://antigravity.google/docs/hooks",
		"1 checked, 1 assumed (not counted; --include-assumed to count)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}
	if err != nil {
		t.Errorf("the counted claude result passes --expect: %v", err)
	}
	if strings.Contains(out, "warning:") || strings.Contains(out, "assumed timeout") {
		t.Errorf("a fresh sync warns, or the documented 30s timeout is marked assumed:\n%s", out)
	}

	out, err = runHookRun(t, "protect-files", "--bash", "rm -rf /", "--expect", "block", "--include-assumed")
	if err != nil || !strings.Contains(out, "2 checked, 1 assumed (counted)") {
		t.Errorf("--include-assumed must count the documented deny reply: %v\n%s", err, out)
	}

	out, err = runHookRun(t, "protect-files", "--bash", "ls", "--expect", "allow", "--include-assumed")
	if err != nil || !strings.Contains(out, "antigravity: allow (exit 0") {
		t.Errorf("decision allow must allow: %v\n%s", err, out)
	}
}

func TestHookRun_AntigravityDoesNotCountAnUndocumentedExitCode(t *testing.T) {
	skipWithoutPOSIXShell(t)
	antigravityProject(t, antigravityHookSpec, "#!/bin/sh\ncat >/dev/null\necho blocked >&2\nexit 2\n")

	out, err := runHookRun(t, "protect-files", "--bash", "rm -rf /", "--expect", "block", "--include-assumed")
	for _, want := range []string{
		"antigravity: error (exit 2",
		"note: not counted: Antigravity does not document exit codes",
		"1 checked, 1 assumed (counted; 1 undocumented result not counted)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}
	if err != nil {
		t.Errorf("an uncounted error must not fail the run that claude passes: %v", err)
	}
	if strings.Contains(out, "differs from") {
		t.Errorf("an undocumented result must not ask for --include-assumed:\n%s", out)
	}

	_, err = runHookRun(t, "protect-files", "--target", "antigravity", "--bash", "rm -rf /", "--expect", "block", "--include-assumed")
	if err == nil || !strings.Contains(err.Error(), "--expect checks nothing") || !strings.Contains(err.Error(), "antigravity: Antigravity does not document exit codes") {
		t.Errorf("--expect on an undocumented result alone must fail and say why: %v", err)
	}
}

func TestHookRun_AntigravityDoesNotCountAReplyWithoutADecision(t *testing.T) {
	skipWithoutPOSIXShell(t)
	antigravityProject(t, antigravityHookSpec, "#!/bin/sh\ncat >/dev/null\necho '{\"reason\":\"fine\"}'\n")

	out, _ := runHookRun(t, "protect-files", "--target", "antigravity", "--bash", "ls", "--include-assumed")
	if !strings.Contains(out, "not counted: Antigravity requires a string decision in the reply") {
		t.Errorf("a reply without decision must be listed as not counted:\n%s", out)
	}
}

func TestHookRun_AntigravitySkipsAnInlineShellCommand(t *testing.T) {
	skipWithoutPOSIXShell(t)
	antigravityProject(t, "name: protect-files\nevent: PreToolUse\ncommand: 'cat | grep -q rm && exit 2'\n", antigravityGuardScript)

	out, _ := runHookRun(t, "protect-files", "--bash", "rm -rf /")
	if !strings.Contains(out, "antigravity: not run (Antigravity does not document its shell; use a script path)") {
		t.Errorf("output = %s", out)
	}
}

func TestHookRun_AntigravityJSONListsItsAssumptions(t *testing.T) {
	skipWithoutPOSIXShell(t)
	antigravityProject(t, antigravityHookSpec, antigravityGuardScript)

	out, _ := runHookRun(t, "protect-files", "--bash", "rm -rf /", "--format", "json")
	var report struct {
		Targets []struct {
			Target      string `json:"target"`
			Decision    string `json:"decision"`
			Trigger     string `json:"trigger"`
			Counted     bool   `json:"counted"`
			Assumptions []struct {
				Item, Value, Reason string
			} `json:"assumptions"`
		} `json:"targets"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	for _, r := range report.Targets {
		if r.Target != "antigravity" {
			continue
		}
		if r.Decision != "block" || r.Trigger != "run_command" || r.Counted || len(r.Assumptions) != 2 || r.Assumptions[0].Item != "shell" || r.Assumptions[0].Value != "sh -c" || r.Assumptions[1].Item != "working directory" || r.Assumptions[1].Value != "project root" {
			t.Errorf("antigravity = %+v; want block on run_command, not counted, shell and cwd assumed", r)
		}
		return
	}
	t.Fatalf("no antigravity result:\n%s", out)
}

func TestHookRun_AntigravityDoesNotCountRepliesThatDependOnSavedPermissions(t *testing.T) {
	skipWithoutPOSIXShell(t)
	for _, decision := range []string{"ask", "deny_unless_prior_grant"} {
		antigravityProject(t, antigravityHookSpec, "#!/bin/sh\ncat >/dev/null\necho '{\"decision\":\""+decision+"\"}'\n")
		for _, flags := range [][]string{nil, {"--include-assumed"}} {
			args := append([]string{"protect-files", "--target", "antigravity", "--bash", "rm -rf /", "--expect", "block"}, flags...)
			out, err := runHookRun(t, args...)
			if err == nil || !strings.Contains(out, "not counted: replied "+decision+": the result depends on Antigravity's saved permissions") {
				t.Errorf("%s %v must not satisfy --expect block: %v\n%s", decision, flags, err, out)
			}
		}
	}

	antigravityProject(t, antigravityHookSpec, "#!/bin/sh\ncat >/dev/null\necho '{\"decision\":\"force_ask\"}'\n")
	out, err := runHookRun(t, "protect-files", "--target", "antigravity", "--bash", "rm -rf /", "--expect", "block", "--include-assumed")
	if err != nil || !strings.Contains(out, "replied force_ask: Antigravity asks the user before the tool runs; read as block") {
		t.Errorf("force_ask ignores saved permissions and must count as block: %v\n%s", err, out)
	}
}
