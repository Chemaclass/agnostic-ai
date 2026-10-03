package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// kiroGuardScript blocks a prompt that mentions deploy with exit 2, and
// otherwise prints the prompt Kiro's IDE gives as USER_PROMPT.
const kiroGuardScript = `#!/bin/sh
payload=$(cat)
case "$payload" in
  *deploy*) echo "no deploys from chat" >&2; exit 2 ;;
esac
echo "prompt: $USER_PROMPT"
`

const kiroGuardSpec = "name: deploy-guard\nevent: UserPromptSubmit\ncommand: .agnostic-ai/scripts/deploy-guard.sh\n"

// kiroProject syncs a claude+kiro project with one hook spec whose
// command runs body.
func kiroProject(t *testing.T, hook, body string) {
	t.Helper()
	dir := testutil.TempCwd(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, kiro]\n")
	script := filepath.Join(dir, ".agnostic-ai", "scripts", "deploy-guard.sh")
	mustWrite(t, script, body)
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "deploy-guard.yaml"), hook)
	mustSync(t)
}

func TestHookRun_KiroBlocksAPromptOnAnAssumedShell(t *testing.T) {
	skipWithoutPOSIXShell(t)
	kiroProject(t, kiroGuardSpec, kiroGuardScript)

	out, err := runHookRun(t, "deploy-guard", "--prompt", "deploy now", "--expect", "block")
	for _, want := range []string{
		"claude: block (exit 2",
		"kiro: block (exit 2", "(assumed: shell)\n", "event: UserPromptSubmit (prompt)", "command: .kiro/scripts/deploy-guard.sh",
		"assumed shell: sh -c (Kiro does not document the shell that runs a hook command)",
		"docs: https://kiro.dev/docs/hooks",
		"1 checked, 1 assumed (not counted; --include-assumed to count)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}
	if err != nil {
		t.Errorf("the counted claude result passes --expect: %v", err)
	}
	if strings.Contains(out, "warning:") || strings.Contains(out, "assumed timeout") || strings.Contains(out, "assumed working directory") {
		t.Errorf("a fresh sync warns, or the documented cwd or 60s timeout is marked assumed:\n%s", out)
	}

	out, err = runHookRun(t, "deploy-guard", "--prompt", "deploy now", "--expect", "block", "--include-assumed")
	if err != nil || !strings.Contains(out, "2 checked, 1 assumed (counted)") {
		t.Errorf("--include-assumed must count the exit 2 block: %v\n%s", err, out)
	}

	out, err = runHookRun(t, "deploy-guard", "--target", "kiro", "--prompt", "hello", "--expect", "allow", "--include-assumed")
	for _, want := range []string{"kiro: allow (exit 0", "stdout: prompt: hello", "context: kiro adds the output to the session"} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q: %v\n%s", want, err, out)
		}
	}
}

