package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// qoderGuardScript blocks a recursive delete: through Qoder's documented
// PreToolUse deny reply when QODER_PROJECT_DIR is set, and through exit 2
// elsewhere.
const qoderGuardScript = `#!/bin/sh
payload=$(cat)
case "$payload" in
  *'rm -rf'*) ;;
  *) exit 0 ;;
esac
if [ -n "$QODER_PROJECT_DIR" ]; then
  echo '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"no recursive delete"}}'
  exit 0
fi
echo "no recursive delete" >&2
exit 2
`

// qoderProject syncs a claude+qoder project with one hook spec whose
// command runs body.
func qoderProject(t *testing.T, hook, body string) {
	t.Helper()
	dir := testutil.TempCwd(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, qoder]\n")
	script := filepath.Join(dir, ".agnostic-ai", "scripts", "protect-files.sh")
	mustWrite(t, script, body)
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "protect-files.yaml"), hook)
	mustSync(t)
}

const qoderHookSpec = "name: protect-files\nevent: PreToolUse\nmatcher: Bash\ncommand: .agnostic-ai/scripts/protect-files.sh\n"

func TestHookRun_QoderRunsOnAnAssumedShellAndCountsOnlyWhenAsked(t *testing.T) {
	skipWithoutPOSIXShell(t)
	qoderProject(t, qoderHookSpec, qoderGuardScript)

	out, err := runHookRun(t, "protect-files", "--bash", "rm -rf /", "--expect", "block")
	for _, want := range []string{
		"claude: block (exit 2",
		"qoder: block (exit 0", "(assumed: shell, working directory)", "event: PreToolUse (Bash)",
		`assumed shell: sh -c (Qoder documents its default shell only as "system default")`,
		"assumed working directory: project root (Qoder does not document the directory a hook runs in)",
		"docs: https://docs.qoder.com/cli/hooks",
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
		t.Errorf("a fresh sync warns, or the documented 600s timeout is marked assumed:\n%s", out)
	}

	out, err = runHookRun(t, "protect-files", "--bash", "rm -rf /", "--expect", "block", "--include-assumed")
	if err != nil || !strings.Contains(out, "2 checked, 1 assumed (counted)") {
		t.Errorf("--include-assumed must count qoder: %v\n%s", err, out)
	}

	out, _ = runHookRun(t, "protect-files", "--bash", "rm -rf /", "--expect", "allow")
	if !strings.Contains(out, "qoder: warning: assumed result block differs from allow and is not counted") {
		t.Errorf("an uncounted disagreement must warn:\n%s", out)
	}
}

func TestHookRun_QoderJSONListsItsAssumptions(t *testing.T) {
	skipWithoutPOSIXShell(t)
	qoderProject(t, qoderHookSpec, qoderGuardScript)

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
		if r.Target != "qoder" {
			continue
		}
		if r.Decision != "block" || r.Trigger != "Bash" || r.Counted || len(r.Assumptions) != 2 || r.Assumptions[0].Item != "shell" || r.Assumptions[0].Value != "sh -c" || r.Assumptions[1].Item != "working directory" || r.Assumptions[1].Value != "project root" {
			t.Errorf("qoder = %+v; want block on Bash, not counted, shell and cwd assumed", r)
		}
		return
	}
	t.Fatalf("no qoder result:\n%s", out)
}

func TestHookRun_QoderSkipsAnInlineShellCommand(t *testing.T) {
	skipWithoutPOSIXShell(t)
	qoderProject(t, "name: protect-files\nevent: PreToolUse\ncommand: 'cat | grep -q rm && exit 2'\n", qoderGuardScript)

	out, _ := runHookRun(t, "protect-files", "--bash", "rm -rf /")
	if !strings.Contains(out, "qoder: not run (Qoder does not document its shell; use a script path)") {
		t.Errorf("output = %s", out)
	}
}

func TestHookRun_QoderRefusesAnUndocumentedEditInput(t *testing.T) {
	skipWithoutPOSIXShell(t)
	qoderProject(t, "name: protect-files\nevent: PreToolUse\nmatcher: Edit\ncommand: .agnostic-ai/scripts/protect-files.sh\n", qoderGuardScript)

	out, _ := runHookRun(t, "protect-files", "--edit", "a.go")
	if !strings.Contains(out, "qoder: not run (Qoder documents no tool_input for its Edit tool") || !strings.Contains(out, "claude: ") {
		t.Errorf("--edit on Edit must list Qoder as not run and still run the others:\n%s", out)
	}
}

func TestHookRun_QoderHonorsIf(t *testing.T) {
	skipWithoutPOSIXShell(t)
	qoderProject(t, qoderHookSpec+"if: Bash(git *)\n", qoderGuardScript)

	out, _ := runHookRun(t, "protect-files", "--bash", "rm -rf /")
	if !strings.Contains(out, `qoder: allow (not run: if "Bash(git *)" does not match this Bash call)`) {
		t.Errorf("an if that does not match must skip the handler:\n%s", out)
	}
}

func TestHookRun_AsyncRewakeIsNotJudged(t *testing.T) {
	skipWithoutPOSIXShell(t)
	qoderProject(t, qoderHookSpec+"asyncRewake: true\n", "#!/bin/sh\ncat >/dev/null\nexit 2\n")

	for _, args := range [][]string{{"--expect", "allow"}, {"--expect", "allow", "--include-assumed"}} {
		out, err := runHookRun(t, append([]string{"protect-files", "--bash", "ls"}, args...)...)
		if err != nil {
			t.Errorf("%v: an asyncRewake hook blocks nothing, so --expect allow passes: %v\n%s", args, err, out)
		}
		for _, target := range []string{"claude", "qoder"} {
			if !strings.Contains(out, target+": not judged (exit 2") || !strings.Contains(out, "note: async hook; "+target+" does not wait for its result") {
				t.Errorf("%v: %s must be shown as async:\n%s", args, target, out)
			}
		}
	}
	out, _ := runHookRun(t, "protect-files", "--bash", "ls", "--format", "json")
	var report struct {
		Targets []struct {
			Target  string `json:"target"`
			Async   bool   `json:"async"`
			Counted bool   `json:"counted"`
		} `json:"targets"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil || len(report.Targets) != 2 {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	for _, r := range report.Targets {
		if !r.Async || r.Counted {
			t.Errorf("%s = %+v; want async and not counted", r.Target, r)
		}
	}
}
