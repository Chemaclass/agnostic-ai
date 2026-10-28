package preview

import "testing"

// commandCredentialCases is the table every command credential check
// must agree on (#2717).
var commandCredentialCases = []struct {
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
}

func TestCommandCredentialDetected(t *testing.T) {
	for _, c := range commandCredentialCases {
		if got := CommandCredentialDetected(c.command); got != c.want {
			t.Errorf("CommandCredentialDetected(%q) = %v, want %v", c.command, got, c.want)
		}
		for _, rule := range []string{"Bash(" + c.command + ")", "shell(" + c.command + ")"} {
			if got := CredentialInRule(rule); got != c.want {
				t.Errorf("CredentialInRule(%q) = %v, want %v", rule, got, c.want)
			}
		}
	}
}
