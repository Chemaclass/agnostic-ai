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
