package factory

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_MCPPreservesFactoryControls(t *testing.T) {
	testutil.TempCwd(t)
	oauth := map[string]any{
		"scopes": []any{"read"}, "resource": "https://example.test/resource",
		"authorizationServerIssuer": "https://example.test/issuer", "clientId": "example",
		"clientSecret": "fixture-secret", "clientMetadataUrl": "https://example.test/client.json",
		"tokenEndpointAuthMethod": "client_secret_post", "callbackPort": float64(8000),
	}
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindMCP, Name: "local", Meta: map[string]any{"command": "server", "disabledTools": []any{"destroy"}, "timeout": 0, "connectTimeout": 0, "oauth": false}},
		{Kind: spec.KindMCP, Name: "remote", Meta: map[string]any{"type": "http", "url": "https://example.test/mcp", "disabledTools": []any{"destroy"}, "timeout": 45000, "connectTimeout": 30000, "oauth": false}},
		{Kind: spec.KindMCP, Name: "custom", Meta: map[string]any{"type": "sse", "url": "https://example.test/sse", "timeout": 99, "oauth": false, "x-factory": map[string]any{"timeout": 0, "connectTimeout": 0, "oauth": oauth, "disabledTools": []any{"destroy"}}}},
	})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Servers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(readFile(t, ".factory/mcp.json")), &got); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"local", "remote", "custom"} {
		server := got.Servers[name]
		if !reflect.DeepEqual(server["disabledTools"], []any{"destroy"}) {
			t.Errorf("%s disabledTools = %#v", name, server["disabledTools"])
		}
		wantTimeout, wantConnect := float64(0), float64(0)
		if name == "remote" {
			wantTimeout, wantConnect = 45000, 30000
		}
		if server["timeout"] != wantTimeout || server["connectTimeout"] != wantConnect {
			t.Errorf("%s timeouts = %#v", name, server)
		}
	}
	if _, ok := got.Servers["local"]["oauth"]; ok {
		t.Error("OAuth must not emit on stdio")
	}
	if value, ok := got.Servers["remote"]["oauth"]; !ok || value != false {
		t.Errorf("oauth disablement lost: %#v", got.Servers["remote"])
	}
	if !reflect.DeepEqual(got.Servers["custom"]["oauth"], oauth) {
		t.Errorf("native OAuth object = %#v", got.Servers["custom"]["oauth"])
	}
}
