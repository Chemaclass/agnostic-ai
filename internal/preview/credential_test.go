package preview

import (
	"strings"
	"testing"
)

func TestCredentialInRule_FindsLiteralsInShellRules(t *testing.T) {
	nested := `curl -H "X-Api-Key: test-api-key" https://example.test`
	for i := 0; i < 6; i++ {
		nested = "sh -c '" + strings.ReplaceAll(nested, "'", "'\"'\"'") + "'"
	}
	for _, command := range []string{
		`curl -H 'X-Api-Key: TOPSECRET123' https://example.test`,
		`curl --header='X-Api-Key: 12345' https://example.test`,
		`curl --api-key=12345 https://example.test`,
		`API_KEY=98765432101234567890 curl https://example.test`,
		`env API_KEY=test-api-key curl https://example.test`,
		`API_KEY=${TOKEN:-test-api-key} curl https://example.test`,
		`sh -c 'curl -H "X-Api-Key: test-api-key" https://example.test'`,
		`bash -lc 'env API_KEY=98765432101234567890 curl https://example.test'`,
		nested,
		`curl -H 'Authorization: Bearer ghp_0123456789abcdefghijklmnop' https://example.test`,
		`echo ghp_0123456789abcdefghijklmnop`,
		`psql postgres://u:pw@db/app`,
	} {
		for _, tool := range []string{"Bash", "shell"} {
			rule := tool + "(" + command + ")"
			if !CredentialInRule(rule) {
				t.Errorf("no credential found in %s", rule)
			}
		}
	}
}

func TestCredentialInRule_KeepsReferencesAndPlainRules(t *testing.T) {
	for _, rule := range []string{
		`shell(curl -H 'Accept: application/json' https://example.test)`,
		`shell(API_KEY=${TOKEN} curl https://example.test)`,
		`shell(env API_KEY=$TOKEN curl https://example.test)`,
		`Bash(curl -H 'X-Api-Key: ${TOKEN}' https://example.test)`,
		`shell(go test:*)`,
		`shell(FOO=bar make)`,
		`shell(git commit -m "rotate token")`,
		`edit(src/**)`,
		`mcp:railway/deploy`,
		`shell`,
		`shell(ls:*)`,
	} {
		if CredentialInRule(rule) {
			t.Errorf("credential reported in %s", rule)
		}
	}
}

func TestRedactRule_KeepsTheToolNameAndHidesTheCommand(t *testing.T) {
	for rule, want := range map[string]string{
		`shell(echo ghp_0123456789abcdefghijklmnop)`:      "shell(<redacted>)",
		`Bash(curl --api-key=12345 https://example.test)`: "Bash(<redacted>)",
		`ghp_0123456789abcdefghijklmnop`:                  Redacted,
		`shell(FOO=bar make)`:                             `shell(FOO=bar make)`,
		`edit(src/**)`:                                    `edit(src/**)`,
	} {
		if got := RedactRule(rule); got != want {
			t.Errorf("RedactRule(%q) = %q, want %q", rule, got, want)
		}
	}
}

func TestRedactRulesIn_HidesOnlyTheRulesThatHoldACredential(t *testing.T) {
	const token = "ghp_0123456789abcdefghijklmnop"
	for text, want := range map[string]string{
		"permissions.allow: use shell(echo " + token + ") instead of Bash(echo " + token + ")": "permissions.allow: use shell(<redacted>) instead of Bash(<redacted>)",
		"deny Bash(curl -H 'X-Api-Key: $(cat k)' " + token + ") blocks nothing":                 "deny Bash(<redacted>) blocks nothing",
		"use shell(go test:*) instead of Bash(go test:*)":                                       "use shell(go test:*) instead of Bash(go test:*)",
		"no rule here (just prose)":                                                             "no rule here (just prose)",
	} {
		if got := RedactRulesIn(text); got != want {
			t.Errorf("RedactRulesIn(%q) = %q, want %q", text, got, want)
		}
	}
}
