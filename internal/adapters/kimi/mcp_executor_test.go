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

func TestEmit_MCPRejectsInvalidExecutor(t *testing.T) {
	for _, executor := range []string{"kaos", "KAOS", "remote"} {
		t.Run(executor, func(t *testing.T) {
			testutil.TempCwd(t)
			bundle := spec.Bundle{MCPs: []spec.Entry{{Name: "obsolete", Meta: map[string]any{
				"command": "example-mcp", "x-kimi": map[string]any{"executor": executor},
			}}}}
			err := New().Emit(emit.NewSession(), bundle, &config.Config{}, false)
			if err == nil {
				t.Fatal("invalid executor was emitted")
			}
			for _, want := range []string{"obsolete", executor, "remove it or set executor: local"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error lacks %q: %v", want, err)
				}
			}
			if _, err := os.Stat(".kimi-code/mcp.json"); !os.IsNotExist(err) {
				t.Errorf("invalid executor wrote native MCP configuration: %v", err)
			}
		})
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

func TestEmit_MCPKeepsEnvironmentID(t *testing.T) {
	for name, native := range map[string]map[string]any{
		"environment_id": {"environment_id": "dev-box"},
		"both keys":      {"environment_id": "dev-box", "runtime_id": "dev-box"},
	} {
		t.Run(name, func(t *testing.T) {
			testutil.TempCwd(t)
			bundle := spec.Bundle{MCPs: []spec.Entry{{Name: "remote-fs", Meta: map[string]any{"command": "example-mcp", "x-kimi": native}}}}
			if err := New().Emit(emit.NewSession(), bundle, &config.Config{}, false); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(".kimi-code/mcp.json")
			if err != nil {
				t.Fatal(err)
			}
			for key, value := range native {
				if want := `"` + key + `": "` + value.(string) + `"`; !strings.Contains(string(data), want) {
					t.Errorf("native MCP lacks %q: %s", want, data)
				}
			}
		})
	}
}

func TestEmit_MCPRejectsEmptyEnvironmentID(t *testing.T) {
	for name, value := range map[string]any{"empty": "", "blank": " ", "number": 3} {
		t.Run(name, func(t *testing.T) {
			testutil.TempCwd(t)
			bundle := spec.Bundle{MCPs: []spec.Entry{{Name: "remote-fs", Meta: map[string]any{"command": "example-mcp", "x-kimi": map[string]any{"environment_id": value}}}}}
			err := New().Emit(emit.NewSession(), bundle, &config.Config{}, false)
			if err == nil || !strings.Contains(err.Error(), "environment_id must name an environment") {
				t.Fatalf("invalid environment_id was not rejected: %v", err)
			}
		})
	}
}
