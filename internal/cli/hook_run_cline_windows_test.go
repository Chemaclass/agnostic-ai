package cli

import (
	"strings"
	"testing"
)

func TestHookRun_ClineIsNotRunOnWindows(t *testing.T) {
	clineProject(t, clineHookSpec, clineGuardScript)

	out, _ := runHookRun(t, "block-rm", "--bash", "ls")
	if !strings.Contains(out, "cline: not run (Cline runs hooks with bash; hook run does not assume which bash Windows resolves)") {
		t.Errorf("output = %s", out)
	}
}
