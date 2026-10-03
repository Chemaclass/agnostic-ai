package cli

import (
	"strings"
	"testing"
)

func TestHookRun_QoderIsNotRunOnWindows(t *testing.T) {
	qoderProject(t, qoderHookSpec, qoderGuardScript)

	out, _ := runHookRun(t, "protect-files", "--bash", "ls")
	if !strings.Contains(out, `qoder: not run (Qoder documents its Windows shell only as "system default")`) {
		t.Errorf("output = %s", out)
	}
}
