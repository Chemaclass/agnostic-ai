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
