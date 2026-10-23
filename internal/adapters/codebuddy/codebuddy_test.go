package codebuddy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func TestEmit_NativeFilesAndConditionalRules(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindRule, Name: "go", Body: "Check errors.", Meta: map[string]any{"globs": []string{"**/*.go"}}},
		{Kind: spec.KindAgent, Name: "reviewer", Body: "Review changes.", Meta: map[string]any{"description": "Review", "tools": []any{"Read", "Grep"}, "effort": "high", "maxTurns": 3}},
		{Kind: spec.KindSkill, Name: "review", Body: "Read references.", Meta: map[string]any{"description": "Review code", "allowed-tools": []any{"Read"}, "model": "lite", "x-codebuddy": map[string]any{"context": "fork"}}},
		{Kind: spec.KindCommand, Name: "check", Body: "Check $ARGUMENTS.", Meta: map[string]any{"description": "Check changes", "argument-hint": "[files]"}},
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "PreToolUse", "command": "echo", "args": []any{"a b"}, "timeout": 5}},
		{Kind: spec.KindSettings, Name: "default", Meta: map[string]any{"model": "lite", "effort": "high", "permissions": map[string]any{"default-mode": "plan", "deny": []any{"Bash(rm *)"}}}},
		{Kind: spec.KindMCP, Name: "local", Meta: map[string]any{"command": "node", "args": []any{"server.js"}}},
	})
	cfg := &config.Config{Outputs: map[string]config.Output{"codebuddy": {RootMCPFile: ".mcp.json"}}}
	if err := New().Emit(emit.NewSession(), b, cfg, false); err != nil {
		t.Fatal(err)
	}
	for p, wants := range map[string][]string{
		".codebuddy/rules/go.md":            {"alwaysApply: false", "paths:", "**/*.go", "Check errors."},
		".codebuddy/agents/reviewer.md":     {"tools: Read, Grep", "effort: high", "maxTurns: 3"},
		".codebuddy/skills/review/SKILL.md": {"allowed-tools: Read", "model: lite", "context: fork"},
		".codebuddy/commands/check.md":      {"argument-hint: \"[files]\"", "Check $ARGUMENTS."},
		".codebuddy/settings.json":          {"\"model\": \"lite\"", "\"reasoningEffort\": \"high\"", "\"defaultMode\": \"plan\"", "echo 'a b'"},
		".mcp.json":                         {"\"type\": \"stdio\"", "server.js"},
	} {
		got := read(t, p)
		for _, want := range wants {
			if !strings.Contains(got, want) {
				t.Errorf("%s missing %q: %s", p, want, got)
			}
		}
	}
}
func TestEmit_MCPRequiresRootOptIn(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	b := spec.Bundle{MCPs: []spec.Entry{{Kind: spec.KindMCP, Name: "local", Meta: map[string]any{"command": "node"}}}}
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{".mcp.json", ".codebuddy/mcp.json"} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("unexpected %s: %v", p, err)
		}
	}
}
func TestEmit_PreservesSettingsAndNativeHookEntries(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	if err := os.MkdirAll(".codebuddy", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".codebuddy/settings.json", []byte(`{"nativeControl":true,"hooks":{"Stop":[{"hooks":[{"type":"command","command":"echo native"}]}]}}`), 0644); err != nil {
		t.Fatal(err)
	}
	b := spec.Bundle{Hooks: []spec.Entry{{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "Stop", "command": "echo portable"}}}, Settings: []spec.Entry{{Kind: spec.KindSettings, Meta: map[string]any{"model": "lite"}}}}
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(read(t, ".codebuddy/settings.json")), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["nativeControl"] != true {
		t.Error("native control lost")
	}
	got := read(t, ".codebuddy/settings.json")
	if !strings.Contains(got, "echo native") || !strings.Contains(got, "echo portable") {
		t.Fatal(got)
	}
}
func TestEmit_RejectsDisabledMCPAndProjectAuto(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	cfg := &config.Config{Outputs: map[string]config.Output{"codebuddy": {RootMCPFile: ".mcp.json"}}}
	b := spec.Bundle{MCPs: []spec.Entry{{Kind: spec.KindMCP, Name: "off", Meta: map[string]any{"command": "node", "disabled": true}}}}
	if err := New().Emit(emit.NewSession(), b, cfg, false); err == nil {
		t.Error("disabled server must not become enabled")
	}
	b = spec.Bundle{Settings: []spec.Entry{{Kind: spec.KindSettings, Meta: map[string]any{"permissions": map[string]any{"default-mode": "auto"}}}}}
	if err := New().Emit(emit.NewSession(), b, cfg, false); err == nil {
		t.Error("project auto is ignored by CodeBuddy")
	}
}
func TestEmit_OutputOverridesAndDryRun(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	cfg := &config.Config{Outputs: map[string]config.Output{"codebuddy": {Dir: "native", RulesDir: "rules", AgentsDir: "agents", SkillsDir: "skills", CommandsDir: "commands", ConfFile: "settings.json", RootMCPFile: "mcp.json"}}}
	b := spec.Bundle{Settings: []spec.Entry{{Kind: spec.KindSettings, Meta: map[string]any{"model": "lite"}}}}
	if err := New().Emit(emit.NewSession(), b, cfg, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("settings.json"); !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := New().Emit(emit.NewSession(), b, cfg, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(t, filepath.Join("settings.json")), "lite") {
		t.Fatal("override ignored")
	}
}

func TestEmit_RejectsRestrictionsWithoutNativeMeaning(t *testing.T) {
	for _, tools := range []any{[]any{}, []any{"Read", "Delete"}, []any{"Read", "Bash(ls *)"}, []any{"mcp__docs"}} {
		t.Run("agent", func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			b := spec.Bundle{Agents: []spec.Entry{{Kind: spec.KindAgent, Name: "reviewer", Meta: map[string]any{"tools": tools}}}}
			if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err == nil {
				t.Fatalf("restriction %v broadened", tools)
			}
		})
	}
}
func TestEmit_KeepsNativeAgentRestrictionsAndSkillResources(t *testing.T) {
	dir := testutil.TempCwd(t)
	folder := filepath.Join(dir, "source")
	if err := os.MkdirAll(filepath.Join(folder, "references"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "references/guide.txt"), []byte("Guide.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	skill := spec.Entry{Kind: spec.KindSkill, Name: "review", Path: filepath.Join(folder, "SKILL.md"), Meta: map[string]any{"description": "Review"}}
	if err := New().Emit(emit.NewSession(), spec.Bundle{Skills: []spec.Entry{skill}, Agents: []spec.Entry{{Kind: spec.KindAgent, Name: "reviewer", Meta: map[string]any{"tools": []any{"Read"}, "x-codebuddy": map[string]any{"tools": "Defer(mcp__docs__get)", "disallowedTools": []any{"Bash"}}}}}}, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if got := read(t, ".codebuddy/skills/review/references/guide.txt"); got != "Guide.\n" {
		t.Fatal(got)
	}
	got := read(t, ".codebuddy/agents/reviewer.md")
	if !strings.Contains(got, "Defer(mcp__docs__get)") || !strings.Contains(got, "disallowedTools:") {
		t.Fatal(got)
	}
}
