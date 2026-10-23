package overview

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestOverview_CredentialValuesAreRedactedInBuildAndServedResponse(t *testing.T) {
	const rule = "shell(curl --api-key plain-secret-value:*)"
	meta := map[string]any{
		"permissions": map[string]any{"allow": []any{rule, "shell(FOO=bar make)", "shell(ls)"}},
		"x-claude":    map[string]any{"customValue": "https://user:plain-password@example.test/mcp", "reference": "${API_TOKEN}", "ordinary": "normal-value"},
	}
	in := Inputs{Cfg: &config.Config{Targets: []string{"claude"}}, Bundle: spec.Bundle{Settings: []spec.Entry{entry(spec.KindSettings, "permissions", ".agnostic-ai/settings/permissions.yaml", meta)}}}
	built, err := json.Marshal(Build(in))
	if err != nil {
		t.Fatal(err)
	}
	served := serve(t, NewHandler(func() (Inputs, error) { return in, nil }), http.MethodGet, OverviewPath, "")
	if served.Code != http.StatusOK {
		t.Fatalf("served status %d: %s", served.Code, served.Body)
	}
	for name, data := range map[string]string{"build": string(built), "served": served.Body.String()} {
		for _, secret := range []string{"plain-secret-value", "plain-password"} {
			if strings.Contains(data, secret) {
				t.Errorf("%s exposed %s", name, secret)
			}
		}
		for _, safe := range []string{"shell(FOO=bar make)", "shell(ls)", "${API_TOKEN}", "normal-value"} {
			if !strings.Contains(data, safe) {
				t.Errorf("%s lost safe value %q", name, safe)
			}
		}
	}
	perms := meta["permissions"].(map[string]any)
	if perms["allow"].([]any)[0] != rule {
		t.Error("display redaction changed the original matching rule")
	}
}
