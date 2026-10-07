package config_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func TestLoad_RejectsUnknownGitignoreCommitKind(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "version: 1\ntargets: [claude]\ngitignore:\n  enabled: true\n  commit: [instructions, rules]\n")

	_, err := config.Load(dir)
	if err == nil || !strings.Contains(err.Error(), `gitignore.commit: unknown kind "rules"`) {
		t.Fatalf("Load() error = %v, want an unknown gitignore.commit kind", err)
	}
}

func TestLoad_AcceptsEveryGitignoreCommitKind(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "version: 1\ntargets: [claude]\ngitignore:\n  commit: ["+strings.Join(config.GitignoreCommitKinds(), ", ")+"]\n")

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Gitignore.Commit) != len(config.GitignoreCommitKinds()) {
		t.Errorf("commit = %v", cfg.Gitignore.Commit)
	}
}

func TestGitignore_CommitKindsScopesEntriesToATarget(t *testing.T) {
	g := config.Gitignore{Commit: []string{"reviews", "cursor:environments", "cursor:reviews"}}
	if got := g.CommitKinds("cursor"); !slices.Equal(got, []string{"reviews", "environments"}) {
		t.Errorf("cursor kinds = %v", got)
	}
	if got := g.CommitKinds("claude"); !slices.Equal(got, []string{"reviews"}) {
		t.Errorf("claude kinds = %v", got)
	}
	for _, bad := range []string{":reviews", "cursor:rules"} {
		if err := (config.Gitignore{Commit: []string{bad}}).Validate("agnostic-ai.yaml"); err == nil {
			t.Errorf("%q validated", bad)
		}
	}
}

func TestGitignore_AllowsPath(t *testing.T) {
	tests := []struct {
		name    string
		allow   []string
		path    string
		allowed bool
	}{
		{"empty", nil, "CLAUDE.md", false},
		{"blank", []string{" ", "!", "./"}, "CLAUDE.md", false},
		{"normalized", []string{" ./!CLAUDE.md "}, "./CLAUDE.md", true},
		{"basename at root", []string{"CLAUDE.md"}, "CLAUDE.md", true},
		{"basename nested", []string{"CLAUDE.md"}, "docs/CLAUDE.md", true},
		{"anchored root", []string{"/CLAUDE.md"}, "CLAUDE.md", true},
		{"anchored nested", []string{"/CLAUDE.md"}, "docs/CLAUDE.md", false},
		{"exact relative path", []string{"docs/CLAUDE.md"}, "docs/CLAUDE.md", true},
		{"relative path stays rooted", []string{"docs/CLAUDE.md"}, "nested/docs/CLAUDE.md", false},
		{"directory descendants", []string{".cursor/"}, ".cursor/rules/memory.mdc", true},
		{"nested directory", []string{".cursor/"}, "nested/.cursor/rules/memory.mdc", true},
		{"directory excludes file", []string{"CLAUDE.md/"}, "CLAUDE.md", false},
		{"ancestor without trailing slash", []string{".cursor"}, ".cursor/rules/memory.mdc", true},
		{"anchored ancestor", []string{"/.cursor/"}, ".cursor/rules/memory.mdc", true},
		{"anchored ancestor excludes nested", []string{"/.cursor/"}, "nested/.cursor/rules/memory.mdc", false},
		{"globstar zero directories", []string{"**/CLAUDE.md"}, "CLAUDE.md", true},
		{"globstar multiple directories", []string{"**/CLAUDE.md"}, "docs/nested/CLAUDE.md", true},
		{"interior globstar zero directories", []string{".cursor/**/memory.mdc"}, ".cursor/memory.mdc", true},
		{"interior globstar directories", []string{".cursor/**/memory.mdc"}, ".cursor/rules/nested/memory.mdc", true},
		{"trailing globstar", []string{".cursor/**"}, ".cursor/rules/memory.mdc", true},
		{"star", []string{"*.md"}, "docs/CLAUDE.md", true},
		{"star does not cross directories", []string{"docs/*/CLAUDE.md"}, "docs/a/b/CLAUDE.md", false},
		{"question mark", []string{"CLAUD?.md"}, "CLAUDE.md", true},
		{"character class", []string{"[AC]*.md"}, "CLAUDE.md", true},
		{"nonmatching character class", []string{"[AB]*.md"}, "CLAUDE.md", false},
		{"negated character class", []string{"[!G]*.md"}, "CLAUDE.md", true},
		{"nonmatching negated character class", []string{"[!G]*.md"}, "GEMINI.md", false},
		{"negated class in path", []string{"docs/[!G]*.md"}, "docs/CLAUDE.md", true},
		{"literal bracket inside class", []string{"[a[!]*.md"}, "!CLAUDE.md", true},
		{"named class fails closed", []string{"[[:upper:]]*.md"}, "CLAUDE.md", true},
		{"unsupported named class fails closed", []string{"[[:unknown:]]*.md"}, "CLAUDE.md", true},
		{"invalid pattern fails closed", []string{"["}, "CLAUDE.md", true},
		{"invalid negated class fails closed", []string{"[!"}, "CLAUDE.md", true},
		{"later matching pattern", []string{"GEMINI.md", "CLAUDE.md"}, "CLAUDE.md", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := config.Gitignore{Allow: tt.allow}
			if got := g.AllowsPath(tt.path); got != tt.allowed {
				t.Errorf("AllowsPath(%q) = %v, want %v", tt.path, got, tt.allowed)
			}
		})
	}
}
