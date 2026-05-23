package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// captureSummary redirects summaryf output for the duration of the
// test and returns a buffer the caller can inspect.
func captureSummary(t *testing.T) *bytes.Buffer {
	t.Helper()
	prev := logOut
	buf := &bytes.Buffer{}
	logOut = buf
	t.Cleanup(func() { logOut = prev })
	return buf
}

func TestScaffold_DefaultBaseDir(t *testing.T) {
	dir := t.TempDir()
	if err := scaffold(scaffoldOptions{Root: dir, Base: "", Targets: allTargetNames()}); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"agents", "skills", "rules", "hooks", "mcps", "commands"} {
		if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", d)); err != nil {
			t.Errorf("missing .agnostic-ai/%s", d)
		}
	}
	cfg, err := os.ReadFile(filepath.Join(dir, "agnostic-ai.yaml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(cfg), "agents: .agnostic-ai/agents") {
		t.Errorf("config missing nested agents path:\n%s", cfg)
	}
}

func TestScaffold_CustomBaseDir(t *testing.T) {
	dir := t.TempDir()
	if err := scaffold(scaffoldOptions{Root: dir, Base: "specs", Targets: allTargetNames()}); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"agents", "skills", "rules", "hooks", "mcps", "commands"} {
		if _, err := os.Stat(filepath.Join(dir, "specs", d)); err != nil {
			t.Errorf("missing specs/%s", d)
		}
	}
	cfg, err := os.ReadFile(filepath.Join(dir, "agnostic-ai.yaml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(cfg), "agents: specs/agents") {
		t.Errorf("config missing custom path:\n%s", cfg)
	}
}

func TestScaffold_BaseDirDot_WritesAtRoot(t *testing.T) {
	dir := t.TempDir()
	if err := scaffold(scaffoldOptions{Root: dir, Base: ".", Targets: allTargetNames()}); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"agents", "skills", "rules", "hooks", "mcps", "commands"} {
		if _, err := os.Stat(filepath.Join(dir, d)); err != nil {
			t.Errorf("missing %s at root", d)
		}
	}
	cfg, err := os.ReadFile(filepath.Join(dir, "agnostic-ai.yaml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(cfg), "agents: agents\n") {
		t.Errorf("config should use bare paths when base is '.':\n%s", cfg)
	}
}

func TestScaffold_NestedBaseDir(t *testing.T) {
	dir := t.TempDir()
	if err := scaffold(scaffoldOptions{Root: dir, Base: filepath.Join("config", "ai"), Targets: allTargetNames()}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config", "ai", "agents")); err != nil {
		t.Errorf("missing config/ai/agents")
	}
	cfg, err := os.ReadFile(filepath.Join(dir, "agnostic-ai.yaml"))
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
	root.SetArgs([]string{"init", "--all", "specs"})
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
	root.SetArgs([]string{"init", "--all"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "agents")); err != nil {
		t.Errorf("expected default .agnostic-ai/agents/, got %v", err)
	}
}

