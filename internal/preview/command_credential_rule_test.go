package preview

import "testing"

// A rule in any tool's spelling reads its command as shell, so Codex
// `Exec(...)` and a bare OpenCode pattern count as `Bash(...)` does
// (#2737).
func TestCredentialInRule_ReadsEveryToolSpelling(t *testing.T) {
	for _, c := range commandCredentialCases {
		for _, rule := range []string{"Exec(" + c.command + ")", "PowerShell(" + c.command + ")", c.command} {
			if got := CredentialInRule(rule); got != c.want {
				t.Errorf("CredentialInRule(%q) = %v, want %v", rule, got, c.want)
			}
		}
	}
}

func TestRedactRule_HidesCredentialInExecRule(t *testing.T) {
	for _, c := range commandCredentialCases {
		rule := "Exec(" + c.command + ")"
		want := rule
		if c.want {
			want = "Exec(" + Redacted + ")"
		}
		if got := RedactRule(rule); got != want {
			t.Errorf("RedactRule(%q) = %q, want %q", rule, got, want)
		}
	}
}
