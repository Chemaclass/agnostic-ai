package kimi

import (
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_MCPRejectsRemovedKaosExecutor(t *testing.T) {
	testutil.TempCwd(t)
	bundle := spec.Bundle{MCPs: []spec.Entry{{Name: "obsolete", Meta: map[string]any{
		"command": "example-mcp", "x-kimi": map[string]any{"executor": "kaos"},
	}}}}
	err := New().Emit(emit.NewSession(), bundle, &config.Config{}, false)
	if err == nil {
		t.Fatal("removed kaos executor was emitted")
	}
	for _, want := range []string{"obsolete", "kaos", "local"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q: %v", want, err)
		}
	}
	if _, err := os.Stat(".kimi-code/mcp.json"); !os.IsNotExist(err) {
		t.Errorf("invalid executor wrote native MCP configuration: %v", err)
	}
}

func TestEmit_MCPKeepsLocalExecutorAndRuntimeID(t *testing.T) {
	testutil.TempCwd(t)
	bundle := spec.Bundle{MCPs: []spec.Entry{{Name: "local", Meta: map[string]any{
		"command": "example-mcp", "x-kimi": map[string]any{"executor": "local", "runtime_id": "workspace"},
	}}}}
	if err := New().Emit(emit.NewSession(), bundle, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(".kimi-code/mcp.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"executor": "local"`, `"runtime_id": "workspace"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("native MCP lacks %q: %s", want, data)
		}
	}
}