func TestScaffold_GitignoreContainsLocalOverrideAndSyncState(t *testing.T) {
	dir := t.TempDir()
	if err := scaffold(scaffoldOptions{Root: dir, Base: "", Targets: allTargetNames()}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	for _, want := range []string{"agnostic-ai.local.yaml", ".agnostic-ai/.sync-state"} {
		if !strings.Contains(string(got), want+"\n") {
			t.Errorf("missing %q in .gitignore:\n%s", want, got)
		}
	}
}

func TestScaffold_GitignoreIdempotent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"),
		[]byte("node_modules/\n.agnostic-ai/.sync-state\nagnostic-ai.local.yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := scaffold(scaffoldOptions{Root: dir, Base: "", Targets: allTargetNames()}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if c := strings.Count(string(got), ".agnostic-ai/.sync-state"); c != 1 {
		t.Errorf("expected one .sync-state line, got %d:\n%s", c, got)
	}
	if c := strings.Count(string(got), "agnostic-ai.local.yaml"); c != 1 {
		t.Errorf("expected one local-override line, got %d:\n%s", c, got)
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
	if err := scaffold(scaffoldOptions{Root: dir, Base: "", Demo: true, Targets: allTargetNames()}); err != nil {
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
	if err := scaffold(scaffoldOptions{Root: dir, Base: "", Demo: true, Targets: allTargetNames()}); err != nil {
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
	if err := scaffold(scaffoldOptions{Root: dir, Base: "", Targets: allTargetNames()}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"agents", "skills", "rules", "hooks", "mcps", "commands"} {
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
	root.SetArgs([]string{"init", "--all", "--demo"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "agents", "code-reviewer.md")); err != nil {
		t.Errorf("expected demo agent file from --demo flag, got %v", err)
	}
}

func TestScaffold_RefusesIfConfigExists(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := scaffold(scaffoldOptions{Root: dir, Base: "", Targets: allTargetNames()}); err == nil {
		t.Error("expected error when config already exists")
	}
}

func TestRenderConfig_DefaultTargetsListAllThirteen(t *testing.T) {
	got := renderConfig("", allTargetNames(), false)
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
	got := renderConfig("", []string{"claude", "codex"}, false)
	if !strings.Contains(got, "  - claude\n") || !strings.Contains(got, "  - codex\n") {
		t.Errorf("missing chosen targets:\n%s", got)
	}
	if strings.Contains(got, "  - gemini\n") {
		t.Errorf("gemini should be absent:\n%s", got)
	}
}

func TestRenderConfig_ContainsSchemaComment(t *testing.T) {
	got := renderConfig("", []string{"claude"}, false)
	want := "# yaml-language-server: $schema=https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/docs/schemas/config.schema.json"
	if !strings.HasPrefix(got, want) {
		t.Errorf("renderConfig output should start with schema comment, got:\n%s", got)
	}
}

func TestRenderConfig_GitignoreDisabledOmitsBlock(t *testing.T) {
	got := renderConfig("", []string{"claude"}, false)
	if strings.Contains(got, "gitignore:") {
		t.Errorf("expected no gitignore block when disabled, got:\n%s", got)
	}
}

func TestRenderConfig_GitignoreEnabledWritesBlock(t *testing.T) {
	got := renderConfig("", []string{"claude"}, true)
	if !strings.Contains(got, "gitignore:\n  enabled: true\n") {
		t.Errorf("expected gitignore block, got:\n%s", got)
	}
}

func TestInitCmd_GitignoreFlagPersistsEnabled(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"init", "--all", "--gitignore"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	cfg, err := os.ReadFile(filepath.Join(dir, "agnostic-ai.yaml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(cfg), "gitignore:\n  enabled: true\n") {
		t.Errorf("expected gitignore enabled block in config:\n%s", cfg)
	}
}

func TestInitCmd_NonTTY_NoPromptDefaultsDisabled(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetIn(strings.NewReader("claude\n"))
	root.SetArgs([]string{"init"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	cfg, err := os.ReadFile(filepath.Join(dir, "agnostic-ai.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(cfg), "gitignore:") {
		t.Errorf("non-TTY init without --gitignore must not write gitignore block:\n%s", cfg)
	}
}

func TestInitCmd_Interactive_PipedSelection(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetIn(strings.NewReader("claude,codex\n"))
	root.SetArgs([]string{"init"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	cfg, err := os.ReadFile(filepath.Join(dir, "agnostic-ai.yaml"))
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
	root.SetArgs([]string{"init", "specs"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "specs", "agents")); err != nil {
		t.Errorf("expected specs/agents/, got %v", err)
	}
	cfg, err := os.ReadFile(filepath.Join(dir, "agnostic-ai.yaml"))
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
	root.SetArgs([]string{"init", "--demo"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "agents", "code-reviewer.md")); err != nil {
		t.Errorf("expected demo agent file, got %v", err)
	}
	cfg, err := os.ReadFile(filepath.Join(dir, "agnostic-ai.yaml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(cfg), "  - claude\n") {
		t.Errorf("config missing claude:\n%s", cfg)
	}
}

func TestInitCmd_PipedEmptyFallsBackToAll(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetIn(strings.NewReader("\n"))
	root.SetArgs([]string{"init"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	cfg, err := os.ReadFile(filepath.Join(dir, "agnostic-ai.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"  - claude\n", "  - codex\n", "  - opencode\n"} {
		if !strings.Contains(string(cfg), want) {
			t.Errorf("config missing %q on fallback:\n%s", want, cfg)
		}
	}
}

func TestInitCmd_Interactive_PipedUnknownTarget(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetIn(strings.NewReader("claude,fnord\n"))
	root.SetArgs([]string{"init"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for unknown target")
	}
	if !strings.Contains(err.Error(), "fnord") {
		t.Errorf("error should mention 'fnord', got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "agnostic-ai.yaml")); statErr == nil {
		t.Error("agnostic-ai.yaml should not be written on validation error")
	}
}

func TestScaffold_PrintsNextStepsGuidance(t *testing.T) {
	dir := t.TempDir()
	buf := captureSummary(t)
	if err := scaffold(scaffoldOptions{Root: dir, Base: "", Targets: allTargetNames()}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"✓ initialized agnostic-ai project at .agnostic-ai/",
		"next steps:",
		"agnostic-ai import <target>",
		"agnostic-ai sync",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestScaffold_PrintsCustomBaseInGuidance(t *testing.T) {
	dir := t.TempDir()
	buf := captureSummary(t)
	if err := scaffold(scaffoldOptions{Root: dir, Base: "specs", Targets: allTargetNames()}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "✓ initialized agnostic-ai project at specs/") {
		t.Errorf("expected custom base label in output:\n%s", buf.String())
	}
}

func TestScaffold_PrintsRootBaseInGuidance(t *testing.T) {
	dir := t.TempDir()
	buf := captureSummary(t)
	if err := scaffold(scaffoldOptions{Root: dir, Base: ".", Targets: allTargetNames()}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "✓ initialized agnostic-ai project at ./") {
		t.Errorf("expected root base label in output:\n%s", buf.String())
	}
}

func TestScaffold_DemoAndPresetLinesPrintBeforeNextSteps(t *testing.T) {
	dir := t.TempDir()
	buf := captureSummary(t)
	if err := scaffold(scaffoldOptions{Root: dir, Base: "", Demo: true, Preset: "go", Targets: allTargetNames()}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	demoIdx := strings.Index(out, "seeded one example spec")
	presetIdx := strings.Index(out, `seeded preset "go"`)
	nextIdx := strings.Index(out, "next steps:")
	if demoIdx < 0 || presetIdx < 0 || nextIdx < 0 {
		t.Fatalf("missing expected lines in output:\n%s", out)
	}
	if demoIdx > nextIdx || presetIdx > nextIdx {
		t.Errorf("demo/preset lines should appear before next steps:\n%s", out)
	}
}

func TestScaffold_EchoesEnabledTargets(t *testing.T) {
	dir := t.TempDir()
	buf := captureSummary(t)
	if err := scaffold(scaffoldOptions{Root: dir, Base: "", Targets: []string{"claude", "codex"}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "  enabled: claude, codex\n") {
		t.Errorf("expected enabled targets line, got:\n%s", buf.String())
	}
}

func TestScaffold_SeededSuggestsSyncNotImport(t *testing.T) {
	dir := t.TempDir()
	buf := captureSummary(t)
	if err := scaffold(scaffoldOptions{Root: dir, Base: "", Demo: true, Targets: allTargetNames()}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "agnostic-ai sync --check") {
		t.Errorf("seeded scaffold should suggest sync --check:\n%s", out)
	}
	if strings.Contains(out, "agnostic-ai import <target>") {
		t.Errorf("seeded scaffold should not show import <target>:\n%s", out)
	}
}

func TestScaffold_DetectsExistingTargetsAndSuggestsImports(t *testing.T) {
	dir := t.TempDir()
	// Drop a marker for the codex CLI; init should surface it as a hint.
	if err := os.MkdirAll(filepath.Join(dir, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	buf := captureSummary(t)
	if err := scaffold(scaffoldOptions{Root: dir, Base: "", Targets: allTargetNames()}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "detected existing config:") {
		t.Errorf("expected detected block:\n%s", out)
	}
	if !strings.Contains(out, "agnostic-ai import codex\n") {
		t.Errorf("expected codex import hint:\n%s", out)
	}
}
