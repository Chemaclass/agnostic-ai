package claude

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_MCP_PreservesBooleanFlags(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "loaded", Meta: map[string]any{"command": "example-server", "alwaysLoad": true, "bareElicitationCapability": false}},
		{Kind: spec.KindMCP, Name: "deferred", Meta: map[string]any{"type": "http", "url": "https://example.invalid/mcp", "alwaysLoad": false, "bareElicitationCapability": true}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(".mcp.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Servers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	want := map[string]map[string]bool{
		"loaded":   {"alwaysLoad": true, "bareElicitationCapability": false},
		"deferred": {"alwaysLoad": false, "bareElicitationCapability": true},
	}
	for name, fields := range want {
		for key, value := range fields {
			got, ok := doc.Servers[name][key].(bool)
			if !ok || got != value {
				t.Errorf("%s.%s = %#v, want %v", name, key, doc.Servers[name][key], value)
			}
		}
	}
}
