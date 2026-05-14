package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// rootSources returns a Sources value pointing at root-level kind dirs,
// matching the importer test fixtures.
func rootSources() config.Sources {
	return config.Sources{
		Agents: "agents",
		Skills: "skills",
		Rules:  "rules",
		Hooks:  "hooks",
		MCPs:   "mcps",
	}
}

func TestImportFromClaude_SplitsRulesByH2(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), `# Project

## conventional-commits

Use feat:, fix:, etc.

## go-style

gofmt clean.
`)
	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"conventional-commits", "go-style"} {
		path := filepath.Join(dir, "rules", name+".md")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("missing %s: %v", path, err)
		}
		if !strings.Contains(string(data), "name: "+name) {
			t.Errorf("expected frontmatter name: %s in %s", name, data)
		}
	}
}

func TestImportFromClaude_MonolithicRules(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), "Just a flat doc with no headings.\n")
	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "rules"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(entries))
	}
}

func TestImportFromClaude_PrefersClaudeRulesDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), "## sliced\n\nFrom CLAUDE.md\n")
	ruleBody := "---\nname: go-style\n---\n\ngofmt clean.\n"
	writeFile(t, filepath.Join(dir, ".claude", "rules", "go-style.md"), ruleBody)
	writeFile(t, filepath.Join(dir, ".claude", "rules", "tests.md"), "tests body\n")

	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "rules", "go-style.md"))
	if err != nil {
		t.Fatalf("missing rules/go-style.md: %v", err)
	}
	if string(got) != ruleBody {
		t.Errorf("rule not byte-identical. got %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "rules", "tests.md")); err != nil {
		t.Errorf("missing rules/tests.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "rules", "sliced.md")); !os.IsNotExist(err) {
		t.Errorf("CLAUDE.md should not have been sliced when .claude/rules exists: %v", err)
	}
}

func TestImportFromClaude_EmptyClaudeRulesDirSkipsSlicing(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), "## sliced\n\nFrom CLAUDE.md\n")
	if err := os.MkdirAll(filepath.Join(dir, ".claude", "rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "rules", "sliced.md")); !os.IsNotExist(err) {
		t.Errorf("empty .claude/rules should still suppress slicing: %v", err)
	}
}

func TestImportFromClaude_WritesagnosticMainFile(t *testing.T) {
	dir := t.TempDir()
	body := "# Project\n\nTop-level instructions.\n\n## conventional-commits\n\nUse feat:.\n"
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), body)
	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, agnosticMainFile))
	if err != nil {
		t.Fatalf("missing %s: %v", agnosticMainFile, err)
	}
	if string(got) != body {
		t.Errorf("AGNOSTIC_AI.md not byte-identical to CLAUDE.md. got %q", got)
	}
}

func TestImportFromClaude_NoagnosticMainFileWhenClaudeMdMissing(t *testing.T) {
	dir := t.TempDir()
	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, agnosticMainFile)); !os.IsNotExist(err) {
		t.Errorf("expected no AGNOSTIC_AI.md when CLAUDE.md absent: %v", err)
	}
}

func TestImportFromClaude_NoClaudeMd(t *testing.T) {
	dir := t.TempDir()
	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "rules"))
	if len(entries) != 0 {
		t.Errorf("expected empty rules/, got %d entries", len(entries))
	}
}

func TestImportFromClaude_CopiesAgents(t *testing.T) {
	dir := t.TempDir()
	body := "---\nname: reviewer\nmodel: sonnet\n---\nReview diffs.\n"
	writeFile(t, filepath.Join(dir, ".claude", "agents", "reviewer.md"), body)
	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "agents", "reviewer.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != body {
		t.Errorf("agent not byte-identical. got %q", got)
	}
}

func TestImportFromClaude_CopiesSkills(t *testing.T) {
	dir := t.TempDir()
	body := "---\nname: validator\n---\nValidate yaml.\n"
	writeFile(t, filepath.Join(dir, ".claude", "skills", "validator", "SKILL.md"), body)
	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "skills", "validator", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != body {
		t.Errorf("skill not byte-identical. got %q", got)
	}
}

func TestImportFromClaude_ImportsHooks(t *testing.T) {
	dir := t.TempDir()
	settings := `{
  "hooks": {
    "PostToolUse": [
      {"matcher": "Edit|Write", "hooks": [{"type": "command", "command": "fmt"}]}
    ]
  }
}`
	writeFile(t, filepath.Join(dir, ".claude", "settings.json"), settings)
	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "hooks", "posttooluse-1.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing %s: %v", path, err)
	}
	for _, want := range []string{"event: PostToolUse", "matcher: Edit|Write", "command: fmt"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("expected %q in %s", want, data)
		}
	}
}

func TestImportFromClaude_MultipleHookCommandsPerGroup(t *testing.T) {
	dir := t.TempDir()
	settings := `{
  "hooks": {
    "PostToolUse": [
      {"matcher": "Edit", "hooks": [
        {"type": "command", "command": "fmt"},
        {"type": "command", "command": "lint"}
      ]}
    ]
  }
}`
	writeFile(t, filepath.Join(dir, ".claude", "settings.json"), settings)
	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "hooks", "posttooluse-1.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"- fmt", "- lint", "matcher: Edit"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("expected %q in %s", want, data)
		}
	}
}

