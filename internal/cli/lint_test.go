package cli

import (
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestLintEmptySpecs_FlagsEmptyBodyAndNoDescription(t *testing.T) {
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "empty", Path: "rules/empty.md", Body: "", Meta: map[string]any{}},
		{Kind: spec.KindAgent, Name: "ok", Path: "agents/ok.md", Body: "Do something.", Meta: map[string]any{}},
		{Kind: spec.KindRule, Name: "desc-only", Path: "rules/desc.md", Body: "", Meta: map[string]any{"description": "has a description"}},
	}
	findings := lintEmptySpecs(entries)
	if len(findings) != 1 {
		t.Fatalf("expected 1 empty-spec finding, got %d", len(findings))
	}
	f := findings[0]
	if f.Code != "LINT001" {
		t.Errorf("expected code LINT001, got %s", f.Code)
	}
	if f.Severity != lintWarn {
		t.Errorf("expected warn severity, got %s", f.Severity)
	}
	if f.Path != "rules/empty.md" {
		t.Errorf("expected path rules/empty.md, got %s", f.Path)
	}
}

func TestLintCommandAgentNameClash(t *testing.T) {
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindAgent, Name: "deploy", Path: "agents/deploy.md"},
		{Kind: spec.KindCommand, Name: "deploy", Path: "commands/deploy.md"},
		{Kind: spec.KindAgent, Name: "review", Path: "agents/review.md"},
	})

	// A command-surface target enabled: the deploy clash warns; review does not.
	findings := lintCommandAgentNameClash(b, []string{"cursor"})
	if len(findings) != 1 {
		t.Fatalf("expected 1 clash finding, got %d: %v", len(findings), findings)
	}
	if findings[0].Code != "LINT006" || findings[0].Severity != lintWarn {
		t.Errorf("expected LINT006 warn, got %s %s", findings[0].Code, findings[0].Severity)
	}

	// No command-surface target enabled (claude/codex separate the dirs): no clash.
	if got := lintCommandAgentNameClash(b, []string{"claude", "codex"}); len(got) != 0 {
		t.Errorf("expected no clash for non-command-surface targets, got %v", got)
	}
}

func TestLintHookCollisions_FlagsDuplicateEventMatcher(t *testing.T) {
	hooks := []spec.Entry{
		{
			Kind: spec.KindHook, Name: "a", Path: "hooks/a.yaml",
			Meta: map[string]any{"event": "PostToolUse", "matcher": "Edit"},
		},
		{
			Kind: spec.KindHook, Name: "b", Path: "hooks/b.yaml",
			Meta: map[string]any{"event": "PostToolUse", "matcher": "Edit"},
		},
		{
			Kind: spec.KindHook, Name: "c", Path: "hooks/c.yaml",
			Meta: map[string]any{"event": "PreToolUse", "matcher": "Edit"},
		},
	}
	findings := lintHookCollisions(hooks)
	if len(findings) != 1 {
		t.Fatalf("expected 1 collision finding, got %d", len(findings))
	}
	f := findings[0]
	if f.Code != "LINT002" {
		t.Errorf("expected code LINT002, got %s", f.Code)
	}
	if f.Severity != lintError {
		t.Errorf("expected error severity, got %s", f.Severity)
	}
	if f.Path != "hooks/b.yaml" {
		t.Errorf("expected path hooks/b.yaml, got %s", f.Path)
	}
}

func TestLintHookCollisions_NoFindingsOnCleanHooks(t *testing.T) {
	hooks := []spec.Entry{
		{Kind: spec.KindHook, Name: "a", Path: "hooks/a.yaml", Meta: map[string]any{"event": "PostToolUse", "matcher": "Edit"}},
		{Kind: spec.KindHook, Name: "b", Path: "hooks/b.yaml", Meta: map[string]any{"event": "PreToolUse", "matcher": "Edit"}},
		{Kind: spec.KindHook, Name: "c", Path: "hooks/c.yaml", Meta: map[string]any{"event": "PostToolUse", "matcher": "Write"}},
	}
	if got := lintHookCollisions(hooks); len(got) != 0 {
		t.Errorf("expected 0 findings on clean hooks, got %d", len(got))
	}
}

