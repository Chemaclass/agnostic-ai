package cli

import (
	"strings"
	"testing"
)

func TestHookRun_FactoryIsNotRunOnWindows(t *testing.T) {
	factoryProject(t, factoryHookSpec, factoryGuardScript)

	out, _ := runHookRun(t, "protect-files", "--bash", "ls")
	if !strings.Contains(out, "factory: not run (Factory does not document how it runs a hook command on Windows)") {
		t.Errorf("output = %s", out)
	}
}
