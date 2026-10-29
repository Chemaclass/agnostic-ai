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
		exec := "Exec(" + c.command + ")"
		checks := map[string]bool{
			"compare native":    displayPermissionNative(exec, false) != exec,
			"compare rule":      preview.RedactRule(exec) != exec,
			"import hook":       claudeHookCredentialField(claudehooks.CommandEntry{Command: c.command}) == "command",
			"import hook if":    claudeHookCredentialField(claudehooks.CommandEntry{If: "PowerShell(" + c.command + ")"}) == "if",
			"import helper":     preview.CommandCredentialDetected(c.command),
			"authority preview": preview.CredentialInRule("Bash(" + c.command + ")"),
		}
		for name, got := range checks {
			if got != c.want {
				t.Errorf("%s(%q) = %v, want %v", name, c.command, got, c.want)
			}
		}
	}
}

// A path rule that does not parse as shell holds no command, so no rule
// check hides it, while a command rule still counts when it does not
// parse (#2737).
func TestRuleCredentialChecksAgreeOnPathRules(t *testing.T) {
	for _, c := range []struct {
		rule string
		want bool
	}{
		{`Edit(app/(marketing)/components/Site-Header.tsx)`, false},
		{`Read(./notes/O'Brien-Hall.md)`, false},
		{`Edit(src/(group)/a=b.ts)`, false},
		{`Read(app/(auth)/login/page.tsx)`, false},
		{`Edit(docs/it's -H here.md)`, false},
		{`Write(app/(shop)/[id]/page=1.tsx)`, false},
		{`Exec(curl -H 'X-Api-Key: abc https://api.example.com)`, true},
		{`Read(curl --header "X-Api-Key: abc" https://api.example.com)`, true},
		{`Shell(curl -H 'X-Api-Key: abc https://api.example.com)`, true},
		{`run_shell_command(curl -H 'X-Api-Key: abc https://api.example.com)`, true},
		{`prefix_rule(curl -H 'X-Api-Key: abc https://api.example.com)`, true},
	} {
		checks := map[string]bool{
			"compare native": displayPermissionNative(c.rule, false) != c.rule,
			"compare rule":   preview.RedactRule(c.rule) != c.rule,
			"import rule":    preview.CredentialInRule(c.rule),
			"import hook if": claudeHookCredentialField(claudehooks.CommandEntry{If: c.rule}) == "if",
		}
		for name, got := range checks {
			if got != c.want {
				t.Errorf("%s(%q) = %v, want %v", name, c.rule, got, c.want)
			}
		}
	}
}
