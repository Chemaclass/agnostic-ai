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

func TestEmit_NativeMCPFields(t *testing.T) {
	for _, tc := range []struct {
		name, field, value string
		meta               map[string]any
	}{
		{"stdio-cwd", "cwd", "backend", map[string]any{"command": "docs-server", "cwd": "backend"}},
		{"stdio-description", "description", "Local documentation", map[string]any{"command": "docs-server", "description": "Local documentation"}},
		{"http-description", "description", "Remote documentation", map[string]any{"url": "https://example.com/mcp", "description": "Remote documentation"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.TempCwd(t)
			bundle := spec.Bundle{MCPs: []spec.Entry{{Kind: spec.KindMCP, Name: "docs", Meta: tc.meta}}}
			cfg := &config.Config{Outputs: map[string]config.Output{"codebuddy": {RootMCPFile: ".mcp.json"}}}
			if err := New().Emit(emit.NewSession(), bundle, cfg, false); err != nil {
				t.Fatal(err)
			}
			var doc struct {
				Servers map[string]map[string]any `json:"mcpServers"`
			}
			if err := json.Unmarshal([]byte(read(t, ".mcp.json")), &doc); err != nil {
				t.Fatal(err)
			}
			if got := doc.Servers["docs"][tc.field]; got != tc.value {
				t.Errorf("native %s = %v, want %s", tc.field, got, tc.value)
			}
		})
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

func TestEmit_PromptHooksOnDocumentedEvents(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	b := spec.Bundle{Hooks: []spec.Entry{
		{Kind: spec.KindHook, Name: "stop-check", Meta: map[string]any{"event": "Stop", "type": "prompt", "prompt": "Check tests pass: $ARGUMENTS", "timeout": 30, "continueOnBlock": true, "model": "fast"}},
		{Kind: spec.KindHook, Name: "bash-check", Meta: map[string]any{"event": "PreToolUse", "matcher": "Bash", "type": "prompt", "prompt": "Is this safe? $ARGUMENTS"}},
	}}
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Hooks map[string][]struct {
			Matcher string           `json:"matcher"`
			Hooks   []map[string]any `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(read(t, ".codebuddy/settings.json")), &doc); err != nil {
		t.Fatal(err)
	}
	stop := doc.Hooks["Stop"][0].Hooks[0]
	want := map[string]any{"type": "prompt", "prompt": "Check tests pass: $ARGUMENTS", "timeout": float64(30), "continueOnBlock": true}
	if len(stop) != len(want) {
		t.Fatalf("Stop handler = %v, want %v", stop, want)
	}
	for key, value := range want {
		if stop[key] != value {
			t.Errorf("Stop handler %s = %v, want %v", key, stop[key], value)
		}
	}
	pre := doc.Hooks["PreToolUse"][0]
	if pre.Matcher != "Bash" || pre.Hooks[0]["type"] != "prompt" || len(pre.Hooks[0]) != 2 {
		t.Errorf("PreToolUse group = %+v", pre)
	}
}

func TestEmit_RejectsPromptHooksOnOtherEventsAndOtherHandlers(t *testing.T) {
	for name, meta := range map[string]map[string]any{
		"prompt on PostToolUse": {"event": "PostToolUse", "type": "prompt", "prompt": "check"},
		"http":                  {"event": "Stop", "type": "http", "url": "https://example.com"},
		"agent":                 {"event": "Stop", "type": "agent", "prompt": "check"},
		"mcp_tool":              {"event": "Stop", "type": "mcp_tool", "server": "docs", "tool": "check"},
	} {
		t.Run(name, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			b := spec.Bundle{Hooks: []spec.Entry{{Kind: spec.KindHook, Path: "p.yaml", Name: "p", Meta: meta}}}
			err := New().Emit(emit.NewSession(), b, &config.Config{}, false)
			if err == nil || !strings.Contains(err.Error(), "Stop, UserPromptSubmit, and PreToolUse") {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestEmit_RejectsMalformedPromptHooks(t *testing.T) {
	for name, tc := range map[string]struct {
		meta map[string]any
		want string
	}{
		"empty prompt":  {map[string]any{"event": "UserPromptSubmit", "type": "prompt", "prompt": ""}, "need a prompt"},
		"extra command": {map[string]any{"event": "Stop", "type": "prompt", "prompt": "check", "command": "./hooks/a.sh"}, "take no command"},
	} {
		t.Run(name, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			b := spec.Bundle{Hooks: []spec.Entry{{Kind: spec.KindHook, Path: "p.yaml", Name: "p", Meta: tc.meta}}}
			err := New().Emit(emit.NewSession(), b, &config.Config{}, false)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			if _, statErr := os.Stat(".codebuddy/settings.json"); !os.IsNotExist(statErr) {
				t.Errorf("settings written for a rejected hook: %v", statErr)
			}
		})
	}
}

func TestEmit_PromptHookModelPrintsNote(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	b := spec.Bundle{Hooks: []spec.Entry{{Kind: spec.KindHook, Name: "ask", Meta: map[string]any{"event": "UserPromptSubmit", "type": "prompt", "prompt": "Check: $ARGUMENTS", "model": "fast"}}}}
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if got := read(t, ".codebuddy/settings.json"); !strings.Contains(got, `"UserPromptSubmit"`) || strings.Contains(got, "fast") {
		t.Errorf("UserPromptSubmit prompt hook = %s", got)
	}
	if !strings.Contains(buf.String(), "model") || !strings.Contains(buf.String(), "no model field") {
		t.Errorf("no model note: %q", buf.String())
	}
}