func TestLintDuplicateNames_FlagsSameKindAndName(t *testing.T) {
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "style", Path: "rules/style.md"},
		{Kind: spec.KindAgent, Name: "style", Path: "agents/style.md"},         // different kind — OK
		{Kind: spec.KindRule, Name: "style", Path: "rules/overrides/style.md"}, // duplicate
	}
	findings := lintDuplicateNames(entries)
	if len(findings) != 1 {
		t.Fatalf("expected 1 duplicate-name finding, got %d", len(findings))
	}
	f := findings[0]
	if f.Code != "LINT003" {
		t.Errorf("expected code LINT003, got %s", f.Code)
	}
	if f.Severity != lintError {
		t.Errorf("expected error severity, got %s", f.Severity)
	}
}

func TestLintDeadSpecs_FlagsKindUnsupportedByAllTargets(t *testing.T) {
	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "h", Path: "hooks/h.yaml"},
	}
	// cursor does not support hooks
	findings := lintDeadSpecs(entries, []string{"cursor"})
	if len(findings) != 1 {
		t.Fatalf("expected 1 dead-spec finding, got %d", len(findings))
	}
	f := findings[0]
	if f.Code != "LINT004" {
		t.Errorf("expected code LINT004, got %s", f.Code)
	}
	if f.Severity != lintWarn {
		t.Errorf("expected warn severity, got %s", f.Severity)
	}
}

func TestLintDeadSpecs_NoFindingWhenTargetSupports(t *testing.T) {
	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "h", Path: "hooks/h.yaml"},
	}
	// claude supports hooks
	if got := lintDeadSpecs(entries, []string{"claude"}); len(got) != 0 {
		t.Errorf("expected 0 findings when target supports kind, got %d", len(got))
	}
}

func TestLintHookMatcherMisuse_FlagsMatcherOnNonToolEvent(t *testing.T) {
	hooks := []spec.Entry{
		{
			Kind: spec.KindHook, Name: "bad", Path: "hooks/bad.yaml",
			Meta: map[string]any{"event": "SessionStart", "matcher": "Bash"},
		},
		{
			Kind: spec.KindHook, Name: "ok", Path: "hooks/ok.yaml",
			Meta: map[string]any{"event": "PostToolUse", "matcher": "Edit"},
		},
		{
			Kind: spec.KindHook, Name: "no-matcher", Path: "hooks/nm.yaml",
			Meta: map[string]any{"event": "SessionStart"},
		},
	}
	findings := lintHookMatcherMisuse(hooks)
	if len(findings) != 1 {
		t.Fatalf("expected 1 matcher-misuse finding, got %d", len(findings))
	}
	f := findings[0]
	if f.Code != "LINT005" {
		t.Errorf("expected code LINT005, got %s", f.Code)
	}
	if f.Severity != lintWarn {
		t.Errorf("expected warn severity, got %s", f.Severity)
	}
	if f.Path != "hooks/bad.yaml" {
		t.Errorf("expected path hooks/bad.yaml, got %s", f.Path)
	}
}

func TestLintHookMatcherMisuse_NoFindingsOnCleanHooks(t *testing.T) {
	hooks := []spec.Entry{
		{Kind: spec.KindHook, Name: "a", Path: "hooks/a.yaml", Meta: map[string]any{"event": "PreToolUse", "matcher": "Bash"}},
		{Kind: spec.KindHook, Name: "b", Path: "hooks/b.yaml", Meta: map[string]any{"event": "AfterTool", "matcher": "shell"}},
		{Kind: spec.KindHook, Name: "c", Path: "hooks/c.yaml", Meta: map[string]any{"event": "SessionStart"}},
	}
	if got := lintHookMatcherMisuse(hooks); len(got) != 0 {
		t.Errorf("expected 0 findings on clean hooks, got %d", len(got))
	}
}
