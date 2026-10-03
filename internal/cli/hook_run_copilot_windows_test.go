package cli

import (
	"strings"
	"testing"
)

func TestHookRun_CopilotCommandFormIsNotRunOnWindows(t *testing.T) {
	copilotProject(t, "name: guard\nevent: preToolUse\ncommand: .agnostic-ai/scripts/guard.sh\n", copilotGuardScript)

	out, _ := runHookRun(t, "guard", "--bash", "ls")
	if !strings.Contains(out, "copilot: not run (Copilot does not document the interpreter behind its powershell field)") {
		t.Errorf("output = %s", out)
	}
}
