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
