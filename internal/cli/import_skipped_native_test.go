package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportFromCursor_ReadsMCPFileAndLegacyRules(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".cursor", "mcp.json"), handWrittenCursorMCP)
	writeFile(t, filepath.Join(dir, ".cursorrules"), "Legacy rule: prefer tabs.\n")

	if err := importFromCursor(dir, rootSources()); err != nil {
		t.Fatal(err)
	}

	if got := readFile(t, filepath.Join(dir, "mcps", "linear.yaml")); !strings.Contains(got, "https://mcp.linear.app/sse") {
		t.Errorf("mcps/linear.yaml = %q", got)
	}
	got := readFile(t, filepath.Join(dir, "rules", "cursorrules.md"))
	for _, want := range []string{"alwaysApply: true", "Legacy rule: prefer tabs."} {
		if !strings.Contains(got, want) {
			t.Errorf("rules/cursorrules.md lacks %q:\n%s", want, got)
		}
	}
}

func TestImportFromCopilot_ReadsPromptFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".github", "prompts", "plan.prompt.md"),
		"---\ndescription: Plan a feature\nmode: agent\n---\nPlan the feature step by step.\n")

	if err := importFromCopilot(dir, rootSources()); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, filepath.Join(dir, "commands", "plan.md"))
	for _, want := range []string{"description: Plan a feature", "Plan the feature step by step."} {
		if !strings.Contains(got, want) {
			t.Errorf("commands/plan.md lacks %q:\n%s", want, got)
		}
	}
}

// OpenCode reads the singular agent/ and command/ folders as well; the
// plural one wins a name both hold.
func TestImportFromOpencode_ReadsSingularFolders(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".opencode", "agent", "planner.md"), "---\ndescription: Plans work\nmode: subagent\n---\nPlan the work.\n")
	writeFile(t, filepath.Join(dir, ".opencode", "command", "test.md"), "---\ndescription: Run tests\n---\nRun the test suite.\n")
	writeFile(t, filepath.Join(dir, ".opencode", "command", "lint.md"), "---\ndescription: old\n---\nOld lint.\n")
	writeFile(t, filepath.Join(dir, ".opencode", "commands", "lint.md"), "---\ndescription: new\n---\nNew lint.\n")

	if err := importFromOpencode(dir, rootSources()); err != nil {
		t.Fatal(err)
	}

	if got := readFile(t, filepath.Join(dir, "agents", "planner.md")); !strings.Contains(got, "Plan the work.") {
		t.Errorf("agents/planner.md = %q", got)
	}
	if got := readFile(t, filepath.Join(dir, "commands", "test.md")); !strings.Contains(got, "Run the test suite.") {
		t.Errorf("commands/test.md = %q", got)
	}
	if got := readFile(t, filepath.Join(dir, "commands", "lint.md")); !strings.Contains(got, "New lint.") {
		t.Errorf("commands/lint.md = %q, want the plural folder's copy", got)
	}
}

func TestFindUnmanagedConfig_FlagsNativeFilesImportUsedToSkip(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".cursor", "mcp.json"), handWrittenCursorMCP)
	writeFile(t, filepath.Join(dir, ".cursorrules"), "Legacy rule.\n")
	writeFile(t, filepath.Join(dir, ".github", "prompts", "plan.prompt.md"), "Plan.\n")
	writeFile(t, filepath.Join(dir, ".opencode", "agent", "planner.md"), "Plan.\n")
	writeFile(t, filepath.Join(dir, ".opencode", "command", "test.md"), "Test.\n")

	got, err := findUnmanagedConfig(dir, &config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]string{}
	for _, f := range got {
		byPath[f.Path] = f.Target
	}
	for path, target := range map[string]string{
		".cursor/mcp.json":               "cursor",
		".cursorrules":                   "cursor",
		".github/prompts/plan.prompt.md": "copilot",
		".opencode/agent/planner.md":     "opencode",
		".opencode/command/test.md":      "opencode",
	} {
		if byPath[path] != target {
			t.Errorf("%s target = %q, want %q (found %+v)", path, byPath[path], target, got)
		}
	}
}

