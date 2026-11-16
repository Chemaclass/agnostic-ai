package conversations

import (
	"encoding/binary"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// procargs builds a kern.procargs2 value the way the kernel lays it out.
func procargs(path string, args, env []string) []byte {
	raw := binary.NativeEndian.AppendUint32(nil, uint32(len(args)))
	raw = append(raw, path...)
	raw = append(raw, 0, 0, 0, 0)
	for _, s := range append(append(append([]string{}, args...), env...), "", "ptr_munge=") {
		raw = append(append(raw, s...), 0)
	}
	return raw
}

func TestProcargsEnvironment_KeepsEachEntryWhole(t *testing.T) {
	raw := procargs("/opt/homebrew/bin/codex",
		[]string{"codex", "exec", "CLAUDE_CODE_SESSION_ID=" + idCache + " CLAUDE_CODE_CHILD_SESSION=1", ""},
		[]string{"REVIEW_TEXT=example CLAUDE_CODE_SESSION_ID=" + idCache, "HOME=/Users/me"})

	entries, ok := procargsEnvironment(raw)

	if !ok || strings.Join(entries, "|") != "REVIEW_TEXT=example CLAUDE_CODE_SESSION_ID="+idCache+"|HOME=/Users/me" {
		t.Fatalf("the environment starts after the arguments and ends at the empty entry, actual %q %v", entries, ok)
	}
	if _, linked := claudeParentIn(claudeEntries(entries)); linked {
		t.Error("names inside arguments or another value must not link")
	}
}

func TestProcargsEnvironment_RejectsAValueThatEndsInItsArguments(t *testing.T) {
	raw := procargs("/bin/codex", []string{"codex", "exec"}, nil)
	raw = raw[:len(raw)-len("exec\x00\x00ptr_munge=\x00")+2]
	if entries, ok := procargsEnvironment(raw); ok {
		t.Errorf("a cut value has no known boundary, actual %q", entries)
	}
	if _, ok := procargsEnvironment([]byte{1, 0}); ok {
		t.Error("a value without its count must not parse")
	}
}

// TestHelperProcessSleeps is no test: it is the child the real-process
// test starts, the test binary itself, since macOS hides the environment
// of its own system binaries such as /bin/sleep.
func TestHelperProcessSleeps(t *testing.T) {
	if os.Getenv("CONVERSATIONS_HELPER_SLEEPS") != "1" {
		t.Skip("only runs as a child of TestClaudeParents_ReadsRealEnvironmentsOnly")
	}
	time.Sleep(30 * time.Second)
}

// A real process: the names sit inside an unrelated value and in an
// argument, and only a process whose environment sets them is linked.
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
