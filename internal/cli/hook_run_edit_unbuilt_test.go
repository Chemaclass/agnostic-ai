package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const editUnbuiltScript = "#!/bin/sh\ncat >/dev/null\nexit 0\n"

func editUnbuiltProject(t *testing.T, targets, event string) {
	t.Helper()
	dir := testutil.TempCwd(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: "+targets+"\n")
	script := filepath.Join(dir, ".agnostic-ai", "scripts", "guard.sh")
	mustWrite(t, script, editUnbuiltScript)
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml"), "name: guard\nevent: "+event+"\ncommand: .agnostic-ai/scripts/guard.sh\n")
	mustSync(t)
}

func TestHookRun_EditListsTargetsThatCannotBuildItAsNotRun(t *testing.T) {
	skipWithoutPOSIXShell(t)
	for _, tc := range []struct{ target, reason string }{
		{"trae", "trae: not run (Trae documents no tool_input for its edit tools"},
		{"openhands", "openhands: not run (OpenHands documents no tool_input for its file editor"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			editUnbuiltProject(t, "[claude, "+tc.target+"]", "PreToolUse")
			out, err := runHookRun(t, "guard", "--edit", "a.go", "--expect", "allow")
			if err != nil || !strings.Contains(out, tc.reason) || !strings.Contains(out, "claude: allow") {
				t.Errorf("err = %v\n%s", err, out)
			}
		})
	}
}

func TestHookRun_EditFailsNamingEachReasonWhenNoTargetCanBuildIt(t *testing.T) {
	skipWithoutPOSIXShell(t)
	editUnbuiltProject(t, "[trae, openhands]", "PreToolUse")
	_, err := runHookRun(t, "guard", "--edit", "a.go")
	if err == nil || !strings.Contains(err.Error(), "Trae documents no tool_input") || !strings.Contains(err.Error(), "OpenHands documents no tool_input") {
		t.Errorf("err = %v, want every reason named", err)
	}

	editUnbuiltProject(t, "[cursor]", "preToolUse")
	_, err = runHookRun(t, "guard", "--edit", "a.go")
	if err == nil || !strings.Contains(err.Error(), "Cursor documents no tool_input for its Write tool") {
		t.Errorf("cursor: err = %v, want the reason named", err)
	}
}
