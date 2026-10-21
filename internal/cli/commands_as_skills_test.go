package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const reviewCommand = "---\nname: review\ndescription: Review the diff.\nargument-hint: \"[path]\"\n---\n\nReview $ARGUMENTS.\n"

func commandsAsSkillsProject(t *testing.T, targets string) *bytes.Buffer {
	t.Helper()
	testutil.TempCwd(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+targets+"]\n")
	writeAgnosticFile(t, "# Shared\n")
	writeFile(t, filepath.Join(".agnostic-ai", "commands", "review.md"), reviewCommand)
	var notes bytes.Buffer
	adapters.ResetCoverageNotes()
	adapters.SetWarner(&notes)
	t.Cleanup(func() { adapters.ResetCoverageNotes(); adapters.SetWarner(os.Stderr) })
	return &notes
}

func TestSync_WritesACommandAsAUserRunSkillOnATargetWithoutCommands(t *testing.T) {
	notes := commandsAsSkillsProject(t, "claude, codex, copilot, gemini")

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	for _, native := range []string{filepath.Join(".claude", "commands", "review.md"), filepath.Join(".gemini", "commands", "review.toml")} {
		if _, err := os.Stat(native); err != nil {
			t.Errorf("want the native command %s: %v", native, err)
		}
	}
	if _, err := os.Stat(filepath.Join(".claude", "skills", "review")); !os.IsNotExist(err) {
		t.Errorf("claude has commands, so no skill, stat err = %v", err)
	}
	copilot := readFile(t, filepath.Join(".github", "skills", "review", "SKILL.md"))
	for _, want := range []string{"name: review", "description: Review the diff.", "disable-model-invocation: true", "Run it only when the user calls it by name", "Review $ARGUMENTS."} {
		if !strings.Contains(copilot, want) {
			t.Errorf("want %q in the copilot skill:\n%s", want, copilot)
		}
	}
	codex := readFile(t, filepath.Join(".agents", "skills", "review", "SKILL.md"))
	if !strings.Contains(codex, "Review $ARGUMENTS.") {
		t.Errorf("want the command body in the codex skill:\n%s", codex)
	}
	if policy := readFile(t, filepath.Join(".agents", "skills", "review", "agents", "openai.yaml")); !strings.Contains(policy, "allow_implicit_invocation: false") {
		t.Errorf("codex must run the skill only when called:\n%s", policy)
	}
	for _, want := range []string{
		"1 command reaches codex, copilot but not as a command (written as a skill the user runs by name)",
		"gemini also reads .agents/skills, so it lists review both as its own command and as the skill written there for codex",
	} {
		if !strings.Contains(notes.String(), want) {
			t.Errorf("want note %q, got:\n%s", want, notes.String())
		}
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check: %v\n%s", err, out)
	}

	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex, gemini]\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync without copilot: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(".github", "skills", "review")); !os.IsNotExist(err) {
		t.Errorf("turning copilot off must remove its skill, stat err = %v", err)
	}
}

func TestSync_KeepsTheCommandFolderOnCodexWithACommandsDir(t *testing.T) {
	commandsAsSkillsProject(t, "codex")
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\noutputs:\n  codex:\n    commands-dir: .codex/prompts\n")

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(".codex", "prompts", "review.md")); err != nil {
		t.Errorf("want the custom prompt: %v", err)
	}
	if _, err := os.Stat(filepath.Join(".agents", "skills", "review")); !os.IsNotExist(err) {
		t.Errorf("no skill beside the custom prompt, stat err = %v", err)
	}
}

func TestSync_ASkillSpecOfTheSameNameWinsOverACommand(t *testing.T) {
	notes := commandsAsSkillsProject(t, "copilot")
	writeFile(t, filepath.Join(".agnostic-ai", "skills", "review", "SKILL.md"), "---\nname: review\ndescription: Review skill.\n---\n\nThe skill body.\n")

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	got := readFile(t, filepath.Join(".github", "skills", "review", "SKILL.md"))
	if !strings.Contains(got, "The skill body.") || strings.Contains(got, "Review $ARGUMENTS.") {
		t.Errorf("the skill spec must win:\n%s", got)
	}
	if want := "1 command reaches copilot but not as a skill (the skill spec of the same name takes its folder: review)"; !strings.Contains(notes.String(), want) {
		t.Errorf("want note %q, got:\n%s", want, notes.String())
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check: %v\n%s", err, out)
	}
}

// Aider writes skills only into its always-loaded rules file.
func TestSync_LeavesCommandsOffAider(t *testing.T) {
	commandsAsSkillsProject(t, "aider")
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [aider]\noutputs:\n  aider:\n    rules-file: AIDER.md\n")
	writeFile(t, filepath.Join(".agnostic-ai", "skills", "ship", "SKILL.md"), "---\nname: ship\ndescription: Ships.\n---\n\nShip it.\n")

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if got := readFile(t, "AIDER.md"); strings.Contains(got, "Review $ARGUMENTS.") {
		t.Errorf("a command must not load in every aider session:\n%s", got)
	}
}

func TestImport_LeavesACommandWrittenAsASkillToTheCommandSpec(t *testing.T) {
	commandsAsSkillsProject(t, "codex")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}

	importCapturing(t, "codex")

	if _, err := os.Stat(filepath.Join(".agnostic-ai", "skills", "review")); !os.IsNotExist(err) {
		t.Errorf("import must not copy the command skill back, stat err = %v", err)
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check after import: %v\n%s", err, out)
	}
}

func TestLint_CountsCommandsWrittenAsSkillsAsConsumed(t *testing.T) {
	commandsAsSkillsProject(t, "copilot")

	out, _ := runCLI(t, "lint")
	if strings.Contains(out, "LINT004") {
		t.Errorf("copilot writes the command as a skill, got:\n%s", out)
	}
}
