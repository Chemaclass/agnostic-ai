//go:build darwin || linux

package conversations

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"testing"
	"time"
)

const helperReady = "ready"

// The test binary is the child because macOS hides system binaries' environments.
func TestHelperProcessSleeps(t *testing.T) {
	if os.Getenv("CONVERSATIONS_HELPER_SLEEPS") != "1" {
		t.Skip("only runs as a child of TestClaudeParents_ReadsRealEnvironmentsOnly")
	}
	fmt.Println(helperReady)
	time.Sleep(30 * time.Second)
}

func TestClaudeParents_ReadsRealEnvironmentsOnly(t *testing.T) {
	start := func(env []string, args ...string) int {
		t.Helper()
		cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestHelperProcessSleeps$"}, args...)...)
		cmd.Env = append(slices.Clip(env), "CONVERSATIONS_HELPER_SLEEPS=1")
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		// Until the child's own code runs, the system may still show the
		// environment of the program it replaces.
		line := make(chan string, 1)
		go func() {
			read, _ := bufio.NewReader(stdout).ReadString('\n')
			line <- read
		}()
		select {
		case got := <-line:
			if got != helperReady+"\n" {
				t.Fatalf("the helper printed %q instead of %q", got, helperReady)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("the helper did not print %q within 10s", helperReady)
		}
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
