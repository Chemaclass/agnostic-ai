package kimi

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_PreservesNativeRestrictions(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindAgent, Name: "reviewer", Body: "Review the patch.", Meta: map[string]any{"tools": []any{}, "x-kimi": map[string]any{"disallowedTools": []any{"Bash"}}}},
		{Kind: spec.KindSkill, Name: "check", Body: "Check the patch.", Meta: map[string]any{"disable-model-invocation": true}},
		{Kind: spec.KindMCP, Name: "docs", Meta: map[string]any{"url": "https://docs.example/mcp", "disabled": true, "x-kimi": map[string]any{"enabledTools": []any{}}}},
	})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	agent, err := os.ReadFile(".kimi-code/agents/reviewer.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"tools: []", "disallowedTools:", "Review the patch."} {
		if !strings.Contains(string(agent), want) {
			t.Errorf("agent missing %q: %s", want, agent)
		}
	}
	skill, err := os.ReadFile(".kimi-code/skills/check/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(skill), "---\n") || !strings.Contains(string(skill), "disableModelInvocation: true") {
		t.Errorf("invalid native skill: %s", skill)
	}
	data, err := os.ReadFile(".kimi-code/mcp.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Servers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Servers["docs"]["enabled"] != false {
		t.Errorf("disabled server lost: %s", data)
	}
	if tools, ok := doc.Servers["docs"]["enabledTools"].([]any); !ok || len(tools) != 0 {
		t.Errorf("empty restriction lost: %s", data)
	}
	if _, err := os.Stat(".mcp.json"); !os.IsNotExist(err) {
		t.Errorf("unexpected root MCP file: %v", err)
	}
	if _, err := os.Stat(".kimi-code/config.toml"); !os.IsNotExist(err) {
		t.Errorf("unexpected default config: %v", err)
	}
}

func TestMCP_BearerReferenceAndIncompleteCredentials(t *testing.T) {
	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "bearer", Meta: map[string]any{"url": "https://docs.example/mcp", "headers": map[string]any{"Authorization": "Bearer ${TOKEN}"}}},
		{Kind: spec.KindMCP, Name: "incomplete", Meta: map[string]any{"url": "https://docs.example/mcp", "headers": map[string]any{"X-Key": "${TOKEN}"}}},
		{Kind: spec.KindMCP, Name: "literal", Meta: map[string]any{"command": "server", "env": map[string]any{"VALUE": "$${TOKEN}"}}},
	}
	got, err := (Adapter{}).CheckedUserMCPServers((Adapter{}).RewriteMCPReferences(entries))
	if err != nil {
		t.Fatal(err)
	}
	if got["bearer"].(map[string]any)["bearerTokenEnvVar"] != "TOKEN" {
		t.Errorf("bearer mapping: %#v", got)
	}
	if _, ok := got["incomplete"]; ok {
		t.Errorf("incomplete credential server retained: %#v", got)
	}
	if got["literal"].(map[string]any)["env"].(map[string]any)["VALUE"] != "${TOKEN}" {
		t.Errorf("literal changed: %#v", got)
	}
}

func TestEmit_RootMCPMirrorIsOptIn(t *testing.T) {
	testutil.TempCwd(t)
	b := spec.Bundle{MCPs: []spec.Entry{{Kind: spec.KindMCP, Name: "docs", Meta: map[string]any{"command": "server", "disabled": true}}}}
	cfg := &config.Config{Outputs: map[string]config.Output{"kimi": {RootMCPFile: ".mcp.json"}}}
	if err := New().Emit(emit.NewSession(), b, cfg, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(".mcp.json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"enabled": false`) {
		t.Errorf("root mirror lost restriction: %s", data)
	}
}
func TestNativeHooks_RejectInvalidSectionFieldsAndTimeouts(t *testing.T) {
	for _, meta := range []map[string]any{
		{"event": "PreToolUse", "command": "check", "timeout": 1.5},
		{"event": "PreToolUse", "command": "check", "timeout": 601},
		{"event": "PreToolUse", "command": "check", "env": map[string]any{"MODE": "test"}},
		{"event": "PreToolUse", "command": "check", "cwd": "somewhere"},
	} {
		if _, err := (Adapter{}).UserHookRecords([]spec.Entry{{Name: "guard", Meta: meta}}, "hooks"); err == nil {
			t.Errorf("invalid native hook accepted: %#v", meta)
		}
	}
}

func TestMCP_RejectsInvalidNativeShape(t *testing.T) {
	for _, extra := range []map[string]any{{"enabledTools": false}, {"startupTimeoutMs": 1.5}, {"auth": true}, {"bearerTokenEnvVar": ""}} {
		meta := map[string]any{"url": "https://example.com/mcp", "x-kimi": extra}
		if _, err := (Adapter{}).CheckedUserMCPServers([]spec.Entry{{Name: "docs", Meta: meta}}); err == nil {
			t.Errorf("invalid MCP accepted: %#v", extra)
		}
	}
}
func TestAgents_OmittedAndEmptyToolsKeepDifferentMeaning(t *testing.T) {
	testutil.TempCwd(t)
	if err := (Adapter{}).EmitAgents(emit.NewSession(), []spec.Entry{{Name: "unrestricted", Body: "Read the patch."}, {Name: "restricted", Body: "Read the patch.", Meta: map[string]any{"tools": []any{}}}}, "agents", false); err != nil {
		t.Fatal(err)
	}
	unlimited, err := os.ReadFile("agents/unrestricted.md")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(unlimited), "tools:") {
		t.Errorf("omitted tools became restrictive: %s", unlimited)
	}
	limited, err := os.ReadFile("agents/restricted.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(limited), "tools: []") {
		t.Errorf("empty tools became unrestricted: %s", limited)
	}
}
func TestNativeHooks_WindowsCommandIsReversible(t *testing.T) {
	records, err := (Adapter{}).UserHookRecordsForPlatform([]spec.Entry{{Name: "guard", Meta: map[string]any{"event": "PreToolUse", "command": "check-posix", "commandWindows": "check-windows", "args": []any{"two words"}}}}, "hooks", "windows")
	if err != nil {
		t.Fatal(err)
	}
	cmd := records[0]["command"].(string)
	if strings.Contains(cmd, "export ") {
		t.Errorf("POSIX syntax reached Windows: %s", cmd)
	}
	if got := DeclaredHookCommand(cmd); !strings.Contains(got, "check-windows") || !strings.Contains(got, "two words") {
		t.Errorf("Windows command changed: %s", got)
	}
}

func TestAgents_NativeCapabilityTranslation(t *testing.T) {
	for _, tc := range []struct{ portable, native string }{{"WebFetch", "FetchURL"}, {"FetchURL", "FetchURL"}, {"mcp__docs", "mcp__docs__*"}, {"mcp__docs__lookup", "mcp__docs__lookup"}, {"Grep", "Grep"}} {
		got, ok := (Adapter{}).TranslateAgentCapability(tc.portable)
		if !ok || len(got) != 1 || got[0] != tc.native {
			t.Errorf("%s translated to %v (%v), want %s", tc.portable, got, ok, tc.native)
		}
	}
}
