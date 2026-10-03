package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// windsurfGuardScript blocks a recursive delete with exit 2, which both
// Claude Code and Devin CLI document as a block. It fails when the
// Devin CLI run does not get DEVIN_PROJECT_DIR.
const windsurfGuardScript = `#!/bin/sh
payload=$(cat)
case "$payload" in
  *'"tool_name":"exec"'*) [ -d "$DEVIN_PROJECT_DIR/.devin" ] || exit 1 ;;
esac
case "$payload" in
  *'rm -rf'*) echo "no recursive delete" >&2; exit 2 ;;
esac
exit 0
`

// windsurfProject syncs a claude+windsurf project with one hook spec
// whose command runs body.
func windsurfProject(t *testing.T, hook, body string) {
	t.Helper()
	dir := testutil.TempCwd(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, windsurf]\n")
	script := filepath.Join(dir, ".agnostic-ai", "scripts", "protect-files.sh")
	mustWrite(t, script, body)
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "protect-files.yaml"), hook)
	mustSync(t)
}

// windsurfHookSpec matches Devin CLI's exec tool and Claude Code's Bash.
const windsurfHookSpec = "name: protect-files\nevent: PreToolUse\nmatcher: exec|Bash\ncommand: .agnostic-ai/scripts/protect-files.sh\n"

