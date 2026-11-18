package cli

import (
	"strings"
	"testing"
)

func TestHookRun_FactoryIsNotRunOnWindows(t *testing.T) {
	factoryProject(t, factoryHookSpec, factoryGuardScript)

	out, _ := runHookRun(t, "protect-files", "--bash", "ls")
	if !strings.Contains(out, "factory: not run (hook run does not implement Factory hook execution on Windows)") {
		t.Errorf("output = %s", out)
	}
}
