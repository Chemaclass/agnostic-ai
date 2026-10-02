package cli

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// A second import used to replace what the first captured, so following
// doctor's `import codex` advice dropped the CLAUDE.md conventions from
// every tool (#1595).
func TestImport_ASecondImportMergesIntoTheInstructions(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, "CLAUDE.md", "# Acme API\n\n## Tooling\n\nUse pnpm.\n")
	mustWriteFile(t, "AGENTS.md", "# Acme API agent guide\n\n## Reviews\n\nKeep PRs small.\n")
	log := captureLog(t)

	for _, source := range []string{"claude", "codex"} {
		if out, err := runCLI(t, "import", source); err != nil {
			t.Fatalf("import %s: %v\n%s", source, err, out)
		}
	}

	got := readFile(t, ".agnostic-ai/AGNOSTIC_AI.md")
	for _, want := range []string{"Use pnpm.", "Keep PRs small."} {
		if !strings.Contains(got, want) {
			t.Errorf("AGNOSTIC_AI.md lacks %q:\n%s", want, got)
		}
	}
	if !strings.Contains(log.String(), "from AGENTS.md into .agnostic-ai/AGNOSTIC_AI.md") {
		t.Errorf("import codex did not say what it merged:\n%s", log.String())
	}
}

// The placeholder sync seeds is not instructions: an import replaces it.
func TestImport_ReplacesTheSeededPlaceholder(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	runSyncOK(t)
	mustWriteFile(t, "CLAUDE.md", "# Acme API\n\nUse pnpm.\n")

	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}

	if got := readFile(t, ".agnostic-ai/AGNOSTIC_AI.md"); strings.Contains(got, "Replace this comment") || !strings.Contains(got, "Use pnpm.") {
		t.Errorf("AGNOSTIC_AI.md = %q, want the imported text alone", got)
	}
}

// Import does not add the tool to targets, so doctor's remedy for a tool
// outside targets says to add it first, and the next step names it.
func TestDoctor_NamesTheAdoptStepForUnmanagedConfig(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, "AGENTS.md", "# Hand written\n")
	runSyncOK(t)

	out, _ := runCLI(t, "doctor")

	if !strings.Contains(out, "add codex to targets in agnostic-ai.yaml, then agnostic-ai import codex") {
		t.Errorf("doctor remedy does not enable codex:\n%s", out)
	}
	if strings.Contains(out, "All checks passed") {
		t.Errorf("doctor passed while listing unmanaged config:\n%s", out)
	}
}

// An edited section comes back as a second copy beside the first, which
// stays; import names it so the user keeps one.
func TestImport_ReimportingAnEditedSectionNamesBothVersions(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, "CLAUDE.md", "# Acme API\n\n## Tooling\n\nUse npm.\n")
	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	mustWriteFile(t, "CLAUDE.md", "# Acme API\n\n## Tooling\n\nUse pnpm.\n")
	log := captureLog(t)

	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}

	if !strings.Contains(log.String(), `now holds two versions of "Tooling"`) {
		t.Errorf("import did not name the second version:\n%s", log.String())
	}
}

// amp writes the root AGENTS.md too, so `import codex` alone adopts it.
func TestDoctor_AdoptStepSkipsAddingCodexWhenAnotherTargetWritesAgentsMd(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, amp]\n")
	runSyncOK(t)
	mustWriteFile(t, "AGENTS.md", "# Hand written\n")

	out, _ := runCLI(t, "doctor")

	if strings.Contains(out, "add codex to targets") {
		t.Errorf("doctor asks to add codex though amp writes AGENTS.md:\n%s", out)
	}
}

// A code example that quotes a section is not that section.
func TestImport_ASectionQuotedInACodeExampleIsStillMerged(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, "CLAUDE.md", "# Acme API\n\n## Docs\n\nAn example file:\n\n```md\n## Deploy\n\nRun make deploy.\n```\n")
	mustWriteFile(t, "AGENTS.md", "# Acme API\n\n## Deploy\n\nRun make deploy.\n")
	for _, source := range []string{"claude", "codex"} {
		if out, err := runCLI(t, "import", source); err != nil {
			t.Fatalf("import %s: %v\n%s", source, err, out)
		}
	}

	if got := readFile(t, ".agnostic-ai/AGNOSTIC_AI.md"); strings.Count(got, "## Deploy") != 2 {
		t.Errorf("the Deploy section was not merged:\n%s", got)
	}
}
