package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// clineGuardScript blocks a recursive delete: through Cline's stdout
// `{"cancel": true}` on its SDK payload, which carries hookName, and
// through exit 2 elsewhere.
const clineGuardScript = `#!/bin/sh
payload=$(cat)
case "$payload" in
  *'"hookName"'*)
    case "$payload" in *'rm -rf'*) echo '{"cancel": true, "errorMessage": "no recursive delete"}' ;; esac
    exit 0 ;;
esac
case "$payload" in *'rm -rf'*) echo "no recursive delete" >&2; exit 2 ;; esac
exit 0
`

// clineExitScript blocks the Claude Code way only: exit 2 and stderr.
const clineExitScript = "#!/bin/sh\ncase \"$(cat)\" in *'rm -rf'*) echo 'no recursive delete' >&2; exit 2 ;; esac\n"

// clineProject syncs a claude+cline project with one hook spec whose
// command runs body, and returns the project root.
func clineProject(t *testing.T, hook, body string) string {
	t.Helper()
	dir := testutil.TempCwd(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, cline]\n")
	script := filepath.Join(dir, ".agnostic-ai", "scripts", "block-rm.sh")
	mustWrite(t, script, body)
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "block-rm.yaml"), hook)
	mustSync(t)
	return dir
}

const clineHookSpec = "name: block-rm\nevent: PreToolUse\ncommand: .agnostic-ai/scripts/block-rm.sh\n"

