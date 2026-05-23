package emit

import "testing"

func TestRewriteHookPath_TranslatesSiblingPrefix(t *testing.T) {
	cases := []struct {
		name, cmd, target, want string
	}{
		{
			name:   "claude to codex",
			cmd:    ".claude/hooks/format-php.sh",
			target: "codex",
			want:   ".codex/hooks/format-php.sh",
		},
		{
			name:   "codex to claude",
			cmd:    ".codex/hooks/protect-files.sh",
			target: "claude",
			want:   ".claude/hooks/protect-files.sh",
		},
		{
			name:   "gemini to claude",
			cmd:    ".gemini/hooks/run.sh",
			target: "claude",
			want:   ".claude/hooks/run.sh",
		},
		{
			name:   "same-target prefix is a no-op",
			cmd:    ".claude/hooks/x.sh",
			target: "claude",
			want:   ".claude/hooks/x.sh",
		},
		{
			name:   "non-hook command passes through",
			cmd:    "gofmt && go vet ./...",
			target: "claude",
			want:   "gofmt && go vet ./...",
		},
		{
			name:   "quoted sibling path keeps the quote",
			cmd:    `".claude/hooks/x.sh"`,
			target: "codex",
			want:   `".codex/hooks/x.sh"`,
		},
		{
			name:   "empty command stays empty",
			cmd:    "",
			target: "codex",
			want:   "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := RewriteHookPath(c.cmd, c.target); got != c.want {
				t.Errorf("RewriteHookPath(%q, %q) = %q, want %q", c.cmd, c.target, got, c.want)
			}
		})
	}
}