func TestHookRun_KiroDoesNotCountAnExitItsDocsDisagreeOn(t *testing.T) {
	skipWithoutPOSIXShell(t)
	kiroProject(t, kiroGuardSpec, "#!/bin/sh\ncat >/dev/null\necho failed >&2\nexit 1\n")

	out, err := runHookRun(t, "deploy-guard", "--target", "kiro", "--prompt", "deploy now", "--include-assumed")
	for _, want := range []string{
		"kiro: error (exit 1",
		"note: not counted: Kiro's docs disagree on whether a non-zero exit other than 2 blocks",
		"0 checked, 1 assumed (counted; 1 result not counted, see its note)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}
	if err != nil {
		t.Errorf("an uncounted error must not fail the run: %v", err)
	}

	_, err = runHookRun(t, "deploy-guard", "--target", "kiro", "--prompt", "deploy now", "--expect", "block", "--include-assumed")
	if err == nil || !strings.Contains(err.Error(), "--expect checks nothing") || !strings.Contains(err.Error(), "kiro: Kiro's docs disagree") {
		t.Errorf("--expect on an uncounted result alone must fail and say why: %v", err)
	}
}

func TestHookRun_KiroSkipsAnInlineShellCommand(t *testing.T) {
	skipWithoutPOSIXShell(t)
	kiroProject(t, "name: deploy-guard\nevent: UserPromptSubmit\ncommand: 'cat | grep -q deploy && exit 2'\n", kiroGuardScript)

	out, _ := runHookRun(t, "deploy-guard", "--prompt", "deploy now")
	if !strings.Contains(out, "kiro: not run (Kiro does not document its shell; use a script path)") {
		t.Errorf("output = %s", out)
	}
}

func TestHookRun_KiroRefusesBashAndEditButRunsAToolPayload(t *testing.T) {
	skipWithoutPOSIXShell(t)
	kiroProject(t, "name: deploy-guard\nevent: PreToolUse\nmatcher: shell\ncommand: .agnostic-ai/scripts/deploy-guard.sh\n", kiroGuardScript)

	out, _ := runHookRun(t, "deploy-guard", "--bash", "deploy prod")
	if !strings.Contains(out, "kiro: not run (Kiro documents no tool_input for its shell tool; pass --payload <file>)") || !strings.Contains(out, "claude: allow") {
		t.Errorf("--bash must leave Kiro out and still run Claude Code:\n%s", out)
	}
	out, _ = runHookRun(t, "deploy-guard", "--target", "kiro", "--edit", "deploy.yml")
	if !strings.Contains(out, "kiro: not run (Kiro documents no tool_input for its write tool; pass --payload <file>)") {
		t.Errorf("--edit must leave Kiro out:\n%s", out)
	}

	payload := filepath.Join(t.TempDir(), "call.json")
	mustWrite(t, payload, `{"hook_event_name":"preToolUse","cwd":"/p","session_id":"s","tool_name":"execute_bash","tool_input":{"command":"deploy prod"}}`)
	out, err := runHookRun(t, "deploy-guard", "--target", "kiro", "--payload", payload, "--expect", "block", "--include-assumed")
	if err != nil || !strings.Contains(out, "kiro: block (exit 2") || !strings.Contains(out, "event: PreToolUse (execute_bash)") {
		t.Errorf("matcher shell must match execute_bash, its canonical name: %v\n%s", err, out)
	}
}

func TestHookRun_KiroReadsAStopBlockDecision(t *testing.T) {
	skipWithoutPOSIXShell(t)
	kiroProject(t, "name: deploy-guard\nevent: Stop\ncommand: .agnostic-ai/scripts/deploy-guard.sh\n",
		"#!/bin/sh\ncase \"$(cat)\" in\n  *'\"hook_event_name\":\"stop\"'*) echo '{\"decision\":\"block\",\"reason\":\"run the tests\"}' ;;\nesac\n")

	out, err := runHookRun(t, "deploy-guard", "--target", "kiro", "--expect", "block", "--include-assumed")
	for _, want := range []string{
		"kiro: block (exit 0", "event: Stop (stop)",
		`note: replied "decision": "block": Kiro keeps the agent running; read as block`,
		"context: kiro adds the output to the session",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}
	if err != nil {
		t.Errorf("a Stop block decision counts as block: %v", err)
	}
}

func TestHookRun_KiroDoesNotRunWhatItCannotRunAsACommand(t *testing.T) {
	skipWithoutPOSIXShell(t)
	for name, tc := range map[string]struct{ spec, input, want string }{
		"agent action": {
			"name: deploy-guard\nevent: UserPromptSubmit\nx-kiro:\n  action:\n    type: agent\n    prompt: Check the prompt\n",
			"--prompt", "kiro: not run (Kiro runs an agent prompt, not a command)",
		},
		"confirm": {
			"name: deploy-guard\nevent: Stop\ncommand: .agnostic-ai/scripts/deploy-guard.sh\nx-kiro:\n  confirm:\n    question: Run it?\n    options:\n      - {id: yes, label: Yes, run: true}\n",
			"", "kiro: not run (Kiro asks the user before a hook with confirm runs)",
		},
		"file trigger": {
			"name: deploy-guard\nevent: PostFileSave\ncommand: .agnostic-ai/scripts/deploy-guard.sh\n",
			"", "kiro: not run (Kiro documents no payload for PostFileSave)",
		},
	} {
		t.Run(name, func(t *testing.T) {
			kiroProject(t, tc.spec, kiroGuardScript)
			args := []string{"deploy-guard", "--target", "kiro"}
			if tc.input != "" {
				args = append(args, tc.input, "hello")
			}
			out, _ := runHookRun(t, args...)
			if !strings.Contains(out, tc.want) {
				t.Errorf("output = %s", out)
			}
		})
	}
}

func TestHookRun_KiroJSONListsItsAssumption(t *testing.T) {
	skipWithoutPOSIXShell(t)
	kiroProject(t, kiroGuardSpec, kiroGuardScript)

	out, _ := runHookRun(t, "deploy-guard", "--prompt", "deploy now", "--format", "json")
	var report struct {
		Targets []struct {
			Target      string `json:"target"`
			Decision    string `json:"decision"`
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
		if r.Target != "kiro" {
			continue
		}
		if r.Decision != "block" || r.Counted || len(r.Assumptions) != 1 || r.Assumptions[0].Item != "shell" || r.Assumptions[0].Value != "sh -c" {
			t.Errorf("kiro = %+v; want block, not counted, shell assumed", r)
		}
		return
	}
	t.Fatalf("no kiro result:\n%s", out)
}
