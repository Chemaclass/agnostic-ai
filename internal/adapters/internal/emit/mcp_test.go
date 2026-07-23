package emit

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestMCPDocument_ServersMap(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestMCPDocument_DescriptionAndDisabled(t *testing.T) {
	t.Parallel()
	mcps := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "myserver",
			Meta: map[string]any{
				"command":     "npx",
				"description": "My server",
				"disabled":    true,
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
	srv := parsed["mcpServers"].(map[string]any)["myserver"].(map[string]any)
	if srv["description"] != "My server" {
		t.Errorf("description not emitted: %v", got)
	}
	if srv["disabled"] != true {
		t.Errorf("disabled not emitted: %v", got)
	}
}

func TestMCPDocument_Roots(t *testing.T) {
	t.Parallel()
	mcps := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{
				"command": "npx",
				"roots": []any{
					map[string]any{"uri": "file:///workspace", "name": "workspace"},
				},
			},
		},
	}
	got, err := MCPDocument(mcps, MCPSchemaServersMap)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "file:///workspace") {
		t.Errorf("roots uri not emitted: %s", got)
	}
}

func TestMCPDocument_SSETransport(t *testing.T) {
	t.Parallel()
	mcps := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "events",
			Meta: map[string]any{
				"type": "sse",
				"url":  "https://example.com/sse",
				"headers": map[string]any{
					"Authorization": "Bearer abc",
				},
			},
		},
	}
	got, err := MCPDocument(mcps, MCPSchemaServersMap)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "https://example.com/sse") {
		t.Errorf("missing sse url: %s", got)
	}
	if !strings.Contains(got, "Bearer abc") {
		t.Errorf("missing sse header: %s", got)
	}
	// VSCode schema preserves the type discriminator.
	gotVS, err := MCPDocument(mcps, MCPSchemaVSCodeServers)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotVS, `"type": "sse"`) {
		t.Errorf("vscode schema should preserve type sse: %s", gotVS)
	}
}

func TestMCPDocument_StdioTypeOnlyWhenVSCodeSchema(t *testing.T) {
	t.Parallel()
	mcps := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{"command": "npx", "args": []any{"-y"}},
		},
	}
	gotMap, err := MCPDocument(mcps, MCPSchemaServersMap)
	if err != nil {
		t.Fatal(err)
	}
	// ServersMap schema omits the type discriminator for stdio (default).
	if strings.Contains(gotMap, `"type"`) {
		t.Errorf("servers-map schema should not emit type for stdio default: %s", gotMap)
	}
	gotVS, err := MCPDocument(mcps, MCPSchemaVSCodeServers)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotVS, `"type": "stdio"`) {
		t.Errorf("vscode schema must emit type stdio: %s", gotVS)
	}
}

func TestMCPDocument_EmptySkips(t *testing.T) {
	t.Parallel()
	got, err := MCPDocument(nil, MCPSchemaServersMap)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}
