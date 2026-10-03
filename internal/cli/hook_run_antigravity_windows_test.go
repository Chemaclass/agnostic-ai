package cli

import (
	"strings"
	"testing"
)

func TestHookRun_AntigravityIsNotRunOnWindows(t *testing.T) {
	antigravityProject(t, antigravityHookSpec, antigravityGuardScript)

	out, _ := runHookRun(t, "protect-files", "--bash", "ls")
	if !strings.Contains(out, "antigravity: not run (Antigravity does not document how it runs a hook command on Windows)") {
		t.Errorf("output = %s", out)
	}
}
