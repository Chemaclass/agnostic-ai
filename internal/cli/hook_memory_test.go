package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func runHookMemory(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"hook", "memory"}, args...))
	if err := root.Execute(); err != nil {
		t.Fatalf("hook memory: %v\n%s", err, out.String())
	}
	return out.String()
}

func memoryHookProject(t *testing.T, index string) {
	t.Helper()
	testutil.TempCwd(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\nbuiltins: [memory]\n")
	if index != "" {
		writeFile(t, filepath.Join(".agnostic-ai", "memory", "MEMORY.md"), index)
	}
}

const sampleIndex = "- [CI is Ubuntu only](ci-ubuntu.md): PR CI runs on Ubuntu alone\n"

func TestHookMemory_PrintsTheIndexAsPlainTextForCodex(t *testing.T) {
	memoryHookProject(t, sampleIndex)

	got := runHookMemory(t, "--target", "codex")
	if !strings.Contains(got, "CI is Ubuntu only") || !strings.Contains(got, ".agnostic-ai/memory/") {
		t.Errorf("got %q", got)
	}
	if strings.HasPrefix(strings.TrimSpace(got), "{") {
		t.Errorf("codex gets plain text, got JSON: %q", got)
	}
}

func TestHookMemory_ReadsTheTargetFromTheHookEnvironment(t *testing.T) {
	memoryHookProject(t, sampleIndex)
	t.Setenv("AGNOSTIC_AI_TARGET", "cursor")

	var reply struct {
		AdditionalContext string `json:"additional_context"`
	}
	if err := json.Unmarshal([]byte(runHookMemory(t)), &reply); err != nil || !strings.Contains(reply.AdditionalContext, "CI is Ubuntu only") {
		t.Errorf("cursor reply = %+v, err %v", reply, err)
	}
}

func TestHookMemory_RepliesWithAdditionalContextForCopilot(t *testing.T) {
	memoryHookProject(t, "- [Quote \"x\" and \\ path](q.md): café\n")

	out := runHookMemory(t, "--target", "copilot")
	var reply struct {
		AdditionalContext string `json:"additionalContext"`
	}
	if err := json.Unmarshal([]byte(out), &reply); err != nil {
		t.Fatalf("invalid JSON %q: %v", out, err)
	}
	if !strings.Contains(reply.AdditionalContext, `Quote "x" and \ path`) || !strings.Contains(reply.AdditionalContext, "café") {
		t.Errorf("context lost text: %q", reply.AdditionalContext)
	}
}

func TestHookMemory_PrintsNothingWithoutAStore(t *testing.T) {
	for _, target := range []string{"codex", "cursor", "copilot", "qoder", "factory"} {
		memoryHookProject(t, "")
		if got := runHookMemory(t, "--target", target); got != "" {
			t.Errorf("%s: got %q, want nothing", target, got)
		}
	}
}

func TestHookMemory_PrintsNothingOutsideAProject(t *testing.T) {
	testutil.TempCwd(t)

	if got := runHookMemory(t, "--target", "codex"); got != "" {
		t.Errorf("got %q, want nothing", got)
	}
}

func TestHookMemory_CutsALongIndexAtAWholeLine(t *testing.T) {
	line := "- [Fact](fact.md): " + strings.Repeat("x", 80) + "\n"
	memoryHookProject(t, strings.Repeat(line, 500))

	got := runHookMemory(t, "--target", "codex")
	if len(got) > memoryContextLimit {
		t.Errorf("output is %d characters, over %d", len(got), memoryContextLimit)
	}
	if !strings.Contains(got, "index continues in .agnostic-ai/memory/MEMORY.md") {
		t.Errorf("no note naming the cut index:\n%s", got[len(got)-200:])
	}
	for _, l := range strings.Split(strings.TrimSpace(got), "\n") {
		if strings.HasPrefix(l, "- [Fact]") && l != strings.TrimSuffix(line, "\n") {
			t.Fatalf("a line was cut in half: %q", l)
		}
	}
}
