package zed

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestName(t *testing.T) {
	if got := New().Name(); got != "zed" {
		t.Errorf("Name() = %q, want %q", got, "zed")
	}
}

// Zed retired its rules library in favor of instruction files + skills,
// so the adapter never writes the merged legacy document by default.
// Sync owns the `.rules` entry-point (pointer body with rules inlined),
// so nothing lands here from the adapter side.
func TestEmit_NoDotRulesByDefault(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".rules")); !os.IsNotExist(err) {
		t.Errorf(".rules should not be written by default, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("zed's entry-point is .rules and sync owns it; the adapter writes neither, err=%v", err)
	}
}

// outputs.zed.rules-file restores the legacy merged document for users
// still on the pre-skills Zed layout.
func TestEmit_LegacyRulesFile_WritesConcatenated(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"zed": {RulesFile: ".rules"}},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
		{Kind: spec.KindAgent, Name: "helper", Path: "agents/helper.md", Body: "agent body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".rules"))
	for _, want := range []string{"rule body", "agent body"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// Skills emit natively as one folder per skill under .agents/skills/,
// the project-local path Zed 1.4.2+ scans. The renderer matches the
// codex/amp output byte-for-byte so the shared tree dedupes.
func TestEmit_Skill_WritesNativeSkillFolder(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindSkill,
			Name: "deploy",
			Meta: map[string]any{"description": "Run deployments."},
			Body: "Deploy to production.",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".agents/skills/deploy/SKILL.md"))
	for _, want := range []string{"name: deploy", "description: Run deployments.", "Deploy to production."} {
		if !strings.Contains(got, want) {
			t.Errorf("SKILL.md missing %q:\n%s", want, got)
		}
	}
}

func TestEmit_Skill_SkillsDirOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"zed": {SkillsDir: "custom/skills"}},
	}
	entries := []spec.Entry{{Kind: spec.KindSkill, Name: "deploy", Body: "body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/skills/deploy/SKILL.md")); err != nil {
		t.Errorf("expected custom/skills/deploy/SKILL.md: %v", err)
	}
}

// Stdio MCP emits to .zed/settings.json under context_servers with the
// nested `command: {path, args, env}` shape.
func TestEmit_MCP_StdioWritesContextServers(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{
				"command": "npx",
				"args":    []any{"-y", "@modelcontextprotocol/server-filesystem", "."},
				"env":     map[string]any{"ALLOWED_PATHS": "."},
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".zed/settings.json"))
	for _, want := range []string{
		`"context_servers"`,
		`"fs"`,
		`"command": "npx"`,
		`"@modelcontextprotocol/server-filesystem"`,
		`"env"`,
		`"ALLOWED_PATHS"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	// The stale nested-command schema must not reappear.
	for _, absent := range []string{`"path"`, `"settings"`} {
		if strings.Contains(got, absent) {
			t.Errorf("unexpected stale key %q in %s", absent, got)
		}
	}
}

// HTTP / SSE MCP entries use Zed's native url/headers shape.
func TestEmit_MCP_HTTPWritesNativeURL(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "linear",
			Meta: map[string]any{
				"type":    "http",
				"url":     "https://mcp.linear.app",
				"headers": map[string]any{"Authorization": "Bearer x"},
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".zed/settings.json"))
	for _, want := range []string{
		`"linear"`,
		`"url": "https://mcp.linear.app"`,
		`"headers"`,
		`"Authorization"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	for _, absent := range []string{`"path"`, `"mcp-remote"`, `"settings"`} {
		if strings.Contains(got, absent) {
			t.Errorf("unexpected stale key %q in %s", absent, got)
		}
	}
}

