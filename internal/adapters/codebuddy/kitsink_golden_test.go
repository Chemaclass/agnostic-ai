package codebuddy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func kitSinkBundle() spec.Bundle {
	return spec.NewBundle([]spec.Entry{
		{Kind: spec.KindRule, Name: "conventions", Path: "rules/conventions.md", Body: "Check all errors."},
		{Kind: spec.KindRule, Name: "go", Path: "rules/go.md", Meta: map[string]any{"globs": "**/*.go"}, Body: "Use Go conventions."},
		{Kind: spec.KindAgent, Name: "reviewer", Path: "agents/reviewer.md", Meta: map[string]any{"description": "Review changes", "tools": []any{"Read", "Grep"}, "model": "lite", "effort": "high", "maxTurns": 4, "background": true, "mcpServers": []any{"docs"}, "x-codebuddy": map[string]any{"disallowedTools": []any{"Bash"}, "memory": "project", "initialPrompt": "Read the diff", "disable-model-invocation": true}}, Body: "Review code."},
		{Kind: spec.KindSkill, Name: "review", Path: "skills/review/SKILL.md", Meta: map[string]any{"description": "Review a diff", "allowed-tools": []any{"Read", "Bash(git:*)", "Edit(src/**/*.go)"}, "disable-model-invocation": true, "model": "lite", "x-codebuddy": map[string]any{"context": "fork", "user-invocable": true, "agent": "reviewer"}}, Body: "Read references and review."},
		{Kind: spec.KindCommand, Name: "check", Path: "commands/check.md", Meta: map[string]any{"description": "Check changes", "argument-hint": "[files]", "allowed-tools": []any{"Read"}, "model": "lite"}, Body: "Check $ARGUMENTS."},
		{Kind: spec.KindSettings, Name: "defaults", Path: "settings/defaults.yaml", Meta: map[string]any{"model": "lite", "effort": "high", "permissions": map[string]any{"default-mode": "plan", "deny": []any{"Bash(rm *)"}}, "x-codebuddy": map[string]any{"nativeControl": true}}},
		{Kind: spec.KindHook, Name: "guard", Path: "hooks/guard.yaml", Meta: map[string]any{"event": "PreToolUse", "matcher": "Bash", "command": "echo", "args": []any{"check changes"}, "timeout": 5}},
		{Kind: spec.KindHook, Name: "stop-check", Path: "hooks/stop-check.yaml", Meta: map[string]any{"event": "Stop", "type": "prompt", "prompt": "Check tests pass: $ARGUMENTS", "timeout": 30, "continueOnBlock": true}},
		{Kind: spec.KindMCP, Name: "local", Path: "mcps/local.yaml", Meta: map[string]any{"command": "node", "cwd": "backend", "description": "Local documentation", "args": []any{"server.js"}, "env": map[string]any{"MODE": "test"}, "x-codebuddy": map[string]any{"defer_loading": true}}},
		{Kind: spec.KindMCP, Name: "docs", Path: "mcps/docs.yaml", Meta: map[string]any{"type": "http", "url": "https://docs.example.com/mcp", "description": "Remote documentation", "headers": map[string]any{"Authorization": "Bearer ${DOCS_TOKEN}"}}},
		{Kind: spec.KindMCP, Name: "events", Path: "mcps/events.yaml", Meta: map[string]any{"type": "sse", "url": "https://docs.example.com/sse"}},
	})
}
func TestKitSink_GoldenSnapshot(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(cwd, "testdata/kitsink")
	dir := testutil.TempCwd(t)
	cfg := &config.Config{Outputs: map[string]config.Output{"codebuddy": {RootMCPFile: ".mcp.json"}}}
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), cfg, false); err != nil {
		t.Fatal(err)
	}
	testutil.AssertGoldenTree(t, dir, want)
}
func TestKitSink_DeclaredKindsEmitContent(t *testing.T) {
	for _, kind := range caps.Supports {
		t.Run(string(kind), func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			b := spec.NewBundle(kitSinkBundle().All())
			var entries []spec.Entry
			for _, e := range b.All() {
				if e.Kind == kind {
					entries = append(entries, e)
				}
			}
			sess := emit.NewSession()
			sess.StartCapture()
			if err := New().Emit(sess, spec.NewBundle(entries), &config.Config{Outputs: map[string]config.Output{"codebuddy": {RootMCPFile: ".mcp.json"}}}, false); err != nil {
				t.Fatal(err)
			}
			files := sess.StopCapture()
			if len(files) == 0 {
				t.Fatalf("%s claims support without an output", kind)
			}
			for _, f := range files {
				if f.Content == "" {
					t.Errorf("empty %s", f.Path)
				}
			}
		})
	}
}
