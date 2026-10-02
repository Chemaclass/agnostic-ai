package emit

import (
	"encoding/json"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestMCPDocument_ClaudeBooleanFlagsPreserveExplicitValues(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		fields map[string]any
		want   map[string]bool
	}{
		{"loaded", map[string]any{"alwaysLoad": true, "bareElicitationCapability": false}, map[string]bool{"alwaysLoad": true, "bareElicitationCapability": false}},
		{"deferred", map[string]any{"alwaysLoad": false, "bareElicitationCapability": true}, map[string]bool{"alwaysLoad": false, "bareElicitationCapability": true}},
		{"absent", nil, nil},
		{"invalid", map[string]any{"alwaysLoad": "false", "bareElicitationCapability": 1}, nil},
		{"null", map[string]any{"alwaysLoad": nil, "bareElicitationCapability": nil}, nil},
	}
	for _, transport := range []string{"stdio", "http", "sse", "ws"} {
		for _, tc := range cases {
			t.Run(transport+"/"+tc.name, func(t *testing.T) {
				meta := map[string]any{"type": transport, "command": "example-server", "url": "https://example.invalid/mcp"}
				for key, value := range tc.fields {
					meta[key] = value
				}
				data, err := MCPDocument([]spec.Entry{{Kind: spec.KindMCP, Name: "example", Meta: meta}}, MCPSchemaServersMap, WithClaudeMCPExtras())
				if err != nil {
					t.Fatal(err)
				}
				var doc struct {
					Servers map[string]map[string]any `json:"mcpServers"`
				}
				if err := json.Unmarshal([]byte(data), &doc); err != nil {
					t.Fatal(err)
				}
				server, ok := doc.Servers["example"]
				if !ok {
					t.Fatalf("missing server: %s", data)
				}
				for _, key := range []string{"alwaysLoad", "bareElicitationCapability"} {
					got, present := server[key]
					want, expected := tc.want[key]
					if present != expected || (expected && got != want) {
						t.Errorf("%s = %#v (present %v), want %v (present %v)", key, got, present, want, expected)
					}
				}
			})
		}
	}
}

func TestMCPDocument_OtherOptionsOmitClaudeBooleanFlags(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		schema MCPSchema
		key    string
		opts   []MCPOption
	}{
		{"shared", MCPSchemaServersMap, "mcpServers", nil},
		{"vscode", MCPSchemaVSCodeServers, "servers", []MCPOption{WithVSCodeMCPExtras()}},
		{"copilot-cli", MCPSchemaServersMap, "mcpServers", []MCPOption{WithCopilotCLIMCPExtras()}},
	}
	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "enabled", Meta: map[string]any{"command": "example-server", "alwaysLoad": true, "bareElicitationCapability": true}},
		{Kind: spec.KindMCP, Name: "disabled", Meta: map[string]any{"command": "example-server", "alwaysLoad": false, "bareElicitationCapability": false}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := MCPDocument(entries, tc.schema, tc.opts...)
			if err != nil {
				t.Fatal(err)
			}
			var doc map[string]map[string]map[string]any
			if err := json.Unmarshal([]byte(data), &doc); err != nil {
				t.Fatal(err)
			}
			servers := doc[tc.key]
			if len(servers) != len(entries) {
				t.Fatalf("servers = %#v, want both entries", servers)
			}
			for name, server := range servers {
				for _, key := range []string{"alwaysLoad", "bareElicitationCapability"} {
					if _, present := server[key]; present {
						t.Errorf("%s inherited Claude field %s: %s", name, key, data)
					}
				}
			}
		})
	}
}
