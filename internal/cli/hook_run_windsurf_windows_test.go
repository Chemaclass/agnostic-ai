package cli

import (
	"strings"
	"testing"
)

func TestHookRun_WindsurfIsNotRunOnWindows(t *testing.T) {
	windsurfProject(t, windsurfHookSpec, windsurfGuardScript)

	out, _ := runHookRun(t, "protect-files", "--bash", "ls")
	if !strings.Contains(out, "windsurf: not run (Devin CLI does not document how it runs a hook command on Windows)") {
		t.Errorf("output = %s", out)
	}
}
