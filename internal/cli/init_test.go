package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestScaffold_DefaultBaseDir(t *testing.T) {
	dir := t.TempDir()
	if err := scaffold(dir, "", false, allTargetNames()); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"agents", "skills", "rules", "hooks", "mcps"} {
		if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", d)); err != nil {
			t.Errorf("missing .agnostic-ai/%s", d)
		}
	}
	cfg, err := os.ReadFile(filepath.Join(dir, "agnostic.config.yaml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(cfg), "agents: .agnostic-ai/agents") {
		t.Errorf("config missing nested agents path:\n%s", cfg)
	}
}

func TestScaffold_CustomBaseDir(t *testing.T) {
	dir := t.TempDir()
	if err := scaffold(dir, "specs", false, allTargetNames()); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"agents", "skills", "rules", "hooks", "mcps"} {
		if _, err := os.Stat(filepath.Join(dir, "specs", d)); err != nil {
			t.Errorf("missing specs/%s", d)
		}
	}
	cfg, err := os.ReadFile(filepath.Join(dir, "agnostic.config.yaml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(cfg), "agents: specs/agents") {
		t.Errorf("config missing custom path:\n%s", cfg)
	}
}

func TestScaffold_BaseDirDot_WritesAtRoot(t *testing.T) {
	dir := t.TempDir()
	if err := scaffold(dir, ".", false, allTargetNames()); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"agents", "skills", "rules", "hooks", "mcps"} {
		if _, err := os.Stat(filepath.Join(dir, d)); err != nil {
			t.Errorf("missing %s at root", d)
		}
	}
	cfg, err := os.ReadFile(filepath.Join(dir, "agnostic.config.yaml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(cfg), "agents: agents\n") {
		t.Errorf("config should use bare paths when base is '.':\n%s", cfg)
	}
}

func TestScaffold_NestedBaseDir(t *testing.T) {
	dir := t.TempDir()
	if err := scaffold(dir, filepath.Join("config", "ai"), false, allTargetNames()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config", "ai", "agents")); err != nil {
		t.Errorf("missing config/ai/agents")
	}
	cfg, err := os.ReadFile(filepath.Join(dir, "agnostic.config.yaml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(cfg), "agents: config/ai/agents") {
		t.Errorf("config missing nested base path:\n%s", cfg)
	}
}

func TestInitCmd_PositionalDirArg(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"init", "specs"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "specs", "agents")); err != nil {
		t.Errorf("expected specs/agents/ from positional dir arg, got %v", err)
	}
}

func TestInitCmd_DefaultsToAgnosticAi(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"init"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "agents")); err != nil {
		t.Errorf("expected default .agnostic-ai/agents/, got %v", err)
	}
}

func TestInitCmd_RejectsExtraArgs(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"init", "specs", "extra"})
	if err := root.Execute(); err == nil {
		t.Error("expected error for too many positional args")
	}
}

func TestScaffold_Demo_SeedsOneFilePerKind(t *testing.T) {
	dir := t.TempDir()
	if err := scaffold(dir, "", true, allTargetNames()); err != nil {
		t.Fatal(err)
	}
	wantFiles := map[string]string{
		"agents/code-reviewer.md":       "name: code-reviewer",
		"skills/yaml-validator.md":      "name: yaml-validator",
		"rules/conventional-commits.md": "name: conventional-commits",
		"hooks/format-on-save.yaml":     "event: PostToolUse",
		"mcps/filesystem.yaml":          "command: npx",
	}
	for rel, want := range wantFiles {
		path := filepath.Join(dir, ".agnostic-ai", rel)
		got, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("expected demo file %s: %v", rel, err)
			continue
		}
		if !strings.Contains(string(got), want) {
			t.Errorf("%s missing %q in:\n%s", rel, want, got)
		}
	}
}

func TestScaffold_Demo_DoesNotOverwriteExistingFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".agnostic-ai", "rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	custom := filepath.Join(dir, ".agnostic-ai", "rules", "conventional-commits.md")
	if err := os.WriteFile(custom, []byte("user content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := scaffold(dir, "", true, allTargetNames()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(custom)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "user content" {
		t.Errorf("demo overwrote existing file: %q", got)
	}
}

