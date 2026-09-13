package claude

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_PreservesStableNativeHookHandlers(t *testing.T) {
	testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "http", Meta: map[string]any{"event": "PreToolUse", "matcher": "Bash", "type": "http", "url": "https://example.test/check", "headers": map[string]any{"Authorization": "Bearer $TOKEN"}, "allowedEnvVars": []any{"TOKEN"}, "timeout": 15, "statusMessage": "Checking request", "if": "Bash(git *)"}},
		{Kind: spec.KindHook, Name: "mcp", Meta: map[string]any{"event": "PreToolUse", "matcher": "Bash", "type": "mcp_tool", "server": "checks", "tool": "verify", "input": map[string]any{"path": "${tool_input.file_path}"}}},
		{Kind: spec.KindHook, Name: "prompt", Meta: map[string]any{"event": "PreToolUse", "matcher": "Bash", "type": "prompt", "prompt": "Allow read-only commands.", "model": "example-model", "once": true}},
		{Kind: spec.KindHook, Name: "command", Meta: map[string]any{"event": "PreToolUse", "matcher": "Bash", "command": "echo", "args": []any{"checked"}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(".claude/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Hooks map[string][]struct {
			Hooks []map[string]any `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	got := map[string]map[string]any{}
	for _, group := range doc.Hooks["PreToolUse"] {
		for _, handler := range group.Hooks {
			kind, _ := handler["type"].(string)
			got[kind] = handler
		}
	}
	want := map[string]map[string]any{
		"http":     {"type": "http", "url": "https://example.test/check", "headers": map[string]any{"Authorization": "Bearer $TOKEN"}, "allowedEnvVars": []any{"TOKEN"}, "timeout": float64(15), "statusMessage": "Checking request", "if": "Bash(git *)"},
		"mcp_tool": {"type": "mcp_tool", "server": "checks", "tool": "verify", "input": map[string]any{"path": "${tool_input.file_path}"}},
		"prompt":   {"type": "prompt", "prompt": "Allow read-only commands.", "model": "example-model", "once": true},
		"command":  {"type": "command", "command": "echo", "args": []any{"checked"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("native handlers = %#v, want %#v", got, want)
	}
}
