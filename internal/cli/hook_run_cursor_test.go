package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const cursorGuardScript = `#!/bin/sh
payload=$(cat)
case "$payload" in
  *'rm -rf'*) echo "no recursive delete" >&2; exit 2 ;;
esac
echo '{"permission":"allow"}'
`

// cursorProject syncs a claude+cursor project with one Cursor hook spec
// whose command is hook.
func cursorProject(t *testing.T, hook string) {
	t.Helper()
	dir := testutil.TempCwd(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, cursor]\n")
	script := filepath.Join(dir, ".agnostic-ai", "scripts", "protect-files.sh")
	mustWrite(t, script, cursorGuardScript)
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "protect-files.yaml"), hook)
	mustSync(t)
}

func TestHookRun_CursorRunsOnAssumptionsAndCountsOnlyWhenAsked(t *testing.T) {
	skipWithoutPOSIXShell(t)
	cursorProject(t, "name: protect-files\nevent: beforeShellExecution\ncommand: .agnostic-ai/scripts/protect-files.sh\n")

	out, err := runHookRun(t, "protect-files", "--bash", "rm -rf /", "--expect", "block")
	for _, want := range []string{
		"cursor: block (exit 2", "(assumed: shell, timeout)",
		"assumed shell: sh -c", "assumed timeout: 30s", "docs: https://cursor.com/docs/hooks",
		"claude: not run (claude has no beforeShellExecution event)",
		"0 checked, 1 assumed (not counted; --include-assumed to count)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}
	if err == nil || !strings.Contains(err.Error(), "ran only where hook run assumes part of the contract (cursor); pass --include-assumed") {
		t.Errorf("an uncounted run must not pass a check silently: %v", err)
	}
	if strings.Contains(out, "warning:") {
		t.Errorf("a fresh sync warns:\n%s", out)
	}

	out, err = runHookRun(t, "protect-files", "--bash", "rm -rf /", "--expect", "block", "--include-assumed")
	if err != nil || !strings.Contains(out, "1 checked, 1 assumed (counted)") {
		t.Errorf("--include-assumed must count cursor: %v\n%s", err, out)
	}

	out, err = runHookRun(t, "protect-files", "--bash", "rm -rf /", "--expect", "allow", "--include-assumed")
	if err == nil || !strings.Contains(err.Error(), "expected allow, got cursor block") {
		t.Errorf("a counted cursor result must fail --expect: %v\n%s", err, out)
	}

	out, _ = runHookRun(t, "protect-files", "--bash", "rm -rf /", "--expect", "allow")
	if !strings.Contains(out, "warning: assumed result block differs from allow and is not counted") {
		t.Errorf("an uncounted disagreement must warn:\n%s", out)
	}
}

func TestHookRun_CursorSkipsAnInlineShellCommand(t *testing.T) {
	skipWithoutPOSIXShell(t)
	cursorProject(t, "name: protect-files\nevent: beforeShellExecution\ncommand: 'cat | grep -q rm && exit 2'\n")

	out, _ := runHookRun(t, "protect-files", "--bash", "rm -rf /")
	if !strings.Contains(out, "cursor: not run (Cursor does not document its shell; use a script path)") {
		t.Errorf("output = %s", out)
	}
}

func TestHookRun_CursorJSONListsAssumptions(t *testing.T) {
	skipWithoutPOSIXShell(t)
	cursorProject(t, "name: protect-files\nevent: beforeShellExecution\ntimeout: 5\ncommand: .agnostic-ai/scripts/protect-files.sh\n")

	out, _ := runHookRun(t, "protect-files", "--bash", "ls", "--format", "json")
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
		if r.Target != "cursor" {
			continue
		}
		if r.Decision != "allow" || r.Counted || len(r.Assumptions) != 1 || r.Assumptions[0].Item != "shell" || r.Assumptions[0].Value != "sh -c" || r.Assumptions[0].Reason == "" {
			t.Errorf("cursor = %+v; want allow, not counted, one shell assumption (the spec sets a timeout)", r)
		}
		return
	}
	t.Fatalf("no cursor result:\n%s", out)
}