func TestImportFromClaude_WritesSettingsOverlay(t *testing.T) {
	dir := t.TempDir()
	settings := `{
  "statusLine": {"type": "command", "command": "echo status"},
  "enabledPlugins": ["foo", "bar"],
  "hooks": {
    "PostToolUse": [
      {"matcher": "Edit", "hooks": [{"type": "command", "command": "fmt"}]}
    ]
  }
}`
	writeFile(t, filepath.Join(dir, ".claude", "settings.json"), settings)
	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".agnostic-ai/overlays/claude.settings.json"))
	if err != nil {
		t.Fatalf("missing overlay: %v", err)
	}
	for _, want := range []string{"statusLine", "enabledPlugins"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("overlay missing %q: %s", want, raw)
		}
	}
	if strings.Contains(string(raw), "\"hooks\"") {
		t.Errorf("overlay should not carry hooks key: %s", raw)
	}
}

func TestImportFromClaude_OnlyHooks_NoOverlay(t *testing.T) {
	dir := t.TempDir()
	settings := `{
  "hooks": {
    "PostToolUse": [
      {"matcher": "Edit", "hooks": [{"type": "command", "command": "fmt"}]}
    ]
  }
}`
	writeFile(t, filepath.Join(dir, ".claude", "settings.json"), settings)
	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai/overlays/claude.settings.json")); !os.IsNotExist(err) {
		t.Errorf("overlay should not exist when only hooks present: %v", err)
	}
}

func TestImportFromClaude_MalformedSettings(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude", "settings.json"), "{not json")
	if err := importFromClaude(dir, rootSources()); err == nil {
		t.Error("expected parse error on malformed settings.json")
	}
}

func TestImportFromClaude_HonorsCustomSourceDirs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), "## r1\n\nbody\n")
	src := config.Sources{
		Agents: ".agnostic-ai/agents",
		Skills: ".agnostic-ai/skills",
		Rules:  ".agnostic-ai/rules",
		Hooks:  ".agnostic-ai/hooks",
		MCPs:   ".agnostic-ai/mcps",
	}
	if err := importFromClaude(dir, src); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "rules", "r1.md")); err != nil {
		t.Errorf("expected .agnostic-ai/rules/r1.md, got %v", err)
	}
}

func TestSlugify_Collisions(t *testing.T) {
	in := "## Go Style\n\nfoo\n\n## go-style\n\nbar\n"
	_, got := splitH2Sections(in)
	if len(got) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(got))
	}
	if got[0].slug != "go-style" || got[1].slug != "go-style-2" {
		t.Errorf("expected go-style, go-style-2; got %s, %s", got[0].slug, got[1].slug)
	}
}

func TestImportFromClaude_PreservesPreamble(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), `# Project Title

Preamble explaining the project before any H2.

## rule-one

Body one.
`)
	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "rules", "project-title.md"))
	if err != nil {
		t.Fatalf("expected rules/project-title.md (preamble file), got %v", err)
	}
	if !strings.Contains(string(data), "Preamble explaining the project") {
		t.Errorf("preamble lost: %s", data)
	}
	if _, err := os.Stat(filepath.Join(dir, "rules", "rule-one.md")); err != nil {
		t.Errorf("missing rule-one.md: %v", err)
	}
}

func TestImportFromClaude_PreambleFallbackSlug(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), "Bare preamble, no H1.\n\n## rule-one\n\nBody.\n")
	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "rules", "intro.md")); err != nil {
		t.Errorf("expected rules/intro.md, got %v", err)
	}
}

func TestImportFromClaude_IgnoresH2InsideFencedBlocks(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), "## rule-one\n\nBody.\n\n"+
		"```markdown\n## not-a-real-rule\n\nExample inside fence.\n```\n\n"+
		"## rule-two\n\nBody two.\n")
	if err := importFromClaude(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "rules", "not-a-real-rule.md")); !os.IsNotExist(err) {
		t.Errorf("phantom rule from fenced block: %v", err)
	}
	for _, name := range []string{"rule-one", "rule-two"} {
		data, err := os.ReadFile(filepath.Join(dir, "rules", name+".md"))
		if err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
		if strings.Contains(string(data), "not-a-real-rule") && name == "rule-two" {
			t.Errorf("%s leaked fenced heading", name)
		}
	}
	one, _ := os.ReadFile(filepath.Join(dir, "rules", "rule-one.md"))
	if !strings.Contains(string(one), "Example inside fence.") {
		t.Errorf("rule-one missing fenced example body: %s", one)
	}
}

func TestImportCmd_UnknownSourceRejected(t *testing.T) {
	dir := t.TempDir()
	writeMinimalConfig(t, dir, ".agnostic-ai")
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"import", "nonesuch"})
	if err := root.Execute(); err == nil {
		t.Error("expected error for unknown source")
	}
}

func TestImportCmd_RequiresConfig(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"import", "claude"})
	if err := root.Execute(); err == nil {
		t.Error("expected error when agnostic-ai.yaml missing")
	}
}

func TestImportCmd_ClaudeWritesIntoConfiguredSources(t *testing.T) {
	dir := t.TempDir()
	writeMinimalConfig(t, dir, ".agnostic-ai")
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), "## r1\n\nbody\n")
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"import", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "rules", "r1.md")); err != nil {
		t.Errorf("expected .agnostic-ai/rules/r1.md, got %v", err)
	}
}

// writeMinimalConfig drops a config that points sources at base/<kind>.
func writeMinimalConfig(t *testing.T, dir, base string) {
	t.Helper()
	cfg := "version: 1\n"
	if base != "" {
		cfg += "sources:\n"
		for _, k := range []string{"agents", "skills", "rules", "hooks", "mcps"} {
			cfg += "  " + k + ": " + filepath.ToSlash(filepath.Join(base, k)) + "\n"
		}
	}
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), cfg)
}
