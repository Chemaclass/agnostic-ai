//go:build darwin || linux

package conversations

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

// The test binary is the child because macOS hides system binaries' environments.
func TestHelperProcessSleeps(t *testing.T) {
	if os.Getenv("CONVERSATIONS_HELPER_SLEEPS") != "1" {
		t.Skip("only runs as a child of TestClaudeParents_ReadsRealEnvironmentsOnly")
	}
	time.Sleep(30 * time.Second)
}

func TestClaudeParents_ReadsRealEnvironmentsOnly(t *testing.T) {
	start := func(env []string, args ...string) int {
		t.Helper()
		cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestHelperProcessSleeps$"}, args...)...)
		cmd.Env = append(env, "CONVERSATIONS_HELPER_SLEEPS=1")
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		return cmd.Process.Pid
	}
	decoy := start([]string{"REVIEW_TEXT=example CLAUDE_CODE_SESSION_ID=" + idSafari + " CLAUDE_CODE_CHILD_SESSION=1"},
		"CLAUDE_CODE_SESSION_ID="+idSafari, "CLAUDE_CODE_CHILD_SESSION=1")
	child := start([]string{"PATH=/bin", "CLAUDE_CODE_SESSION_ID=" + idCache, "CLAUDE_CODE_CHILD_SESSION=1"})

	parents, err := SystemProcesses{}.ClaudeParents([]int{decoy, child, 999999})

	if err != nil {
		t.Fatal(err)
	}
	if len(parents) != 1 || parents[child] != idCache {
		t.Errorf("only the process whose environment names the session is linked, actual %v", parents)
	}
}
