package emit

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestMCPDocument_ServersMap(t *testing.T) {
	mcps := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{
				"command": "npx",
				"args":    []any{"-y", "@modelcontextprotocol/server-filesystem"},
				"env":     map[string]any{"ROOT": "/tmp"},
			},
		},
	}
	got, err := MCPDocument(mcps, MCPSchemaServersMap)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	servers, ok := parsed["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("expected mcpServers key: %s", got)
	}
	fs, ok := servers["fs"].(map[string]any)
	if !ok {
		t.Fatalf("expected fs server: %s", got)
	}
	if fs["command"] != "npx" {
		t.Errorf("command mismatch: %v", fs["command"])
	}
	args, _ := fs["args"].([]any)
	if len(args) != 2 {
		t.Errorf("expected 2 args, got %d", len(args))
	}
	if _, ok := fs["type"]; ok {
		t.Errorf("ServersMap schema should not include type field")
	}
}

func TestMCPDocument_VSCodeServers(t *testing.T) {
	mcps := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{
				"command": "npx",
				"args":    []any{"-y"},
			},
		},
	}
	got, err := MCPDocument(mcps, MCPSchemaVSCodeServers)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `"servers"`) {
		t.Errorf("expected servers key: %s", got)
	}
	if !strings.Contains(got, `"type": "stdio"`) {
		t.Errorf("expected type stdio: %s", got)
	}
}

func TestMCPDocument_HTTPTransport(t *testing.T) {
	mcps := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "remote",
			Meta: map[string]any{
				"type": "http",
				"url":  "https://example.com/mcp",
				"headers": map[string]any{
					"Authorization": "Bearer x",
				},
			},
		},
	}
	got, err := MCPDocument(mcps, MCPSchemaServersMap)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "https://example.com/mcp") {
		t.Errorf("missing url: %s", got)
	}
	if !strings.Contains(got, "Bearer x") {
		t.Errorf("missing header: %s", got)
	}
}

func TestMCPDocument_EmptySkips(t *testing.T) {
	got, err := MCPDocument(nil, MCPSchemaServersMap)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}