func TestHookRun_WindsurfBlocksOnExit2OnAnAssumedShell(t *testing.T) {
	skipWithoutPOSIXShell(t)
	windsurfProject(t, windsurfHookSpec, windsurfGuardScript)

	out, err := runHookRun(t, "protect-files", "--bash", "rm -rf /", "--expect", "block")
	for _, want := range []string{
		"claude: block (exit 2",
		"windsurf: block (exit 2", "(assumed: shell, working directory, timeout)", "event: PreToolUse (exec)",
		"command: .devin/hooks/protect-files.sh",
		"assumed shell: sh -c (Devin CLI does not document the shell that runs a hook command)",
		"assumed working directory: project root (Devin CLI does not document the directory a hook command runs in)",
		"assumed timeout: 30s (Devin CLI documents no default timeout; set timeout in the spec)",
		"docs: https://docs.devin.ai/cli/extensibility/hooks",
		"1 checked, 1 assumed (not counted; --include-assumed to count)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}
	if err != nil {
		t.Errorf("the counted claude result passes --expect: %v", err)
	}
	if strings.Contains(out, "warning:") {
		t.Errorf("a fresh sync must not warn:\n%s", out)
	}

	out, err = runHookRun(t, "protect-files", "--bash", "rm -rf /", "--expect", "block", "--include-assumed")
	if err != nil || !strings.Contains(out, "2 checked, 1 assumed (counted)") {
		t.Errorf("--include-assumed must count windsurf: %v\n%s", err, out)
	}

	out, err = runHookRun(t, "protect-files", "--bash", "ls", "--expect", "allow", "--include-assumed")
	if err != nil || !strings.Contains(out, "windsurf: allow (exit 0") {
		t.Errorf("exit 0 must allow, with DEVIN_PROJECT_DIR set: %v\n%s", err, out)
	}
}

func TestHookRun_WindsurfReadsADecisionReply(t *testing.T) {
	skipWithoutPOSIXShell(t)
	windsurfProject(t, windsurfHookSpec, "#!/bin/sh\ncat >/dev/null\necho '{\"decision\":\"block\",\"reason\":\"no\"}'\n")

	out, err := runHookRun(t, "protect-files", "--bash", "rm -rf /", "--expect", "block", "--include-assumed")
	if err != nil || !strings.Contains(out, "windsurf: block (exit 0") || strings.Contains(out, "not counted:") {
		t.Errorf("decision block at exit 0 must count as block: %v\n%s", err, out)
	}

	windsurfProject(t, windsurfHookSpec, "#!/bin/sh\ncat >/dev/null\necho '{\"decision\":\"approve\"}'\n")
	out, err = runHookRun(t, "protect-files", "--bash", "git status", "--expect", "allow", "--include-assumed")
	if err != nil || !strings.Contains(out, "windsurf: allow (exit 0") {
		t.Errorf("decision approve must allow: %v\n%s", err, out)
	}
}

func TestHookRun_WindsurfDoesNotCountAReplyOnANonZeroExit(t *testing.T) {
	skipWithoutPOSIXShell(t)
	for body, want := range map[string]string{
		"echo '{\"decision\":\"block\"}'\nexit 1\n":   "windsurf: error (exit 1",
		"echo '{\"decision\":\"approve\"}'\nexit 2\n": "windsurf: block (exit 2",
	} {
		windsurfProject(t, windsurfHookSpec, "#!/bin/sh\ncat >/dev/null\n"+body)
		out, err := runHookRun(t, "protect-files", "--target", "windsurf", "--bash", "rm -rf /", "--expect", "block", "--include-assumed")
		for _, s := range []string{
			want,
			"note: not counted: Devin CLI does not document whether it reads a reply on a non-zero exit",
			"0 checked, 1 assumed (counted; 1 result not counted, see its note)",
		} {
			if !strings.Contains(out, s) {
				t.Errorf("output misses %q:\n%s", s, out)
			}
		}
		if err == nil || !strings.Contains(err.Error(), "--expect checks nothing") {
			t.Errorf("--expect on an uncounted result alone must fail and say why: %v", err)
		}
	}
}

func TestHookRun_WindsurfDoesNotCountABlockOnPostToolUse(t *testing.T) {
	skipWithoutPOSIXShell(t)
	windsurfProject(t, "name: protect-files\nevent: PostToolUse\nmatcher: exec\ncommand: .agnostic-ai/scripts/protect-files.sh\n", windsurfGuardScript)

	out, _ := runHookRun(t, "protect-files", "--target", "windsurf", "--bash", "rm -rf /", "--include-assumed")
	if !strings.Contains(out, "windsurf: block (exit 2") || !strings.Contains(out, "not counted: Devin CLI does not document what a block does on PostToolUse") {
		t.Errorf("a PostToolUse block must be listed as not counted:\n%s", out)
	}
}

func TestHookRun_WindsurfSkipsAnInlineShellCommand(t *testing.T) {
	skipWithoutPOSIXShell(t)
	windsurfProject(t, "name: protect-files\nevent: PreToolUse\nmatcher: exec\ncommand: 'cat | grep -q rm && exit 2'\n", windsurfGuardScript)

	out, _ := runHookRun(t, "protect-files", "--bash", "rm -rf /")
	if !strings.Contains(out, "windsurf: not run (Devin CLI does not document its shell; use a script path)") {
		t.Errorf("output = %s", out)
	}
}

func TestHookRun_WindsurfRefusesTheEditInput(t *testing.T) {
	skipWithoutPOSIXShell(t)
	windsurfProject(t, "name: protect-files\nevent: PreToolUse\nmatcher: exec|Write\ncommand: .agnostic-ai/scripts/protect-files.sh\n", windsurfGuardScript)

	out, _ := runHookRun(t, "protect-files", "--edit", "a.go")
	if !strings.Contains(out, "windsurf: not run (Devin CLI documents no tool_input for its edit, write, or apply_patch tools; pass --payload <file>)") || !strings.Contains(out, "claude: allow (exit 0") {
		t.Errorf("--edit must list Windsurf as not run and still run the others:\n%s", out)
	}
}

func TestHookRun_WindsurfDoesNotFireOnAClaudeMatcher(t *testing.T) {
	skipWithoutPOSIXShell(t)
	windsurfProject(t, "name: protect-files\nevent: PreToolUse\nmatcher: Bash\ncommand: .agnostic-ai/scripts/protect-files.sh\n", windsurfGuardScript)

	out, _ := runHookRun(t, "protect-files", "--bash", "rm -rf /")
	if !strings.Contains(out, `windsurf: allow (not run: matcher "Bash" does not match exec)`) {
		t.Errorf("Devin CLI names its shell tool exec, so Bash must not fire:\n%s", out)
	}
}

func TestHookRun_WindsurfJSONListsItsAssumptions(t *testing.T) {
	skipWithoutPOSIXShell(t)
	windsurfProject(t, windsurfHookSpec, windsurfGuardScript)

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
		if r.Target != "windsurf" {
			continue
		}
		if r.Decision != "block" || r.Trigger != "exec" || r.Counted || len(r.Assumptions) != 3 {
			t.Fatalf("windsurf = %+v; want block on exec, not counted, three assumptions", r)
		}
		for i, want := range [][2]string{{"shell", "sh -c"}, {"working directory", "project root"}, {"timeout", "30s"}} {
			if a := r.Assumptions[i]; a.Item != want[0] || a.Value != want[1] || a.Reason == "" {
				t.Errorf("assumption %d = %+v, want %s %s with a reason", i, a, want[0], want[1])
			}
		}
		return
	}
	t.Fatalf("no windsurf result:\n%s", out)
}
