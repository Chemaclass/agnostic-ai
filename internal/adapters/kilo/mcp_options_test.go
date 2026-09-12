package kilo

import (
	"encoding/json"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_MCPPreservesTimeoutAndOAuthDisablement(t *testing.T) {
	testutil.TempCwd(t)
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindMCP, Name: "local", Meta: map[string]any{"command": "server", "timeout": 0, "oauth": false}},
		{Kind: spec.KindMCP, Name: "remote", Meta: map[string]any{"type": "http", "url": "https://example.test/mcp", "timeout": 45000, "oauth": false}},
		{Kind: spec.KindMCP, Name: "custom", Meta: map[string]any{"type": "sse", "url": "https://example.test/sse", "timeout": 99, "x-kilo": map[string]any{"timeout": 0, "oauth": false}}},
	})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Servers map[string]map[string]any `json:"mcp"`
	}
	if err := json.Unmarshal([]byte(readFile(t, "kilo.jsonc")), &got); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"local", "remote", "custom"} {
		want := float64(0)
		if name == "remote" {
			want = 45000
		}
		if got.Servers[name]["timeout"] != want {
			t.Errorf("%s timeout = %#v", name, got.Servers[name]["timeout"])
		}
		value, exists := got.Servers[name]["oauth"]
		if name == "local" {
			if exists {
				t.Error("OAuth must not emit on stdio")
			}
		} else if !exists || value != false {
			t.Errorf("%s OAuth disablement lost: %#v", name, got.Servers[name])
		}
	}
}
