package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportCopilotHooks_RoundTripsHandlerForms(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [copilot]\n")
	const native = `{"version":1,"hooks":{"sessionStart":[
{"type":"prompt","prompt":"Read project notes."},
{"type":"http","url":"https://example.test/check","headers":{"Authorization":"Bearer $TOKEN"},"allowedEnvVars":["TOKEN"],"timeoutSec":20}
],"PreToolUse":[{"type":"command","matcher":"Bash","exec":"./scripts/check","args":["--strict"],"timeoutSec":10}]}}`
	writeFile(t, filepath.Join(copilotHooksDir, "custom.json"), native)
	execCLI(t, "import", "copilot")
	execCLI(t, "sync", "-t", "copilot")

	data := readFile(t, filepath.Join(copilotHooksDir, "agnostic-ai.json"))
	var got struct {
		Hooks map[string][]map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(data), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Hooks["sessionStart"]) != 2 || len(got.Hooks["PreToolUse"]) != 1 {
		t.Fatalf("round-trip handlers = %#v", got.Hooks)
	}
	byType := map[string]map[string]any{}
	for _, handler := range got.Hooks["sessionStart"] {
		kind, _ := handler["type"].(string)
		byType[kind] = handler
	}
	prompt, http := byType["prompt"], byType["http"]
	if prompt["type"] != "prompt" || prompt["prompt"] != "Read project notes." {
		t.Errorf("prompt = %#v", prompt)
	}
	if http["type"] != "http" || http["url"] != "https://example.test/check" || http["timeoutSec"] != float64(20) {
		t.Errorf("http = %#v", http)
	}
	command := got.Hooks["PreToolUse"][0]
	if command["exec"] != "./scripts/check" || command["timeoutSec"] != float64(10) {
		t.Errorf("command = %#v", command)
	}
}

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

func TestImportFromCopilot_ImportsProjectModel(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".github", "copilot", "settings.json"), `{
  "model": "gpt-5.4",
  "respectGitignore": true
}`)
	if err := importFromCopilot(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "settings", "imported.yaml"))
	if !strings.Contains(got, "model: gpt-5.4") {
		t.Errorf("project model not imported:\n%s", got)
	}
}

// Native agent profiles and skill folders round-trip into the agents
// and skills sources; the .agent infix drops from the filename.
func TestImportFromCopilot_NativeAgentsAndSkills(t *testing.T) {
	dir := t.TempDir()
	agentBody := "---\nname: reviewer\ndescription: Review diffs.\n---\nReview diffs.\n"
	writeFile(t, filepath.Join(dir, copilotAgentsDir, "reviewer.agent.md"), agentBody)
	writeFile(t, filepath.Join(dir, copilotSkillsDirs[0], "greet", "SKILL.md"), "---\nname: greet\n---\nhi\n")
	writeFile(t, filepath.Join(dir, copilotSkillsDirs[0], "greet", "helper.sh"), "echo hi\n")

	if err := importFromCopilot(dir, rootSources()); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "agents", "reviewer.md"))
	if err != nil {
		t.Fatalf("missing agents/reviewer.md: %v", err)
	}
	if string(got) != agentBody {
		t.Errorf("agent not byte-identical. got %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "skills", "greet", "helper.sh")); err != nil {
		t.Errorf("skill folder should import with assets: %v", err)
	}
}

// The vendor documents three project skill directories, not one. A repo
// on the shared `.agents/skills` layout used to import zero skills (#854).
func TestImportFromCopilot_ImportsEveryProjectSkillPathWithPrecedence(t *testing.T) {
	dir := t.TempDir()
	for _, path := range copilotSkillsDirs {
		name := strings.TrimPrefix(filepath.Dir(path), ".")
		writeFile(t, filepath.Join(dir, path, name, "SKILL.md"),
			"---\nname: "+name+"\n---\n\n"+name+" body\n")
		writeFile(t, filepath.Join(dir, path, "shared", "SKILL.md"),
			"---\nname: shared\n---\n\nfrom "+name+"\n")
	}

	if err := importFromCopilot(dir, rootSources()); err != nil {
		t.Fatal(err)
	}

	for _, path := range copilotSkillsDirs {
		name := strings.TrimPrefix(filepath.Dir(path), ".")
		got := readFile(t, filepath.Join(dir, "skills", name, "SKILL.md"))
		if !strings.Contains(got, name+" body") {
			t.Errorf("%s skill not imported:\n%s", path, got)
		}
	}
	shared := readFile(t, filepath.Join(dir, "skills", "shared", "SKILL.md"))
	if !strings.Contains(shared, "from github") {
		t.Errorf(".github/skills should win a same-name collision:\n%s", shared)
	}
}