func TestHookRun_ClineBlocksOnACancelReply(t *testing.T) {
	skipWithoutPOSIXShell(t)
	clineProject(t, clineHookSpec, clineGuardScript)

	out, err := runHookRun(t, "block-rm", "--bash", "rm -rf /", "--expect", "block")
	for _, want := range []string{
		"claude: block (exit 2",
		"cline: block (exit 0", "(assumed: working directory)", "event: PreToolUse (run_commands)",
		"command: .clinerules/hooks/PreToolUse",
		"assumed working directory: project root (Cline runs a hook from the directory the CLI started in)",
		"docs: https://github.com/cline/cline/tree/39ff2359f7e08231281539696e48a166ce49270c/sdk/packages/core/src/hooks",
		"note: cancel: Cline skips the tool call and stops the run",
		"1 checked, 1 assumed (not counted; --include-assumed to count)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}
	if err != nil {
		t.Errorf("the counted claude result passes --expect: %v", err)
	}
	if strings.Contains(out, "warning:") || strings.Contains(out, "assumed shell") || strings.Contains(out, "assumed timeout") {
		t.Errorf("a fresh sync warns, or bash or the 120s timeout from source is marked assumed:\n%s", out)
	}

	out, err = runHookRun(t, "block-rm", "--bash", "rm -rf /", "--expect", "block", "--include-assumed")
	if err != nil || !strings.Contains(out, "2 checked, 1 assumed (counted)") {
		t.Errorf("--include-assumed must count the cancel reply: %v\n%s", err, out)
	}

	out, err = runHookRun(t, "block-rm", "--bash", "ls", "--expect", "allow", "--include-assumed")
	if err != nil || !strings.Contains(out, "cline: allow (exit 0") {
		t.Errorf("empty stdout must allow: %v\n%s", err, out)
	}
}

func TestHookRun_ClineBlocksOnAClaudeStyleExit2(t *testing.T) {
	skipWithoutPOSIXShell(t)
	clineProject(t, "name: block-rm\nevent: PreToolUse\nmatcher: Bash\ntimeout: 5\ncommand: .agnostic-ai/scripts/block-rm.sh\n", clineExitScript)

	out, err := runHookRun(t, "block-rm", "--bash", "rm -rf /", "--expect", "block")
	for _, want := range []string{
		"claude: block (exit 2",
		"cline: block (exit 0",
		"note: cancel: Cline skips the tool call and stops the run",
		`note: Cline has no matcher: sync drops "Bash", and the script runs on every tool call`,
		"note: Cline has no per-hook timeout: sync drops the spec's 5s, and hook run uses Cline's 120s",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}
	if err != nil || strings.Contains(out, "warning:") {
		t.Errorf("the synced script turns exit 2 into a cancel reply: %v\n%s", err, out)
	}

	if _, err = runHookRun(t, "block-rm", "--bash", "rm -rf /", "--expect", "block", "--include-assumed"); err != nil {
		t.Errorf("a counted Cline block passes --expect block: %v", err)
	}
}

func TestHookRun_ClineReadsStdoutThatIsNotJSONAsAnError(t *testing.T) {
	skipWithoutPOSIXShell(t)
	clineProject(t, clineHookSpec, "#!/bin/sh\ncat >/dev/null\necho checked\n")

	out, err := runHookRun(t, "block-rm", "--target", "cline", "--bash", "ls", "--include-assumed")
	if !strings.Contains(out, "cline: error (exit 0") || !strings.Contains(out, "note: stdout is not JSON: Cline logs it and goes on as if the hook allowed") {
		t.Errorf("output = %s", out)
	}
	if err == nil || !strings.Contains(err.Error(), "failed on cline") {
		t.Errorf("a counted error fails the run: %v", err)
	}
}

func TestHookRun_ClinePromptHookIsNotJudged(t *testing.T) {
	skipWithoutPOSIXShell(t)
	clineProject(t, "name: block-rm\nevent: UserPromptSubmit\ncommand: .agnostic-ai/scripts/block-rm.sh\n", clineExitScript)

	out, err := runHookRun(t, "block-rm", "--prompt", "rm -rf /", "--expect", "block", "--include-assumed")
	for _, want := range []string{
		"cline: not judged (exit 0", "event: UserPromptSubmit (prompt)", "command: .clinerules/hooks/UserPromptSubmit",
		"note: cline runs UserPromptSubmit fire-and-forget; it does not wait for the result",
		"note: Cline may not send this event: its source says orchestrated sessions, which the CLI runs, seed the prompt without it",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}
	if err != nil {
		t.Errorf("an async Cline result stays out of --expect even with --include-assumed: %v", err)
	}
}

func TestHookRun_ClineJSONListsItsAssumption(t *testing.T) {
	skipWithoutPOSIXShell(t)
	clineProject(t, clineHookSpec, clineGuardScript)

	for _, tc := range []struct {
		flags   []string
		counted bool
	}{{nil, false}, {[]string{"--include-assumed"}, true}} {
		out, _ := runHookRun(t, append([]string{"block-rm", "--bash", "rm -rf /", "--format", "json"}, tc.flags...)...)
		var report struct {
			Targets []struct {
				Target      string `json:"target"`
				Decision    string `json:"decision"`
				Trigger     string `json:"trigger"`
				Async       bool   `json:"async"`
				Counted     bool   `json:"counted"`
				Assumptions []struct {
					Item, Value, Reason string
				} `json:"assumptions"`
			} `json:"targets"`
		}
		if err := json.Unmarshal([]byte(out), &report); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, out)
		}
		found := false
		for _, r := range report.Targets {
			if r.Target != "cline" {
				continue
			}
			found = true
			if r.Decision != "block" || r.Trigger != "run_commands" || r.Async || r.Counted != tc.counted || len(r.Assumptions) != 1 ||
				r.Assumptions[0].Item != "working directory" || r.Assumptions[0].Value != "project root" || r.Assumptions[0].Reason != "Cline runs a hook from the directory the CLI started in" {
				t.Errorf("%v: cline = %+v; want block on run_commands, counted %t, the working directory assumed", tc.flags, r, tc.counted)
			}
		}
		if !found {
			t.Fatalf("no cline result:\n%s", out)
		}
	}
}

func TestHookRun_ClineWarnsWhenTheSyncedScriptDrops(t *testing.T) {
	skipWithoutPOSIXShell(t)
	dir := clineProject(t, clineHookSpec, clineGuardScript)
	script := filepath.Join(dir, ".clinerules", "hooks", "PreToolUse")

	mustWrite(t, script, "set -e\nexport AGNOSTIC_AI_TARGET=cline\n\n./old.sh\n")
	out, _ := runHookRun(t, "block-rm", "--target", "cline", "--bash", "ls")
	if !strings.Contains(out, "warning: .clinerules/hooks/PreToolUse does not run this spec's PreToolUse commands; run agnostic-ai sync") {
		t.Errorf("a stale script must warn:\n%s", out)
	}

	if err := os.Remove(script); err != nil {
		t.Fatal(err)
	}
	out, _ = runHookRun(t, "block-rm", "--target", "cline", "--bash", "ls")
	if !strings.Contains(out, "warning: .clinerules/hooks/PreToolUse does not exist; run agnostic-ai sync") {
		t.Errorf("a missing script must warn:\n%s", out)
	}
}