// Once import holds every server, the MCP file is not reported.
func TestFindUnmanagedConfig_SkipsAnMCPFileWhoseServersHaveSpecs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".cursor", "mcp.json"), handWrittenCursorMCP)
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "linear.yaml"), "name: linear\nurl: https://mcp.linear.app/sse\n")

	got, err := findUnmanagedConfig(dir, &config.Config{Sources: config.Sources{MCPs: ".agnostic-ai/mcps"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range got {
		if f.Path == ".cursor/mcp.json" {
			t.Errorf("flagged an MCP file whose servers all have specs: %+v", got)
		}
	}
}

// Import then sync left `"enabled": true` out and warned that the spec
// replaced the entry import had just read.
func TestImportThenSync_OpencodeKeepsEnabledMCP(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [opencode]\n")
	writeFile(t, "opencode.json", `{"mcp": {"everything": {"type": "local", "command": ["npx", "server"], "enabled": true}}}`+"\n")
	if out, err := runCLI(t, "import", "opencode"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	var warned bytes.Buffer
	adapters.SetWarner(&warned)
	t.Cleanup(func() { adapters.SetWarner(os.Stderr) })

	runSyncOK(t)

	if got := readFile(t, "opencode.json"); !strings.Contains(got, `"enabled": true`) {
		t.Errorf("opencode.json lost enabled:\n%s", got)
	}
	if strings.Contains(warned.String(), "replaces the one already there") {
		t.Errorf("sync warned about the entry import read: %s", warned.String())
	}
}

// Cursor reads its MCP file as JSONC, so import does too.
func TestImportFromCursor_ReadsMCPFileWithComments(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".cursor", "mcp.json"), "{\n  // team server\n  \"mcpServers\": {\"linear\": {\"url\": \"https://mcp.linear.app/sse\"},},\n}\n")

	if err := importFromCursor(dir, rootSources()); err != nil {
		t.Fatal(err)
	}

	if got := readFile(t, filepath.Join(dir, "mcps", "linear.yaml")); !strings.Contains(got, "https://mcp.linear.app/sse") {
		t.Errorf("mcps/linear.yaml = %q", got)
	}
}

// setUpDuplicatedOriginals imports an OpenCode agent from the singular
// folder and a Copilot prompt file, then syncs, so each original sits
// beside the file sync wrote from its spec.
func setUpDuplicatedOriginals(t *testing.T) string {
	t.Helper()
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [opencode, copilot]\n")
	writeFile(t, ".opencode/agent/planner.md", "---\ndescription: Plans work.\n---\n\nPlan it.\n")
	writeFile(t, ".github/prompts/plan.prompt.md", "---\ndescription: Make a plan.\n---\n\nPlan.\n")
	buf := captureSummary(t)
	execCLI(t, "import", "opencode")
	execCLI(t, "import", "copilot")
	execCLI(t, "sync")
	return buf.String()
}

// After import and sync, OpenCode and Copilot load both the original and
// the generated copy, and a later spec edit reaches only the copy.
func TestImport_NamesOpenCodeAndCopilotOriginalsToDeleteAfterSync(t *testing.T) {
	out := setUpDuplicatedOriginals(t)
	for _, want := range []string{
		"! delete .opencode/agent/planner.md after the next sync: sync writes it to .opencode/agents/planner.md, and OpenCode loads both",
		"! delete .github/prompts/plan.prompt.md after the next sync: sync writes it to .github/skills/plan/SKILL.md, and Copilot loads both",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("import output lacks %q:\n%s", want, out)
		}
	}
}

func TestDoctor_ListsAnOriginalThatDuplicatesAGeneratedFile(t *testing.T) {
	setUpDuplicatedOriginals(t)

	out, _ := runCLI(t, "doctor")

	_, section, ok := strings.Cut(out, "Unmanaged config")
	if !ok {
		t.Fatalf("doctor printed no unmanaged config section:\n%s", out)
	}
	section, _, _ = strings.Cut(section, "\n\n")
	for _, want := range []string{
		"✗ .opencode/agent/planner.md\n    → delete it: sync writes .opencode/agents/planner.md from its spec, and OpenCode loads both",
		"✗ .github/prompts/plan.prompt.md\n    → delete it: sync writes .github/skills/plan/SKILL.md from its spec, and Copilot loads both",
	} {
		if !strings.Contains(section, want) {
			t.Errorf("doctor lacks %q while the original duplicates a generated file:\n%s", want, section)
		}
	}
}

// A spec that never reaches the tool leaves the original as its only copy.
func TestImport_KeepsQuietAboutAnOriginalWhoseSpecSkipsTheTool(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, copilot]\n")
	writeFile(t, ".agnostic-ai/commands/plan.md", "---\ndescription: Make a plan.\ntargets: [claude]\n---\n\nPlan.\n")
	writeFile(t, ".github/prompts/plan.prompt.md", "---\ndescription: Make another plan.\n---\n\nPlan more.\n")
	buf := captureSummary(t)

	execCLI(t, "import", "copilot")

	if !strings.Contains(buf.String(), "imported") || strings.Contains(buf.String(), "delete .github/prompts/plan.prompt.md") {
		t.Errorf("import said to delete the only copy Copilot has:\n%s", buf.String())
	}
}
