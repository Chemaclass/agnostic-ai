package cli

import (
	"slices"
	"strings"
	"testing"
)

// VS Code applies .github/copilot-instructions.md and, with
// chat.useAgentsMdFile on by default, the root AGENTS.md too, so a
// Copilot session there loads both files (#2247).
func TestLintBudget_CopilotCountsTheRootAGENTSMdVSCodeAlsoLoads(t *testing.T) {
	budgetProject(t, "targets: [copilot, codex]\nlint:\n  instructions-words: 100\n")
	mustWriteFile(t, ".agnostic-ai/AGNOSTIC_AI.md", words(80)+"\n")

	out, _ := runCLI(t, "lint")
	var copilot string
	for _, line := range findingLines(out, "LINT011") {
		if strings.Contains(line, "copilot loads") {
			copilot = line
		}
	}
	if copilot == "" {
		t.Fatalf("copilot loads both 80-word files, so it passes the 100-word budget:\n%s", out)
	}
	if !strings.Contains(copilot, "AGENTS.md ") {
		t.Errorf("copilot's line lacks the AGENTS.md VS Code loads:\n%s", copilot)
	}
}

func TestLintBudget_CopilotAloneCountsOneFile(t *testing.T) {
	budgetProject(t, "targets: [copilot]\nlint:\n  instructions-words: 100\n")
	mustWriteFile(t, ".agnostic-ai/AGNOSTIC_AI.md", words(80)+"\n")

	out, _ := runCLI(t, "lint")
	if lines := findingLines(out, "LINT011"); len(lines) != 0 {
		t.Errorf("no AGENTS.md is synced, so copilot stays under budget:\n%s", out)
	}
}

func TestSync_NotesThatVSCodeLoadsCopilotInstructionsAndAGENTSMd(t *testing.T) {
	budgetProject(t, "targets: [copilot, codex]\n")
	mustWriteFile(t, ".agnostic-ai/AGNOSTIC_AI.md", words(20)+"\n")

	notes := syncNotes(t)
	want := "note: copilot: VS Code chat, the cloud agent, and GitHub.com code review load both .github/copilot-instructions.md and AGENTS.md"
	if !slices.ContainsFunc(notes, func(n string) bool { return strings.Contains(n, want) }) {
		t.Errorf("no note %q in:\n%s", want, strings.Join(notes, "\n"))
	}
}

func TestLintBudget_CopilotWithALegacyRulesFileStillCountsAGENTSMd(t *testing.T) {
	budgetProject(t, "targets: [copilot, codex]\noutputs:\n  copilot:\n    rules-file: .github/copilot-instructions.md\nlint:\n  instructions-words: 50\n")
	mustWriteFile(t, ".agnostic-ai/AGNOSTIC_AI.md", words(80)+"\n")

	notes := syncNotes(t)
	if !slices.ContainsFunc(notes, func(n string) bool { return strings.Contains(n, "note: copilot: VS Code chat") }) {
		t.Errorf("lint counts AGENTS.md for copilot, so sync notes it:\n%s", strings.Join(notes, "\n"))
	}
	out, _ := runCLI(t, "lint")
	if !slices.ContainsFunc(findingLines(out, "LINT011"), func(l string) bool { return strings.Contains(l, "copilot loads") && strings.Contains(l, "AGENTS.md ") }) {
		t.Errorf("copilot's line lacks AGENTS.md:\n%s", out)
	}
}

func TestSync_NoCopilotNoteForAnUnmanagedAGENTSMd(t *testing.T) {
	budgetProject(t, "targets: [copilot, codex]\nunmanaged: [AGENTS.md]\n")
	mustWriteFile(t, ".agnostic-ai/AGNOSTIC_AI.md", words(20)+"\n")

	notes := syncNotes(t)
	if slices.ContainsFunc(notes, func(n string) bool { return strings.Contains(n, "note: copilot: VS Code chat") }) {
		t.Errorf("sync does not write an unmanaged AGENTS.md:\n%s", strings.Join(notes, "\n"))
	}
}
