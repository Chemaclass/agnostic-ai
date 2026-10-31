package emit

import "testing"

func TestRewriteGlobalHookPath_GrokKeepsNativeDirectPaths(t *testing.T) {
	for _, tc := range []struct{ name, dir, want string }{
		{"short Windows name", "C:/Users/RUNNER~1/hooks", "C:/Users/RUNNER~1/hooks/guard.sh"},
		{"leading tilde", "~/hooks", "'~/hooks'/guard.sh"},
		{"POSIX spaces", "/tmp/user hooks", "'/tmp/user hooks'/guard.sh"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := RewriteGlobalHookPath("$CLAUDE_PROJECT_DIR/.agnostic-ai/scripts/guard.sh", "grok", tc.dir); got != tc.want {
				t.Errorf("command = %q, want %q", got, tc.want)
			}
		})
	}
	if got := RewriteGlobalHookPath(".agnostic-ai/scripts/guard.sh", "kiro", "C:/Users/RUNNER~1/hooks"); got != "'C:/Users/RUNNER~1/hooks'/guard.sh" {
		t.Errorf("another target's quoting changed: %q", got)
	}
	if got := RewriteGlobalHookPath("$CLAUDE_PROJECT_DIR/.grok/hook-scripts/a.sh && .agnostic-ai/scripts/b.sh", "grok", "native/hooks"); got != "native/hooks/a.sh && native/hooks/b.sh" {
		t.Errorf("controlled project roots were not removed per reference: %q", got)
	}
}

func TestRewriteHookPath_TranslatesSiblingPrefix(t *testing.T) {
	t.Parallel()
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
		{
			name:   "shell-expanded path rewrites embedded prefix",
			cmd:    `"$(git rev-parse --show-toplevel)/.codex/hooks/protect-files.sh"`,
			target: "claude",
			want:   `"$(git rev-parse --show-toplevel)/.claude/hooks/protect-files.sh"`,
		},
		{
			name:   "home, variable and absolute sibling paths stay",
			cmd:    "~/.claude/hooks/notify.sh && bash $HOME/.claude/hooks/x.sh && /opt/team/.gemini/hooks/lint.sh",
			target: "codex",
			want:   "~/.claude/hooks/notify.sh && bash $HOME/.claude/hooks/x.sh && /opt/team/.gemini/hooks/lint.sh",
		},
		{
			name:   "every project copy of a prefix rewrites",
			cmd:    ".claude/hooks/a.sh && .claude/hooks/b.sh ~/.claude/hooks/c.sh",
			target: "codex",
			want:   ".codex/hooks/a.sh && .codex/hooks/b.sh ~/.claude/hooks/c.sh",
		},
		{
			name:   "quoted root before the path rewrites",
			cmd:    `"$CLAUDE_PROJECT_DIR"/.claude/hooks/f.sh`,
			target: "gemini",
			want:   `"${GEMINI_PROJECT_DIR}"/.gemini/hooks/f.sh`,
		},
		{
			name:   "working directory root rewrites",
			cmd:    `"$(pwd)/.codex/hooks/x.sh" $PWD/.codex/hooks/y.sh`,
			target: "claude",
			want:   `"$(pwd)/.claude/hooks/x.sh" $PWD/.claude/hooks/y.sh`,
		},
		{
			name:   "project root with a default rewrites",
			cmd:    `${CLAUDE_PROJECT_DIR:-.}/.claude/hooks/a.sh "${CLAUDE_PROJECT_DIR-.}"/.claude/hooks/b.sh "${CLAUDE_PROJECT_DIR:-.}/.claude/hooks/c.sh" ${HOME:-.}/.claude/hooks/d.sh`,
			target: "claude",
			want:   `${CLAUDE_PROJECT_DIR:-.}/.claude/hooks/a.sh "${CLAUDE_PROJECT_DIR-.}"/.claude/hooks/b.sh "${CLAUDE_PROJECT_DIR:-.}/.claude/hooks/c.sh" ${HOME:-.}/.claude/hooks/d.sh`,
		},
		{
			name:   "project root with a default moves to another tool",
			cmd:    `${CLAUDE_PROJECT_DIR:-.}/.codex/hooks/a.sh ${HOME:-.}/.codex/hooks/d.sh`,
			target: "claude",
			want:   `${CLAUDE_PROJECT_DIR:-.}/.claude/hooks/a.sh ${HOME:-.}/.codex/hooks/d.sh`,
		},
		{
			name:   "project root variable sibling path rewrites",
			cmd:    `"$CLAUDE_PROJECT_DIR/.gemini/hooks/run.sh" ~/.gemini/hooks/keep.sh`,
			target: "claude",
			want:   `"$CLAUDE_PROJECT_DIR/.claude/hooks/run.sh" ~/.gemini/hooks/keep.sh`,
		},
		{
			name:   "multiple sibling prefixes in one command all rewrite",
			cmd:    ".codex/hooks/pre.sh && .gemini/hooks/post.sh",
			target: "claude",
			want:   ".claude/hooks/pre.sh && .claude/hooks/post.sh",
		},
		{
			name:   "same-target shell expansion is a no-op",
			cmd:    `"$(pwd)/.claude/hooks/x.sh"`,
			target: "claude",
			want:   `"$(pwd)/.claude/hooks/x.sh"`,
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
