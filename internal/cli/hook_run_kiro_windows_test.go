package cli

import (
	"strings"
	"testing"
)

func TestHookRun_KiroIsNotRunOnWindows(t *testing.T) {
	kiroProject(t, kiroGuardSpec, kiroGuardScript)

	out, _ := runHookRun(t, "deploy-guard", "--prompt", "hello")
	if !strings.Contains(out, "kiro: not run (Kiro does not document how it runs a hook command on Windows)") {
		t.Errorf("output = %s", out)
	}
}
