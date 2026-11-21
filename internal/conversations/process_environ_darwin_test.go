package conversations

import (
	"encoding/binary"
	"strings"
	"testing"
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
	if _, linked := claudeParentIn(keptEntries(entries)); linked {
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
