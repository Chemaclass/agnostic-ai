package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncGlobal_CheckPassesAfterRemovingOnlySkill(t *testing.T) {
	_, source := globalAgentTestHome(t)
	skill := filepath.Join(source, "skills", "foo", "SKILL.md")
	mustWriteGlobalTest(t, skill, "---\nname: foo\ndescription: Foo\n---\nFoo.\n")
	if _, w, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	if err := os.RemoveAll(filepath.Dir(skill)); err != nil {
		t.Fatal(err)
	}
	if _, w, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatalf("second sync: %v\n%s", err, w)
	}
	if out, _, err := runGlobalAgentTest("--only", "claude", "--check", "--diff"); err != nil {
		t.Fatalf("check right after sync: %v\n%s", err, out)
	}
	if out, _, err := runGlobalAgentTest("--only", "claude", "--plan"); err != nil || strings.TrimSpace(out) != "No changes." {
		t.Errorf("plan right after sync: %v\n%s", err, out)
	}
}
