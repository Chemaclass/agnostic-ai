package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncGlobal_NotesAgentFieldsTheTargetDoesNotWrite(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agents", "reviewer.md"), "---\nname: reviewer\ndescription: Review code\nmaxTurns: 5\nbackground: true\n---\nReview carefully.\n")

	out, warnings, err := runGlobalAgentTest("--only", "codex", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	all := out + warnings
	for _, field := range []string{"`maxTurns`", "`background`"} {
		if n := strings.Count(all, field+" on 1 agent has no effect on codex"); n != 1 {
			t.Errorf("note for %s printed %d times, want once:\n%s", field, n, all)
		}
	}
}

func TestSyncGlobal_PrintsQwensOwnAgentNoteOnce(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agents", "reviewer.md"), "---\nname: reviewer\ndescription: Review code\npermissionMode: plan\neffort: high\n---\nReview carefully.\n")

	out, warnings, err := runGlobalAgentTest("--only", "qwen", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	all := out + warnings
	for _, field := range []string{"`permissionMode`", "`effort`"} {
		if n := strings.Count(all, field+" on 1 agent has no effect on qwen"); n != 1 {
			t.Errorf("note for %s printed %d times, want once:\n%s", field, n, all)
		}
	}
}
