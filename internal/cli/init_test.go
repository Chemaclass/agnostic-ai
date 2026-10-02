package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
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

// Git drops empty folders, and default source paths are noise, so the
// default scaffold writes neither (#1331).
func TestScaffold_DefaultBaseDir(t *testing.T) {
	dir := t.TempDir()
	if err := scaffold(scaffoldOptions{Root: dir, Base: "", Targets: allTargetNames()}); err != nil {
		t.Fatal(err)
	}
	for _, d := range scaffoldKinds {
		if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", d)); !os.IsNotExist(err) {
			t.Errorf(".agnostic-ai/%s should not exist before a spec needs it, stat err = %v", d, err)
		}
	}
	cfg, err := os.ReadFile(filepath.Join(dir, "agnostic-ai.yaml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(cfg), "sources:") {
		t.Errorf("config should leave default sources out:\n%s", cfg)
	}
	if !strings.Contains(string(cfg), "\n# Set to error to fail sync when a target cannot represent a spec.\non-unsupported: warn\n") {
		t.Errorf("config should scaffold on-unsupported: warn with a comment naming error:\n%s", cfg)
	}
	loaded, err := config.Load(dir)
	if err != nil {
		t.Fatalf("load scaffolded config: %v", err)
	}
	if loaded.Sources.Agents != ".agnostic-ai/agents" {
		t.Errorf("sources.agents = %q, want the default", loaded.Sources.Agents)
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
	if _, err := os.Stat(filepath.Join(dir, "agnostic-ai.yaml")); err != nil {
		t.Errorf("expected agnostic-ai.yaml, got %v", err)
	}
	root = NewRootCmd("test")
	root.SetArgs([]string{"new", "rule", "tone"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "rules", "tone.md")); err != nil {
		t.Errorf("new should create .agnostic-ai/rules/ on first use, got %v", err)
	}
}

func TestInitCmd_PinsSchemaToTheRunningRelease(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("0.71.0")
	root.SetArgs([]string{"init", "--all"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	cfg, err := os.ReadFile(filepath.Join(dir, "agnostic-ai.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cfg), "/agnostic-ai/v0.71.0/docs/schemas/config.schema.json") {
		t.Errorf("schema URL should name v0.71.0:\n%s", cfg)
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
	for _, want := range []string{"agnostic-ai.local.yaml", ".agnostic-ai/.sync-state", ".agnostic-ai/local/"} {
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

func TestScaffold_Demo_SeedsExampleSpecs(t *testing.T) {
	dir := t.TempDir()
	if err := scaffold(scaffoldOptions{Root: dir, Base: "", Demo: true, Targets: allTargetNames()}); err != nil {
		t.Fatal(err)
	}
	wantFiles := map[string]string{
		"agents/code-reviewer.md":       "name: code-reviewer",
		"skills/yaml-validator.md":      "name: yaml-validator",
		"skills/memory-curator.md":      "name: memory-curator",
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

func TestScaffold_Demo_CreatesOnlySeededFolders(t *testing.T) {
	dir := t.TempDir()
	if err := scaffold(scaffoldOptions{Root: dir, Base: "", Demo: true, Targets: allTargetNames()}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"commands", "settings", "reviews", "environments", "ignore"} {
		if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", kind)); !os.IsNotExist(err) {
			t.Errorf(".agnostic-ai/%s holds no demo spec and should not exist, stat err = %v", kind, err)
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
	got := renderConfig("", allTargetNames(), false, "test")
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
	got := renderConfig("", []string{"claude", "codex"}, false, "test")
	if !strings.Contains(got, "  - claude\n") || !strings.Contains(got, "  - codex\n") {
		t.Errorf("missing chosen targets:\n%s", got)
	}
	if strings.Contains(got, "  - gemini\n") {
		t.Errorf("gemini should be absent:\n%s", got)
	}
}

// The schema a config validates against is the one of the release that
// wrote it; a dev build has no tag, so it follows main.
func TestRenderConfig_PinsSchemaToTheRelease(t *testing.T) {
	cases := map[string]string{"0.71.0": "v0.71.0", "v0.71.0": "v0.71.0", "test": "main", "0.72.0-SNAPSHOT-abc123": "main", "": "main"}
	for version, ref := range cases {
		got := renderConfig("", []string{"claude"}, false, version)
		want := "# yaml-language-server: $schema=https://raw.githubusercontent.com/Chemaclass/agnostic-ai/" + ref + "/docs/schemas/config.schema.json\n"
		if !strings.HasPrefix(got, want) {
			t.Errorf("version %q: renderConfig should start with %q, got:\n%s", version, want, got)
		}
	}
}

func TestRenderConfig_GitignoreDisabledOmitsBlock(t *testing.T) {
	got := renderConfig("", []string{"claude"}, false, "test")
	if strings.Contains(got, "gitignore:") {
		t.Errorf("expected no gitignore block when disabled, got:\n%s", got)
	}
}

func TestRenderConfig_GitignoreEnabledWritesBlock(t *testing.T) {
	got := renderConfig("", []string{"claude"}, true, "test")
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

func TestInitCmd_NonTTY_NoPromptDefaultsEnabled(t *testing.T) {
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
	if !strings.Contains(string(cfg), "gitignore:\n  enabled: true\n") {
		t.Errorf("non-TTY init must default to the managed gitignore block:\n%s", cfg)
	}
}

func TestInitCmd_NonTTY_SaysWhichGitignoreChoiceItTook(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)

	var stderr strings.Builder
	root := NewRootCmd("test")
	root.SetIn(strings.NewReader("claude\n"))
	root.SetErr(&stderr)
	root.SetArgs([]string{"init"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(stderr.String(), "git-ignored, so each clone needs agnostic-ai sync (pass --gitignore=off") {
		t.Errorf("init without a terminal must say it ignored generated files and how to commit them:\n%s", stderr.String())
	}
}

func TestInitCmd_GitignoreDefaultNote(t *testing.T) {
	cases := map[string]struct {
		args []string
		want bool
	}{
		"--all":     {[]string{"init", "--all"}, true},
		"--quiet":   {[]string{"init", "--quiet"}, false},
		"--dry-run": {[]string{"init", "--dry-run"}, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			silence(t)

			var stderr strings.Builder
			root := NewRootCmd("test")
			root.SetIn(strings.NewReader("claude\n"))
			root.SetErr(&stderr)
			root.SetArgs(tc.args)
			if err := root.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if got := strings.Contains(stderr.String(), gitignoreDefaultNote); got != tc.want {
				t.Errorf("note printed = %v, want %v:\n%s", got, tc.want, stderr.String())
			}
		})
	}
}

func TestInitCmd_NoGitignoreNoteWhenInitFails(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")

	var stderr strings.Builder
	root := NewRootCmd("test")
	root.SetIn(strings.NewReader("claude\n"))
	root.SetErr(&stderr)
	root.SetArgs([]string{"init"})
	if err := root.Execute(); err == nil {
		t.Fatal("init over an existing project must fail")
	}
	if strings.Contains(stderr.String(), gitignoreDefaultNote) {
		t.Errorf("a failed init must not claim a gitignore setting:\n%s", stderr.String())
	}
}

func TestInitCmd_ExplicitGitignoreSaysNothing(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)

	var stderr strings.Builder
	root := NewRootCmd("test")
	root.SetIn(strings.NewReader("claude\n"))
	root.SetErr(&stderr)
	root.SetArgs([]string{"init", "--gitignore=off"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if strings.Contains(stderr.String(), "git-ignored") {
		t.Errorf("an explicit --gitignore needs no note:\n%s", stderr.String())
	}
}

func TestInitCmd_GitignoreOptOut(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"init", "--all", "--gitignore=false"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	cfg, err := os.ReadFile(filepath.Join(dir, "agnostic-ai.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(cfg), "gitignore:") {
		t.Errorf("--gitignore=false must commit generated outputs (no gitignore block):\n%s", cfg)
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

func TestInitCmd_PipedEmptyFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetIn(strings.NewReader("\n"))
	root.SetArgs([]string{"init"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got, want := configuredTargets(t, dir), config.DefaultTargets(); !slices.Equal(got, want) {
		t.Errorf("empty piped line must fall back to the default targets\ngot  %v\nwant %v", got, want)
	}
}

// devNullStdin opens the null device as an *os.File so init sees what a
// CI job or `init < /dev/null` gives it: stdin that is neither a
// terminal nor a pipe with data.
func devNullStdin(t *testing.T) *os.File {
	t.Helper()
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

// configuredTargets returns the targets list init wrote to the config.
func configuredTargets(t *testing.T, dir string) []string {
	t.Helper()
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return cfg.Targets
}

func TestInitCmd_NoTTYNoPipe_EmptyRepoEnablesDefaultTargets(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	stderr := &bytes.Buffer{}
	root := NewRootCmd("test")
	root.SetIn(devNullStdin(t))
	root.SetErr(stderr)
	root.SetArgs([]string{"init", "--demo"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := configuredTargets(t, dir)
	if want := config.DefaultTargets(); !slices.Equal(got, want) {
		t.Errorf("non-interactive init must enable the default targets, not all\ngot  %v\nwant %v", got, want)
	}
	for _, colliding := range []string{"amp", "warp"} {
		if slices.Contains(got, colliding) {
			t.Errorf("%s collides with codex on AGENTS.md and must not be enabled by default: %v", colliding, got)
		}
	}
	msg := stderr.String()
	for _, want := range []string{
		"no target list piped; enabled 20 default targets: claude, codex,",
		`(pass --all, or pipe "claude,codex")`,
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("stderr missing %q:\n%s", want, msg)
		}
	}
}

func TestInitCmd_NoTTYNoPipe_EnablesDetectedTargets(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	for _, marker := range []string{".claude", ".cursor"} {
		if err := os.MkdirAll(filepath.Join(dir, marker), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	stderr := &bytes.Buffer{}
	root := NewRootCmd("test")
	root.SetIn(devNullStdin(t))
	root.SetErr(stderr)
	root.SetArgs([]string{"init"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got, want := configuredTargets(t, dir), []string{"claude", "cursor"}; !slices.Equal(got, want) {
		t.Errorf("non-interactive init must enable the detected targets\ngot  %v\nwant %v", got, want)
	}
	want := "no target list piped; enabled 2 detected targets: claude, cursor"
	if !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr missing %q:\n%s", want, stderr.String())
	}
	if strings.Contains(stderr.String(), "AGENTS.md") {
		t.Errorf("no root AGENTS.md, so no codex hint:\n%s", stderr.String())
	}
}

func TestInitCmd_NoTTYNoPipe_SuggestsCodexForRootAgentsMainFile(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "AGENTS.md"), "# Repository Guidelines\n")

	stderr := &bytes.Buffer{}
	root := NewRootCmd("test")
	root.SetIn(devNullStdin(t))
	root.SetErr(stderr)
	root.SetArgs([]string{"init"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got, want := configuredTargets(t, dir), []string{"claude"}; !slices.Equal(got, want) {
		t.Errorf("the hint must not change the targets\ngot  %v\nwant %v", got, want)
	}
	want := `AGENTS.md exists; enable codex so sync manages it (pipe "claude,codex")`
	if !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr missing %q:\n%s", want, stderr.String())
	}
}

func TestInitCmd_AllFlagEnablesEveryTarget(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	stderr := &bytes.Buffer{}
	root := NewRootCmd("test")
	root.SetIn(devNullStdin(t))
	root.SetErr(stderr)
	root.SetArgs([]string{"init", "--all"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got, want := configuredTargets(t, dir), allTargetNames(); !slices.Equal(got, want) {
		t.Errorf("--all must enable every supported target\ngot  %v\nwant %v", got, want)
	}
	if strings.Contains(stderr.String(), "no target list piped") {
		t.Errorf("--all is an explicit choice and must not print the fallback notice:\n%s", stderr.String())
	}
}

func TestInitCmd_PipedListWinsOverDetectedTargets(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	if err := os.MkdirAll(filepath.Join(dir, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}

	root := NewRootCmd("test")
	root.SetIn(strings.NewReader("claude,codex\n"))
	root.SetArgs([]string{"init"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got, want := configuredTargets(t, dir), []string{"claude", "codex"}; !slices.Equal(got, want) {
		t.Errorf("a piped list must win over detection\ngot  %v\nwant %v", got, want)
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
		"agnostic-ai new rule <name>",
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
	demoIdx := strings.Index(out, "seeded example specs")
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

func TestScaffold_NextStepsPointAtCompletion(t *testing.T) {
	dir := t.TempDir()
	buf := captureSummary(t)
	if err := scaffold(scaffoldOptions{Root: dir, Base: "", Targets: allTargetNames()}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "agnostic-ai completion <shell>") {
		t.Errorf("next steps should point at shell completion:\n%s", buf.String())
	}
}

// --quiet means errors only, so the non-interactive fallback notice goes
// quiet with it while the choice itself is unchanged.
func TestFallbackInitTargets_QuietPrintsNothing(t *testing.T) {
	prev := verbosity
	verbosity = levelQuiet
	t.Cleanup(func() { verbosity = prev })
	var buf bytes.Buffer
	got := fallbackInitTargets(&buf, nil)
	if buf.Len() != 0 {
		t.Errorf("--quiet must print nothing, got %q", buf.String())
	}
	if len(got) != len(config.DefaultTargets()) {
		t.Errorf("quiet must not change the choice: got %v", got)
	}
}

func TestFallbackInitTargets_SingularForOneTarget(t *testing.T) {
	var buf bytes.Buffer
	fallbackInitTargets(&buf, []string{"claude"})
	if want := "enabled 1 detected target: claude"; !strings.Contains(buf.String(), want) {
		t.Errorf("want %q, got %q", want, buf.String())
	}
}
