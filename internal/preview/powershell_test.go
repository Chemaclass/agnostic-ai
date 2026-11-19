package preview

import "testing"

func TestDisplay_ReadsAPowerShellCommandAsPowerShell(t *testing.T) {
	for _, c := range []struct {
		body   string
		hidden bool
	}{
		{"name: a\ncommandWindows: '$s = \"$CLAUDE_PROJECT_DIR/x.sh\"; if (-not (Get-Command sh)) { exit 1 }; sh $s'\n", false},
		{"name: a\npowershell: '$out = Get-Date; if ($out) { exit 0 }'\n", false},
		{"name: a\ncommandWindows: '$token = \"hunter2hunter2\"; if (x) { ./notify.ps1 $token }'\n", true},
		{"name: a\ncommandWindows: '$env:API_KEY = \"abc123def456\"; ./notify.ps1'\n", true},
		{"name: a\ncommandWindows: 'iwr -Headers @{Authorization = \"Bearer ghp_abcdefghijklmnopqrstuvwxyz0123456789\"} https://x'\n", true},
		{"name: a\ncommandWindows: '$token = $env:GITHUB_TOKEN; ./notify.ps1 $token'\n", false},
	} {
		if got := Display("hook.yaml", c.body); got.Hidden != c.hidden {
			t.Errorf("hidden = %v, want %v for %q", got.Hidden, c.hidden, c.body)
		}
	}
}
