package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportFromGemini_NoSources(t *testing.T) {
	dir := t.TempDir()
	if err := importFromGemini(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, agnosticMainFile)); !os.IsNotExist(err) {
		t.Errorf("expected no AGNOSTIC_AI.md when GEMINI.md absent: %v", err)
	}
}

func TestImportFromGemini_MirrorsRootGeminiMd(t *testing.T) {
	dir := t.TempDir()
	body := "# GEMINI.md\n\n## rule-a\n\nbody.\n"
	writeFile(t, filepath.Join(dir, geminiMainFile), body)
	if err := importFromGemini(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, agnosticMainFile))
	if err != nil {
		t.Fatalf("missing %s: %v", agnosticMainFile, err)
	}
	if string(got) != body {
		t.Errorf("AGNOSTIC_AI.md not byte-identical to GEMINI.md. got %q", got)
	}
}

func TestImportFromGemini_NestedInfersGlobs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, geminiMainFile), "## root-rule\n\nbody.\n")
	writeFile(t, filepath.Join(dir, "src", geminiMainFile), "## src-rule\n\nscoped.\n")
	if err := importFromGemini(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	src, _ := os.ReadFile(filepath.Join(dir, "rules", "src-rule.md"))
	if !strings.Contains(string(src), "globs: src/**") {
		t.Errorf("src rule missing globs: src/**, got:\n%s", src)
	}
	root, _ := os.ReadFile(filepath.Join(dir, "rules", "root-rule.md"))
	if strings.Contains(string(root), "globs:") {
		t.Errorf("root rule should not have globs, got:\n%s", root)
	}
}

func TestImportFromGemini_ImportsCommands(t *testing.T) {
	dir := t.TempDir()
	toml := "description = \"Ship it\"\nprompt = \"\"\"\nrun ./deploy.sh\n\"\"\"\n"
	writeFile(t, filepath.Join(dir, geminiCommandsDir, "deploy.toml"), toml)
	if err := importFromGemini(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "agents", "deploy.md"))
	if err != nil {
		t.Fatalf("missing agents/deploy.md: %v", err)
	}
	out := string(data)
	for _, want := range []string{"name: deploy", "description: Ship it", "run ./deploy.sh"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in agent file:\n%s", want, out)
		}
	}
}

func TestImportFromGemini_ImportsSettings(t *testing.T) {
	dir := t.TempDir()
	settings := `{
  "mcpServers": {
    "fs": {"command": "fs-server"}
  },
  "hooks": {
    "PostToolUse": [
      {"matcher": "Edit", "command": "fmt"}
    ]
  }
}`
	writeFile(t, filepath.Join(dir, geminiSettings), settings)
	if err := importFromGemini(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	mcp, err := os.ReadFile(filepath.Join(dir, "mcps", "fs.yaml"))
	if err != nil {
		t.Fatalf("missing mcps/fs.yaml: %v", err)
	}
	if !strings.Contains(string(mcp), "command: fs-server") {
		t.Errorf("mcp missing command: %s", mcp)
	}
	hookPath := findOneHookFile(t, filepath.Join(dir, "hooks"), "posttooluse")
	hook, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("missing hooks file: %v", err)
	}
	for _, want := range []string{"event: PostToolUse", "matcher: Edit", "command: fmt"} {
		if !strings.Contains(string(hook), want) {
			t.Errorf("expected %q in hook file: %s", want, hook)
		}
	}
}

func TestImportFromGemini_HookWithoutCommandOmitsField(t *testing.T) {
	dir := t.TempDir()
	settings := `{
  "hooks": {
    "PreToolUse": [
      {"matcher": "Edit"}
    ]
  }
}`
	writeFile(t, filepath.Join(dir, geminiSettings), settings)
	if err := importFromGemini(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	hookPath := findOneHookFile(t, filepath.Join(dir, "hooks"), "pretooluse")
	hook, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("missing hooks file: %v", err)
	}
	if strings.Contains(string(hook), "command:") {
		t.Errorf("hook should omit command: when source has none, got: %s", hook)
	}
	if strings.Contains(string(hook), "null") {
		t.Errorf("hook should not contain null command, got: %s", hook)
	}
}

func TestExtractTOMLString_KeyBoundary(t *testing.T) {
	in := `description-extra = "wrong"
description = "right"
`
	if got := extractTOMLString(in, "description"); got != "right" {
		t.Errorf("expected boundary match to skip 'description-extra', got %q", got)
	}
}

// Native .gemini/skills folders round-trip into the skills source with
// their bundled assets.
func TestImportFromGemini_NativeSkillFolders(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".gemini", "skills", "greet", "SKILL.md"), "---\nname: greet\n---\nhi\n")
	writeFile(t, filepath.Join(dir, ".gemini", "skills", "greet", "helper.sh"), "echo hi\n")

	if err := importFromGemini(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "skills", "greet", "SKILL.md")); err != nil {
		t.Errorf("skill should import: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "skills", "greet", "helper.sh")); err != nil {
		t.Errorf("skill assets should import: %v", err)
	}
}
