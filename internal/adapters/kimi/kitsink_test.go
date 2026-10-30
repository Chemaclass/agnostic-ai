package kimi

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

func kitSinkBundle() spec.Bundle {
	return spec.NewBundle([]spec.Entry{
		{Kind: spec.KindRule, Name: "small", Body: "Keep changes small."},
		{Kind: spec.KindAgent, Name: "reviewer", Body: "Review changes.", Meta: map[string]any{"description": "Review changes", "can": []any{"read"}, "x-kimi": map[string]any{"disallowedTools": []any{"Bash"}}}},
		{Kind: spec.KindSkill, Name: "check", Body: "Run checks.", Meta: map[string]any{"description": "Run tests", "disable-model-invocation": true}},
		{Kind: spec.KindSettings, Name: "model", Meta: map[string]any{"model": "existing-model", "effort": "high", "x-kimi": map[string]any{"telemetry": false}}},
		{Kind: spec.KindMCP, Name: "docs", Meta: map[string]any{"type": "sse", "url": "https://example.com/mcp", "disabled": true, "x-kimi": map[string]any{"enabledTools": []any{}, "bearerTokenEnvVar": "DOCS_TOKEN"}}},
		{Kind: spec.KindMCP, Name: "local", Meta: map[string]any{"command": "node", "args": []any{"server.js"}, "x-kimi": map[string]any{"executor": "local", "runtime_id": "workspace"}}},
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "PreToolUse", "matcher": "Bash", "command": "check", "timeout": 10}},
	})
}
func kitSinkConfig() *config.Config {
	return &config.Config{Outputs: map[string]config.Output{"kimi": {RulesFile: "rules.md", ConfFile: "kimi-profile/config.toml"}}}
}
func TestKitSink_GoldenSnapshot(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), kitSinkConfig(), false); err != nil {
		t.Fatal(err)
	}
	if err := emitConfigForPlatform(emit.NewSession(), kitSinkBundle(), kitSinkConfig(), false, "linux"); err != nil {
		t.Fatal(err)
	}
	testutil.AssertGoldenTree(t, dir, filepath.Join(cwd, "testdata", "kitsink"))
}
func TestEmit_NativeSixKinds(t *testing.T) {
	testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), kitSinkConfig(), false); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{"rules.md": "Keep changes small.", ".kimi-code/agents/reviewer.md": "tools:", ".kimi-code/skills/check/SKILL.md": "disableModelInvocation: true", ".kimi-code/mcp.json": "\"enabled\": false", "kimi-profile/config.toml": "effort = \"high\""} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), want) {
			t.Errorf("%s missing %q: %s", path, want, data)
		}
	}
	data, err := os.ReadFile("kimi-profile/config.toml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "[[hooks]]") {
		t.Errorf("hook output missing: %s", data)
	}
}
