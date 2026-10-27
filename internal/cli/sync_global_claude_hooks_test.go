package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncGlobal_NotesClaudeHookIfOnNonToolEvent(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "stop.yaml"), "name: stop\nevent: Stop\ncommand: echo hi\nif: \"Bash(ls *)\"\n")

	_, errOut, err := runGlobalAgentTest("--only", "claude")
	if err != nil {
		t.Fatalf("sync --global: %v", err)
	}
	if want := "`if` on 1 hook has no effect on claude"; !strings.Contains(errOut, want) {
		t.Errorf("want %q in:\n%s", want, errOut)
	}
}

func TestSyncGlobal_ClaudeHookIfOnToolEventGetsNoNote(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "pre.yaml"), "name: pre\nevent: PreToolUse\nmatcher: Bash\ncommand: echo hi\nif: \"Bash(ls *)\"\n")

	_, errOut, err := runGlobalAgentTest("--only", "claude")
	if err != nil {
		t.Fatalf("sync --global: %v", err)
	}
	if strings.Contains(errOut, "`if` on") {
		t.Errorf("PreToolUse evaluates if, got:\n%s", errOut)
	}
}
