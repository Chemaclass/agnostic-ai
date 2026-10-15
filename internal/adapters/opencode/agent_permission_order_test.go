package opencode

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_AgentPreservesNativePermissionOrder(t *testing.T) {
	for _, tc := range []struct {
		name            string
		override, tools bool
	}{
		{name: "direct"},
		{name: "override", override: true},
		{name: "direct-with-tools", tools: true},
		{name: "override-with-tools", override: true, tools: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			native := "permission:\n  read:\n    secrets/public/*: allow\n    secrets/*: deny\n  bash:\n    git status: allow\n    git *: ask\n  github_create_issue: allow\n  github_*: deny\n"
			front := "description: Review project files\n"
			if tc.tools {
				front += "tools: [Read, Bash]\n"
			}
			if tc.override {
				front += "permission: allow\nx-opencode:\n  custom: keep\n"
				for _, line := range strings.Split(strings.TrimSuffix(native, "\n"), "\n") {
					front += "  " + line + "\n"
				}
			} else {
				front += native
			}
			path := filepath.Join(dir, "reviewer.md")
			if err := os.WriteFile(path, []byte("---\n"+front+"---\nReview.\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			cfg := &config.Config{Sources: config.Sources{Agents: dir}}
			bundle, err := spec.LoadBundle(dir, cfg)
			if err != nil {
				t.Fatal(err)
			}
			if err := New().Emit(emit.NewSession(), bundle, cfg, false); err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(dir, ".opencode/agents/reviewer.md")
			rules := simAgentPermission(t, output)
			want := []simRule{
				{"read", "secrets/public/*", "allow"},
				{"read", "secrets/*", "deny"},
				{"bash", "git status", "allow"},
				{"bash", "git *", "ask"},
				{"github_create_issue", "*", "allow"},
				{"github_*", "*", "deny"},
			}
			if len(rules) < len(want) || !slices.Equal(rules[len(rules)-len(want):], want) {
				t.Errorf("native rules = %#v, want original order at end: %#v", rules, want)
			}
			for _, check := range []struct{ tool, input, action string }{
				{"read", "secrets/public/token.txt", "deny"},
				{"bash", "git status", "ask"},
				{"github_create_issue", "issue", "deny"},
			} {
				if got := simEvaluate(rules, check.tool, check.input, false); got != check.action {
					t.Errorf("%s(%s) = %s, want %s", check.tool, check.input, got, check.action)
				}
			}
			if tc.tools && simEvaluate(rules, "edit", "source.go", false) != "deny" {
				t.Error("portable tools must still deny unlisted edit")
			}
			if tc.override && !strings.Contains(readFile(t, output), "custom: keep\n") {
				t.Error("unrelated native metadata was lost")
			}
		})
	}
}