func TestEmit_MCP_PreservesExistingUserKeys(t *testing.T) {
	dir := testutil.TempCwd(t)

	if err := os.MkdirAll(filepath.Join(dir, ".zed"), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{"theme": "Solarized Dark", "buffer_font_size": 14}`
	if err := os.WriteFile(filepath.Join(dir, ".zed/settings.json"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".zed/settings.json"))
	for _, want := range []string{
		`"theme": "Solarized Dark"`,
		`"buffer_font_size": 14`,
		`"context_servers"`,
		`"fs"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestEmit_MCP_FileOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"zed": {MCPFile: "vendor/zed.json"},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "x"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor/zed.json")); err != nil {
		t.Errorf("expected override path written: %v", err)
	}
}

func TestEmit_MCP_NoFileWhenNoEntries(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "x"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".zed/settings.json")); !os.IsNotExist(err) {
		t.Errorf("expected no settings file when no MCP entries, err=%v", err)
	}
}

// Stdio entry without `command` is skipped (no `path` to set).
func TestEmit_MCP_SkipsStdioWithoutCommand(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "bad", Meta: map[string]any{}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".zed/settings.json")); !os.IsNotExist(err) {
		t.Errorf("expected no settings file when entry has no command, err=%v", err)
	}
}

func TestEmit_TasksFileEmitsHooksAsTasks(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook,
			Name: "fmt",
			Meta: map[string]any{
				"event":       "PostToolUse",
				"command":     "gofmt -w .",
				"description": "format Go on save",
			},
		},
		{
			Kind: spec.KindHook,
			Name: "lint",
			Meta: map[string]any{"event": "Stop", "command": "golangci-lint run"},
		},
	}
	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"zed": {TasksFile: ".zed/tasks.json"},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".zed/tasks.json"))
	for _, want := range []string{
		`"label": "fmt — format Go on save"`,
		`"label": "lint"`,
		`"command": "sh"`,
		`"-c"`,
		`"gofmt -w ."`,
		`"golangci-lint run"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestEmit_NoTasksFileNoEmit(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "fmt", Meta: map[string]any{"command": "gofmt"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".zed/tasks.json")); !os.IsNotExist(err) {
		t.Errorf("expected no tasks file; err=%v", err)
	}
}

func TestEmit_TasksSkipsHooksWithoutCommand(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "noop", Meta: map[string]any{"event": "Stop"}},
	}
	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"zed": {TasksFile: ".zed/tasks.json"},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".zed/tasks.json")); !os.IsNotExist(err) {
		t.Errorf("expected no tasks file when hooks lack command; err=%v", err)
	}
}

// zed.dev/docs/tasks documents optional cwd/env/shell/reveal/hide/save/
// allow_concurrent_runs/use_new_terminal/tags/reevaluate_context
// alongside label/command/args. None of them are cross-tool hook
// fields, so they pass through under `x-zed`. See #539.
func TestEmit_TasksEmitsXZedFields(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindHook,
			Name: "fmt",
			Meta: map[string]any{
				"event":   "PostToolUse",
				"command": "gofmt -w .",
				"x-zed": map[string]any{
					"cwd":                   "$ZED_WORKTREE_ROOT",
					"shell":                 "bash",
					"reveal":                "always",
					"allow_concurrent_runs": true,
					"tags":                  []any{"formatting"},
				},
			},
		},
	}
	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"zed": {TasksFile: ".zed/tasks.json"},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".zed/tasks.json"))
	for _, want := range []string{
		`"cwd": "$ZED_WORKTREE_ROOT"`,
		`"shell": "bash"`,
		`"reveal": "always"`,
		`"allow_concurrent_runs": true`,
		`"formatting"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// A hook with no x-zed block must not gain any extra task fields.
func TestEmit_TasksNoXZedFieldsWhenUnset(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "fmt", Meta: map[string]any{"event": "Stop", "command": "gofmt -w ."}},
	}
	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"zed": {TasksFile: ".zed/tasks.json"},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".zed/tasks.json"))
	for _, unwanted := range []string{"cwd", "shell", "reveal"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("unexpected %q with no x-zed block set:\n%s", unwanted, got)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
