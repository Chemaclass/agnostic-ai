package opencode

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestNativeV2MCP_GoldenSnapshot(t *testing.T) {
	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	expectedDir := filepath.Join(origCwd, "testdata", "native-v2-mcp")
	dir := testutil.TempCwd(t)
	settings := spec.Entry{Kind: spec.KindSettings, Name: "native-mcp", Meta: map[string]any{
		"target": "opencode",
		"x-opencode": map[string]any{
			"mcp": map[string]any{
				"codemode": false,
				"timeout":  map[string]any{"catalog": 3000, "execution": 9000},
				"servers": map[string]any{
					"docs": map[string]any{
						"type":        "local",
						"command":     []any{"node", "server.js", "--read-only"},
						"cwd":         "tools/docs",
						"environment": map[string]any{"DOCS_TOKEN": "{env:DOCS_TOKEN}"},
						"disabled":    true,
						"timeout":     map[string]any{"catalog": 2000, "execution": 5000},
						"options":     map[string]any{"vendor_option": false},
					},
					"remote": map[string]any{
						"type":     "remote",
						"url":      "https://example.test/mcp",
						"headers":  map[string]any{"Authorization": "Bearer {env:MCP_TOKEN}"},
						"disabled": false,
						"oauth":    false,
					},
				},
			},
		},
	}}
	if err := New().Emit(emit.NewSession(), spec.Bundle{Settings: []spec.Entry{settings}}, &config.Config{}, false); err != nil {
		t.Fatalf("emit native V2 MCP: %v", err)
	}
	testutil.AssertGoldenTree(t, dir, expectedDir)
}
