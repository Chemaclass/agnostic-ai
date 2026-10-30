package gemini

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_MCPKeepsGeminiOAuthOptions(t *testing.T) {
	testutil.TempCwd(t)
	oauth := map[string]any{"enabled": true, "allowPrivateNetwork": true, "authorizationResponseIssParameterSupported": false, "clientId": "public-client"}
	entries := []spec.Entry{{Name: "docs", Meta: map[string]any{
		"type": "http", "url": "https://docs.example/mcp", "x-gemini": map[string]any{"oauth": oauth},
		"oauth": map[string]any{"clientId": "c", "clientSecret": "s", "callbackPort": 8080, "scopes": "a b"},
	}}}
	if err := New().Emit(emit.NewSession(), spec.Bundle{MCPs: entries}, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(".gemini/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Servers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if got := doc.Servers["docs"]["oauth"]; !reflect.DeepEqual(got, oauth) {
		t.Errorf("native OAuth options = %#v, want %#v", got, oauth)
	}
}

func TestEmit_MCPOmitsSharedOAuth(t *testing.T) {
	testutil.TempCwd(t)
	entries := []spec.Entry{{Name: "docs", Meta: map[string]any{
		"type": "http", "url": "https://docs.example/mcp",
		"oauth": map[string]any{"clientId": "c", "clientSecret": "s", "callbackPort": 8080, "scopes": "a b"},
	}}}
	if err := New().Emit(emit.NewSession(), spec.Bundle{MCPs: entries}, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(".gemini/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Servers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if got := doc.Servers["docs"]["httpUrl"]; got != "https://docs.example/mcp" {
		t.Errorf("server endpoint = %v, want https://docs.example/mcp", got)
	}
	if _, ok := doc.Servers["docs"]["oauth"]; ok {
		t.Error("shared OAuth reached Gemini settings")
	}
}
