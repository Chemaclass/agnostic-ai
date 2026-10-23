package preview

import (
	"fmt"
	"strings"
	"testing"
)

func TestRedactText_HidesCredentialsAndQuotedRules(t *testing.T) {
	token := "ghp_0123456789abcdefghijklmnop"
	for _, text := range []string{
		"shell(echo " + token + ")",
		"shell(curl -H 'X-Api-Key: plain-secret-value' https://example.test)",
		fmt.Sprintf("rule %q is invalid", `shell(curl -H "X-Api-Key: plain-secret-value")`),
		"env API_TOKEN=plain-secret-value npx server",
		fmt.Sprintf("command %q failed", "env API_TOKEN=plain-secret-value npx server"),
		"Get https://user:plain-secret-value@example.test/mcp?token=another-secret: failed",
		"https://hooks.example.test/services/T0A1/B0C2/token123value",
	} {
		shown := RedactText(text)
		for _, secret := range []string{token, "plain-secret-value", "another-secret", "T0A1", "B0C2", "token123value"} {
			if strings.Contains(shown, secret) {
				t.Errorf("%q retains %q", shown, secret)
			}
		}
	}
}

func TestRedactText_UsesExplicitResolvedValues(t *testing.T) {
	const value = "plain secret/value"
	for _, text := range []string{value, "plain+secret%2Fvalue", "plain%20secret%2Fvalue"} {
		if shown := RedactText(text, value); strings.Contains(shown, "secret") {
			t.Errorf("resolved value retained: %q", shown)
		}
	}
}

func TestRedactText_KeepsSafeCommandsAndReferences(t *testing.T) {
	for _, text := range []string{"shell(FOO=bar make)", "npx", "https://${HOST}/mcp?key=${KEY}", "Bearer ${TOKEN}"} {
		if got := RedactText(text); got != text {
			t.Errorf("%q became %q", text, got)
		}
	}
}
