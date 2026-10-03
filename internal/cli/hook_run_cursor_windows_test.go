package cli

import (
	"strings"
	"testing"
)

func TestHookRun_CursorIsNotRunOnWindows(t *testing.T) {
	cursorProject(t, "name: protect-files\nevent: beforeShellExecution\ncommand: .agnostic-ai/scripts/protect-files.sh\n")

	out, _ := runHookRun(t, "protect-files", "--bash", "ls")
	if !strings.Contains(out, "cursor: not run (Cursor does not document how it runs a hook command on Windows)") {
		t.Errorf("output = %s", out)
	}
}
