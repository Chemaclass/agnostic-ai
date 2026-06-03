package zed

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestName(t *testing.T) {
	if got := New().Name(); got != "zed" {
		t.Errorf("Name() = %q, want %q", got, "zed")
	}
}

func TestEmit_WritesDotRules(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
	}
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".rules"))
	for _, want := range []string{"rule body", "<!-- source: rules/r1.md -->"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
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
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
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
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
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
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
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
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
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
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
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
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
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
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
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
	if err := New().Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
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
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".zed/tasks.json")); !os.IsNotExist(err) {
		t.Errorf("expected no tasks file when hooks lack command; err=%v", err)
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
