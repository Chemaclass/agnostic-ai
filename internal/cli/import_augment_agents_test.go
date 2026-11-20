package cli

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportAugment_AgentThinkingBecomesEffort(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [augment]\n")
	writeFile(t, ".augment/agents/rev.md", "---\nname: rev\ndescription: r\nmodel: sonnet\nthinking: high\ncolor: blue\nmax_turns: 5\n---\n\nbody\n")

	if _, err := runCLI(t, "import", "augment"); err != nil {
		t.Fatalf("import: %v", err)
	}
	got := readFile(t, ".agnostic-ai/agents/rev.md")
	if !strings.Contains(got, "effort: high") || strings.Contains(got, "thinking") {
		t.Errorf("thinking should become effort:\n%s", got)
	}
	if !strings.Contains(got, "x-augment:") || !strings.Contains(got, "max_turns: 5") {
		t.Errorf("native keys should move under x-augment:\n%s", got)
	}
	runSyncOK(t)
	if out := readFile(t, ".augment/agents/rev.md"); !strings.Contains(out, "thinking: high") || !strings.Contains(out, "max_turns: 5") {
		t.Errorf("sync did not write the agent back:\n%s", out)
	}
	if _, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check: %v", err)
	}
}

func TestImportAugment_AgentThinkingOutsideLevelsStaysNative(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [augment]\n")
	writeFile(t, ".augment/agents/rev.md", "---\nname: rev\ndescription: r\nthinking: ultra\n---\n\nbody\n")

	if _, err := runCLI(t, "import", "augment"); err != nil {
		t.Fatalf("import: %v", err)
	}
	got := readFile(t, ".agnostic-ai/agents/rev.md")
	if strings.Contains(got, "effort:") || !strings.Contains(got, "x-augment:") || !strings.Contains(got, "thinking: ultra") {
		t.Errorf("unknown thinking should stay under x-augment:\n%s", got)
	}
}