func TestScaffold_NoDemo_LeavesFoldersEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := scaffold(dir, "", false, allTargetNames()); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"agents", "skills", "rules", "hooks", "mcps"} {
		entries, err := os.ReadDir(filepath.Join(dir, ".agnostic-ai", kind))
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Errorf("expected empty %s/, got %d entries", kind, len(entries))
		}
	}
}

func TestInitCmd_DemoFlag(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"init", "--demo"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "agents", "code-reviewer.md")); err != nil {
		t.Errorf("expected demo agent file from --demo flag, got %v", err)
	}
}

func TestScaffold_RefusesIfConfigExists(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agnostic.config.yaml"),
		[]byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := scaffold(dir, "", false, allTargetNames()); err == nil {
		t.Error("expected error when config already exists")
	}
}

func TestRenderConfig_DefaultTargetsListAllThirteen(t *testing.T) {
	got := renderConfig("", allTargetNames())
	for _, name := range []string{
		"claude", "codex", "gemini", "cursor", "copilot",
		"aider", "cline", "windsurf", "continue", "amp",
		"zed", "warp", "opencode",
	} {
		if !strings.Contains(got, "  - "+name+"\n") {
			t.Errorf("renderConfig missing %q in:\n%s", name, got)
		}
	}
	if count := strings.Count(got, "\n  - "); count != len(allTargets) {
		t.Errorf("expected %d targets in output, got %d", len(allTargets), count)
	}
}

func TestRenderConfig_TrimmedTargetsList(t *testing.T) {
	got := renderConfig("", []string{"claude", "codex"})
	if !strings.Contains(got, "  - claude\n") || !strings.Contains(got, "  - codex\n") {
		t.Errorf("missing chosen targets:\n%s", got)
	}
	if strings.Contains(got, "  - gemini\n") {
		t.Errorf("gemini should be absent:\n%s", got)
	}
}

func TestInitCmd_Interactive_PipedSelection(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetIn(strings.NewReader("claude,codex\n"))
	root.SetArgs([]string{"init", "-i"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	cfg, err := os.ReadFile(filepath.Join(dir, "agnostic.config.yaml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	got := string(cfg)
	for _, want := range []string{"  - claude\n", "  - codex\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("config missing %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"  - gemini\n", "  - cursor\n", "  - opencode\n"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("config should not contain %q:\n%s", unwanted, got)
		}
	}
}

func TestInitCmd_Interactive_PipedWithDir(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetIn(strings.NewReader("claude\n"))
	root.SetArgs([]string{"init", "-i", "specs"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "specs", "agents")); err != nil {
		t.Errorf("expected specs/agents/, got %v", err)
	}
	cfg, err := os.ReadFile(filepath.Join(dir, "agnostic.config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cfg), "agents: specs/agents") {
		t.Errorf("config missing custom path:\n%s", cfg)
	}
	if !strings.Contains(string(cfg), "  - claude\n") {
		t.Errorf("config missing claude:\n%s", cfg)
	}
}

func TestInitCmd_Interactive_PipedWithDemo(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetIn(strings.NewReader("claude\n"))
	root.SetArgs([]string{"init", "-i", "--demo"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "agents", "code-reviewer.md")); err != nil {
		t.Errorf("expected demo agent file, got %v", err)
	}
	cfg, _ := os.ReadFile(filepath.Join(dir, "agnostic.config.yaml"))
	if !strings.Contains(string(cfg), "  - claude\n") {
		t.Errorf("config missing claude:\n%s", cfg)
	}
}

func TestInitCmd_Interactive_PipedEmptyErrors(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetIn(strings.NewReader("\n"))
	root.SetArgs([]string{"init", "-i"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected error for empty piped selection")
	}
	if _, err := os.Stat(filepath.Join(dir, "agnostic.config.yaml")); err == nil {
		t.Error("agnostic.config.yaml should not be written on empty selection")
	}
}

func TestInitCmd_Interactive_PipedUnknownTarget(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetIn(strings.NewReader("claude,fnord\n"))
	root.SetArgs([]string{"init", "-i"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for unknown target")
	}
	if !strings.Contains(err.Error(), "fnord") {
		t.Errorf("error should mention 'fnord', got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "agnostic.config.yaml")); statErr == nil {
		t.Error("agnostic.config.yaml should not be written on validation error")
	}
}
