package opencode

import (
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmitNativeMCP_RefusesConflictingSpecOwners(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	native := spec.Entry{Kind: spec.KindSettings, Path: "settings/native.yaml", Meta: map[string]any{
		"x-opencode": map[string]any{"mcp": map[string]any{"servers": map[string]any{}}},
	}}
	portable := spec.Entry{Kind: spec.KindMCP, Name: "docs", Path: "mcps/docs.yaml", Meta: map[string]any{"command": "srv"}}
	for _, bundle := range []spec.Bundle{
		{Settings: []spec.Entry{native}, MCPs: []spec.Entry{portable}},
		{Settings: []spec.Entry{native, {Kind: spec.KindSettings, Path: "settings/other.yaml", Meta: native.Meta}}},
	} {
		if err := New().Emit(emit.NewSession(), bundle, &config.Config{}, false); err == nil || !strings.Contains(err.Error(), "claim") {
			t.Errorf("conflicting owners error = %v", err)
		}
		if _, err := os.Stat("opencode.json"); !os.IsNotExist(err) {
			t.Errorf("conflicting owners wrote native output: %v", err)
		}
	}
}

func TestEmitNativeMCP_KeepsUserServersAndDefaults(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	if err := os.WriteFile("opencode.json", []byte(`{"mcp":{"servers":{"user":{"type":"local","command":["user-srv"]}},"timeout":{"catalog":17},"user_option":false},"permissions":[{"effect":"deny","action":"shell","resource":"*"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	native := spec.Entry{Kind: spec.KindSettings, Meta: map[string]any{
		"x-opencode": map[string]any{"mcp": map[string]any{
			"servers": map[string]any{"docs": map[string]any{"type": "local", "command": []any{"srv"}}},
		}},
	}}
	if err := New().Emit(emit.NewSession(), spec.Bundle{Settings: []spec.Entry{native}}, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, "opencode.json")
	for _, want := range []string{"user-srv", "srv", `"catalog": 17`, `"user_option": false`, `"permissions"`, `"action": "shell"`, `"resource": "*"`, `"effect": "deny"`} {
		if !strings.Contains(got, want) {
			t.Errorf("native merge loses %q: %s", want, got)
		}
	}
}

func TestEmitNativeMCP_NoAgentsDoesNotInterpretProjectPermissions(t *testing.T) {
	testutil.TempCwd(t)
	if err := os.WriteFile("opencode.json", []byte(`{"permissions":[{"effect":"deny","action":"shell"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	native := spec.Entry{Kind: spec.KindSettings, Meta: map[string]any{
		"x-opencode": map[string]any{"mcp": map[string]any{
			"servers": map[string]any{"docs": map[string]any{"type": "local", "command": []any{"srv"}}},
		}},
	}}
	if err := New().Emit(emit.NewSession(), spec.Bundle{Settings: []spec.Entry{native}}, &config.Config{}, false); err != nil {
		t.Fatalf("MCP-only sync must leave unrelated permission values to OpenCode: %v", err)
	}
	got := readFile(t, "opencode.json")
	for _, want := range []string{`"servers"`, `"srv"`, `"permissions"`, `"action": "shell"`, `"effect": "deny"`} {
		if !strings.Contains(got, want) {
			t.Errorf("native MCP sync loses %q: %s", want, got)
		}
	}
}
