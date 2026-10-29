package cli

import (
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/claudehooks"
	"github.com/chemaclass/agnostic-ai/internal/preview"
)

// compare, import, and sync --global read a shell command the same way
// (#2717).
func TestCommandCredentialChecksAgree(t *testing.T) {
	for _, c := range []struct {
		command string
		want    bool
	}{
		{`curl -H 'Authorization: Bearer x' https://api.example.com`, true},
		{`curl -H Authorization:%20Bearer%20x https://api.example.com`, true},
		{`sh -c "export TOKEN=abc"`, true},
		{`TOKEN=${TOKEN} make release`, false},
		{`deploy --api-key=abc`, true},
		{`curl --header "X-Api-Key: abc" https://api.example.com`, true},
		{`go test ./...`, false},
		{`DB_PASSPHRASE=hunter2 make release`, true},
		{`DB_PASSPHRASE=${DB_PASSPHRASE} make release`, false},
		{`age --passphrase *`, true},
	} {
		checks := map[string]bool{
			"compare":           permissionRuleCredential("shell(" + c.command + ")"),
			"import hook":       claudeHookCredentialField(claudehooks.CommandEntry{Command: c.command}) == "command",
			"import helper":     globalPermissionCommandCredentialDetected(c.command),
			"sync --global":     globalPermissionCredentialDetected("Bash(" + c.command + ")"),
			"authority preview": preview.CredentialInRule("Bash(" + c.command + ")"),
		}
		for name, got := range checks {
			if got != c.want {
				t.Errorf("%s(%q) = %v, want %v", name, c.command, got, c.want)
			}
		}
	}
}
