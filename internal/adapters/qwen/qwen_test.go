package qwen

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_NativeContracts(t *testing.T) {
	testutil.TempCwd(t)
	if err := os.MkdirAll(".qwen", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".qwen/settings.json", []byte(`{"model":{"temperature":0.3},"custom":{"keep":true},"mcpServers":{"user":{"command":"user"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindAgent, Name: "review", Body: "Review carefully.", Meta: map[string]any{"tools": []any{"UnknownTool"}, "effort": "high", "maxTurns": 4}},
		{Kind: spec.KindCommand, Name: "deploy", Body: "Ship {{args}}", Meta: map[string]any{"description": "Ship"}},
		{Kind: spec.KindMCP, Name: "http", Meta: map[string]any{"type": "http", "url": "https://example.test/http"}},
		{Kind: spec.KindMCP, Name: "sse", Meta: map[string]any{"type": "sse", "url": "https://example.test/sse"}},
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "PreToolUse", "command": "guard", "args": []any{"two words"}, "timeout": 7}},
		{Kind: spec.KindSettings, Name: "model", Meta: map[string]any{"model": "custom-model", "permissions": map[string]any{"deny": []any{"write"}}}},
		{Kind: spec.KindIgnore, Name: "secret", Body: ".env"},
	})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(".qwen/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	servers := got["mcpServers"].(map[string]any)
	if servers["http"].(map[string]any)["httpUrl"] != "https://example.test/http" {
		t.Errorf("HTTP uses httpUrl: %s", raw)
	}
	if servers["sse"].(map[string]any)["url"] != "https://example.test/sse" || servers["user"] == nil {
		t.Errorf("SSE and user server: %s", raw)
	}
	if got["custom"] == nil || got["model"].(map[string]any)["temperature"] != 0.3 {
		t.Errorf("user settings lost: %s", raw)
	}
	handler := got["hooks"].(map[string]any)["PreToolUse"].([]any)[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	if handler["timeout"] != float64(7) || handler["command"] != "guard 'two words'" {
		t.Errorf("hook = %#v", handler)
	}
	if handler["env"].(map[string]any)["AGNOSTIC_AI_TARGET"] != "qwen" {
		t.Errorf("hook target = %#v", handler)
	}
	agent, err := os.ReadFile(".qwen/agents/review.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"UnknownTool", "maxTurns: 4", "Review carefully."} {
		if !strings.Contains(string(agent), want) {
			t.Errorf("missing %q: %s", want, agent)
		}
	}
	if strings.Contains(string(agent), "effort:") {
		t.Errorf("unimplemented effort emitted: %s", agent)
	}
	for p, want := range map[string]string{".qwen/commands/deploy.md": "Ship {{args}}", ".qwenignore": ".env"} {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), want) {
			t.Errorf("%s: %s", p, data)
		}
	}
}

func TestEmit_OutputOverrides(t *testing.T) {
	testutil.TempCwd(t)
	cfg := &config.Config{Outputs: map[string]config.Output{"qwen": {ConfFile: "native/settings.json", MCPFile: "native/mcp.json", HooksFile: "native/hooks.json", AgentsDir: "native/agents", CommandsDir: "native/commands", SkillsDir: "native/skills", IgnoreFile: "native/ignore", RulesFile: "native/rules.md"}}}
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), cfg, false); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]string{"native/settings.json": "custom-model", "native/mcp.json": "httpUrl", "native/hooks.json": "PreToolUse", "native/agents/review.md": "Review", "native/commands/deploy.md": "Ship", "native/skills/check/SKILL.md": "Check", "native/ignore": ".env", "native/rules.md": "Use small functions"} {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), want) {
			t.Errorf("%s: %s", p, data)
		}
	}
}

func kitSinkBundle() spec.Bundle {
	return spec.NewBundle([]spec.Entry{
		{Kind: spec.KindRule, Name: "style", Body: "Use small functions."},
		{Kind: spec.KindRule, Name: "react", Body: "Use function components.", Meta: map[string]any{"globs": []any{"src/**/*.tsx"}}},
		{Kind: spec.KindAgent, Name: "review", Body: "Review", Meta: map[string]any{"tools": []any{"Read", "Edit", "Bash"}, "maxTurns": 3}},
		{Kind: spec.KindSkill, Name: "check", Body: "Check", Meta: map[string]any{"description": "Check the build", "x-qwen": map[string]any{"model": "custom-skill-model"}}},
		{Kind: spec.KindCommand, Name: "deploy", Body: "Ship {{args}}", Meta: map[string]any{"description": "Ship"}},
		{Kind: spec.KindMCP, Name: "remote", Meta: map[string]any{"type": "http", "url": "https://example.test/mcp"}},
		{Kind: spec.KindMCP, Name: "local", Meta: map[string]any{"command": "runner", "args": []any{"--quiet"}, "env": map[string]any{"KEY": "${TOKEN}"}}},
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "PreToolUse", "command": "guard", "timeout": 7}},
		{Kind: spec.KindSettings, Name: "model", Meta: map[string]any{"model": "custom-model", "permissions": map[string]any{"deny": []any{"write"}}}},
		{Kind: spec.KindIgnore, Name: "secret", Body: ".env"},
	})
}

func TestEmit_PathRulesWriteNativeRulesDirectory(t *testing.T) {
	testutil.TempCwd(t)
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindRule, Name: "style", Body: "Use small functions.\n"},
		{Kind: spec.KindRule, Name: "react", Body: "Use function components.\n", Meta: map[string]any{"description": "React", "globs": "src/**/*.tsx,src/**/*.jsx"}},
		{Kind: spec.KindRule, Name: "always", Body: "Always.\n", Meta: map[string]any{"globs": "src/**", "alwaysApply": true}},
	})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(".qwen/rules/react.md")
	if err != nil {
		t.Fatal(err)
	}
	want := emit.WithHeader("---\ndescription: React\npaths:\n  - src/**/*.tsx\n  - src/**/*.jsx\n---\n\nUse function components.\n", emit.FormatMarkdown)
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	for _, name := range []string{"style", "always"} {
		if _, err := os.Stat(".qwen/rules/" + name + ".md"); !os.IsNotExist(err) {
			t.Errorf("%s belongs in QWEN.md: %v", name, err)
		}
	}
}

func TestKitSink_GoldenSnapshot(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(cwd, "testdata", "kitsink")
	dir := testutil.TempCwd(t)
	cfg := &config.Config{Outputs: map[string]config.Output{"qwen": {RulesFile: "QWEN-rules.md"}}}
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), cfg, false); err != nil {
		t.Fatal(err)
	}
	testutil.AssertGoldenTree(t, dir, expected)
}

func TestEmit_PortableDecisionWrapperRuns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX shell")
	}
	testutil.TempCwd(t)
	hook := spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"on": "before-tool", "match": "shell", "decision": "stdout", "command": "true"}}
	native, reason := hook.NativeHook("qwen")
	if reason != "" {
		t.Fatal(reason)
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{native}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	path := ".qwen/hooks/" + emit.DecisionWrapperName
	for _, tc := range []struct {
		command string
		exit    int
		deny    bool
	}{
		{`printf '{"decision":"allow"}'`, 0, false},
		{`printf '{"decision":"deny","reason":"{}"}'`, 2, true},
		{`printf '{"decision":"ask","reason":"review required"}'`, 2, true},
		{`echo broken`, 2, true},
	} {
		cmd := exec.Command("bash", path, "--decision", tc.command)
		var stderr strings.Builder
		cmd.Stderr = &stderr
		stdout, err := cmd.Output()
		code := 0
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		if code != tc.exit || len(stdout) > 0 {
			t.Errorf("%q: exit=%d stdout=%q", tc.command, code, stdout)
		}
		if tc.deny {
			var result map[string]any
			if err := json.Unmarshal([]byte(stderr.String()), &result); err != nil {
				t.Fatal(err)
			}
			if result["decision"] != "deny" {
				t.Errorf("decision=%v", result)
			}
		}
	}
}

func TestEmit_CapabilityMatrixCoversEveryDeclaredKind(t *testing.T) {
	testutil.TempCwd(t)
	cfg := &config.Config{Outputs: map[string]config.Output{"qwen": {RulesFile: "QWEN-rules.md"}}}
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), cfg, false); err != nil {
		t.Fatal(err)
	}
	outputs := map[spec.Kind]struct{ path, body string }{
		spec.KindRule: {"QWEN-rules.md", "Use small functions"}, spec.KindAgent: {".qwen/agents/review.md", "Review"}, spec.KindSkill: {".qwen/skills/check/SKILL.md", "Check"}, spec.KindCommand: {".qwen/commands/deploy.md", "Ship"}, spec.KindHook: {".qwen/settings.json", "PreToolUse"}, spec.KindMCP: {".qwen/settings.json", "httpUrl"}, spec.KindSettings: {".qwen/settings.json", "custom-model"}, spec.KindIgnore: {".qwenignore", ".env"},
	}
	for _, kind := range caps.Supports {
		output, ok := outputs[kind]
		if !ok {
			t.Errorf("unverified kind %s", kind)
			continue
		}
		data, err := os.ReadFile(output.path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), output.body) {
			t.Errorf("%s missing body: %s", kind, data)
		}
		if !strings.HasSuffix(output.path, ".json") && !strings.Contains(string(data), "Generated by agnostic-ai") {
			t.Errorf("%s lacks provenance", output.path)
		}
	}
}

func TestEmit_NativeHookHandlersAndUnknownEvents(t *testing.T) {
	testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "native", Meta: map[string]any{"event": "PreToolUse", "x-qwen": map[string]any{"hooks": []any{map[string]any{"type": "http", "url": "https://example.test/guard", "headers": map[string]any{"X-Key": "${KEY}"}, "timeout": 3}, map[string]any{"type": "prompt", "prompt": "Check $ARGUMENTS", "model": "local", "timeout": 9}}}}},
		{Kind: spec.KindHook, Name: "unknown", Meta: map[string]any{"event": "UnknownEvent", "command": "never"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(".qwen/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"https://example.test/guard", "Check $ARGUMENTS", "local"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("missing %q: %s", want, data)
		}
	}
	if strings.Contains(string(data), "UnknownEvent") {
		t.Errorf("unknown event emitted: %s", data)
	}
}

func TestEmit_RejectsCommandNamespaceTraversal(t *testing.T) {
	testutil.TempCwd(t)
	for _, name := range []string{"..:..:escape", ":deploy", "safe::deploy"} {
		bundle := spec.NewBundle([]spec.Entry{{Kind: spec.KindCommand, Name: name, Body: "never"}})
		if err := New().Emit(emit.NewSession(), bundle, &config.Config{}, false); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
}
