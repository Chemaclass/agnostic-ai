package cli

import (
	"encoding/json"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/claudehooks"
)

// The Claude hook, Claude settings env, Codex overlay, and Grok hook
// imports leave out the same setting values (#2716).
func TestLiteralSettingValueRuleAgrees(t *testing.T) {
	captureLogOut(t)
	for _, c := range []struct {
		key, value string
		out        bool
	}{
		{"API_TOKEN", "x", true},
		{"API_TOKEN", "${API_TOKEN}", false},
		{"API_TOKEN", "${API_TOKEN:-x}", true},
		{"PATH", "/usr/bin", false},
		{"API_TOKEN", "12345678", true},
		{"Authorization", "x", true},
		{"Authorization", "correct-horse-battery", true},
		{"Authorization", "Bearer abc", true},
		{"Authorization", "${AUTH}", false},
		{"Authorization", "Bearer $AUTH", false},
		{"Authorization", "Bearer ${TOKEN}", false},
		{"API_TOKEN", "hunter $X", true},
		{"PASSWORD", "correcthorsebatterystaple ${SUFFIX}", true},
		{"API_TOKEN", "$${API_TOKEN}", true},
		{"API_TOKEN", "${API_TOKEN:-}", false},
		{"CLAUDE_CODE_SKIP_BEDROCK_AUTH", "1", false},
		{"GOOGLE_APPLICATION_CREDENTIALS", "/path/sa.json", false},
		{"CLAUDE_CODE_CLIENT_KEY", "/path/client.key", false},
		{"API_KEY", "/9j4kXq2bLw8Zp0Rv7TnYc3Hs6Ue1Mf5", true},
		{"PASSWORD", "~hunter2", true},
		{"API_TOKEN", "sk-ant-api03abcdef123456.json", true},
		{"API_TOKEN", "/home/me/ghp_abcdefgh12345678", true},
		{"CLIENT_KEY", "~/.config/x", false},
		{"CLIENT_KEY", `C:\keys\client.key`, false},
		{"CLIENT_KEY", "client.key", false},
		{"Authorization", "ApiKey ${ES_KEY}", false},
		{"Proxy-Authorization", "SSWS ${OKTA_TOKEN}", false},
		{"API_TOKEN", "hunter ${X}", true},
		{"API_TOKEN", "$A $B", false},
		{"API_TOKEN", "${A}$B", false},
		{"API_KEY", "/9j/4AAQSkZJRgABAQAAAQABAAD2wBDAAgGBgcGBQgH", true},
		{"AWS_SECRET_ACCESS_KEY", "/Jalr/XUtnFEMIK7MDENGbPxRfiCYEXAMPLEKEY", true},
		{"CLIENT_KEY", "/home/me/.ssh/id_ed25519", false},
		{"API_KEY", "/Jalr/XUtnFEMIKMDENGbPxRfiCYEXAMPLEKEY", true},
		{"API_KEY", "/Ab3/Cd4Ef5Gh/6Ij7Kl8Mn/9Op0Qr1St/2Uv3Wx4Yz5Ab", true},
		{"CLIENT_KEY", "/Users/x/Library/ApplicationSupport/y", false},
		{"CLIENT_KEY", "/home/me/project1/Src2", false},
		{"CLIENT_KEY", "/opt/app2/config.yaml", false},
		{"GOOGLE_APPLICATION_CREDENTIALS", "/var/run/secrets/gcp1/key", false},
		{"GOOGLE_APPLICATION_CREDENTIALS", "/home/user1/credentials/gcp", false},
	} {
		checks := map[string]bool{
			"claude hook":         claudeHookCredentialField(claudehooks.CommandEntry{URL: "https://hooks.example.com", Headers: map[string]string{c.key: c.value}}) != "",
			"claude settings env": claudeSettingsEnvLeftOut(t, c.key, c.value),
			"codex overlay":       codexCredentialIn(c.key, c.value, false),
			"grok hook":           validateGrokRetainedCredentials(map[string]any{"hook": map[string]any{"env": map[string]any{c.key: c.value}}}) != nil,
		}
		for name, got := range checks {
			if got != c.out {
				t.Errorf("%s: %s=%q left out = %v, want %v", name, c.key, c.value, got, c.out)
			}
		}
	}
}

func claudeSettingsEnvLeftOut(t *testing.T, key, value string) bool {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"env": map[string]string{key: value}})
	if err != nil {
		t.Fatal(err)
	}
	doc := adapters.NewOrderedJSON()
	if err := json.Unmarshal(raw, doc); err != nil {
		t.Fatal(err)
	}
	if err := excludeClaudeCredentials(doc); err != nil {
		t.Fatal(err)
	}
	_, kept := doc.Get("env")
	return !kept
}
