package cli

import (
	"os"
	"slices"
	"strings"
	"testing"
)

const (
	vscodeSettings  = ".vscode/settings.json"
	agentsMdOff     = `"chat.useAgentsMdFile": false`
	copilotBothNote = "note: copilot: .vscode/settings.json keeps chat.useAgentsMdFile on"
)

func hasNote(notes []string, want string) bool {
	return slices.ContainsFunc(notes, func(n string) bool { return strings.Contains(n, want) })
}

func copilotBudgetLine(t *testing.T) string {
	t.Helper()
	out, _ := runCLI(t, "lint")
	for _, line := range findingLines(out, "LINT011") {
		if strings.Contains(line, "copilot loads") {
			return line
		}
	}
	return ""
}

// VS Code reads both .github/copilot-instructions.md and the root
// AGENTS.md, which carry the same body, so sync turns AGENTS.md off in
// the workspace settings and VS Code loads the text once (#2247).
func TestSync_TurnsOffVSCodeAGENTSMdWhenCopilotSharesTheRootFile(t *testing.T) {
	budgetProject(t, "targets: [copilot, codex]\nlint:\n  instructions-words: 100\n")
	mustWriteFile(t, ".agnostic-ai/AGNOSTIC_AI.md", words(80)+"\n")

	notes := syncNotes(t)
	if got := readFile(t, vscodeSettings); !strings.Contains(got, agentsMdOff) {
		t.Errorf("%s lacks %s:\n%s", vscodeSettings, agentsMdOff, got)
	}
	if hasNote(notes, "note: copilot:") {
		t.Errorf("VS Code loads the text once, so no note:\n%s", strings.Join(notes, "\n"))
	}
	if line := copilotBudgetLine(t); line != "" {
		t.Errorf("VS Code no longer reads AGENTS.md, so copilot stays under budget:\n%s", line)
	}
	runSyncOK(t, "--check")
}

// The user's own value wins: sync leaves it, notes the double load, and
// lint counts both files.
func TestSync_KeepsTheUsersVSCodeAGENTSMdSetting(t *testing.T) {
	budgetProject(t, "targets: [copilot, codex]\nlint:\n  instructions-words: 100\n")
	mustWriteFile(t, ".agnostic-ai/AGNOSTIC_AI.md", words(80)+"\n")
	mustWriteFile(t, vscodeSettings, "{\n  \"chat.useAgentsMdFile\": true\n}\n")

	notes := syncNotes(t)
	if got := readFile(t, vscodeSettings); !strings.Contains(got, `"chat.useAgentsMdFile": true`) {
		t.Errorf("sync replaced the user's value:\n%s", got)
	}
	if !hasNote(notes, copilotBothNote) {
		t.Errorf("no note %q in:\n%s", copilotBothNote, strings.Join(notes, "\n"))
	}
	if line := copilotBudgetLine(t); !strings.Contains(line, "AGENTS.md ") {
		t.Errorf("copilot's line lacks the AGENTS.md VS Code loads:\n%s", line)
	}
}

// Without another target writing AGENTS.md, nothing is turned off.
func TestSync_LeavesVSCodeSettingsAloneWithoutARootAGENTSMd(t *testing.T) {
	for name, config := range map[string]string{
		"copilot alone":       "targets: [copilot]\n",
		"unmanaged AGENTS.md": "targets: [copilot, codex]\nsync:\n  unmanaged: [AGENTS.md]\n",
	} {
		t.Run(name, func(t *testing.T) {
			budgetProject(t, config)
			mustWriteFile(t, ".agnostic-ai/AGNOSTIC_AI.md", words(20)+"\n")

			notes := syncNotes(t)
			if _, err := os.Stat(vscodeSettings); err == nil {
				t.Errorf("%s written:\n%s", vscodeSettings, readFile(t, vscodeSettings))
			}
			if hasNote(notes, "note: copilot:") {
				t.Errorf("unexpected note:\n%s", strings.Join(notes, "\n"))
			}
		})
	}
}

// Dropping the AGENTS.md target releases the key sync set.
func TestSync_RemovingTheAGENTSMdTargetReleasesTheVSCodeSetting(t *testing.T) {
	budgetProject(t, "targets: [copilot, codex]\n")
	mustWriteFile(t, ".agnostic-ai/AGNOSTIC_AI.md", words(20)+"\n")
	mustWriteFile(t, vscodeSettings, "{\n  \"editor.tabSize\": 2\n}\n")
	runSyncOK(t)

	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [copilot]\n")
	runSyncOK(t)
	got := readFile(t, vscodeSettings)
	if strings.Contains(got, "chat.useAgentsMdFile") || !strings.Contains(got, "editor.tabSize") {
		t.Errorf("want only the user's key left:\n%s", got)
	}
}

// A legacy rules file at Copilot's entry point is still read next to
// AGENTS.md, so the setting goes in too.
func TestSync_TurnsOffVSCodeAGENTSMdWithALegacyRulesFile(t *testing.T) {
	budgetProject(t, "targets: [copilot, codex]\noutputs:\n  copilot:\n    rules-file: .github/copilot-instructions.md\n")
	mustWriteFile(t, ".agnostic-ai/AGNOSTIC_AI.md", words(20)+"\n")

	runSyncOK(t)
	if got := readFile(t, vscodeSettings); !strings.Contains(got, agentsMdOff) {
		t.Errorf("%s lacks %s:\n%s", vscodeSettings, agentsMdOff, got)
	}
}
