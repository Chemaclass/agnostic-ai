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
	path := filepath.Join(dir, "hooks", "posttooluse-1-1.yaml")
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
	for _, name := range []string{"posttooluse-1-1.yaml", "posttooluse-1-2.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, "hooks", name)); err != nil {
			t.Errorf("missing %s", name)
		}
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
	got := splitH2Sections(in)
	if len(got) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(got))
	}
	if got[0].slug != "go-style" || got[1].slug != "go-style-2" {
		t.Errorf("expected go-style, go-style-2; got %s, %s", got[0].slug, got[1].slug)
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
		t.Error("expected error when agnostic.config.yaml missing")
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
	writeFile(t, filepath.Join(dir, "agnostic.config.yaml"), cfg)
}
