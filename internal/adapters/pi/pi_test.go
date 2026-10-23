package pi

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

func kitSinkBundle() spec.Bundle {
	return spec.NewBundle([]spec.Entry{
		{Kind: spec.KindRule, Name: "style", Body: "Use plain names."},
		{Kind: spec.KindSkill, Name: "review", Body: "Review changes.", Meta: map[string]any{"description": "Review the patch", "x-pi": map[string]any{"disable-model-invocation": true}}},
		{Kind: spec.KindCommand, Name: "check", Body: "Check $ARGUMENTS and $1.", Meta: map[string]any{"description": "Check a patch", "argument-hint": "<path>"}},
		{Kind: spec.KindMCP, Name: "local", Meta: map[string]any{"command": "npx", "args": []any{"server"}, "cwd": "tools", "disabled": true}},
		{Kind: spec.KindMCP, Name: "remote", Meta: map[string]any{"type": "http", "url": "https://example.com/mcp", "headers": map[string]any{"Authorization": "${TOKEN}"}, "disabled": false, "x-pi": map[string]any{"exposure": "direct"}}},
		{Kind: spec.KindSettings, Name: "defaults", Meta: map[string]any{"model": "model-a", "effort": "high", "x-pi": map[string]any{"defaultProvider": "provider-a"}}},
	})
}

func TestEmit_NativeFilesAndSettingsMerge(t *testing.T) {
	testutil.TempCwd(t)
	if err := os.MkdirAll(".pi", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".pi/settings.json", []byte("{\"theme\":\"light\"}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	for path, wants := range map[string][]string{
		".pi/skills/review/SKILL.md": {"Review changes.", "disable-model-invocation: true"},
		".pi/prompts/check.md":       {"Check $ARGUMENTS and $1.", "argument-hint: <path>"},
		".pi/settings.json":          {"\"theme\": \"light\"", "\"defaultModel\": \"model-a\"", "\"defaultThinkingLevel\": \"high\"", "\"defaultProvider\": \"provider-a\""},
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range wants {
			if !strings.Contains(string(data), want) {
				t.Errorf("%s missing %s: %s", path, want, data)
			}
		}
	}
	data, err := os.ReadFile(".pi/mcp.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Servers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Servers["local"]["enabled"] != false || doc.Servers["remote"]["enabled"] != true {
		t.Errorf("enabled flags: %s", data)
	}
	if _, ok := doc.Servers["local"]["disabled"]; ok {
		t.Errorf("portable flag leaked: %s", data)
	}
	if doc.Servers["local"]["cwd"] != "tools" || doc.Servers["remote"]["exposure"] != "direct" {
		t.Errorf("native fields: %s", data)
	}
	if _, err := os.Stat(".mcp.json"); !os.IsNotExist(err) {
		t.Errorf("unexpected root MCP: %v", err)
	}
}

func TestEmit_RejectsUnreadPromptsAndSSE(t *testing.T) {
	for _, entry := range []spec.Entry{
		{Kind: spec.KindCommand, Name: "nested/check", Body: "check"},
		{Kind: spec.KindMCP, Name: "remote", Meta: map[string]any{"type": "sse", "url": "https://example.com/sse"}},
	} {
		t.Run(string(entry.Kind), func(t *testing.T) {
			testutil.TempCwd(t)
			if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), &config.Config{}, false); err == nil {
				t.Error("expected unsupported native shape error")
			}
		})
	}
}

func TestEmit_OutputOverridesAndDryRun(t *testing.T) {
	dir := testutil.TempCwd(t)
	cfg := &config.Config{Outputs: map[string]config.Output{"pi": {SkillsDir: "native/skills", CommandsDir: "native/prompts", MCPFile: "native/mcp.json", ConfFile: "native/settings.json"}}}
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), cfg, true); err != nil {
		t.Fatal(err)
	}
	if got := testutil.WalkRel(t, dir); len(got) != 0 {
		t.Errorf("dry run wrote %v", got)
	}
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), cfg, false); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"skills/review/SKILL.md", "prompts/check.md", "mcp.json", "settings.json"} {
		data, err := os.ReadFile(filepath.Join("native", p))
		if err != nil || len(data) == 0 {
			t.Errorf("%s: %s %v", p, data, err)
		}
	}
}

