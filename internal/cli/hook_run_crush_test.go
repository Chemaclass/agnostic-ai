package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const crushGuardScript = `#!/bin/sh
payload=$(cat)
case "$payload" in
  *'rm -rf'*) echo "no recursive delete" >&2; exit 2 ;;
  *shutdown*) echo "stop the turn" >&2; exit 49 ;;
esac
`

func crushProject(t *testing.T, hook, script string) {
	t.Helper()
	dir := testutil.TempCwd(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [crush]\n")
	path := filepath.Join(dir, ".agnostic-ai", "scripts", "guard.sh")
	mustWrite(t, path, script)
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml"), hook)
	mustSync(t)
}

func TestHookRun_CrushBlocksHaltsAndPasses(t *testing.T) {
	skipWithoutPOSIXShell(t)
	crushProject(t, "name: guard\nevent: PreToolUse\nmatcher: ^bash$\ncommand: .agnostic-ai/scripts/guard.sh\n", crushGuardScript)

	for _, tc := range []struct {
		bash, expect string
		want         []string
	}{
		{"rm -rf /", "block", []string{"crush: block (exit 2", "event: PreToolUse (bash)", "stderr: no recursive delete"}},
		{"shutdown now", "block", []string{"crush: block (exit 49", "note: halt: Crush ends the whole turn"}},
		{"ls", "allow", []string{"crush: allow (exit 0"}},
	} {
		out, err := runHookRun(t, "guard", "--bash", tc.bash, "--expect", tc.expect)
		if err != nil {
			t.Errorf("%s: %v\n%s", tc.bash, err, out)
		}
		for _, want := range tc.want {
			if !strings.Contains(out, want) {
				t.Errorf("%s: output misses %q:\n%s", tc.bash, want, out)
			}
		}
		if strings.Contains(out, "warning:") || strings.Contains(out, "assumed") {
			t.Errorf("%s: a fresh sync with no Crush builtin warns or assumes:\n%s", tc.bash, out)
		}
	}
}

func TestHookRun_CrushMatcherIsAnUnanchoredRegexOnItsToolName(t *testing.T) {
	skipWithoutPOSIXShell(t)
	crushProject(t, "name: guard\nevent: pre_tool_use\nmatcher: Bash\ncommand: .agnostic-ai/scripts/guard.sh\n", crushGuardScript)

	out, err := runHookRun(t, "guard", "--bash", "rm -rf /")
	if err != nil || !strings.Contains(out, `crush: allow (not run: matcher "Bash" does not match bash)`) {
		t.Errorf("%v\n%s", err, out)
	}
}

func TestHookRun_CrushHookThatCallsJqIsAssumed(t *testing.T) {
	skipWithoutPOSIXShell(t)
	crushProject(t, "name: guard\nevent: PreToolUse\ncommand: 'jq --version >/dev/null 2>&1; exit 2'\n", crushGuardScript)

	out, err := runHookRun(t, "guard", "--bash", "ls", "--format", "json")
	var report struct {
		Targets []struct {
			Target      string                  `json:"target"`
			Decision    string                  `json:"decision"`
			Counted     bool                    `json:"counted"`
			Assumptions []struct{ Item string } `json:"assumptions"`
		} `json:"targets"`
	}
	if jsonErr := json.Unmarshal([]byte(out), &report); jsonErr != nil || len(report.Targets) != 1 {
		t.Fatalf("invalid JSON: %v %v\n%s", jsonErr, err, out)
	}
	got := report.Targets[0]
	if got.Decision != "block" || got.Counted || len(got.Assumptions) != 1 || got.Assumptions[0].Item != "jq" {
		t.Errorf("a hook that runs jq from PATH is assumed and not counted: %+v", got)
	}
}
