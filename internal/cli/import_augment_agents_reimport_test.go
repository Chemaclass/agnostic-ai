package cli

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportAugment_ReimportKeepsExplicitThinkingOverride(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [augment]\n")
	mustWriteFile(t, ".agnostic-ai/agents/rev.md", "---\nname: rev\ndescription: r\neffort: high\nx-augment:\n  thinking: low\n---\n\nbody\n")
	runSyncOK(t)
	mustWriteFile(t, ".augment/agents/rev.md", "---\nname: rev\ndescription: r\nthinking: low\n---\n\nedited body\n")

	if _, err := runCLI(t, "import", "augment"); err != nil {
		t.Fatalf("import: %v", err)
	}
	got := readFile(t, ".agnostic-ai/agents/rev.md")
	if !strings.Contains(got, "effort: high") || !strings.Contains(got, "thinking: low") {
		t.Errorf("effort and the x-augment override must both survive:\n%s", got)
	}
}

func TestImportAugment_DeletingThinkingRemovesEffortButKeepsABudget(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [augment]\n")
	mustWriteFile(t, ".agnostic-ai/agents/lvl.md", "---\nname: lvl\ndescription: r\neffort: high\n---\n\nbody\n")
	mustWriteFile(t, ".agnostic-ai/agents/budget.md", "---\nname: budget\ndescription: r\neffort: 8000\n---\n\nbody\n")
	runSyncOK(t)
	mustWriteFile(t, ".augment/agents/lvl.md", "---\nname: lvl\ndescription: r\n---\n\nbody\n")

	if _, err := runCLI(t, "import", "augment"); err != nil {
		t.Fatalf("import: %v", err)
	}
	if got := readFile(t, ".agnostic-ai/agents/lvl.md"); strings.Contains(got, "effort:") {
		t.Errorf("deleting thinking should remove effort:\n%s", got)
	}
	if got := readFile(t, ".agnostic-ai/agents/budget.md"); !strings.Contains(got, "effort: 8000") {
		t.Errorf("an effort Augment cannot write must survive:\n%s", got)
	}
	runSyncOK(t)
	if _, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check: %v", err)
	}
}
