package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportFromCopilot_NoSources(t *testing.T) {
	dir := t.TempDir()
	if err := importFromCopilot(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, agnosticMainFile)); !os.IsNotExist(err) {
		t.Errorf("expected no AGNOSTIC_AI.md when copilot-instructions absent: %v", err)
	}
}

func TestImportFromCopilot_MirrorsMainFile(t *testing.T) {
	dir := t.TempDir()
	body := "# Copilot\n\n## rule-a\n\nbody.\n"
	writeFile(t, filepath.Join(dir, copilotMainFile), body)
	if err := importFromCopilot(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, agnosticMainFile))
	if err != nil {
		t.Fatalf("missing %s: %v", agnosticMainFile, err)
	}
	if string(got) != body {
		t.Errorf("AGNOSTIC_AI.md not byte-identical. got %q", got)
	}
}

func TestImportFromCopilot_PrefersInstructionsDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, copilotMainFile), "## sliced\n\nfrom main.\n")
	writeFile(t, filepath.Join(dir, copilotInstructionsDir, "go-style.instructions.md"),
		"---\napplyTo: \"**/*.go\"\n---\n\ngofmt clean.\n")

	if err := importFromCopilot(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "rules", "go-style.md"))
	if err != nil {
		t.Fatalf("missing rules/go-style.md: %v", err)
	}
	out := string(got)
	for _, want := range []string{"name: go-style", "globs: '**/*.go'", "gofmt clean."} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "rules", "sliced.md")); !os.IsNotExist(err) {
		t.Errorf("main file should not be sliced when instructions dir exists: %v", err)
	}
}

func TestImportFromCopilot_DropsCatchAllApplyTo(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, copilotInstructionsDir, "rule-a.instructions.md"),
		"---\napplyTo: \"**\"\n---\n\nbody.\n")
	if err := importFromCopilot(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "rules", "rule-a.md"))
	if strings.Contains(string(got), "globs:") {
		t.Errorf("catch-all ** should not become globs:\n%s", got)
	}
	if strings.Contains(string(got), "applyTo") {
		t.Errorf("applyTo should be translated, got:\n%s", got)
	}
}

func TestImportFromCopilot_ChatmodesBecomeAgents(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, copilotChatmodesDir, "reviewer.chatmode.md"),
		"---\ndescription: Review diffs\nmodel: gpt-4\n---\n\nReview diffs.\n")
	if err := importFromCopilot(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "agents", "reviewer.md"))
	if err != nil {
		t.Fatalf("missing agents/reviewer.md: %v", err)
	}
	for _, want := range []string{"name: reviewer", "description: Review diffs", "Review diffs."} {
		if !strings.Contains(string(got), want) {
			t.Errorf("expected %q in agent file:\n%s", want, got)
		}
	}
}

func TestImportFromCopilot_AgentAndSkillPrefixesRoute(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, copilotInstructionsDir, "agent-reviewer.instructions.md"),
		"---\napplyTo: \"**\"\n---\n\nReview diffs.\n")
	writeFile(t, filepath.Join(dir, copilotInstructionsDir, "skill-yaml-validator.instructions.md"),
		"---\napplyTo: \"**\"\n---\n\nValidate yaml.\n")
	writeFile(t, filepath.Join(dir, copilotInstructionsDir, "go-style.instructions.md"),
		"---\napplyTo: \"**/*.go\"\n---\n\ngofmt clean.\n")

	if err := importFromCopilot(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "agents", "reviewer.md")); err != nil {
		t.Errorf("agent-* should land in agents/: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "skills", "yaml-validator.md")); err != nil {
		t.Errorf("skill-* should land in skills/: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "rules", "go-style.md")); err != nil {
		t.Errorf("plain instruction should land in rules/: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "rules", "agent-reviewer.md")); !os.IsNotExist(err) {
		t.Errorf("agent-* should NOT land in rules/: %v", err)
	}
}

func TestImportFromCopilot_ItalicDescriptionMovesToFrontmatter(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, copilotInstructionsDir, "rule-a.instructions.md"),
		"---\napplyTo: \"**\"\n---\n\n_The reason in italics_\n\nBody text.\n")
	if err := importFromCopilot(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "rules", "rule-a.md"))
	if err != nil {
		t.Fatal(err)
	}
	out := string(got)
	if !strings.Contains(out, "description: The reason in italics") {
		t.Errorf("expected description lifted into frontmatter:\n%s", out)
	}
	if strings.Contains(out, "_The reason in italics_") {
		t.Errorf("italic line should be stripped from body:\n%s", out)
	}
}

func TestImportFromCopilot_ImportsMCP(t *testing.T) {
	dir := t.TempDir()
	settings := `{
  "servers": {
    "fs": {"command": "fs-server"}
  }
}`
	writeFile(t, filepath.Join(dir, copilotMCPFile), settings)
	if err := importFromCopilot(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "mcps", "fs.yaml"))
	if err != nil {
		t.Fatalf("missing mcps/fs.yaml: %v", err)
	}
	if !strings.Contains(string(got), "command: fs-server") {
		t.Errorf("expected command in mcp: %s", got)
	}
}
