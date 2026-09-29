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
