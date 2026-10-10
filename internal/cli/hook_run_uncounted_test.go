package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// uncountedProject syncs a project for target with one hook spec and the
// scripts its commands run, keyed by file name.
func uncountedProject(t *testing.T, target, hook string, scripts map[string]string) {
	t.Helper()
	dir := testutil.TempCwd(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: ["+target+"]\n")
	for name, body := range scripts {
		script := filepath.Join(dir, ".agnostic-ai", "scripts", name)
		mustWrite(t, script, body)
		if err := os.Chmod(script, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml"), hook)
	mustSync(t)
}

func TestHookRun_AnUncountedReplyKeepsAnotherCommandsTimeout(t *testing.T) {
	skipWithoutPOSIXShell(t)
	scripts := map[string]string{
		"disputed.sh": "#!/bin/sh\ncat >/dev/null\nexit 1\n",
		"slow.sh":     "#!/bin/sh\ncat >/dev/null\nsleep 5\n",
	}
	for _, order := range []string{"disputed.sh, slow.sh", "slow.sh, disputed.sh"} {
		t.Run(order, func(t *testing.T) {
			commands := strings.ReplaceAll(".agnostic-ai/scripts/"+order, ", ", ", .agnostic-ai/scripts/")
			uncountedProject(t, "kiro", "name: guard\nevent: PreToolUse\ntimeout: 1\ncommand: ["+commands+"]\n", scripts)

			payload := filepath.Join(t.TempDir(), "call.json")
			mustWrite(t, payload, `{"hook_event_name":"preToolUse","tool_name":"execute_bash","tool_input":{}}`)

			out, err := runHookRun(t, "guard", "--target", "kiro", "--payload", payload, "--include-assumed")
			if !strings.Contains(out, "not counted: Kiro's docs disagree") {
				t.Errorf("the exit 1 must leave Kiro uncounted:\n%s", out)
			}
			if err == nil || !strings.Contains(err.Error(), "timed out on kiro") {
				t.Errorf("the other command's timeout must still fail the run: %v\n%s", err, out)
			}

			if _, err := runHookRun(t, "guard", "--target", "kiro", "--payload", payload); err != nil {
				t.Errorf("without --include-assumed an assumed result is not judged: %v", err)
			}
		})
	}
}

func TestHookRun_AnUncountedReplyKeepsAnotherCommandsError(t *testing.T) {
	skipWithoutPOSIXShell(t)
	uncountedProject(t, "windsurf", "name: guard\nevent: PreToolUse\nmatcher: exec\ncommand: [.agnostic-ai/scripts/deny.sh, .agnostic-ai/scripts/broken.sh]\n", map[string]string{
		"deny.sh":   "#!/bin/sh\ncat >/dev/null\necho '{\"decision\": \"deny\"}'\n",
		"broken.sh": "#!/bin/sh\ncat >/dev/null\necho broken >&2\nexit 1\n",
	})

	out, err := runHookRun(t, "guard", "--target", "windsurf", "--bash", "ls", "--include-assumed")
	if !strings.Contains(out, `not counted: Devin CLI documents decision "approve" or "block"`) {
		t.Errorf("the unlisted decision must leave Windsurf uncounted:\n%s", out)
	}
	if !strings.Contains(out, "1 result not counted, see its note") {
		t.Errorf("the summary must count the result left out:\n%s", out)
	}
	if err == nil || !strings.Contains(err.Error(), "failed on windsurf") {
		t.Errorf("the documented exit 1 must still fail the run: %v\n%s", err, out)
	}
}

func TestHookRun_ASharedClineScriptIsNotJudged(t *testing.T) {
	skipWithoutPOSIXShell(t)
	dir := clineProject(t, clineHookSpec, "#!/bin/sh\ncat >/dev/null\necho checked\n")
	clineSibling(t, dir, "allow-all", "event: PreToolUse\ntargets: [cline]\n", "#!/bin/sh\ncat >/dev/null\nprintf 'HOOK_CONTROL\\t{\"cancel\":false}\\n'\n")

	out, err := runHookRun(t, "block-rm", "--bash", "ls", "--include-assumed")
	if !strings.Contains(out, "cline: error") || !strings.Contains(out, "not counted: Cline runs this hook in one script with allow-all") {
		t.Errorf("output = %s", out)
	}
	if err != nil {
		t.Errorf("the spec's error alone says nothing about the shared script, which allows: %v", err)
	}
}