func TestKitSink_GoldenSnapshot(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), &config.Config{Outputs: map[string]config.Output{"pi": {RulesFile: "native-rules.md"}}}, false); err != nil {
		t.Fatal(err)
	}
	testutil.AssertGoldenTree(t, dir, filepath.Join(cwd, "testdata/kitsink"))
}

func TestEmit_CapabilityMatrixCoversEveryDeclaredKind(t *testing.T) {
	testutil.TempCwd(t)
	cfg := &config.Config{Outputs: map[string]config.Output{"pi": {RulesFile: "native-rules.md"}}}
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), cfg, false); err != nil {
		t.Fatal(err)
	}
	paths := map[spec.Kind]string{spec.KindRule: "native-rules.md", spec.KindSkill: ".pi/skills/review/SKILL.md", spec.KindCommand: ".pi/prompts/check.md", spec.KindMCP: ".pi/mcp.json", spec.KindSettings: ".pi/settings.json"}
	for _, kind := range caps.Supports {
		data, err := os.ReadFile(paths[kind])
		if err != nil || len(data) == 0 {
			t.Errorf("declared %s has no content at %s: %s %v", kind, paths[kind], data, err)
		}
	}
}

func TestEmit_UnsupportedAgentsAndHooksFailBeforeWriting(t *testing.T) {
	dir := testutil.TempCwd(t)
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindAgent, Name: "review", Body: "Review."}, {Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "stop", "command": "check"}}})
	if err := New().Emit(emit.NewSession(), b, &config.Config{OnUnsupported: "error"}, false); err == nil {
		t.Error("unsupported kinds accepted")
	}
	if got := testutil.WalkRel(t, dir); len(got) > 0 {
		t.Errorf("unsupported kinds wrote %v", got)
	}
}

func TestEmit_ProvenanceHeaderOnMarkdown(t *testing.T) {
	testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{".pi/skills/review/SKILL.md", ".pi/prompts/check.md"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "Generated by agnostic-ai") {
			t.Errorf("%s missing provenance: %s", path, data)
		}
	}
}

func TestEmit_MissingSkillDescriptionGetsLoadableFallback(t *testing.T) {
	testutil.TempCwd(t)
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindSkill, Name: "review", Body: "Review."}})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(".pi/skills/review/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "description: review") {
		t.Errorf("unloadable skill: %s", data)
	}
}

func TestEmit_RejectsInvalidOrCollidingMCPNames(t *testing.T) {
	cases := [][]spec.Entry{
		{{Kind: spec.KindMCP, Name: "@scope/server", Meta: map[string]any{"command": "server"}}},
		{{Kind: spec.KindMCP, Name: "my-server", Meta: map[string]any{"command": "one"}}, {Kind: spec.KindMCP, Name: "my_server", Meta: map[string]any{"command": "two"}}},
		{{Kind: spec.KindMCP, Name: "missing", Meta: map[string]any{"disabled": true}}},
	}
	for _, entries := range cases {
		t.Run(entries[0].Name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err == nil {
				t.Error("expected invalid MCP error")
			}
			if got := testutil.WalkRel(t, dir); len(got) > 0 {
				t.Errorf("invalid MCP wrote %v", got)
			}
		})
	}
}

func TestEmit_NativeControlOnlyOverride(t *testing.T) {
	testutil.TempCwd(t)
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindMCP, Name: "global", Meta: map[string]any{"x-pi": map[string]any{"enabled": false, "exposure": "direct"}}}})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(".pi/mcp.json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "\"enabled\": false") || strings.Contains(string(data), "\"type\"") {
		t.Errorf("override: %s", data)
	}
}