func TestHookRun_ClineDoesNotRunPreCompact(t *testing.T) {
	skipWithoutPOSIXShell(t)
	dir := clineProject(t, "name: block-rm\nevent: PreCompact\ncommand: .agnostic-ai/scripts/block-rm.sh\n", clineGuardScript)
	payload := filepath.Join(dir, "compact.json")
	mustWrite(t, payload, "{}")

	out, err := runHookRun(t, "block-rm", "--target", "cline", "--payload", payload)
	if !strings.Contains(out, "cline: not run (Cline has no PreCompact hook event)") || err == nil {
		t.Errorf("PreCompact never runs on Cline: %v\n%s", err, out)
	}
}

// clineSibling adds a second hook spec, run by its own script, and syncs.
func clineSibling(t *testing.T, dir, name, meta, body string) {
	t.Helper()
	script := filepath.Join(dir, ".agnostic-ai", "scripts", name+".sh")
	mustWrite(t, script, body)
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", name+".yaml"), "name: "+name+"\n"+meta+"command: .agnostic-ai/scripts/"+name+".sh\n")
	mustSync(t)
}

func TestHookRun_ClineSharedEventScriptIsNotCounted(t *testing.T) {
	skipWithoutPOSIXShell(t)
	dir := clineProject(t, clineHookSpec, clineGuardScript)
	clineSibling(t, dir, "allow-all", "event: PreToolUse\ntargets: [cline]\n", "#!/bin/sh\ncat >/dev/null\nprintf 'HOOK_CONTROL\\t{\"cancel\":false}\\n'\n")
	if synced, err := os.ReadFile(filepath.Join(dir, ".clinerules", "hooks", "PreToolUse")); err != nil || !strings.Contains(string(synced), "allow-all.sh") || !strings.Contains(string(synced), "block-rm.sh") {
		t.Fatalf("sync joins both specs in one script: %v\n%s", err, synced)
	}
	reason := "Cline runs this hook in one script with allow-all, which can change its result"

	out, err := runHookRun(t, "block-rm", "--bash", "rm -rf /", "--expect", "block", "--include-assumed")
	if !strings.Contains(out, "cline: block (exit 0") || !strings.Contains(out, "note: not counted: "+reason) {
		t.Errorf("output = %s", out)
	}
	if err != nil {
		t.Errorf("claude still passes --expect: %v", err)
	}

	out, _ = runHookRun(t, "block-rm", "--bash", "rm -rf /", "--include-assumed", "--format", "json")
	var report struct {
		Targets []struct {
			Target  string `json:"target"`
			Counted bool   `json:"counted"`
		} `json:"targets"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	for _, r := range report.Targets {
		if r.Target == "cline" && r.Counted {
			t.Errorf("a shared Cline script must not be counted:\n%s", out)
		}
	}

	_, err = runHookRun(t, "block-rm", "--target", "cline", "--bash", "rm -rf /", "--expect", "block", "--include-assumed")
	if err == nil || !strings.Contains(err.Error(), "--expect checks nothing") || !strings.Contains(err.Error(), reason) {
		t.Errorf("--expect on a shared Cline script alone must fail and say why: %v", err)
	}
}

func TestHookRun_ClineCountsASpecAloneInItsScript(t *testing.T) {
	skipWithoutPOSIXShell(t)
	dir := clineProject(t, clineHookSpec, clineGuardScript)
	clineSibling(t, dir, "claude-only", "event: PreToolUse\ntargets: [claude]\n", "#!/bin/sh\ncat >/dev/null\n")
	clineSibling(t, dir, "after-edit", "event: PostToolUse\n", "#!/bin/sh\ncat >/dev/null\n")

	out, err := runHookRun(t, "block-rm", "--target", "cline", "--bash", "rm -rf /", "--expect", "block", "--include-assumed")
	if err != nil || strings.Contains(out, "not counted") || !strings.Contains(out, "1 checked, 1 assumed (counted)") {
		t.Errorf("specs that sync keeps out of the PreToolUse script must not uncount it: %v\n%s", err, out)
	}
}
